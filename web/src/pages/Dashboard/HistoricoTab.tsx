import { useEffect, useRef, useState, type FormEvent } from "react";
import { Chart } from "chart.js/auto";
import { useActivePlant } from "../../context/PlantContext";
import { api, type Annotation, type HistoryRecords, type HistoryResponse } from "../../lib/api";

type Range = "semana" | "mes" | "ano";

function fmtNum(v: number | null | undefined, decimals = 1): string {
  if (v === null || v === undefined) return "--";
  return v.toLocaleString("pt-BR", { minimumFractionDigits: decimals, maximumFractionDigits: decimals });
}

export function HistoricoTab() {
  const plant = useActivePlant();
  const [range, setRange] = useState<Range>("mes");
  const [history, setHistory] = useState<HistoryResponse | null>(null);
  const [records, setRecords] = useState<HistoryRecords | null>(null);
  const [annotations, setAnnotations] = useState<Annotation[]>([]);
  const [annoDate, setAnnoDate] = useState("");
  const [annoText, setAnnoText] = useState("");
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const chartRef = useRef<Chart | null>(null);

  useEffect(() => {
    void api.get<HistoryRecords>(`/api/plants/${plant.id}/history/records`).then(setRecords);
  }, [plant.id]);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      const [h, a] = await Promise.all([
        api.get<HistoryResponse>(`/api/plants/${plant.id}/history?range=${range}`),
        api.get<{ rows: Annotation[] }>(`/api/plants/${plant.id}/annotations?range=${range}`),
      ]);
      if (cancelled) return;
      setHistory(h);
      setAnnotations(a.rows);
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [plant.id, range]);

  useEffect(() => {
    if (!history || !canvasRef.current) return;
    const labels = history.rows.map((r) => r.date.slice(5));
    const data = history.rows.map((r) => r.generated_kwh ?? 0);
    const avg = history.rows.length ? history.total_kwh / history.rows.length : 0;

    chartRef.current?.destroy();
    chartRef.current = new Chart(canvasRef.current, {
      type: "bar",
      data: {
        labels,
        datasets: [
          { label: "Gerado (kWh)", data, backgroundColor: "#3987e5", borderRadius: 4, maxBarThickness: 22 },
          { label: "Média do período", data: labels.map(() => avg), type: "line", borderColor: "#d03b3b", borderDash: [6, 4], pointRadius: 0 },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: "index", intersect: false },
        scales: {
          x: { ticks: { color: "#898781" }, grid: { display: false } },
          y: { ticks: { color: "#898781" }, grid: { color: "#2c2c2a" }, beginAtZero: true },
        },
      },
    });
  }, [history]);

  async function handleAnnotationSubmit(e: FormEvent) {
    e.preventDefault();
    if (!annoDate || !annoText) return;
    await api.post(`/api/plants/${plant.id}/annotations`, { date: annoDate, note: annoText });
    setAnnoText("");
    const a = await api.get<{ rows: Annotation[] }>(`/api/plants/${plant.id}/annotations?range=${range}`);
    setAnnotations(a.rows);
  }

  const streak = (() => {
    if (!history || history.rows.length === 0) return null;
    const avg = history.total_kwh / history.rows.length;
    let count = 0;
    for (let i = history.rows.length - 1; i >= 0; i--) {
      if ((history.rows[i].generated_kwh ?? 0) > avg) count++;
      else break;
    }
    const best = history.rows.reduce((a, b) => ((a.generated_kwh ?? 0) > (b.generated_kwh ?? 0) ? a : b));
    const worst = history.rows.reduce((a, b) => ((a.generated_kwh ?? 0) < (b.generated_kwh ?? 0) ? a : b));
    return { count, avg, best, worst };
  })();

  const yieldCurrent = history ? history.total_kwh / plant.installed_power_kwp : null;
  const yieldPrevious = history ? history.previous_total_kwh / plant.installed_power_kwp : null;

  return (
    <div>
      <div className="card" style={{ padding: "16px 20px", marginBottom: 14, display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 12 }}>
        <p style={{ margin: 0, fontSize: 12.5, color: "var(--ink-muted)", maxWidth: "60ch" }}>
          O que sua usina já registrou: quanto gerou e seus melhores resultados, no período escolhido.
        </p>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <span className="btn-upload" title="Depende do gerador de relatório PDF, ainda não portado" style={{ opacity: 0.5, cursor: "not-allowed" }}>
            📄 Baixar relatório em PDF (em breve)
          </span>
          <div className="range-toggle">
            {(["semana", "mes", "ano"] as Range[]).map((r) => (
              <span key={r} className={range === r ? "active" : ""} style={{ cursor: "pointer" }} onClick={() => setRange(r)}>
                {r === "semana" ? "Semana" : r === "mes" ? "Mês" : "Ano"}
              </span>
            ))}
          </div>
        </div>
      </div>

      <div className="card alert-card">
        <div className="alert-head" style={{ padding: "14px 16px" }}>
          <h3>Quanto sua usina gerou</h3>
        </div>
        <div className="alert-body">
          <div style={{ marginBottom: 14 }}>
            <div className="chart-total-value" style={{ textAlign: "left" }}>{fmtNum(history?.total_kwh)} kWh</div>
            {history && history.previous_total_kwh > 0 && (
              <div className="chart-total-avg" style={{ textAlign: "left", color: "var(--good)" }}>
                {history.total_kwh >= history.previous_total_kwh ? "▲" : "▼"}{" "}
                {fmtNum(Math.abs(((history.total_kwh - history.previous_total_kwh) / history.previous_total_kwh) * 100), 0)}% vs. período anterior
              </div>
            )}
          </div>
          <div className="chart-box" style={{ height: 200 }}>
            <canvas ref={canvasRef} />
          </div>
        </div>
      </div>

      <div className="card alert-card">
        <div className="alert-head" style={{ padding: "14px 16px" }}>
          <h3>Seus melhores dias e meses</h3>
        </div>
        <div className="alert-body">
          <div className="day-grid" style={{ gridTemplateColumns: "repeat(3,1fr)" }}>
            <div>
              <div className="l">Melhor dia</div>
              <div className="v">{records?.best_day_kwh !== null ? `${fmtNum(records?.best_day_kwh)} kWh` : "--"}</div>
              <div className="aux">{records?.best_day_date ?? ""}</div>
            </div>
            <div>
              <div className="l">Melhor mês</div>
              <div className="v">{records?.best_month_kwh !== null ? `${fmtNum(records?.best_month_kwh)} kWh` : "--"}</div>
              <div className="aux">{records?.best_month_label ?? ""}</div>
            </div>
            <div>
              <div className="l">Maior potência já vista</div>
              <div className="v">{records?.peak_power_kw !== null ? `${fmtNum(records?.peak_power_kw, 2)} kW` : "--"}</div>
            </div>
          </div>
        </div>
      </div>

      {streak && (
        <div className="card alert-card">
          <div className="alert-head" style={{ padding: "14px 16px" }}>
            <h3>Sequência de dias acima da média</h3>
          </div>
          <div className="alert-body">
            <div className="hero-line">
              <span className="v" style={{ fontSize: 30 }}>{streak.count}</span>
              <span className="aux">dia(s) seguido(s) acima da média ({fmtNum(streak.avg)} kWh/dia)</span>
            </div>
            <div className="day-grid" style={{ gridTemplateColumns: "repeat(2,1fr)", marginTop: 16 }}>
              <div>
                <div className="l">Melhor dia do período</div>
                <div className="v">{fmtNum(streak.best.generated_kwh)} kWh ({streak.best.date})</div>
              </div>
              <div>
                <div className="l">Pior dia do período</div>
                <div className="v">{fmtNum(streak.worst.generated_kwh)} kWh ({streak.worst.date})</div>
              </div>
            </div>
          </div>
        </div>
      )}

      <div className="card alert-card">
        <div className="alert-head" style={{ padding: "14px 16px" }}>
          <h3>Rendimento comparado ao período anterior</h3>
        </div>
        <div className="alert-body">
          <div className="day-grid" style={{ gridTemplateColumns: "repeat(2,1fr)" }}>
            <div>
              <div className="l">Rendimento deste período</div>
              <div className="v">{yieldCurrent !== null ? `${fmtNum(yieldCurrent, 2)} kWh/kWp` : "--"}</div>
            </div>
            <div>
              <div className="l">Rendimento período anterior</div>
              <div className="v">{yieldPrevious ? `${fmtNum(yieldPrevious, 2)} kWh/kWp` : "--"}</div>
            </div>
          </div>
        </div>
      </div>

      <div className="card alert-card">
        <div className="alert-head" style={{ padding: "14px 16px" }}>
          <h3>Anotações sobre eventos importantes</h3>
        </div>
        <div className="alert-body">
          <form className="anno-form" onSubmit={handleAnnotationSubmit} style={{ display: "flex", gap: 8, marginBottom: 12 }}>
            <input type="date" required value={annoDate} onChange={(e) => setAnnoDate(e.target.value)} />
            <input
              type="text"
              placeholder="Ex.: limpeza dos painéis"
              maxLength={280}
              required
              value={annoText}
              onChange={(e) => setAnnoText(e.target.value)}
              style={{ flex: 1 }}
            />
            <button type="submit" className="btn-upload">
              Salvar
            </button>
          </form>
          <div className="anno-list">
            {annotations.length === 0 ? (
              <div className="anno-empty">Nenhuma anotação nesse período ainda.</div>
            ) : (
              annotations.map((a) => (
                <div className="anno-list-item" key={a.date}>
                  <span className="d">{a.date}</span>
                  <span className="n">{a.note}</span>
                </div>
              ))
            )}
          </div>
        </div>
      </div>

      <div className="card" style={{ overflow: "hidden", marginTop: 14 }}>
        <div style={{ padding: "16px 20px", borderBottom: "1px solid var(--border)" }}>
          <h3 style={{ margin: 0, fontSize: 13, fontWeight: 600, color: "var(--ink-2)" }}>Histórico recente</h3>
        </div>
        <table className="hist-table">
          <thead>
            <tr>
              <th>Data</th>
              <th>Gerado</th>
            </tr>
          </thead>
          <tbody>
            {history?.rows
              .slice()
              .reverse()
              .slice(0, 15)
              .map((r) => (
                <tr key={r.date}>
                  <td>{r.date}</td>
                  <td>{fmtNum(r.generated_kwh)} kWh</td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
