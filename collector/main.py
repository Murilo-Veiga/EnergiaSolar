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


def _apply_daily_reset_guard(inverter: str, power_kw: float, day_kwh: float) -> float:
    """A Huawei/FoxESS cacheiam o total do dia na nuvem deles e só atualizam
    quando o inversor manda telemetria nova — à noite o inversor dorme e não
    manda nada novo, então da meia-noite local até ele acordar (perto do
    nascer do sol) o valor que a API retorna ainda é o total de ONTEM, não
    zero. Sem um campo de "última atualização" confiável pra esse valor
    específico, usamos geração real (power_kw > 0) como prova de que o
    inversor já acordou hoje — até isso acontecer, tratamos o dia como
    zerado. Estado por inversor, reiniciado a cada troca de dia local; some
    num restart do container, o que na pior hipótese só reseta a marca de
    "já gerou hoje" (segura, volta a True no próximo ciclo com sol)."""
    today = datetime.now(BRAZIL_TZ).date()
    state = _daily_reset_state.setdefault(inverter, {"date": today, "started": False})
    if state["date"] != today:
        state["date"] = today
        state["started"] = False
    if power_kw > 0:
        state["started"] = True
    return day_kwh if state["started"] else 0.0


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


def _collect_huawei(huawei: HuaweiClient, station_code: str, dev_dn: str) -> dict:
    huawei.login()
    station_kpi = huawei.get_station_real_kpi(station_code)[0]["dataItemMap"]
    dev_kpi = huawei.get_dev_real_kpi(dev_dn, HUAWEI_DEV_TYPE_ID)[0]["dataItemMap"]
    alarms = huawei.get_alarm_list(station_code)
    return {
        "power_kw": dev_kpi.get("active_power") or 0.0,
        "day_kwh": dev_kpi.get("day_cap") or station_kpi.get("day_power") or 0.0,
        "temperature_c": dev_kpi.get("temperature"),
        "has_alarm": len(alarms) > 0,
        "alarm_detail": _extract_alarm_detail(alarms),
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

    try:
        huawei_data = _collect_huawei(huawei, station_code, dev_dn)
        huawei_data["day_kwh"] = _apply_daily_reset_guard("huawei", huawei_data["power_kw"], huawei_data["day_kwh"])
        points.append(_inverter_point("huawei", huawei_data))
    except Exception:
        log.exception("Falha ao coletar dados da Huawei nesse ciclo")

    try:
        fox_data = _collect_foxess(foxess, foxess_sn)
        fox_data["day_kwh"] = _apply_daily_reset_guard("foxess", fox_data["power_kw"], fox_data["day_kwh"])
        points.append(_inverter_point("foxess", fox_data))
        points.extend(_fox_history_points(foxess, foxess_sn))
    except Exception:
        log.exception("Falha ao coletar dados da FoxESS nesse ciclo")

    if not points:
        log.warning("Nenhum inversor respondeu nesse ciclo, nada gravado")
        return

    huawei_power_kw = huawei_data["power_kw"] if huawei_data else 0.0
    huawei_day_kwh = huawei_data["day_kwh"] if huawei_data else 0.0
    fox_power_kw = fox_data["power_kw"] if fox_data else 0.0
    fox_day_kwh = fox_data["day_kwh"] if fox_data else 0.0
    total_power_kw = huawei_power_kw + fox_power_kw
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
