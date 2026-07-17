import json
import logging
import os
import time
from datetime import datetime, timedelta, timezone

from influxdb_client import InfluxDBClient, Point
from influxdb_client.client.write_api import SYNCHRONOUS

from foxess_client import FoxessClient
from huawei_client import HuaweiClient

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("collector")

INTERVAL_SECONDS = int(os.environ.get("COLLECT_INTERVAL_SECONDS", "300"))
HUAWEI_DEV_TYPE_ID = 38  # inversor (fixo na taxonomia de tipos de dispositivo da NBI)
PLANT_TAG = "casa"
BRAZIL_TZ = timezone(timedelta(hours=-3))

# Guarda em memória pra detectar "dia novo, contador do fabricante ainda não
# resetou" (ver _apply_daily_reset_guard) — por inversor.
_daily_reset_state: dict[str, dict] = {}

# Último day_kwh bem-sucedido por inversor, pra sustentar o total quando a
# consulta desse ciclo falha (ver _carry_forward_day_kwh) — e contador de
# falhas consecutivas, pra alertar quando a API fica ruim de verdade (ver
# _record_attempt).
_last_known_day_kwh: dict[str, dict] = {}
_consecutive_failures: dict[str, int] = {"huawei": 0, "foxess": 0}

# 2 ciclos seguidos = ~10min de falha real na API (não é 1 blip isolado) —
# a partir daqui vale logar como alerta e o webapp expõe pro dashboard.
FAILURE_ALERT_THRESHOLD = 2

# getAlarmList tem um limite de taxa próprio, mais restrito que getStationRealKpi/
# getDevRealKpi: medido empiricamente em produção (2026-07-16), o intervalo real
# exigido pela Huawei pra esse endpoint especifico fica entre ~592s e ~888s — bem
# acima dos 300s assumidos em MIN_INTERFACE_INTERVAL_SECONDS, que valem pros outros
# dois. Consultar alarme a cada ciclo de coleta (300s) gerava ACCESS_FREQUENCY_IS_
# TOO_HIGH em 2 a cada 3 tentativas, de forma determinística. Status de alarme não
# precisa de frescor de 5min, então a consulta roda numa cadência própria e mais
# lenta (com folga sobre os 888s observados), desacoplada da coleta de potência/
# energia — que nunca apresentou esse problema e continua a cada 300s.
ALARM_POLL_INTERVAL_SECONDS = 900
_alarm_cache: dict = {"checked_at": None, "has_alarm": False, "alarm_detail": None, "alarm_raw": None}


def _apply_daily_reset_guard(inverter: str, power_kw: float, day_kwh: float) -> float:
    """A Huawei/FoxESS cacheiam o total do dia na nuvem deles e só atualizam
    quando o inversor manda telemetria nova — à noite o inversor dorme e não
    manda nada novo, então da meia-noite local até ele acordar (perto do
    nascer do sol) o valor que a API retorna ainda é o total de ONTEM, não
    zero. Sem um campo de "última atualização" confiável pra esse valor
    específico, usamos geração real (power_kw > 0) como prova de que o
    inversor já acordou hoje — até isso acontecer, tratamos o dia como
    zerado. Estado por inversor, reiniciado a cada troca de dia local.

    INCIDENTE 2026-07-17: esse estado vive só em memória — um restart do
    container que aconteça DEPOIS que a usina já gerou hoje, mas num momento
    em que power_kw está momentaneamente 0 (pôr do sol, ou uma nuvem passando
    ao meio-dia), fazia essa função tratar o dia inteiro como "ainda não
    começou" e zerar um day_kwh que já era real e positivo — sem sol
    remanescente pra "started" virar True de novo, o zero ficava travado pro
    resto do dia. Corrigido semeando `_daily_reset_state`/`_last_known_day_kwh`
    a partir do InfluxDB no startup (ver `_seed_daily_state_from_influx`), não
    mais assumindo que um restart só acontece de manhã antes do sol nascer."""
    today = datetime.now(BRAZIL_TZ).date()
    state = _daily_reset_state.setdefault(inverter, {"date": today, "started": False})
    if state["date"] != today:
        state["date"] = today
        state["started"] = False
    if power_kw > 0:
        state["started"] = True
    return day_kwh if state["started"] else 0.0


