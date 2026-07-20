import { useEffect, useRef, useState } from "react";
import { Chart } from "chart.js/auto";
import { api, type HistoryResponse } from "../lib/api";
import { fmtNum, fmtBRL } from "../lib/fmt";

type Metric = "gerado" | "economia";
type Range = "semana" | "mes" | "ano";

const RANGE_LABEL: Record<Range, string> = { semana: "semana (7 dias)", mes: "mês (30 dias)", ano: "ano (12 meses)" };
const AVG_UNIT_LABEL: Record<Range, string> = { semana: "diária", mes: "diária", ano: "mensal" };

// Espelha initGeracaoChart() em templates/index.html — usado só no
// Dashboard (Histórico tem seus próprios gráficos gerado/economia fixos,
// sem toggle de métrica).
export function GeracaoChart({ plantId }: { plantId: string }) {
  const [metric, setMetric] = useState<Metric>("gerado");
  const [range, setRange] = useState<Range>("mes");
  const [data, setData] = useState<HistoryResponse | null>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const chartRef = useRef<Chart | null>(null);

  useEffect(() => {
    let cancelled = false;
    void api.get<HistoryResponse>(`/api/plants/${plantId}/history?range=${range}`).then((d) => {
      if (!cancelled) setData(d);
    });
    return () => {
      cancelled = true;
    };
  }, [plantId, range]);

  useEffect(() => {
    if (!data || !canvasRef.current) return;
    const isEconomia = metric === "economia";
    const labels = data.rows.map((r) => r.date.slice(5));
    const values = data.rows.map((r) => (isEconomia ? r.valor_estimado_brl : r.generated_kwh));
    const color = isEconomia ? "#199e70" : "#3987e5";

    const validValues = values.filter((v): v is number => v !== null && v !== undefined);
    const avg = validValues.length ? validValues.reduce((a, b) => a + b, 0) / validValues.length : 0;
    const avgUnitLabel = AVG_UNIT_LABEL[range];

    chartRef.current?.destroy();
    chartRef.current = new Chart(canvasRef.current, {
      type: "bar",
      data: {
        labels,
        datasets: [
          { type: "bar", data: values, backgroundColor: color, borderRadius: 4, borderSkipped: false, maxBarThickness: 26 },
          {
            type: "line",
            data: labels.map(() => avg),
            borderColor: "rgba(208,59,59,0.55)",
            borderWidth: 1.5,
            borderDash: [6, 4],
            pointRadius: 0,
            pointHitRadius: 0,
            pointHoverRadius: 0,
            fill: false,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: "index", intersect: false },
        plugins: {
          legend: { display: false },
          tooltip: {
            backgroundColor: "#050505",
            borderColor: "rgba(255,255,255,0.10)",
            borderWidth: 1,
            padding: 10,
            titleColor: "#c3c2b7",
            bodyColor: "#ffffff",
            cornerRadius: 7,
            displayColors: true,
            callbacks: {
              label: (ctx) => {
                const v = isEconomia ? fmtBRL(ctx.parsed.y) : `${fmtNum(ctx.parsed.y)} kWh`;
                return ctx.dataset.type === "line" ? `Média ${avgUnitLabel}: ${v}` : v;
              },
            },
          },
        },
        scales: {
          x: { ticks: { color: "#898781" }, grid: { display: false } },
          y: { ticks: { color: "#898781" }, grid: { color: "#2c2c2a" }, beginAtZero: true },
        },
      },
    });
  }, [data, metric, range]);

  const isEconomia = metric === "economia";
  const total = data ? (isEconomia ? data.total_brl : data.total_kwh) : null;
  const validValues = data ? data.rows.map((r) => (isEconomia ? r.valor_estimado_brl : r.generated_kwh)).filter((v): v is number => v !== null) : [];
  const avg = validValues.length ? validValues.reduce((a, b) => a + b, 0) / validValues.length : null;

  return (
    <div className="card chart-card">
      <div className="chart-head">
        <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
          <h3>{isEconomia ? "Economia" : "Gerado"}</h3>
          <div className="range-toggle">
            <span className={metric === "gerado" ? "active" : ""} onClick={() => setMetric("gerado")}>
              Gerado (kWh)
            </span>
            <span className={metric === "economia" ? "active" : ""} onClick={() => setMetric("economia")}>
              Economia (R$)
            </span>
          </div>
        </div>
        <div className="chart-total">
          <div className="chart-total-label">
            Total {isEconomia ? "economizado" : "gerado"} — {RANGE_LABEL[range]}
          </div>
          <div className="chart-total-value">{isEconomia ? fmtBRL(total) : <>{fmtNum(total)}<span className="unit"> kWh</span></>}</div>
          <div className="chart-total-avg">
            {avg !== null ? `média ${AVG_UNIT_LABEL[range]}: ${isEconomia ? fmtBRL(avg) : `${fmtNum(avg)} kWh`}` : ""}
          </div>
        </div>
        <div className="range-toggle">
          {(["semana", "mes", "ano"] as Range[]).map((r) => (
            <span key={r} className={range === r ? "active" : ""} onClick={() => setRange(r)}>
              {r === "semana" ? "Semana" : r === "mes" ? "Mês" : "Ano"}
            </span>
          ))}
        </div>
      </div>
      <div className="chart-box" style={{ height: 180 }}>
        <canvas ref={canvasRef} />
      </div>
    </div>
  );
}
