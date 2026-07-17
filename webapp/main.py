import os
from datetime import date as date_type
from datetime import datetime, timedelta, timezone

import requests
from fastapi import FastAPI, File, HTTPException, Request, UploadFile
from fastapi.responses import HTMLResponse, Response
from fastapi.templating import Jinja2Templates
from influxdb_client import InfluxDBClient, Point
from influxdb_client.client.write_api import SYNCHRONOUS
from pydantic import BaseModel, Field

from celesc_bill_parser import CelescBillParseError, parse_bill
from report_pdf import build_history_report_pdf

INFLUX_URL = os.environ["INFLUXDB_URL"]
INFLUX_TOKEN = os.environ["INFLUXDB_TOKEN"]
INFLUX_ORG = os.environ["INFLUXDB_ORG"]
INFLUX_BUCKET = os.environ["INFLUXDB_BUCKET"]
PLANT_LAT = os.environ["PLANT_LAT"]
PLANT_LON = os.environ["PLANT_LON"]

# Precisa bater com o PLANT_TAG do collector/main.py — sem esse filtro, uma
# série com tag de plant_id diferente ficaria numa tabela separada no Flux e
# poderia ser retornada no lugar da série atual.
PLANT_TAG = "casa"

# Mesmo fuso do collector/main.py — usado só pra agrupar pontos de
# inverter_status (gravados em UTC "de verdade", não à meia-noite BRT como
# daily_generation) no dia-calendário correto de Joinville.
BRAZIL_TZ = timezone(timedelta(hours=-3))

# Códigos de clima WMO (usados pelo Open-Meteo) -> (descrição em pt-BR, favorabilidade p/ geração)
WMO_CODES = {
    0: ("Céu limpo", "bom"),
    1: ("Principalmente limpo", "bom"),
    2: ("Parcialmente nublado", "bom"),
    3: ("Nublado", "moderado"),
    45: ("Nevoeiro", "moderado"),
    48: ("Nevoeiro com geada", "moderado"),
    51: ("Garoa fraca", "moderado"),
    53: ("Garoa moderada", "moderado"),
    55: ("Garoa forte", "ruim"),
    56: ("Garoa congelante fraca", "ruim"),
    57: ("Garoa congelante forte", "ruim"),
    61: ("Chuva fraca", "moderado"),
    63: ("Chuva moderada", "ruim"),
    65: ("Chuva forte", "ruim"),
    66: ("Chuva congelante fraca", "ruim"),
    67: ("Chuva congelante forte", "ruim"),
    71: ("Neve fraca", "ruim"),
    73: ("Neve moderada", "ruim"),
    75: ("Neve forte", "ruim"),
    77: ("Grãos de neve", "ruim"),
    80: ("Pancadas de chuva fracas", "moderado"),
    81: ("Pancadas de chuva moderadas", "ruim"),
    82: ("Pancadas de chuva fortes", "ruim"),
    85: ("Pancadas de neve fracas", "ruim"),
    86: ("Pancadas de neve fortes", "ruim"),
    95: ("Trovoada", "ruim"),
    96: ("Trovoada com granizo fraco", "ruim"),
    99: ("Trovoada com granizo forte", "ruim"),
}

app = FastAPI()
templates = Jinja2Templates(directory="templates")
client = InfluxDBClient(url=INFLUX_URL, token=INFLUX_TOKEN, org=INFLUX_ORG)
query_api = client.query_api()
write_api = client.write_api(write_options=SYNCHRONOUS)

# Só temos 2 unidades consumidoras (uso pessoal/familiar) — mapa fixo em vez de
# um cadastro dinâmico, mesmo espírito do PLANT_TAG acima.
UC_LABELS = {
    "19647901154": "Guanabara (usina)",
    "298240601131": "Elizabeth Rech",
}


def _hour_in_daylight(hour_index: int, sunrise_hhmm: str, sunset_hhmm: str) -> bool:
    sunrise_h = int(sunrise_hhmm[:2]) + int(sunrise_hhmm[3:5]) / 60
    sunset_h = int(sunset_hhmm[:2]) + int(sunset_hhmm[3:5]) / 60
    return sunrise_h <= (hour_index + 0.5) <= sunset_h