def _seed_daily_state_from_influx(query_api, bucket: str) -> None:
    """Reconstrói `_daily_reset_state`/`_last_known_day_kwh` a partir do
    InfluxDB no startup do coletor — sem isso, um restart do container
    (deploy, crash, `restart: unless-stopped`) que aconteça depois que a
    usina já gerou hoje, mas com power_kw momentaneamente em 0, zera um
    day_kwh que já era real (ver incidente em `_apply_daily_reset_guard`).

    Só semeia se já existir um ponto de `inverter_status.day_kwh` gravado
    HOJE (fuso BRT) — cada ponto gravado hoje já passou pelo guard em tempo
    real antes de ser escrito, então reaproveitar o último é sempre seguro:
    nunca reintroduz o problema original de mostrar sobra de ontem antes do
    inversor acordar (se hoje ainda não tem nenhum ponto, não semeia nada, e
    o guard segue tratando o dia como zerado, correto nesse caso)."""
    today = datetime.now(BRAZIL_TZ).date()
    start_of_today_utc = datetime.combine(today, datetime.min.time(), tzinfo=BRAZIL_TZ).astimezone(timezone.utc)
    for inverter in ("huawei", "foxess"):
        flux = f'''
        from(bucket: "{bucket}")
          |> range(start: {start_of_today_utc.isoformat()})
          |> filter(fn: (r) => r._measurement == "inverter_status" and r.inverter == "{inverter}" and r.plant_id == "{PLANT_TAG}" and r._field == "day_kwh")
          |> sort(columns: ["_time"], desc: true)
          |> limit(n: 1)
        '''
        try:
            for table in query_api.query(flux):
                for record in table.records:
                    value = record.get_value()
                    _last_known_day_kwh[inverter] = {"date": today, "value": value}
                    _daily_reset_state[inverter] = {"date": today, "started": True}
                    log.info("Estado do dia recuperado do InfluxDB pra %s: day_kwh=%.2f", inverter, value)
        except Exception:
            log.exception("Falha ao recuperar estado do dia do InfluxDB pra %s (segue com estado zerado)", inverter)


def _carry_forward_day_kwh(inverter: str, day_kwh: float | None) -> float:
    """Geração diária só cresce, então uma falha pontual na consulta da API
    não pode fazer o total "Gerado hoje" regredir — usamos o último valor
    bem-sucedido de hoje em vez de tratar a contribuição desse inversor
    como zero. Sem valor bem-sucedido hoje ainda (ex.: primeira tentativa
    do dia já falha), assume 0, igual ao comportamento de antes do
    inversor acordar."""
    today = datetime.now(BRAZIL_TZ).date()
    if day_kwh is not None:
        _last_known_day_kwh[inverter] = {"date": today, "value": day_kwh}
        return day_kwh
    cached = _last_known_day_kwh.get(inverter)
    return cached["value"] if cached and cached["date"] == today else 0.0


def _record_attempt(inverter: str, error: str | None) -> int:
    """Atualiza o contador de falhas consecutivas por inversor e retorna o
    valor atualizado — sucesso zera, falha incrementa."""
    _consecutive_failures[inverter] = 0 if error is None else _consecutive_failures[inverter] + 1
    return _consecutive_failures[inverter]


def _health_point(inverter: str, consecutive_failures: int, last_error: str | None) -> Point:
    point = (
        Point("collector_health")
        .tag("plant_id", PLANT_TAG)
        .tag("inverter", inverter)
        .field("consecutive_failures", consecutive_failures)
    )
    if last_error:
        point = point.field("last_error", last_error[:200])
    return point


def _fox_history_points(foxess: FoxessClient, sn: str, hours_back: int = 3) -> list[Point]:
    """Curva de potência de alta resolução das últimas horas (FoxESS history/query).
    A Huawei não oferece equivalente — fica limitada a 1 ponto por ciclo de coleta."""
    end_ms = round(time.time() * 1000)
    begin_ms = end_ms - hours_back * 3600 * 1000
    result = foxess.get_history_query(sn, ["generationPower"], begin_ms, end_ms)
    points = []
    for entry in result[0]["datas"][0]["data"]:
        naive = datetime.strptime(entry["time"][:19], "%Y-%m-%d %H:%M:%S")
        ts = naive.replace(tzinfo=BRAZIL_TZ)
        points.append(
            Point("inverter_status")
            .tag("plant_id", PLANT_TAG)
            .tag("inverter", "foxess")
            .field("power_kw", float(entry["value"]))
            .time(ts.astimezone(timezone.utc))
        )
    return points


