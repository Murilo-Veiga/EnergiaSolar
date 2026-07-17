import io
from datetime import datetime

from reportlab.lib import colors
from reportlab.lib.pagesizes import A4
from reportlab.lib.units import mm
from reportlab.pdfgen import canvas

INK = colors.HexColor("#17181c")
MUTED = colors.HexColor("#6b6d76")
LINE = colors.HexColor("#e4e4e0")
PANEL = colors.HexColor("#f7f7f5")
BLUE = colors.HexColor("#3987e5")
AQUA = colors.HexColor("#199e70")
GOOD = colors.HexColor("#0ca30c")
CRITICAL = colors.HexColor("#d03b3b")

MARGIN = 20 * mm
MAX_BARS = 60  # barra fica ilegível além disso numa página A4 — mostra só os últimos N dias do período
MAX_ANNOTATIONS = 6  # idem — anotação demais estoura a página, resto fica só no acesso pelo painel


def _fmt_kwh(v):
    return "--" if v is None else f"{v:.1f}".replace(".", ",")


def _fmt_brl(v):
    return "--" if v is None else f"R$ {v:.2f}".replace(".", ",")


def _fmt_pct(v):
    return "--" if v is None else f"{v:.1f}".replace(".", ",")


def _center(c, x_center, y, text, font, size, color):
    c.setFont(font, size)
    c.setFillColor(color)
    c.drawString(x_center - c.stringWidth(text, font, size) / 2, y, text)


def _draw_section_title(c, x0, y, text):
    c.setFont("Helvetica-Bold", 8.5)
    c.setFillColor(INK)
    c.drawString(x0, y, text)
    return y - 13


def _draw_bars(c, x0, x1, y_top, values, dates, color, bar_h=42):
    """Barra com valor em cima e data embaixo — só cabe legível com poucos
    dias. Com muitos (ex.: período de 1 ano), a data sai primeiro e depois o
    valor também, sobrando só a barra (a tabela mais abaixo cobre os
    números dos últimos dias em detalhe)."""
    values = [v if v is not None else 0 for v in values]
    n = len(values)
    max_v = max(values) * 1.3 if values and max(values) > 0 else 1
    width = x1 - x0
    gap = 2
    bw = max(1.0, (width - gap * (n - 1)) / n) if n else width
    show_values = n <= 31
    show_dates = n <= 15

    top_pad = 10 if show_values else 2
    bottom_pad = 9 if show_dates else 2
    baseline = y_top - top_pad - bar_h

    c.setStrokeColor(LINE)
    c.setLineWidth(0.5)
    c.line(x0, baseline, x1, baseline)

    xcur = x0
    for v, d in zip(values, dates):
        bh = (v / max_v) * (bar_h - 4) if max_v else 0
        if bh > 0:
            c.setFillColor(color)
            c.roundRect(xcur, baseline, bw, bh, min(1.3, bw / 2), stroke=0, fill=1)
        if show_values:
            _center(c, xcur + bw / 2, baseline + bh + 3, _fmt_kwh(v), "Helvetica-Bold", 5.6, INK)
        if show_dates and d:
            _center(c, xcur + bw / 2, baseline - 7.5, d, "Helvetica", 5.3, MUTED)
        xcur += bw + gap

    return baseline - (bottom_pad + 4)


def _draw_wrapped_text(c, x0, x1, y, text, font="Helvetica", size=7, leading=9, color=MUTED):
    """Quebra o texto em linhas que cabem entre x0 e x1, desenhando de cima
    pra baixo — reportlab não quebra linha sozinho num drawString simples."""
    c.setFont(font, size)
    c.setFillColor(color)
    words = text.split(" ")
    line = ""
    for word in words:
        candidate = f"{line} {word}".strip()
        if c.stringWidth(candidate, font, size) > (x1 - x0):
            c.drawString(x0, y, line)
            y -= leading
            line = word
        else:
            line = candidate
    if line:
        c.drawString(x0, y, line)
        y -= leading
    return y


