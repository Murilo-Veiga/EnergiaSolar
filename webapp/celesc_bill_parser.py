import io
import re
from datetime import date

MONTH_ABBR = {
    "JAN": 1, "FEV": 2, "MAR": 3, "ABR": 4, "MAI": 5, "JUN": 6,
    "JUL": 7, "AGO": 8, "SET": 9, "OUT": 10, "NOV": 11, "DEZ": 12,
}

_REFERENCIA_LINE = re.compile(r"^(\d{2})/(\d{4})\s+(\d{2}/\d{2}/\d{4})\s+R\$\s+([\d.,]+)$")
_NOME_LINE = re.compile(r"^NOME:\s*(.+)$")
_DIAS_LINE = re.compile(r"^\d{2}/\d{2}/\d{4}\s+\d{2}/\d{2}/\d{4}\s+(\d+)\s+\d{2}/\d{2}/\d{4}$")
_MEDIDOR_LINE = re.compile(r"Energia\s+Único\s+[\d.]+\s+[\d.]+\s+[\d,]+\s+[\d,]+\s+(\d+)")
_BANDEIRA_LINE = re.compile(r"^(Verde|Amarela|Vermelha ?\d?)\s+R\$\s+([\d,]+)\s+\d+$")
_MONTH_LABEL = re.compile(r"^([A-Z]{3})/(\d{2})$")
_INT_LINE = re.compile(r"^\d+$")
_MONTH_COMBINED = re.compile(r"^([A-Z]{3})/(\d{2})\s+(\d+)\s+(\d+)$")
_NOVO_FORMATO_UC = re.compile(r"passará a ser\s+([\d.\-]+)")


class CelescBillParseError(Exception):
    pass


def _brl_to_float(s: str) -> float:
    return float(s.replace(".", "").replace(",", "."))


def _find_uc(lines: list[str]) -> str | None:
    """UC exibida no topo da fatura. Faturas em transição pro novo formato da
    ANEEL (REN 1095/2024) ainda mostram a UC antiga aqui — ver _find_uc_novo_formato."""
    for i, line in enumerate(lines):
        if line.startswith("Cliente:") and i > 0:
            raw = lines[i - 1].strip()
            digits = re.sub(r"[^\d]", "", raw)
            if digits:
                return digits
    return None


def _find_uc_novo_formato(lines: list[str]) -> str | None:
    """Durante a transição, a fatura avisa 'sua UC antiga era X e seu novo
    número e passará a ser Y' — usamos Y como identificador canônico pra não
    quebrar a série no InfluxDB quando a fatura seguinte já vier só com o
    número novo."""
    for line in lines:
        m = _NOVO_FORMATO_UC.search(line)
        if m:
            digits = re.sub(r"[^\d]", "", m.group(1))
            if digits:
                return digits
    return None


def _parse_historico(lines: list[str]) -> list[dict]:
    """Best-effort: extrai a tabela de histórico de 12+ meses (kWh) que cada
    fatura da Celesc já traz. Se o layout não bater com o esperado, devolve
    lista vazia em vez de derrubar o parse dos campos principais."""
    try:
        header_idx = next(i for i, l in enumerate(lines) if l.startswith("Consumo Faturado"))
    except StopIteration:
        return []

    months = []
    i = header_idx + 1
    while i < len(lines) and _MONTH_LABEL.match(lines[i]):
        months.append(lines[i])
        i += 1
    if not months:
        return []

    kwh_values = []
    while i < len(lines) and _INT_LINE.match(lines[i]) and len(kwh_values) < len(months):
        kwh_values.append(int(lines[i]))
        i += 1

    if len(kwh_values) != len(months):
        return []

    historico = []
    for month_label, kwh in zip(months, kwh_values):
        m = _MONTH_LABEL.match(month_label)
        mes, ano = m.group(1), int(m.group(2))
        if mes not in MONTH_ABBR:
            continue
        historico.append({"ano": 2000 + ano, "mes": MONTH_ABBR[mes], "consumo_kwh": kwh})

    # linha final combinada tipo "JUN/25 0 29" (mês mais antigo, formato diferente)
    while i < len(lines):
        m = _MONTH_COMBINED.match(lines[i])
        if not m:
            i += 1
            continue
        mes, ano, kwh = m.group(1), int(m.group(2)), int(m.group(3))
        if mes in MONTH_ABBR:
            historico.append({"ano": 2000 + ano, "mes": MONTH_ABBR[mes], "consumo_kwh": kwh})
        break

    return historico


def parse_bill_text(text: str) -> dict:
    lines = [l.strip() for l in text.splitlines() if l.strip()]

    uc = _find_uc_novo_formato(lines) or _find_uc(lines)
    if uc is None:
        raise CelescBillParseError("Não encontrei a Unidade Consumidora no PDF")

    titular = None
    referencia_ano = referencia_mes = None
    vencimento = None
    total_pagar = None
    dias_faturados = None
    consumo_kwh = None
    bandeira = None
    bandeira_valor_kwh = None

    for line in lines:
        if titular is None:
            m = _NOME_LINE.match(line)
            if m:
                titular = m.group(1).strip()
                continue

        if referencia_ano is None:
            m = _REFERENCIA_LINE.match(line)
            if m:
                referencia_mes = int(m.group(1))
                referencia_ano = int(m.group(2))
                vencimento = m.group(3)
                total_pagar = _brl_to_float(m.group(4))
                continue

        if dias_faturados is None:
            m = _DIAS_LINE.match(line)
            if m:
                dias_faturados = int(m.group(1))
                continue

        if consumo_kwh is None:
            m = _MEDIDOR_LINE.search(line)
            if m:
                consumo_kwh = int(m.group(1))
                continue

        if bandeira is None:
            m = _BANDEIRA_LINE.match(line)
            if m:
                bandeira = m.group(1)
                bandeira_valor_kwh = _brl_to_float(m.group(2))
                continue

    faltando = [
        nome
        for nome, valor in (
            ("referência/vencimento/total", referencia_ano),
            ("consumo (kWh)", consumo_kwh),
        )
        if valor is None
    ]
    if faltando:
        raise CelescBillParseError(f"Campos obrigatórios não encontrados no PDF: {', '.join(faltando)}")

    return {
        "uc": uc,
        "titular": titular,
        "referencia_ano": referencia_ano,
        "referencia_mes": referencia_mes,
        "vencimento": vencimento,
        "total_pagar_brl": total_pagar,
        "consumo_kwh": consumo_kwh,
        "dias_faturados": dias_faturados,
        "bandeira": bandeira,
        "bandeira_valor_kwh": bandeira_valor_kwh,
        "historico": _parse_historico(lines),
    }


def parse_bill(pdf_bytes: bytes) -> dict:
    import pdfplumber

    with pdfplumber.open(io.BytesIO(pdf_bytes)) as pdf:
        text = "\n".join(page.extract_text() or "" for page in pdf.pages)
    return parse_bill_text(text)


def referencia_date(ano: int, mes: int) -> date:
    return date(ano, mes, 1)