def _extract_alarm_detail(alarms: list[dict]) -> str | None:
    """Formato exato do getAlarmList não foi confirmado contra um alarme
    real (só testamos com lista vazia, e não há documentação oficial nem de
    terceiros com os nomes de campo). Tenta os candidatos mais prováveis e
    cai para uma mensagem genérica — ajustar aqui quando um alarme real
    aparecer nos logs e revelar o formato de verdade."""
    if not alarms:
        return None
    alarm = alarms[0]
    for key in ("alarmName", "name", "desc", "alarmCause"):
        if alarm.get(key):
            return str(alarm[key])
    return "Alarme ativo"


def _get_alarm_status(huawei: HuaweiClient, station_code: str) -> tuple[bool, str | None, list | None]:
    """Consulta getAlarmList numa cadência própria (ver ALARM_POLL_INTERVAL_SECONDS),
    bem mais lenta que o ciclo de coleta principal — e nunca deixa uma falha nessa
    consulta específica derrubar a coleta de potência/energia do ciclo (que não tem
    relação com esse endpoint). Fora da janela de consulta, ou se a tentativa falhar,
    retorna o último status conhecido em vez de assumir "sem alarme".

    O 3º valor retornado (`alarm_raw`) é a lista crua de alarmes, sem nenhuma
    extração — guardada (log + InfluxDB, ver `collect_once`) só pra quando um
    alarme real acontecer dar pra confirmar o formato de verdade depois, sem
    precisar forçar uma falha real pra testar (ver `_extract_alarm_detail`)."""
    now = time.monotonic()
    due = _alarm_cache["checked_at"] is None or (now - _alarm_cache["checked_at"]) >= ALARM_POLL_INTERVAL_SECONDS
    if not due:
        return _alarm_cache["has_alarm"], _alarm_cache["alarm_detail"], _alarm_cache["alarm_raw"]

    _alarm_cache["checked_at"] = now  # marca a tentativa antes de chamar, pra não martelar de novo no próximo ciclo se falhar
    try:
        alarms = huawei.get_alarm_list(station_code)
        _alarm_cache["has_alarm"] = len(alarms) > 0
        _alarm_cache["alarm_detail"] = _extract_alarm_detail(alarms)
        _alarm_cache["alarm_raw"] = alarms if alarms else None
        if alarms:
            log.info("Alarme real recebido da Huawei, payload bruto: %s", alarms)
    except Exception:
        log.exception("Falha ao consultar getAlarmList (status de alarme mantido no ultimo valor conhecido)")
    return _alarm_cache["has_alarm"], _alarm_cache["alarm_detail"], _alarm_cache["alarm_raw"]


def _collect_huawei(huawei: HuaweiClient, station_code: str, dev_dn: str) -> dict:
    huawei.login()
    station_kpi = huawei.get_station_real_kpi(station_code)[0]["dataItemMap"]
    dev_kpi = huawei.get_dev_real_kpi(dev_dn, HUAWEI_DEV_TYPE_ID)[0]["dataItemMap"]
    has_alarm, alarm_detail, alarm_raw = _get_alarm_status(huawei, station_code)
    return {
        "power_kw": dev_kpi.get("active_power") or 0.0,
        "day_kwh": dev_kpi.get("day_cap") or station_kpi.get("day_power") or 0.0,
        "temperature_c": dev_kpi.get("temperature"),
        "has_alarm": has_alarm,
        "alarm_detail": alarm_detail,
        "alarm_raw": alarm_raw,
    }


def _collect_foxess(foxess: FoxessClient, sn: str) -> dict:
    fox_real = foxess.get_real_query(sn, ["generationPower", "todayYield", "invTemperation"])
    values = {d["variable"]: d["value"] for d in fox_real[0]["datas"]}
    return {
        "power_kw": values.get("generationPower") or 0.0,
        "day_kwh": values.get("todayYield") or 0.0,
        "temperature_c": values.get("invTemperation"),
    }