def _draw_table(c, x0, x1, y, rows):
    col_date = x0
    col_kwh = x0 + (x1 - x0) * 0.4
    col_brl = x0 + (x1 - x0) * 0.68

    c.setFont("Helvetica-Bold", 7.5)
    c.setFillColor(MUTED)
    c.drawString(col_date, y, "DATA")
    c.drawString(col_kwh, y, "GERADO")
    c.drawString(col_brl, y, "ECONOMIA ESTIMADA")
    y -= 4
    c.setStrokeColor(INK)
    c.setLineWidth(1)
    c.line(x0, y, x1, y)
    y -= 12

    c.setFont("Helvetica", 8)
    for row in rows:
        c.setFillColor(INK)
        date_str = row["date"][8:10] + "/" + row["date"][5:7] + "/" + row["date"][0:4]
        c.drawString(col_date, y, date_str)
        kwh = row.get("generated_kwh")
        c.drawString(col_kwh, y, f"{_fmt_kwh(kwh)} kWh")
        c.setFillColor(MUTED)
        val = row.get("valor_estimado_brl")
        c.drawString(col_brl, y, _fmt_brl(val))
        c.setStrokeColor(LINE)
        c.setLineWidth(0.4)
        c.line(x0, y - 4, x1, y - 4)
        y -= 15
    return y


