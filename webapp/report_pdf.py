import io
from datetime import datetime

from reportlab.lib import colors
from reportlab.lib.pagesizes import A4
from reportlab.lib.units import mm
from reportlab.pdfgen import canvas

INK = colors.HexColor("#17181c")
MUTED = colors.HexColor("#6b6d76")
LINE = colors.HexColor("#e4e4e0")
BLUE = colors.HexColor("#3987e5")
AQUA = colors.HexColor("#199e70")
GOOD = colors.HexColor("#0ca30c")

MARGIN = 20 * mm
MAX_BARS = 60  # barra fica ilegível além disso numa página A4 — mostra só os últimos N dias do período


def _draw_section_title(c, x0, y, text):
    c.setFont("Helvetica-Bold", 8.5)
    c.setFillColor(INK)
    c.drawString(x0, y, text)
    return y - 14


def _draw_bars(c, x0, x1, y_top, values, color, height=60):
    values = [v if v is not None else 0 for v in values]
    max_v = max(values) * 1.15 if values and max(values) > 0 else 1
    n = len(values)
    width = x1 - x0
    gap = 2
    bw = max(1.2, (width - gap * (n - 1)) / n) if n else width
    baseline = y_top - height
    c.setStrokeColor(LINE)
    c.setLineWidth(0.5)
    c.line(x0, baseline, x1, baseline)
    c.setFillColor(color)
    xcur = x0
    for v in values:
        bh = (v / max_v) * (height - 4) if max_v else 0
        if bh > 0:
            c.roundRect(xcur, baseline, bw, bh, min(1.5, bw / 2), stroke=0, fill=1)
        xcur += bw + gap
    return baseline - 6


def _draw_wrapped_text(c, x0, x1, y, text, font="Helvetica", size=7, leading=9):
    """Quebra o texto em linhas que cabem entre x0 e x1, desenhando de cima
    pra baixo — reportlab não quebra linha sozinho num drawString simples."""
    c.setFont(font, size)
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
        c.drawString(col_date, y, row["date"])
        kwh = row.get("generated_kwh")
        c.drawString(col_kwh, y, f"{kwh:.1f} kWh" if kwh is not None else "--")
        c.setFillColor(MUTED)
        val = row.get("valor_estimado_brl")
        c.drawString(col_brl, y, f"R$ {val:.2f}".replace(".", ",") if val is not None else "--")
        c.setStrokeColor(LINE)
        c.setLineWidth(0.4)
        c.line(x0, y - 4, x1, y - 4)
        y -= 15
    return y


def build_history_report_pdf(*, period_label, rows, total_kwh, total_brl, installed_kwp, delta_label=None):
    """Monta o relatório de geração + economia do período num único PDF, layout
    fixo em página A4 (sem paginação — pensado pra caber num único
    demonstrativo simples, não um extrato completo de todos os dias)."""
    buf = io.BytesIO()
    c = canvas.Canvas(buf, pagesize=A4)
    page_w, page_h = A4
    x0, x1 = MARGIN, page_w - MARGIN
    y = page_h - MARGIN

    c.setFont("Helvetica-Bold", 16)
    c.setFillColor(INK)
    c.drawString(x0, y, "Solar Home")
    c.setFont("Helvetica", 9)
    c.setFillColor(MUTED)
    c.drawString(x0, y - 14, "R. Guanabara, 3787 — Fátima, Joinville-SC")

    c.setFont("Helvetica-Bold", 8)
    c.setFillColor(MUTED)
    c.drawRightString(x1, y - 2, "RELATÓRIO DO PERÍODO")
    c.setFont("Helvetica-Bold", 11)
    c.setFillColor(INK)
    c.drawRightString(x1, y - 16, period_label)

    y -= 24
    c.setStrokeColor(INK)
    c.setLineWidth(1.4)
    c.line(x0, y, x1, y)
    y -= 28

    yield_kwh_kwp = (total_kwh / installed_kwp) if installed_kwp else None
    tiles = [
        ("ENERGIA GERADA", f"{total_kwh:.1f} kWh", delta_label),
        ("ECONOMIA ESTIMADA", f"R$ {total_brl:.2f}".replace(".", ","), delta_label),
        (
            "RENDIMENTO",
            f"{yield_kwh_kwp:.1f} kWh/kWp" if yield_kwh_kwp is not None else "--",
            f"usina de {installed_kwp:.1f} kWp" if installed_kwp else None,
        ),
    ]
    tile_w = (x1 - x0 - 2 * 10) / 3
    tx = x0
    for label, value, sub in tiles:
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
            c.setFillColor(GOOD)
            c.drawString(tx + 10, y - 42, sub)
        tx += tile_w + 10
    y -= 46 + 26

    chart_rows = rows[-MAX_BARS:]
    truncated = len(rows) > MAX_BARS

    y = _draw_section_title(c, x0, y, "GERAÇÃO DIÁRIA (KWH)")
    y = _draw_bars(c, x0, x1, y, [r["generated_kwh"] for r in chart_rows], BLUE)
    y -= 20

    y = _draw_section_title(c, x0, y, "ECONOMIA DIÁRIA ESTIMADA (R$)")
    y = _draw_bars(c, x0, x1, y, [r["valor_estimado_brl"] for r in chart_rows], AQUA)
    if truncated:
        c.setFont("Helvetica-Oblique", 7)
        c.setFillColor(MUTED)
        c.drawString(x0, y - 2, f"Gráficos mostram os últimos {MAX_BARS} dias do período — a tabela abaixo mostra os últimos 10.")
        y -= 12
    y -= 8

    y = _draw_table(c, x0, x1, y, list(reversed(rows[-10:])))

    c.setFillColor(MUTED)
    now = datetime.now().strftime("%d/%m/%Y às %H:%M")
    footer = (
        f"Documento gerado automaticamente pelo Solar Home em {now} · Valores de economia são uma "
        "estimativa (geração × tarifa efetiva da última fatura Celesc), não um crédito de "
        "compensação oficial."
    )
    _draw_wrapped_text(c, x0, x1, MARGIN + 12, footer)

    c.showPage()
    c.save()
    return buf.getvalue()