def _inverter_point(inverter: str, data: dict) -> Point:
    point = (
        Point("inverter_status")
        .tag("plant_id", PLANT_TAG)
        .tag("inverter", inverter)
        .field("power_kw", data["power_kw"])
        .field("day_kwh", data["day_kwh"])
    )
    if data.get("temperature_c") is not None:
        point = point.field("temperature_c", float(data["temperature_c"]))
    return point


def collect_once(huawei, foxess, station_code, dev_dn, foxess_sn, write_api, bucket, org):
    # Cada inversor é coletado e gravado de forma independente: se um falhar,
    # o outro continua gravando normalmente nesse ciclo. Isso é o que permite
    # o webapp inferir "sem comunicação" por inversor (ausência de ponto
    # recente), sem precisar de um campo dedicado pra isso.
    points = []
    huawei_data = fox_data = None
    huawei_error = fox_error = None

    try:
        huawei_data = _collect_huawei(huawei, station_code, dev_dn)
        huawei_data["day_kwh"] = _apply_daily_reset_guard("huawei", huawei_data["power_kw"], huawei_data["day_kwh"])
        points.append(_inverter_point("huawei", huawei_data))
    except Exception as e:
        log.exception("Falha ao coletar dados da Huawei nesse ciclo")
        huawei_error = str(e)

    try:
        fox_data = _collect_foxess(foxess, foxess_sn)
        fox_data["day_kwh"] = _apply_daily_reset_guard("foxess", fox_data["power_kw"], fox_data["day_kwh"])
        points.append(_inverter_point("foxess", fox_data))
        points.extend(_fox_history_points(foxess, foxess_sn))
    except Exception as e:
        log.exception("Falha ao coletar dados da FoxESS nesse ciclo")
        fox_error = str(e)

    # O contador de falhas e o ponto de saúde são gravados sempre — inclusive
    # quando os dois inversores falham nesse ciclo — pra que o alerta de
    # "falha constante" funcione mesmo num apagão total de comunicação, que é
    # exatamente quando ele mais importa.
    huawei_failures = _record_attempt("huawei", huawei_error)
    fox_failures = _record_attempt("foxess", fox_error)
    health_points = [
        _health_point("huawei", huawei_failures, huawei_error),
        _health_point("foxess", fox_failures, fox_error),
    ]
    if huawei_failures == FAILURE_ALERT_THRESHOLD:
        log.error("Huawei falhando ha %d ciclos seguidos: %s", huawei_failures, huawei_error)
    if fox_failures == FAILURE_ALERT_THRESHOLD:
        log.error("FoxESS falhando ha %d ciclos seguidos: %s", fox_failures, fox_error)

    if not points:
        log.warning("Nenhum inversor respondeu nesse ciclo, nada gravado (exceto saude)")
        write_api.write(bucket=bucket, org=org, record=health_points)
        return

    points.extend(health_points)

    huawei_power_kw = huawei_data["power_kw"] if huawei_data else 0.0
    fox_power_kw = fox_data["power_kw"] if fox_data else 0.0
    total_power_kw = huawei_power_kw + fox_power_kw
    huawei_day_kwh = _carry_forward_day_kwh("huawei", huawei_data["day_kwh"] if huawei_data else None)
    fox_day_kwh = _carry_forward_day_kwh("foxess", fox_data["day_kwh"] if fox_data else None)
    total_day_kwh = huawei_day_kwh + fox_day_kwh
    has_alarm = bool(huawei_data and huawei_data["has_alarm"])

    # generated_kwh é gravado sempre no mesmo timestamp (meia-noite local) para
    # o dia corrente, para que cada ciclo sobrescreva o mesmo ponto em vez de
    # criar um ponto novo por ciclo — mantém 1 registro por dia no InfluxDB.
    # Importante usar meia-noite em horário do Brasil (não meio-dia UTC): meio-dia
    # UTC = 9h no Brasil, então das 0h às 9h locais esse timestamp ficaria no
    # futuro — e toda query do webapp usa range() sem stop explícito, que por
    # padrão exclui pontos futuros (stop: now()), fazendo "hoje" mostrar o
    # último dia visível (ontem) até as 9h da manhã. Meia-noite local já
    # nasce no passado assim que o dia começa, então nunca é excluída.
    today_midnight_brt = datetime.now(BRAZIL_TZ).replace(hour=0, minute=0, second=0, microsecond=0)
    today_point_ts = today_midnight_brt.astimezone(timezone.utc)

    plant_point = (
        Point("plant_status")
        .tag("plant_id", PLANT_TAG)
        .field("instantaneous_power_kw", total_power_kw)
        .field("installed_power_kwp", float(os.environ.get("PLANT_INSTALLED_POWER_KWP", "12.2")))
        .field("has_alarm", has_alarm)
    )
    if huawei_data and huawei_data["alarm_detail"]:
        plant_point = plant_point.field("alarm_detail", huawei_data["alarm_detail"])
    if huawei_data and huawei_data.get("alarm_raw"):
        # Payload cru do getAlarmList, sem extração — guardado só pra confirmar
        # o formato real na primeira vez que um alarme de verdade acontecer
        # (ver _get_alarm_status). Não usado por nenhuma tela do painel hoje.
        plant_point = plant_point.field("alarm_raw_json", json.dumps(huawei_data["alarm_raw"])[:4000])
    points.append(plant_point)

    points.append(
        Point("daily_generation")
        .tag("plant_id", PLANT_TAG)
        .field("generated_kwh", total_day_kwh)
        .time(today_point_ts)
    )

    write_api.write(bucket=bucket, org=org, record=points)

    log.info(
        "Coleta concluida: potencia_total=%.2fkW (huawei=%s foxess=%s) gerado_hoje=%.2fkWh alarme=%s",
        total_power_kw,
        "sem_dados" if huawei_data is None else f"{huawei_power_kw:.2f}",
        "sem_dados" if fox_data is None else f"{fox_power_kw:.2f}",
        total_day_kwh,
        has_alarm,
    )