# O frontend consulta /api/day-status a cada 30s e /api/forecast a cada 30min
# (ver templates/index.html) — sem cache isso batia na Open-Meteo ~2900x/dia
# pra um dado que o modelo deles só atualiza a cada poucas horas. Cache simples
# em memória (1 processo uvicorn, sem múltiplos workers — não precisa de nada
# além de uma variável de módulo) resolve sem perder responsividade real.
_FORECAST_CACHE_TTL = timedelta(hours=2)
_forecast_cache: dict = {"data": None, "fetched_at": None}


def _forecast_days():
    now = datetime.now(timezone.utc)
    if _forecast_cache["data"] is not None and (now - _forecast_cache["fetched_at"]) < _FORECAST_CACHE_TTL:
        return _forecast_cache["data"]

    resp = requests.get(
        "https://api.open-meteo.com/v1/forecast",
        params={
            "latitude": PLANT_LAT,
            "longitude": PLANT_LON,
            "daily": "weathercode,temperature_2m_max,temperature_2m_min,"
            "shortwave_radiation_sum,precipitation_sum,precipitation_probability_max,"
            "sunrise,sunset",
            "hourly": "weathercode,cloudcover",
            "timezone": "America/Sao_Paulo",
            "forecast_days": 5,
        },
        timeout=10,
    )
    resp.raise_for_status()
    data = resp.json()
    daily = data["daily"]
    hourly = data["hourly"]

    days = []
    for i, date in enumerate(daily["time"]):
        code = daily["weathercode"][i]
        description, rating = WMO_CODES.get(code, ("Desconhecido", "moderado"))
        sunrise = daily["sunrise"][i][11:16]
        sunset = daily["sunset"][i][11:16]

        day = {
            "date": date,
            "weather": description,
            "rating": rating,
            "temp_max": daily["temperature_2m_max"][i],
            "temp_min": daily["temperature_2m_min"][i],
            "solar_radiation_mj_m2": daily["shortwave_radiation_sum"][i],
            "precipitation_mm": daily["precipitation_sum"][i],
            "precipitation_probability_pct": daily["precipitation_probability_max"][i],
            "sunrise": sunrise,
            "sunset": sunset,
        }

        if i == 0:
            # "weather" (acima) resume as 24h do dia inteiro — inclui madrugada/noite,
            # que não afeta geração solar nenhuma. Pro card "Status do dia" (só hoje),
            # recalculamos usando só as horas entre nascer e pôr do sol, que é o que
            # realmente importa pra usina. Ver README > "Status do dia".
            start = i * 24
            hour_codes = hourly["weathercode"][start:start + 24]
            hour_clouds = hourly["cloudcover"][start:start + 24]
            codes_in_daylight = [c for h, c in enumerate(hour_codes) if _hour_in_daylight(h, sunrise, sunset)]
            if codes_in_daylight:
                daylight_code = max(set(codes_in_daylight), key=codes_in_daylight.count)
                daylight_desc, daylight_rating = WMO_CODES.get(daylight_code, ("Desconhecido", "moderado"))
                day["weather_daylight"] = daylight_desc
                # rating (usado pro ícone) precisa acompanhar a mesma recalculação do
                # texto — senão o ícone mostra o resumo de 24h enquanto o texto mostra
                # só as horas de sol, podendo contradizer um ao outro (mesma classe de
                # bug que motivou o weather_daylight, ver README > "Status do dia").
                day["rating_daylight"] = daylight_rating
            else:
                day["weather_daylight"] = description
                day["rating_daylight"] = day["rating"]
            day["cloudcover_hourly"] = hour_clouds

        days.append(day)

    _forecast_cache["data"] = days
    _forecast_cache["fetched_at"] = now
    return days


