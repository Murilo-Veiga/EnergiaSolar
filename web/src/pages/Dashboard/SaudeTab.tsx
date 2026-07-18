import { useEffect, useRef, useState } from "react";
import { Chart } from "chart.js/auto";
import { useActivePlant } from "../../context/PlantContext";
import { api, type CollectorHealthResponse, type HistoryInvertersRow } from "../../lib/api";

const INV_LABELS: Record<string, string> = { huawei: "Huawei", foxess: "FoxESS" };
const INV_COLOR: Record<string, string> = { huawei: "#3987e5", foxess: "#199e70" };

function fmtNum(v: number | null | undefined, decimals = 1): string {
  if (v === null || v === undefined) return "--";
  return v.toLocaleString("pt-BR", { minimumFractionDigits: decimals, maximumFractionDigits: decimals });
}

export function SaudeTab() {
  const plant = useActivePlant();
  const [rows, setRows] = useState<HistoryInvertersRow[]>([]);
  const [health, setHealth] = useState<CollectorHealthResponse>({});
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
      setRows(h.rows);
      setHealth(c);
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [plant.id]);

  useEffect(() => {
    if (!canvasRef.current || rows.length === 0) return;
    const labels = rows.map((r) => r.date.slice(5));
    const brands = Object.keys(health);
    const datasets = brands.map((brand) => ({
      label: INV_LABELS[brand] ?? brand,
      data: rows.map((r) => (brand === "huawei" ? r.huawei_kwh ?? 0 : r.foxess_kwh ?? 0)),
      backgroundColor: INV_COLOR[brand] ?? "#898781",
      borderRadius: 4,
      maxBarThickness: 22,
      stack: "s",
    }));

    chartRef.current?.destroy();
    chartRef.current = new Chart(canvasRef.current, {
      type: "bar",
      data: { labels, datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        scales: {
          x: { stacked: true, ticks: { color: "#898781" }, grid: { display: false } },
          y: { stacked: true, ticks: { color: "#898781" }, grid: { color: "#2c2c2a" }, beginAtZero: true },
        },
      },
    });
  }, [rows, health]);

  const totalByBrand: Record<string, number> = {};
  for (const r of rows) {
    totalByBrand.huawei = (totalByBrand.huawei ?? 0) + (r.huawei_kwh ?? 0);
    totalByBrand.foxess = (totalByBrand.foxess ?? 0) + (r.foxess_kwh ?? 0);
  }
  const totalKwh = Object.values(totalByBrand).reduce((a, b) => a + b, 0);

  return (
    <div>
      <div className="card alert-card">
        <div className="alert-head" style={{ padding: "14px 16px" }}>
          <h3>Quanto cada inversor contribuiu (mês)</h3>
        </div>
        <div className="alert-body">
          <div className="chart-box" style={{ height: 200 }}>
            <canvas ref={canvasRef} />
          </div>
        </div>
      </div>

      <div className="card alert-card">
        <div className="alert-head" style={{ padding: "14px 16px" }}>
          <h3>Contribuição real por inversor (mês)</h3>
          <span className="tip tip-wide">
            A comparação com a capacidade instalada de cada inversor ainda não foi portada — só o % real de cada um sobre o
            total gerado no mês.
          </span>
        </div>
        <div className="alert-body">
          <div className="day-grid" style={{ gridTemplateColumns: `repeat(${Object.keys(health).length || 1},1fr)` }}>
            {Object.keys(health).map((brand) => (
              <div key={brand}>
                <div className="l">{INV_LABELS[brand] ?? brand}</div>
                <div className="v">{totalKwh ? `${fmtNum(((totalByBrand[brand] ?? 0) / totalKwh) * 100, 0)}%` : "--"}</div>
                <div className="aux">{fmtNum(totalByBrand[brand])} kWh</div>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="card alert-card">
        <div className="alert-head" style={{ padding: "14px 16px" }}>
          <h3>Confiabilidade da coleta de dados</h3>
          <span className="tip tip-wide">De cada 100 tentativas de buscar dados nos inversores, quantas deram certo nos últimos 30 dias.</span>
        </div>
        <div className="alert-body">
          <div className="day-grid" style={{ gridTemplateColumns: `repeat(${Object.keys(health).length || 1},1fr)` }}>
            {Object.entries(health).map(([brand, entry]) => (
              <div key={brand}>
                <div className="l">{INV_LABELS[brand] ?? brand}</div>
                <div
                  className="v"
                  style={{
                    color:
                      entry.reliability_pct === null
                        ? undefined
                        : entry.reliability_pct >= 95
                          ? "var(--good)"
                          : entry.reliability_pct >= 80
                            ? "var(--ink-2)"
                            : "var(--critical)",
                  }}
                >
                  {entry.reliability_pct !== null ? `${fmtNum(entry.reliability_pct)}%` : "--"}
                </div>
                <div className="aux">
                  {entry.total_cycles - entry.failed_cycles} de {entry.total_cycles} coletas
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="card" style={{ padding: "16px 20px", marginTop: 14, fontSize: 12, color: "var(--ink-muted)", lineHeight: 1.6 }}>
        Outras análises de saúde da usina (eficiência vs. sol do dia, radiação medida, impacto ambiental, diagnóstico por
        string) ainda não foram portadas — dependem de endpoints novos da Huawei, não coletados hoje.
      </div>
    </div>
  );
}