def main():
    huawei = HuaweiClient(
        os.environ["HUAWEI_USERNAME"],
        os.environ["HUAWEI_SYSTEM_CODE"],
        base_url=os.environ.get("HUAWEI_BASE_URL", "https://la5.fusionsolar.huawei.com"),
    )
    foxess = FoxessClient(
        os.environ["FOXESS_API_KEY"],
        base_url=os.environ.get("FOXESS_BASE_URL", "https://www.foxesscloud.com"),
    )

    bucket = os.environ["INFLUXDB_BUCKET"]
    org = os.environ["INFLUXDB_ORG"]
    influx_client = InfluxDBClient(
        url=os.environ["INFLUXDB_URL"],
        token=os.environ["INFLUXDB_TOKEN"],
        org=org,
    )
    write_api = influx_client.write_api(write_options=SYNCHRONOUS)
    query_api = influx_client.query_api()
    _seed_daily_state_from_influx(query_api, bucket)

    # Retry com espera na descoberta inicial: se isso levantar exceção sem
    # tratamento, o container reinicia instantaneamente (restart:
    # unless-stopped) e martela as APIs em loop — o que já derrubou o rate
    # limit de login da Huawei uma vez durante os testes.
    station_code = dev_dn = foxess_sn = None
    while station_code is None or foxess_sn is None:
        try:
            log.info("Descobrindo usina e inversores...")
            huawei.login()
            station_code = huawei.get_station_list()[0]["stationCode"]
            dev_dn = huawei.get_dev_list(station_code)[0]["devDn"]
            log.info("Huawei: stationCode=%s devDn=%s", station_code, dev_dn)

            foxess_sn = foxess.get_device_list()[0]["deviceSN"]
            log.info("FoxESS: deviceSN=%s", foxess_sn)
        except Exception:
            log.exception("Falha na descoberta inicial, tentando novamente em 60s")
            station_code = dev_dn = foxess_sn = None
            time.sleep(60)

    log.info("Coletor iniciado. Intervalo: %ss", INTERVAL_SECONDS)
    while True:
        try:
            collect_once(huawei, foxess, station_code, dev_dn, foxess_sn, write_api, bucket, org)
        except Exception:
            log.exception("Falha na coleta, tentando novamente no proximo ciclo")
        time.sleep(INTERVAL_SECONDS)


if __name__ == "__main__":
    main()