def _latest_fatura():
    """Última fatura (source=='fatura') entre as duas UCs, a mais recente
    das duas por data de referência."""
    latest = None
    for uc in UC_LABELS:
        flux = f'''
        from(bucket: "{INFLUX_BUCKET}")
          |> range(start: -400d)
          |> filter(fn: (r) => r._measurement == "consumption" and r.uc == "{uc}" and r.source == "fatura")
          |> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
          |> sort(columns: ["_time"], desc: true)
          |> limit(n: 1)
        '''
        for table in query_api.query(flux):
            for record in table.records:
                if latest is None or record.get_time() > latest.get_time():
                    latest = record
    return latest


def _tarifa_efetiva():
    """Valor pago ÷ kWh da fatura mais recente — usado como estimativa de
    tarifa pra converter geração em R$. Nenhuma fatura até agora cobre um
    período com compensação solar, então isso ainda não inclui nenhum
    crédito de geração (ver /api/consumption)."""
    fatura = _latest_fatura()
    if fatura is None:
        return None
    kwh = fatura.values.get("consumed_kwh")
    brl = fatura.values.get("total_value_brl")
    if not kwh or not brl:
        return None
    return brl / kwh


def _last_value(measurement: str, field: str, range_start: str = "-6h"):
    flux = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: {range_start})
      |> filter(fn: (r) => r._measurement == "{measurement}" and r._field == "{field}" and r.plant_id == "{PLANT_TAG}")
      |> last()
    '''
    for table in query_api.query(flux):
        for record in table.records:
            return record.get_value()
    return None


@app.get("/", response_class=HTMLResponse)
def index(request: Request):
    return templates.TemplateResponse("index.html", {"request": request})


def _yesterday_generated_kwh():
    """Penúltimo ponto de daily_generation (o último é hoje) — 1 ponto por
    dia, então basta pegar os 2 mais recentes e descartar o de hoje."""
    flux = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: -4d)
      |> filter(fn: (r) => r._measurement == "daily_generation" and r._field == "generated_kwh" and r.plant_id == "{PLANT_TAG}")
      |> sort(columns: ["_time"], desc: true)
      |> limit(n: 2)
    '''
    values = [record.get_value() for table in query_api.query(flux) for record in table.records]
    return values[1] if len(values) >= 2 else None