def build_history_report_pdf(
    *,
    period_label,
    generated_at,
    rows,
    total_kwh,
    total_brl,
    installed_kwp,
    avg_daily_kwh=None,
    kwh_delta_label=None,
    brl_delta_label=None,
    best_day_period=None,
    best_day_alltime=None,
    peak_power_alltime=None,
    inverter_split=None,
    annotations=None,
):
    """Monta o relatório de geração + economia do período num único PDF, layout
    fixo em página A4 (sem paginação — pensado pra caber num único
    demonstrativo simples, não um extrato completo de todos os dias)."""
    buf = io.BytesIO()
    c = canvas.Canvas(buf, pagesize=A4)
    page_w, page_h = A4
    x0, x1 = MARGIN, page_w - MARGIN
    y = page_h - MARGIN

    # ---- cabeçalho: marca + data de geração à esquerda, período coberto à direita ----
    c.setFont("Helvetica-Bold", 16)
    c.setFillColor(INK)
    c.drawString(x0, y, "Solar Home")
    c.setFont("Helvetica", 9)
    c.setFillColor(MUTED)
    c.drawString(x0, y - 13, "R. Guanabara, 3787 — Fátima, Joinville-SC")
    c.setFont("Helvetica-Bold", 8.5)
    c.setFillColor(BLUE)
    c.drawString(x0, y - 26, f"Relatório gerado em {generated_at.strftime('%d/%m/%Y às %H:%M')}")

    c.setFont("Helvetica-Bold", 8)
    c.setFillColor(MUTED)
    c.drawRightString(x1, y - 2, "PERÍODO COBERTO")
    c.setFont("Helvetica-Bold", 11)
    c.setFillColor(INK)
    c.drawRightString(x1, y - 16, period_label)

    y -= 34
    c.setStrokeColor(INK)
    c.setLineWidth(1.4)
    c.line(x0, y, x1, y)
    y -= 24

    # ---- 3 tiles-resumo ----
    def _sub_color(label):
        if label and label.startswith("▲"):
            return GOOD
        if label and label.startswith("▼"):
            return CRITICAL
        return MUTED

    yield_kwh_kwp = (total_kwh / installed_kwp) if installed_kwp else None
    avg_label = f"média {_fmt_kwh(avg_daily_kwh)} kWh/dia" if avg_daily_kwh is not None else None
    tiles = [
        ("ENERGIA GERADA", f"{_fmt_kwh(total_kwh)} kWh", kwh_delta_label or avg_label, _sub_color(kwh_delta_label)),
        ("ECONOMIA ESTIMADA", _fmt_brl(total_brl), brl_delta_label, _sub_color(brl_delta_label)),
        (
            "RENDIMENTO",
            f"{_fmt_kwh(yield_kwh_kwp)} kWh/kWp" if yield_kwh_kwp is not None else "--",
            f"usina de {_fmt_kwh(installed_kwp)} kWp" if installed_kwp else None,
            MUTED,
        ),
    ]
    tile_w = (x1 - x0 - 2 * 10) / 3
    tx = x0
    for label, value, sub, sub_color in tiles:
        c.setStrokeColor(LINE)
        c.setLineWidth(0.7)
        c.rect(tx, y - 46, tile_w, 46, stroke=1, fill=0)
        c.setFont("Helvetica-Bold", 7.5)
        c.setFillColor(MUTED)
        c.drawString(tx + 10, y - 14, label)
        c.setFont("Helvetica-Bold", 15)
        c.setFillColor(INK)
        c.drawString(tx + 10, y - 30, value)
        if sub:
            c.setFont("Helvetica", 7.5)
            c.setFillColor(sub_color)
            c.drawString(tx + 10, y - 42, sub)
        tx += tile_w + 10
    y -= 46 + 18

    # ---- recordes: melhor dia do período, melhor dia histórico, maior potência histórica ----
    if best_day_period or best_day_alltime or peak_power_alltime:
        c.setFillColor(PANEL)
        c.roundRect(x0, y - 40, x1 - x0, 40, 3, stroke=0, fill=1)
        rec_w = (x1 - x0) / 3
        recs = [
            (
                "MELHOR DIA DO PERÍODO",
                f"{best_day_period['date'][8:10]}/{best_day_period['date'][5:7]} · {_fmt_kwh(best_day_period['generated_kwh'])} kWh"
                if best_day_period else "--",
                None,
            ),
            (
                "MELHOR DIA — HISTÓRICO",
                f"{best_day_alltime['date'][8:10]}/{best_day_alltime['date'][5:7]} · {_fmt_kwh(best_day_alltime['kwh'])} kWh"
                if best_day_alltime and best_day_alltime.get("date") else "--",
                None,
            ),
            (
                "MAIOR POTÊNCIA — HISTÓRICO",
                f"{_fmt_kwh(peak_power_alltime['kw'])} kW" if peak_power_alltime and peak_power_alltime.get("kw") is not None else "--",
                peak_power_alltime["at"][8:10] + "/" + peak_power_alltime["at"][5:7] + " às " + peak_power_alltime["at"][11:16]
                if peak_power_alltime and peak_power_alltime.get("at") else None,
            ),
        ]
        rx = x0
        for label, value, sub in recs:
            c.setFont("Helvetica-Bold", 6.5)
            c.setFillColor(MUTED)
            c.drawString(rx + 10, y - 14, label)
            c.setFont("Helvetica-Bold", 10)
            c.setFillColor(INK)
            c.drawString(rx + 10, y - 27, value)
            if sub:
                c.setFont("Helvetica", 6.5)
                c.setFillColor(MUTED)
                c.drawString(rx + 10, y - 36, sub)
            rx += rec_w
        y -= 40 + 16

    # ---- gráficos diários, com rótulo de valor + data por barra ----
    chart_rows = rows[-MAX_BARS:]
    truncated = len(rows) > MAX_BARS
    chart_dates = [r["date"][8:10] + "/" + r["date"][5:7] for r in chart_rows]

    y = _draw_section_title(c, x0, y, "GERAÇÃO DIÁRIA (KWH)")
    y = _draw_bars(c, x0, x1, y, [r["generated_kwh"] for r in chart_rows], chart_dates, BLUE)
    y -= 16

    y = _draw_section_title(c, x0, y, "ECONOMIA DIÁRIA ESTIMADA (R$)")
    y = _draw_bars(c, x0, x1, y, [r["valor_estimado_brl"] for r in chart_rows], chart_dates, AQUA)
    if truncated:
        c.setFont("Helvetica-Oblique", 6.5)
        c.setFillColor(MUTED)
        c.drawString(x0, y - 2, f"Gráficos mostram os últimos {MAX_BARS} dias do período — a tabela abaixo mostra os últimos 10.")
        y -= 11
    y -= 6

    # ---- contribuição por inversor ----
    if inverter_split and (inverter_split["huawei_kwh"] or inverter_split["foxess_kwh"]):
        total_split = inverter_split["huawei_kwh"] + inverter_split["foxess_kwh"]
        huawei_pct = (inverter_split["huawei_kwh"] / total_split * 100) if total_split else 0
        foxess_pct = 100 - huawei_pct

        y = _draw_section_title(c, x0, y, "CONTRIBUIÇÃO POR INVERSOR")
        bar_h = 12
        w_h = (x1 - x0) * huawei_pct / 100
        c.setFillColor(BLUE)
        c.roundRect(x0, y - bar_h, max(w_h, 2), bar_h, 2, stroke=0, fill=1)
        c.setFillColor(AQUA)
        c.roundRect(x0 + w_h, y - bar_h, max((x1 - x0) - w_h, 2), bar_h, 2, stroke=0, fill=1)
        y -= bar_h + 12

        c.setFont("Helvetica-Bold", 8)
        c.setFillColor(BLUE)
        c.drawString(x0, y, "■")
        c.setFillColor(INK)
        c.setFont("Helvetica", 7.5)
        c.drawString(x0 + 10, y, f"Huawei {_fmt_kwh(inverter_split['huawei_kwh'])} kWh ({_fmt_pct(huawei_pct)}%)")
        c.setFont("Helvetica-Bold", 8)
        c.setFillColor(AQUA)
        c.drawString(x0 + 190, y, "■")
        c.setFillColor(INK)
        c.setFont("Helvetica", 7.5)
        c.drawString(x0 + 200, y, f"FoxESS {_fmt_kwh(inverter_split['foxess_kwh'])} kWh ({_fmt_pct(foxess_pct)}%)")
        y -= 13

        y = _draw_wrapped_text(
            c, x0, x1, y,
            f"Baseado nos dias com dado por inversor disponível dentro do período "
            f"({inverter_split['days_with_data']} de {inverter_split['days_in_period']} dias). "
            f"Capacidade instalada: Huawei 3 kW ({_fmt_pct(inverter_split['huawei_expected_pct'])}% esperado) · "
            f"FoxESS 5 kW ({_fmt_pct(inverter_split['foxess_expected_pct'])}% esperado).",
            size=6.5, leading=8.5,
        )
        y -= 4

    # ---- anotações do período ----
    if annotations:
        y = _draw_section_title(c, x0, y, "ANOTAÇÕES DO PERÍODO")
        shown = annotations[:MAX_ANNOTATIONS]
        c.setFont("Helvetica", 7.5)
        for note in shown:
            c.setFillColor(MUTED)
            date_str = note["date"][8:10] + "/" + note["date"][5:7] + "/" + note["date"][0:4]
            c.drawString(x0, y, date_str)
            c.setFillColor(INK)
            c.drawString(x0 + 55, y, note["note"][:90])
            y -= 11
        if len(annotations) > MAX_ANNOTATIONS:
            c.setFont("Helvetica-Oblique", 6.5)
            c.setFillColor(MUTED)
            c.drawString(x0, y, f"+ {len(annotations) - MAX_ANNOTATIONS} anotação(ões) a mais nesse período — ver no painel.")
            y -= 11
        y -= 6

    y = _draw_table(c, x0, x1, y, list(reversed(rows[-10:])))

    footer = (
        "Valores de economia são uma estimativa (geração × tarifa efetiva da última fatura Celesc), não um "
        "crédito de compensação oficial. Contribuição por inversor considera só os dias com dado disponível "
        "pra cada um dentro do período."
    )
    _draw_wrapped_text(c, x0, x1, MARGIN + 12, footer, size=6.5, leading=8.5)

    c.showPage()
    c.save()
    return buf.getvalue()
