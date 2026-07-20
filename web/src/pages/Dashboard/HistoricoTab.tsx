import { useEffect, useRef, useState, type FormEvent } from "react";
import { Chart } from "chart.js/auto";
import { useActivePlant } from "../../context/PlantContext";
import { api, type Annotation, type HistoryRecords, type HistoryResponse } from "../../lib/api";
import { fmtNum, fmtBRL } from "../../lib/fmt";
import { Collapsible } from "../../components/Collapsible";

type Range = "semana" | "mes" | "ano";

function deltaLabel(current: number | null, previous: number | null): string {
  if (!previous) return "";
  const pct = Math.round(((current! - previous) / previous) * 100);
  if (pct === 0) return "≈ igual ao período anterior";
  const arrow = pct > 0 ? "▲" : "▼";
  return `${arrow} ${Math.abs(pct)}% ${pct > 0 ? "a mais" : "a menos"} que no período anterior`;
}

function useBarChart(canvasRef: React.RefObject<HTMLCanvasElement | null>, labels: string[], values: (number | null)[], color: string) {
  const chartRef = useRef<Chart | null>(null);
  useEffect(() => {
    if (!canvasRef.current) return;
    chartRef.current?.destroy();
    chartRef.current = new Chart(canvasRef.current, {
      type: "bar",
      data: { labels, datasets: [{ data: values, backgroundColor: color, borderRadius: 4, borderSkipped: false, maxBarThickness: 26 }] },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false } },
        scales: {
          x: { ticks: { color: "#898781" }, grid: { display: false } },
          y: { ticks: { color: "#898781" }, grid: { color: "#2c2c2a" }, beginAtZero: true },
        },
      },
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [labels.join(","), values.join(","), color]);
}