@app.get("/api/summary")
def summary():
    instantaneous = _last_value("plant_status", "instantaneous_power_kw")
    installed = _last_value("plant_status", "installed_power_kwp", "-30d")
    today_generated = _last_value("daily_generation", "generated_kwh", "-3d")
    has_alarm = _last_value("plant_status", "has_alarm", "-1h")

    yesterday_generated = _yesterday_generated_kwh()
    today_vs_yesterday_pct = None
    if yesterday_generated and today_generated is not None:
        today_vs_yesterday_pct = round((today_generated - yesterday_generated) / yesterday_generated * 100)

    is_online = instantaneous is not None and (instantaneous > 0 or (today_generated or 0) > 0)
    status = "alerta" if has_alarm else ("online" if is_online else "pendente")

    peak_power_kw = peak_power_at = None
    start_of_day_brazil = datetime.now(BRAZIL_TZ).replace(hour=0, minute=0, second=0, microsecond=0)
    flux_peak = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: {start_of_day_brazil.astimezone(timezone.utc).isoformat()})
      |> filter(fn: (r) => r._measurement == "plant_status" and r._field == "instantaneous_power_kw" and r.plant_id == "{PLANT_TAG}")
    '''
    for table in query_api.query(flux_peak):
        for record in table.records:
            value = record.get_value()
            if value is not None and (peak_power_kw is None or value > peak_power_kw):
                peak_power_kw = value
                peak_power_at = record.get_time().isoformat()

    today_economia_brl = None
    tarifa = _tarifa_efetiva()
    if tarifa and today_generated:
        today_economia_brl = round(today_generated * tarifa, 2)

    return {
        "instantaneous_power_kw": instantaneous,
        "installed_power_kwp": installed,
        "today_generated_kwh": today_generated,
        "today_economia_brl": today_economia_brl,
        "today_vs_yesterday_pct": today_vs_yesterday_pct,
        "peak_power_kw": peak_power_kw,
        "peak_power_at": peak_power_at,
        "status": status,
        "updated_at": datetime.now(timezone.utc).isoformat(),
    }


# "Sem comunicação" não tem campo próprio no InfluxDB — é derivado aqui pela
# ausência de um ponto recente. 15 min = 3x o COLLECT_INTERVAL_SECONDS real
# do coletor (300s), a mesma cadência usada em collector/main.py.
COMM_TIMEOUT_MINUTES = 15


@app.get("/api/inverters")
def inverters():
    result = {}
    now = datetime.now(timezone.utc)
    for inverter in ("huawei", "foxess"):
        flux = f'''
        from(bucket: "{INFLUX_BUCKET}")
          |> range(start: -1h)
          |> filter(fn: (r) => r._measurement == "inverter_status" and r.inverter == "{inverter}" and r.plant_id == "{PLANT_TAG}")
          |> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
          |> sort(columns: ["_time"], desc: true)
          |> limit(n: 1)
        '''
        entry = {"power_kw": None, "day_kwh": None, "temperature_c": None, "status": "sem_comunicacao"}
        for table in query_api.query(flux):
            for record in table.records:
                age_minutes = (now - record.get_time()).total_seconds() / 60
                power_kw = record.values.get("power_kw")
                if age_minutes > COMM_TIMEOUT_MINUTES:
                    status = "sem_comunicacao"
                elif power_kw is not None and power_kw > 0:
                    status = "gerando"
                else:
                    status = "online_sem_geracao"
                entry = {
                    "power_kw": power_kw,
                    "day_kwh": record.values.get("day_kwh"),
                    "temperature_c": record.values.get("temperature_c"),
                    "status": status,
                }
        entry["consecutive_failures"], entry["last_error"] = _health_status(inverter)
        result[inverter] = entry
    return result


def _health_status(inverter: str) -> tuple[int, str | None]:
    """Falhas consecutivas do coletor ao consultar a API desse inversor —
    gravadas a cada ciclo (sucesso ou falha), independente do ponto de
    inverter_status (que só existe em ciclos bem-sucedidos)."""
    flux = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: -1h)
      |> filter(fn: (r) => r._measurement == "collector_health" and r.inverter == "{inverter}" and r.plant_id == "{PLANT_TAG}")
      |> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
      |> sort(columns: ["_time"], desc: true)
      |> limit(n: 1)
    '''
    for table in query_api.query(flux):
        for record in table.records:
            return int(record.values.get("consecutive_failures") or 0), record.values.get("last_error")
    return 0, None


@app.get("/api/day-status")
def day_status():
    has_alarm = _last_value("plant_status", "has_alarm", "-1h")
    alarm_detail = _last_value("plant_status", "alarm_detail", "-1h") if has_alarm else None
    today = _forecast_days()[0]

    fatura = _latest_fatura()
    bandeira = fatura.values.get("bandeira") if fatura else None
    bandeira_valor_kwh = fatura.values.get("bandeira_valor_kwh") if fatura else None

    common = {
        "weather": today["weather"],
        "weather_daylight": today.get("weather_daylight"),
        "solar_radiation_mj_m2": today.get("solar_radiation_mj_m2"),
        "cloudcover_hourly": today.get("cloudcover_hourly"),
        "sunrise": today["sunrise"],
        "sunset": today["sunset"],
        "has_alarm": has_alarm,
        "alarm_detail": alarm_detail,
        "bandeira": bandeira,
        "bandeira_valor_kwh": bandeira_valor_kwh,
    }

    flux = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: -3d)
      |> filter(fn: (r) => r._measurement == "daily_generation" and r._field == "generated_kwh" and r.plant_id == "{PLANT_TAG}")
      |> sort(columns: ["_time"], desc: true)
      |> limit(n: 1)
    '''
    for table in query_api.query(flux):
        for record in table.records:
            return {"date": record.get_time().date().isoformat(), "generated_kwh": record.get_value(), **common}
    return {"date": None, "generated_kwh": None, **common}


@app.get("/api/forecast")
def forecast():
    return _forecast_days()


RANGE_DAYS = {"dia": 1, "semana": 7, "mes": 30, "ano": 365}


def _period_total_kwh(days: int, start_offset_days: int = 0) -> float:
    """Soma de generated_kwh num intervalo de `days` dias terminando `start_offset_days`
    dias atrás — usado pra comparar o período atual com o imediatamente anterior."""
    flux = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: -{days + start_offset_days}d, stop: -{start_offset_days}d)
      |> filter(fn: (r) => r._measurement == "daily_generation" and r._field == "generated_kwh" and r.plant_id == "{PLANT_TAG}")
      |> sum()
    '''
    for table in query_api.query(flux):
        for record in table.records:
            return record.get_value() or 0.0
    return 0.0


