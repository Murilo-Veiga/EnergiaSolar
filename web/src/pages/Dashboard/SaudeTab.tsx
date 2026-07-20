import { useEffect, useRef, useState } from "react";
import { Chart } from "chart.js/auto";
import { useActivePlant } from "../../context/PlantContext";
import { api, type CollectorHealthResponse, type HistoryInvertersRow } from "../../lib/api";
import { fmtNum } from "../../lib/fmt";
import { Collapsible } from "../../components/Collapsible";

type Range = "dia" | "semana" | "mes" | "ano";

const INV_LABELS: Record<string, string> = { huawei: "Huawei", foxess: "FoxESS" };
const INV_COLOR: Record<string, string> = { huawei: "#3987e5", foxess: "#199e70" };
// Mockado, igual ao original (INVERTER_CAPACITY_KWP em templates/index.html)
// — o schema novo ainda não guarda capacidade por inversor.
const INVERTER_CAPACITY_KWP: Record<string, number> = { huawei: 3.0, foxess: 5.0 };

export function SaudeTab() {
  const plant = useActivePlant();
  const [monthRows, setMonthRows] = useState<HistoryInvertersRow[]>([]);
  const [health, setHealth] = useState<CollectorHealthResponse>({});
  const [contribRange, setContribRange] = useState<Range>("dia");
  const [contribRows, setContribRows] = useState<HistoryInvertersRow[]>([]);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const chartRef = useRef<Chart | null>(null);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      const [h, c] = await Promise.all([
        api.get<{ rows: HistoryInvertersRow[] }>(`/api/plants/${plant.id}/history/inverters?range=mes`),
        api.get<CollectorHealthResponse>(`/api/plants/${plant.id}/collector-health?days=30`),
      ]);
      if (cancelled) return;
      setMonthRows(h.rows);
      setHealth(c);
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [plant.id]);

  useEffect(() => {
    void api.get<{ rows: HistoryInvertersRow[] }>(`/api/plants/${plant.id}/history/inverters?range=${contribRange}`).then((d) => setContribRows(d.rows));
  }, [plant.id, contribRange]);

  const brands = Object.keys(health);

  useEffect(() => {
    if (!canvasRef.current || monthRows.length === 0) return;
    const labels = monthRows.map((r) => r.date.slice(5));
    const datasets = brands.map((brand) => ({
      label: INV_LABELS[brand] ?? brand,
      data: monthRows.map((r) => (brand === "huawei" ? r.huawei_kwh ?? 0 : r.foxess_kwh ?? 0)),
      backgroundColor: INV_COLOR[brand] ?? "#898781",
      borderRadius: 4,
      borderSkipped: false,
      maxBarThickness: 26,
      stack: "s",
    }));

    chartRef.current?.destroy();
    chartRef.current = new Chart(canvasRef.current, {
      type: "bar",
      data: { labels, datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false } },
        scales: {
          x: { stacked: true, ticks: { color: "#898781" }, grid: { display: false } },
          y: { stacked: true, ticks: { color: "#898781" }, grid: { color: "#2c2c2a" }, beginAtZero: true },
        },
      },
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [monthRows, brands.join(",")]);

  const kwhByInv: Record<string, number> = {};
  for (const inv of brands) {
    kwhByInv[inv] = contribRows.reduce((s, r) => s + (inv === "huawei" ? r.huawei_kwh ?? 0 : r.foxess_kwh ?? 0), 0);
  }
  const totalKwh = brands.reduce((s, inv) => s + kwhByInv[inv], 0);
  const totalCapacity = brands.reduce((s, inv) => s + (INVERTER_CAPACITY_KWP[inv] ?? 0), 0);
  const realPct: Record<string, number | null> = {};
  const expectedPct: Record<string, number> = {};
  for (const inv of brands) {
    expectedPct[inv] = totalCapacity ? ((INVERTER_CAPACITY_KWP[inv] ?? 0) / totalCapacity) * 100 : 0;
    realPct[inv] = totalKwh ? (kwhByInv[inv] / totalKwh) * 100 : null;
  }

  return (
    <div>
      <div className="card" style={{ padding: "16px 20px", marginBottom: 14 }}>
        <p style={{ margin: 0, fontSize: 12.5, color: "var(--ink-muted)", maxWidth: "78ch", lineHeight: 1.6 }}>
          Isto <b style={{ color: "var(--ink-2)" }}>não é sobre quanto gerou</b> — é sobre se a instalação está gerando o que
          deveria, pra pegar um problema (sombra, sujeira, um inversor rendendo menos) cedo, antes que vire meses de perda.
        </p>
      </div>

      <Collapsible
        icon="shuffle"
        iconColor="blue"
        title="Quanto cada inversor contribuiu"
        tooltip="Compara a geração da Huawei com a da FoxESS ao longo do tempo, pra notar se um lado da instalação está rendendo menos que o outro."
        defaultOpen
      >
        <div className="chart-box" style={{ height: 200 }}>
          <canvas ref={canvasRef} />
        </div>
        <div className="chart-legend" style={{ marginTop: 12 }}>
          {brands.map((inv) => (
            <div className="item" key={inv}>
              <span className="sw" style={{ background: INV_COLOR[inv], width: 10, height: 10, borderRadius: 2, display: "inline-block" }} />
              {INV_LABELS[inv] ?? inv} ({INVERTER_CAPACITY_KWP[inv] ?? "?"} kW)
            </div>
          ))}
        </div>
      </Collapsible>

      <Collapsible
        icon="target"
        iconColor="gold"
        title="Contribuição real vs. esperada pela capacidade"
        tooltip="Cada inversor tem uma capacidade diferente (Huawei 3 kW, FoxESS 5 kW). Aqui comparamos o quanto cada um gerou de verdade com o quanto seria esperado só pelo tamanho — pra notar se algum está rendendo menos do que deveria."
        newKey="saude-contrib-range"
      >
        <div className="range-toggle" style={{ marginBottom: 14 }}>
          {(["dia", "semana", "mes", "ano"] as Range[]).map((r) => (
            <span key={r} className={contribRange === r ? "active" : ""} onClick={() => setContribRange(r)}>
              {r === "dia" ? "Dia" : r === "semana" ? "Semana" : r === "mes" ? "Mês" : "Ano"}
            </span>
          ))}
        </div>
        <div className="day-grid" style={{ gridTemplateColumns: `repeat(${brands.length || 1},1fr)` }}>
          {brands.map((inv) => (
            <div key={inv}>
              <div className="l">{INV_LABELS[inv] ?? inv} — real / esperado</div>
              <div className="v">{realPct[inv] !== null ? `${fmtNum(realPct[inv])}%` : "--"}</div>
              <div className="aux">
                esperado {fmtNum(expectedPct[inv])}% · {fmtNum(kwhByInv[inv])} kWh
              </div>
            </div>
          ))}
        </div>
        <div className="chart-legend" style={{ marginTop: 14 }}>
          <div style={{ width: "100%" }}>
            <div style={{ display: "flex", height: 14, borderRadius: 7, overflow: "hidden", background: "var(--surface-2)" }}>
              {totalKwh > 0 &&
                brands.map((inv) => <div key={inv} style={{ width: `${realPct[inv] ?? 0}%`, background: INV_COLOR[inv] }} />)}
            </div>
          </div>
        </div>
        <div className="chart-legend" style={{ marginTop: 10 }}>
          {brands.map((inv) => (
            <div className="item" key={inv}>
              <span className="sw" style={{ background: INV_COLOR[inv], width: 10, height: 10, borderRadius: 2, display: "inline-block" }} />
              {INV_LABELS[inv] ?? inv} ({INVERTER_CAPACITY_KWP[inv] ?? "?"} kW)
            </div>
          ))}
        </div>
      </Collapsible>

      <Collapsible
        icon="shield"
        iconColor="blue"
        title="Confiabilidade da coleta de dados"
        tooltip="De cada 100 tentativas de buscar dados nos inversores, quantas deram certo nos últimos 30 dias. Uma taxa baixa não significa que a instalação parou — só que o painel teve mais dificuldade de consultar aquele inversor."
        newKey="saude-reliability"
      >
        <div className="day-grid" style={{ gridTemplateColumns: `repeat(${brands.length || 1},1fr)` }}>
          {brands.map((inv) => {
            const entry = health[inv];
            const pct = entry?.reliability_pct ?? null;
            return (
              <div key={inv}>
                <div className="l">{INV_LABELS[inv] ?? inv}</div>
                <div className="v" style={{ color: pct === null ? undefined : pct >= 95 ? "var(--good)" : pct >= 80 ? "var(--ink-2)" : "var(--critical)" }}>
                  {pct !== null ? `${fmtNum(pct)}%` : "--"}
                </div>
                <div className="aux">{entry ? `${entry.total_cycles - entry.failed_cycles} de ${entry.total_cycles} coletas nos últimos 30 dias` : ""}</div>
              </div>
            );
          })}
        </div>
      </Collapsible>

      <div className="card" style={{ padding: "16px 20px", marginTop: 14, fontSize: 12, color: "var(--ink-muted)", lineHeight: 1.6 }}>
        Outras análises de saúde da instalação (eficiência vs. sol do dia, radiação medida, impacto ambiental) dependem de dados
        novos da API da Huawei e ainda não foram implementadas.
      </div>
    </div>
  );
}
