import os
from datetime import datetime, timezone

import requests
from fastapi import FastAPI, File, HTTPException, Request, UploadFile
from fastapi.responses import HTMLResponse
from fastapi.templating import Jinja2Templates
from influxdb_client import InfluxDBClient, Point
from influxdb_client.client.write_api import SYNCHRONOUS

from celesc_bill_parser import CelescBillParseError, parse_bill

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


def _forecast_days():
    resp = requests.get(
        "https://api.open-meteo.com/v1/forecast",
        params={
            "latitude": PLANT_LAT,
            "longitude": PLANT_LON,
            "daily": "weathercode,temperature_2m_max,temperature_2m_min,"
            "shortwave_radiation_sum,precipitation_sum,precipitation_probability_max,"
            "sunrise,sunset",
            "timezone": "America/Sao_Paulo",
            "forecast_days": 5,
        },
        timeout=10,
    )
    resp.raise_for_status()
    daily = resp.json()["daily"]

    days = []
    for i, date in enumerate(daily["time"]):
        code = daily["weathercode"][i]
        description, rating = WMO_CODES.get(code, ("Desconhecido", "moderado"))
        days.append(
            {
                "date": date,
                "weather": description,
                "rating": rating,
                "temp_max": daily["temperature_2m_max"][i],
                "temp_min": daily["temperature_2m_min"][i],
                "solar_radiation_mj_m2": daily["shortwave_radiation_sum"][i],
                "precipitation_mm": daily["precipitation_sum"][i],
                "precipitation_probability_pct": daily["precipitation_probability_max"][i],
                "sunrise": daily["sunrise"][i][11:16],
                "sunset": daily["sunset"][i][11:16],
            }
        )
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


@app.get("/api/summary")
def summary():
    instantaneous = _last_value("plant_status", "instantaneous_power_kw")
    installed = _last_value("plant_status", "installed_power_kwp", "-30d")
    today_generated = _last_value("daily_generation", "generated_kwh", "-3d")
    has_alarm = _last_value("plant_status", "has_alarm", "-1h")

    is_online = instantaneous is not None and (instantaneous > 0 or (today_generated or 0) > 0)
    status = "alerta" if has_alarm else ("online" if is_online else "pendente")

    peak_power_kw = peak_power_at = None
    flux_peak = f'''
    from(bucket: "{INFLUX_BUCKET}")
      |> range(start: -24h)
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
        result[inverter] = entry
    return result


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


RANGE_DAYS = {"semana": 7, "mes": 30, "ano": 365}


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

    return {
        "rows": rows,
        "total_kwh": round(total_kwh, 1),
        "total_brl": round(total_kwh * tarifa, 2) if tarifa else None,
    }


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