@app.get("/api/history")
def history(range: str = "mes"):
    days = RANGE_DAYS.get(range, 30)
    flux = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: -{days}d)
      |> filter(fn: (r) => r._measurement == "daily_generation" and r._field == "generated_kwh" and r.plant_id == "{PLANT_TAG}")
      |> sort(columns: ["_time"])
    '''
    tarifa = _tarifa_efetiva()
    rows = []
    total_kwh = 0.0
    for table in query_api.query(flux):
        for record in table.records:
            kwh = record.get_value()
            valor = round(kwh * tarifa, 2) if (tarifa and kwh is not None) else None
            rows.append(
                {
                    "date": record.get_time().date().isoformat(),
                    "generated_kwh": kwh,
                    "valor_estimado_brl": valor,
                }
            )
            if kwh is not None:
                total_kwh += kwh

    # período imediatamente anterior, mesma duração — pra comparação tipo "12% a mais que mês passado"
    previous_total_kwh = _period_total_kwh(days, start_offset_days=days)

    return {
        "rows": rows,
        "total_kwh": round(total_kwh, 1),
        "total_brl": round(total_kwh * tarifa, 2) if tarifa else None,
        "previous_total_kwh": round(previous_total_kwh, 1),
        "previous_total_brl": round(previous_total_kwh * tarifa, 2) if tarifa else None,
    }


@app.get("/api/history/records")
def history_records():
    """Recordes de todo o período de operação da usina (não só o range selecionado)."""
    best_day_flux = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: 0)
      |> filter(fn: (r) => r._measurement == "daily_generation" and r._field == "generated_kwh" and r.plant_id == "{PLANT_TAG}")
      |> sort(columns: ["_value"], desc: true)
      |> limit(n: 1)
    '''
    best_day = best_day_date = None
    for table in query_api.query(best_day_flux):
        for record in table.records:
            best_day = record.get_value()
            best_day_date = record.get_time().date().isoformat()

    best_month_flux = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: 0)
      |> filter(fn: (r) => r._measurement == "daily_generation" and r._field == "generated_kwh" and r.plant_id == "{PLANT_TAG}")
      |> aggregateWindow(every: 1mo, fn: sum, createEmpty: false)
      |> sort(columns: ["_value"], desc: true)
      |> limit(n: 1)
    '''
    best_month = best_month_label = None
    for table in query_api.query(best_month_flux):
        for record in table.records:
            best_month = record.get_value()
            best_month_label = record.get_time().strftime("%m/%Y")

    peak_flux = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: 0)
      |> filter(fn: (r) => r._measurement == "plant_status" and r._field == "instantaneous_power_kw" and r.plant_id == "{PLANT_TAG}")
      |> sort(columns: ["_value"], desc: true)
      |> limit(n: 1)
    '''
    peak_power_kw = peak_power_at = None
    for table in query_api.query(peak_flux):
        for record in table.records:
            peak_power_kw = record.get_value()
            peak_power_at = record.get_time().isoformat()

    return {
        "best_day_kwh": round(best_day, 1) if best_day is not None else None,
        "best_day_date": best_day_date,
        "best_month_kwh": round(best_month, 1) if best_month is not None else None,
        "best_month_label": best_month_label,
        "peak_power_kw": peak_power_kw,
        "peak_power_at": peak_power_at,
    }


RANGE_LABEL_PDF = {"semana": "Últimos 7 dias", "mes": "Últimos 30 dias", "ano": "Últimos 12 meses"}