export function HistoricoTab() {
  const plant = useActivePlant();
  const [range, setRange] = useState<Range>("mes");
  const [history, setHistory] = useState<HistoryResponse | null>(null);
  const [installedKwp, setInstalledKwp] = useState<number | null>(null);
  const [records, setRecords] = useState<HistoryRecords | null>(null);
  const [annotations, setAnnotations] = useState<Annotation[]>([]);
  const [annoDate, setAnnoDate] = useState(new Date().toISOString().slice(0, 10));
  const [annoText, setAnnoText] = useState("");
  const [submittingAnno, setSubmittingAnno] = useState(false);

  const geradoCanvas = useRef<HTMLCanvasElement>(null);
  const economiaCanvas = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    void api.get<HistoryRecords>(`/api/plants/${plant.id}/history/records`).then(setRecords);
    void api.get<{ installed_power_kwp: number | null }>(`/api/plants/${plant.id}/summary`).then((s) => setInstalledKwp(s.installed_power_kwp));
    void loadAnnotations();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [plant.id]);

  useEffect(() => {
    void api.get<HistoryResponse>(`/api/plants/${plant.id}/history?range=${range}`).then(setHistory);
  }, [plant.id, range]);

  async function loadAnnotations() {
    const a = await api.get<{ rows: Annotation[] }>(`/api/plants/${plant.id}/annotations?range=ano`);
    setAnnotations(a.rows);
  }

  const labels = history?.rows.map((r) => r.date.slice(5)) ?? [];
  const geradoValues = history?.rows.map((r) => r.generated_kwh) ?? [];
  const economiaValues = history?.rows.map((r) => r.valor_estimado_brl) ?? [];
  useBarChart(geradoCanvas, labels, geradoValues, "#3987e5");
  useBarChart(economiaCanvas, labels, economiaValues, "#199e70");

  // Sequência de dias acima da média + melhor/pior dia do período.
  const validRows = history?.rows.filter((r) => r.generated_kwh !== null && r.generated_kwh !== undefined) ?? [];
  const avgKwh = history && validRows.length ? history.total_kwh / validRows.length : null;
  let streak = 0;
  if (history) {
    for (let i = history.rows.length - 1; i >= 0; i--) {
      const v = history.rows[i].generated_kwh;
      if (avgKwh !== null && v !== null && v !== undefined && v > avgKwh) streak++;
      else break;
    }
  }
  const best = validRows.length ? validRows.reduce((a, b) => ((b.generated_kwh ?? 0) > (a.generated_kwh ?? 0) ? b : a)) : null;
  const worst = validRows.length ? validRows.reduce((a, b) => ((b.generated_kwh ?? 0) < (a.generated_kwh ?? 0) ? b : a)) : null;

  const yieldCurrent = history && installedKwp ? history.total_kwh / installedKwp : null;
  const yieldPrevious = history && installedKwp && history.previous_total_kwh ? history.previous_total_kwh / installedKwp : null;

  async function handleAnnotationSubmit(e: FormEvent) {
    e.preventDefault();
    if (!annoDate || !annoText.trim()) return;
    setSubmittingAnno(true);
    try {
      await api.post(`/api/plants/${plant.id}/annotations`, { date: annoDate, note: annoText.trim() });
      setAnnoText("");
      await loadAnnotations();
    } finally {
      setSubmittingAnno(false);
    }
  }

  return (
    <div>
      <div
        className="card"
        style={{ padding: "16px 20px", marginBottom: 14, display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 12 }}
      >
        <p style={{ margin: 0, fontSize: 12.5, color: "var(--ink-muted)", maxWidth: "60ch", lineHeight: 1.6 }}>
          O que sua instalação já registrou: quanto gerou, quanto isso valeu em dinheiro, e seus melhores resultados — sempre no período
          escolhido ao lado.
        </p>
        <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
          <span
            className="btn-upload"
            title="Depende do gerador de relatório em PDF, ainda não portado pro novo backend"
            style={{ textDecoration: "none", display: "inline-flex", alignItems: "center", gap: 6, opacity: 0.5, cursor: "not-allowed" }}
          >
            📄 Baixar relatório em PDF
          </span>
          <div className="range-toggle">
            {(["semana", "mes", "ano"] as Range[]).map((r) => (
              <span key={r} className={range === r ? "active" : ""} onClick={() => setRange(r)}>
                {r === "semana" ? "Semana" : r === "mes" ? "Mês" : "Ano"}
              </span>
            ))}
          </div>
        </div>
      </div>

      <Collapsible
        icon="sun"
        iconColor="blue"
        title="Quanto sua instalação gerou"
        tooltip="Mostra a energia produzida no período escolhido, dia a dia, e se está gerando mais ou menos do que no período anterior."
        defaultOpen
      >
        <div style={{ marginBottom: 14 }}>
          <div className="chart-total-value" style={{ textAlign: "left" }}>
            {fmtNum(history?.total_kwh)}
            <span className="unit"> kWh</span>
          </div>
          {installedKwp && history && (
            <div className="chart-total-avg" style={{ textAlign: "left", color: "var(--ink-muted)" }}>
              {fmtNum(history.total_kwh / installedKwp)} kWh pra cada kW instalado
            </div>
          )}
          <div className="chart-total-avg" style={{ textAlign: "left", color: "var(--good)" }}>
            {history ? deltaLabel(history.total_kwh, history.previous_total_kwh) : ""}
          </div>
        </div>
        <div className="chart-box" style={{ height: 200 }}>
          <canvas ref={geradoCanvas} />
        </div>
      </Collapsible>

      <Collapsible
        icon="wallet"
        iconColor="aqua"
        title="Quanto você economizou"
        tooltip="Estima quanto você deixou de pagar pra Celesc com a energia que a instalação gerou, usando a tarifa da sua fatura mais recente."
      >
        <div style={{ marginBottom: 14 }}>
          <div className="chart-total-value" style={{ textAlign: "left" }}>
            {fmtBRL(history?.total_brl)}
          </div>
          <div className="chart-total-avg" style={{ textAlign: "left", color: "var(--good)" }}>
            {history ? deltaLabel(history.total_brl, history.previous_total_brl) : ""}
          </div>
        </div>
        <div className="chart-box" style={{ height: 200 }}>
          <canvas ref={economiaCanvas} />
        </div>
        <div className="chart-caption" style={{ marginTop: 10 }}>
          Valor estimado = kWh × tarifa efetiva da fatura mais recente — mesma estimativa da aba Consumo.
        </div>
      </Collapsible>

      <Collapsible
        icon="star"
        iconColor="gold"
        title="Seus melhores dias e meses"
        tooltip="Guarda os recordes de geração e potência já alcançados, pra você ter um parâmetro de comparação no futuro. Independe do período selecionado acima — é sempre desde que a instalação ligou."
      >
        <div className="day-grid" style={{ gridTemplateColumns: "repeat(3,1fr)" }}>
          <div>
            <div className="l">Melhor dia</div>
            <div className="v">{records?.best_day_kwh !== null && records?.best_day_kwh !== undefined ? `${fmtNum(records.best_day_kwh)} kWh · ${records.best_day_date}` : "--"}</div>
          </div>
          <div>
            <div className="l">Melhor mês</div>
            <div className="v">
              {records?.best_month_kwh !== null && records?.best_month_kwh !== undefined ? `${fmtNum(records.best_month_kwh)} kWh · ${records.best_month_label}` : "--"}
            </div>
          </div>
          <div>
            <div className="l">Maior potência já vista</div>
            <div className="v">
              {records?.peak_power_kw !== null && records?.peak_power_kw !== undefined && records?.peak_power_at
                ? `${fmtNum(records.peak_power_kw, 2)} kW · ${new Date(records.peak_power_at).toLocaleDateString("pt-BR")}`
                : "--"}
            </div>
          </div>
        </div>
      </Collapsible>

      <Collapsible
        icon="trendingUp"
        iconColor="gold"
        title="Sequência de dias acima da média"
        tooltip="Quantos dias seguidos a instalação vem gerando acima da média do período — e o melhor/pior dia de dentro desse mesmo período."
        newKey="hist-streak"
      >
        <div className="hero-line">
          <span className="v" style={{ fontSize: 30 }}>
            {streak}
          </span>
          <span className="aux">
            {avgKwh !== null
              ? `dia${streak === 1 ? "" : "s"} seguido${streak === 1 ? "" : "s"} gerando acima da média do período (${fmtNum(avgKwh)} kWh/dia)`
              : "sem dado suficiente ainda"}
          </span>
        </div>
        <div className="day-grid" style={{ gridTemplateColumns: "repeat(2,1fr)", marginTop: 16 }}>
          <div>
            <div className="l">Melhor dia do período</div>
            <div className="v">{best ? `${fmtNum(best.generated_kwh)} kWh · ${best.date}` : "--"}</div>
          </div>
          <div>
            <div className="l">Pior dia do período</div>
            <div className="v">{worst ? `${fmtNum(worst.generated_kwh)} kWh · ${worst.date}` : "--"}</div>
          </div>
        </div>
      </Collapsible>

      <Collapsible
        icon="sun"
        iconColor="blue"
        title="Rendimento comparado ao período anterior"
        tooltip="kWh gerados pra cada kW instalado, comparando este período com o imediatamente anterior — mais justo que comparar só o total, porque não depende do tamanho do período."
        newKey="hist-yield"
      >
        <div className="day-grid" style={{ gridTemplateColumns: "repeat(2,1fr)" }}>
          <div>
            <div className="l">Rendimento deste período</div>
            <div className="v">{yieldCurrent !== null ? `${fmtNum(yieldCurrent)} kWh/kWp` : "--"}</div>
          </div>
          <div>
            <div className="l">Rendimento período anterior</div>
            <div className="v">{yieldPrevious !== null ? `${fmtNum(yieldPrevious)} kWh/kWp` : "--"}</div>
          </div>
        </div>
      </Collapsible>

      <Collapsible
        icon="mapPin"
        iconColor="blue"
        title="Anotações sobre eventos importantes"
        tooltip="Espaço pra você anotar algo que aconteceu num dia específico (uma limpeza, um reparo) e ajuda a explicar um salto no gráfico depois."
      >
        <form className="anno-form" onSubmit={handleAnnotationSubmit}>
          <input type="date" required value={annoDate} onChange={(e) => setAnnoDate(e.target.value)} />
          <input
            type="text"
            placeholder="Ex.: limpeza dos painéis"
            maxLength={280}
            required
            value={annoText}
            onChange={(e) => setAnnoText(e.target.value)}
          />
          <button type="submit" className="btn-upload" disabled={submittingAnno}>
            Salvar
          </button>
        </form>
        <div className="anno-list">
          {annotations.map((a) => (
            <div className="anno-list-item" key={a.date}>
              <span className="d">{a.date}</span>
              <span className="n">{a.note}</span>
            </div>
          ))}
        </div>
        {annotations.length === 0 && <div className="anno-empty">Nenhuma anotação nesse período ainda.</div>}
      </Collapsible>

      <div className="card" style={{ overflow: "hidden", marginTop: 14 }}>
        <div style={{ padding: "16px 20px", borderBottom: "1px solid var(--border)" }}>
          <h3 style={{ margin: 0, fontSize: 13, fontWeight: 600, color: "var(--ink-2)" }}>Histórico recente</h3>
        </div>
        <table className="hist-table">
          <thead>
            <tr>
              <th>Data</th>
              <th>Gerado</th>
              <th>Valor estimado</th>
            </tr>
          </thead>
          <tbody>
            {history?.rows
              .slice()
              .reverse()
              .map((r) => (
                <tr key={r.date}>
                  <td className="date">{r.date}</td>
                  <td className="num">{fmtNum(r.generated_kwh)} kWh</td>
                  <td className="num" style={{ color: "var(--ink-2)" }}>
                    {r.valor_estimado_brl !== null ? fmtBRL(r.valor_estimado_brl) : "--"}
                  </td>
                </tr>
              ))}
          </tbody>
        </table>
        <div style={{ padding: "10px 20px", fontSize: 11, color: "var(--ink-muted)" }}>
          Valor estimado = kWh × tarifa efetiva da fatura mais recente — mesma estimativa da aba Consumo.
        </div>
      </div>
    </div>
  );
}