# Capacidade instalada por inversor (ver "Detalhes da instalação" no README)
# — constante, não muda com o tempo, por isso não vem de nenhuma API.
INVERTER_CAPACITY_KWP = {"huawei": 3.0, "foxess": 5.0}


def _pct_delta(current, previous):
    if not previous:
        return None
    pct = round((current - previous) / previous * 100)
    arrow = "▲" if pct >= 0 else "▼"
    return f"{arrow} {abs(pct)}% vs. período anterior"


@app.get("/api/history/report.pdf")
def history_report_pdf(range: str = "mes"):
    data = history(range)
    records = history_records()
    inverters = history_inverters(range)
    annotations = list_annotations(range)
    installed_kwp = _last_value("plant_status", "installed_power_kwp", "-30d")

    rows = data["rows"]
    best_row = max(rows, key=lambda r: r["generated_kwh"] or 0) if rows else None
    avg_daily_kwh = (data["total_kwh"] / len(rows)) if rows else None

    huawei_kwh = sum(r["huawei_kwh"] or 0 for r in inverters["rows"])
    foxess_kwh = sum(r["foxess_kwh"] or 0 for r in inverters["rows"])
    total_capacity = sum(INVERTER_CAPACITY_KWP.values())

    pdf_bytes = build_history_report_pdf(
        period_label=RANGE_LABEL_PDF.get(range, "Período selecionado"),
        generated_at=datetime.now(BRAZIL_TZ),
        rows=rows,
        total_kwh=data["total_kwh"],
        total_brl=data["total_brl"] or 0.0,
        installed_kwp=installed_kwp,
        avg_daily_kwh=avg_daily_kwh,
        kwh_delta_label=_pct_delta(data["total_kwh"], data["previous_total_kwh"]),
        brl_delta_label=_pct_delta(data["total_brl"] or 0, data["previous_total_brl"] or 0),
        best_day_period=best_row,
        best_day_alltime={"date": records["best_day_date"], "kwh": records["best_day_kwh"]},
        peak_power_alltime={"kw": records["peak_power_kw"], "at": records["peak_power_at"]},
        inverter_split={
            "huawei_kwh": huawei_kwh,
            "foxess_kwh": foxess_kwh,
            "huawei_expected_pct": round(INVERTER_CAPACITY_KWP["huawei"] / total_capacity * 100, 1),
            "foxess_expected_pct": round(INVERTER_CAPACITY_KWP["foxess"] / total_capacity * 100, 1),
            "days_with_data": len(inverters["rows"]),
            "days_in_period": len(rows),
        },
        annotations=annotations["rows"],
    )
    return Response(
        content=pdf_bytes,
        media_type="application/pdf",
        headers={"Content-Disposition": f'attachment; filename="solar-home-{range}.pdf"'},
    )


@app.get("/api/history/inverters")
def history_inverters(range: str = "mes"):
    """Geração diária por inversor. Não precisa de nenhum dado novo do
    coletor: deriva do último inverter_status.day_kwh de cada dia (mesma
    lógica de "o último valor do dia é o total do dia" que já vale pro
    daily_generation), só que por inversor em vez do total da usina.

    range="dia" usa a meia-noite BRT de hoje como início em vez da janela
    rolante de -1d: com -1d, o último ponto de ontem (gravado ~23:55) cai
    dentro da janela e entra como um dia extra, fazendo a soma dobrar
    ontem-inteiro + hoje-parcial em vez de só hoje."""
    if range == "dia":
        range_start = datetime.now(BRAZIL_TZ).replace(hour=0, minute=0, second=0, microsecond=0).astimezone(timezone.utc).isoformat()
    else:
        days = RANGE_DAYS.get(range, 30)
        range_start = f"-{days}d"
    flux = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: {range_start})
      |> filter(fn: (r) => r._measurement == "inverter_status" and r._field == "day_kwh" and r.plant_id == "{PLANT_TAG}")
      |> sort(columns: ["_time"])
    '''
    by_day: dict[str, dict[str, float]] = {}
    for table in query_api.query(flux):
        for record in table.records:
            date = record.get_time().astimezone(BRAZIL_TZ).date().isoformat()
            inverter = record.values.get("inverter")
            by_day.setdefault(date, {})[inverter] = record.get_value()

    rows = [
        {"date": date, "huawei_kwh": vals.get("huawei"), "foxess_kwh": vals.get("foxess")}
        for date, vals in sorted(by_day.items())
    ]
    return {"rows": rows}


@app.get("/api/collector-health")
def collector_health_reliability(days: int = 30):
    """% de ciclos de coleta sem falha, por inversor — usa o measurement
    collector_health, que já grava 1 ponto por ciclo (sucesso ou falha) pra
    sempre, sem sobrescrever (ver "Falhas de coleta e fallback seguro").
    Não precisa de nenhuma coleta nova, só nunca foi exposto antes."""
    result = {}
    for inverter in ("huawei", "foxess"):
        flux = f'''
        from(bucket: "{INFLUX_BUCKET}")
          |> range(start: -{days}d)
          |> filter(fn: (r) => r._measurement == "collector_health" and r._field == "consecutive_failures" and r.inverter == "{inverter}" and r.plant_id == "{PLANT_TAG}")
        '''
        total = failed = 0
        for table in query_api.query(flux):
            for record in table.records:
                total += 1
                if (record.get_value() or 0) > 0:
                    failed += 1
        result[inverter] = {
            "total_cycles": total,
            "failed_cycles": failed,
            "reliability_pct": round((total - failed) / total * 100, 1) if total else None,
        }
    return result


class AnnotationIn(BaseModel):
    date: date_type
    note: str = Field(min_length=1, max_length=280)


@app.post("/api/annotations")
def create_annotation(body: AnnotationIn):
    """Uma nota por dia: gravar de novo no mesmo dia sobrescreve a anterior
    (mesma tag/timestamp, comportamento padrão do InfluxDB) — mantém simples
    em vez de virar uma lista por dia."""
    ts = datetime(body.date.year, body.date.month, body.date.day, tzinfo=BRAZIL_TZ)
    point = Point("annotation").tag("plant_id", PLANT_TAG).field("note", body.note.strip()).time(ts.astimezone(timezone.utc))
    write_api.write(bucket=INFLUX_BUCKET, org=INFLUX_ORG, record=point)
    return {"date": body.date.isoformat(), "note": body.note.strip()}


@app.get("/api/annotations")
def list_annotations(range: str = "mes"):
    days = RANGE_DAYS.get(range, 30)
    flux = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: -{days}d)
      |> filter(fn: (r) => r._measurement == "annotation" and r._field == "note" and r.plant_id == "{PLANT_TAG}")
      |> sort(columns: ["_time"], desc: true)
    '''
    rows = []
    for table in query_api.query(flux):
        for record in table.records:
            rows.append({"date": record.get_time().astimezone(BRAZIL_TZ).date().isoformat(), "note": record.get_value()})
    return {"rows": rows}


def _consumption_point(uc, ano, mes, kwh, *, total_brl=None, dias=None, bandeira=None, bandeira_valor=None, source):
    ts = datetime(ano, mes, 1, 12, 0, 0, tzinfo=timezone.utc)
    p = (
        Point("consumption")
        .tag("uc", uc)
        .tag("source", source)
        .field("consumed_kwh", float(kwh))
        .field("parsed_at", datetime.now(timezone.utc).isoformat())
        .time(ts)
    )
    if total_brl is not None:
        p = p.field("total_value_brl", float(total_brl))
    if dias is not None:
        p = p.field("dias_faturados", int(dias))
    if bandeira is not None:
        p = p.field("bandeira", bandeira)
    if bandeira_valor is not None:
        p = p.field("bandeira_valor_kwh", float(bandeira_valor))
    return p


@app.post("/api/consumption/upload")
async def upload_consumption(file: UploadFile = File(...)):
    pdf_bytes = await file.read()
    try:
        parsed = parse_bill(pdf_bytes)
    except CelescBillParseError as e:
        raise HTTPException(status_code=422, detail=str(e))

    points = [
        _consumption_point(
            parsed["uc"],
            parsed["referencia_ano"],
            parsed["referencia_mes"],
            parsed["consumo_kwh"],
            total_brl=parsed["total_pagar_brl"],
            dias=parsed["dias_faturados"],
            bandeira=parsed["bandeira"],
            bandeira_valor=parsed["bandeira_valor_kwh"],
            source="fatura",
        )
    ]
    for h in parsed["historico"]:
        if h["ano"] == parsed["referencia_ano"] and h["mes"] == parsed["referencia_mes"]:
            continue  # esse mes ja foi coberto acima, com mais detalhe (R$, dias, bandeira)
        points.append(
            _consumption_point(parsed["uc"], h["ano"], h["mes"], h["consumo_kwh"], source="backfill_historico")
        )

    write_api.write(bucket=INFLUX_BUCKET, org=INFLUX_ORG, record=points)

    return {
        "uc": parsed["uc"],
        "uc_label": UC_LABELS.get(parsed["uc"], parsed["uc"]),
        "titular": parsed["titular"],
        "referencia": f"{parsed['referencia_mes']:02d}/{parsed['referencia_ano']}",
        "consumo_kwh": parsed["consumo_kwh"],
        "total_pagar_brl": parsed["total_pagar_brl"],
        "meses_historico_importados": len(points) - 1,
    }


@app.get("/api/consumption/summary")
def consumption_summary():
    unidades = {}
    for uc, label in UC_LABELS.items():
        flux = f'''
        from(bucket: "{INFLUX_BUCKET}")
          |> range(start: -400d)
          |> filter(fn: (r) => r._measurement == "consumption" and r.uc == "{uc}" and r.source == "fatura")
          |> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
          |> sort(columns: ["_time"], desc: true)
          |> limit(n: 1)
        '''
        latest = None
        for table in query_api.query(flux):
            for record in table.records:
                latest = {
                    "referencia": record.get_time().strftime("%m/%Y"),
                    "consumed_kwh": record.values.get("consumed_kwh"),
                    "total_value_brl": record.values.get("total_value_brl"),
                }
        unidades[uc] = {"label": label, "latest": latest}

    # Estimativa (não é o valor oficial da Celesc): geração acumulada desde
    # que a usina começou a ligar (ver [[project_plant_timeline]]) x tarifa
    # efetiva (total pago / kWh) da fatura mais recente que temos. Some que
    # nenhuma fatura até agora cobre um periodo com compensacao solar, entao
    # essa tarifa efetiva ainda nao inclui nenhum credito de geracao.
    economia_estimada_brl = None
    tarifa_efetiva = _tarifa_efetiva()
    if tarifa_efetiva:
        flux_geracao = f'''
        from(bucket: "{INFLUX_BUCKET}")
          |> range(start: -365d)
          |> filter(fn: (r) => r._measurement == "daily_generation" and r._field == "generated_kwh" and r.plant_id == "{PLANT_TAG}")
          |> sum()
        '''
        geracao_total_kwh = None
        for table in query_api.query(flux_geracao):
            for record in table.records:
                geracao_total_kwh = record.get_value()
        if geracao_total_kwh:
            economia_estimada_brl = round(geracao_total_kwh * tarifa_efetiva, 2)

    return {
        "unidades": unidades,
        "economia_estimada_brl": economia_estimada_brl,
        "economia_e_estimativa": True,
    }


@app.get("/api/consumption/history")
def consumption_history(uc: str, months: int = 13):
    flux = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: -400d)
      |> filter(fn: (r) => r._measurement == "consumption" and r.uc == "{uc}")
      |> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
      |> sort(columns: ["_time"])
    '''
    rows = []
    for table in query_api.query(flux):
        for record in table.records:
            rows.append(
                {
                    "referencia": record.get_time().strftime("%Y-%m"),
                    "consumed_kwh": record.values.get("consumed_kwh"),
                    "total_value_brl": record.values.get("total_value_brl"),
                    "source": record.values.get("source"),
                }
            )
    return rows[-months:]
