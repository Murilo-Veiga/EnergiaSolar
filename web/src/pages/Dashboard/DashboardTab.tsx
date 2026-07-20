import { useEffect, useState } from "react";
import { useActivePlant } from "../../context/PlantContext";
import { api, type HistoryResponse, type InvertersResponse, type Summary } from "../../lib/api";
import { fmtNum, fmtBRL } from "../../lib/fmt";
import { computeAlerts, INV_LABELS, INV_STATUS_META } from "../../lib/alerts";
import { AlertCenter } from "../../components/AlertCenter";
import { GeracaoChart } from "../../components/GeracaoChart";
import { DayStatusCard } from "../../components/DayStatusCard";
import { Tooltip } from "../../components/Tooltip";

export function DashboardTab({ onUpdatedAt }: { onUpdatedAt: (iso: string) => void }) {
  const plant = useActivePlant();
  const [summary, setSummary] = useState<Summary | null>(null);
  const [inverters, setInverters] = useState<InvertersResponse>({});
  const [weekHistory, setWeekHistory] = useState<HistoryResponse | null>(null);
  const [bandeira, setBandeira] = useState<{ bandeira: string | null; bandeira_valor_kwh: number | null }>({
    bandeira: null,
    bandeira_valor_kwh: null,
  });

  useEffect(() => {
    let cancelled = false;

    async function refresh() {
      const [s, inv, wh, ds] = await Promise.all([
        api.get<Summary>(`/api/plants/${plant.id}/summary`),
        api.get<InvertersResponse>(`/api/plants/${plant.id}/inverters`),
        api.get<HistoryResponse>(`/api/plants/${plant.id}/history?range=semana`),
        api.get<{ bandeira: string | null; bandeira_valor_kwh: number | null }>(`/api/plants/${plant.id}/day-status`),
      ]);
      if (cancelled) return;
      setSummary(s);
      setInverters(inv);
      setWeekHistory(wh);
      setBandeira({ bandeira: ds.bandeira, bandeira_valor_kwh: ds.bandeira_valor_kwh });
      onUpdatedAt(s.updated_at);
    }

    void refresh();
    const id = setInterval(refresh, 30_000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [plant.id]);

  const weekAvgKwh = weekHistory && weekHistory.rows.length ? weekHistory.total_kwh / weekHistory.rows.length : null;
  const alerts = computeAlerts({
    inverters,
    bandeira: bandeira.bandeira,
    bandeiraValorKwh: bandeira.bandeira_valor_kwh,
    todayKwh: summary?.today_generated_kwh ?? null,
    weekAvgKwh,
  });

  const invEntries = Object.entries(inverters);

  return (
    <div>
      <div className="hero-row">
        <div className="card hero-card">
          <div className="l">Potência instantânea</div>
          <div className="v-row">
            <span className="v">{fmtNum(summary?.instantaneous_power_kw, 2)}</span>
            <span className="unit">kW</span>
          </div>
          <div className="foot">
            {summary?.peak_power_kw !== null && summary?.peak_power_kw !== undefined && summary?.peak_power_at ? (
              <>
                Pico hoje{" "}
                <span className="up">
                  {fmtNum(summary.peak_power_kw, 2)} kW
                </span>{" "}
                às {new Date(summary.peak_power_at).toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" })}
              </>
            ) : (
              ""
            )}
          </div>
        </div>
        <div className="card stat-card">
          <div className="l">Gerado hoje</div>
          <div className="v">
            <span>{fmtNum(summary?.today_generated_kwh)}</span>
            <span className="unit"> kWh</span>
          </div>
          {summary?.today_vs_yesterday_pct !== null && summary?.today_vs_yesterday_pct !== undefined && (
            <div className="sub" style={{ color: summary.today_vs_yesterday_pct >= 0 ? "var(--good)" : "var(--critical)" }}>
              {summary.today_vs_yesterday_pct >= 0 ? "▲" : "▼"} {Math.abs(summary.today_vs_yesterday_pct)}%{" "}
              <span style={{ color: "var(--ink-muted)" }}>vs. ontem</span>
            </div>
          )}
          {summary?.today_economia_brl !== null && summary?.today_economia_brl !== undefined && (
            <div className="sub">
              ≈ {fmtBRL(summary.today_economia_brl)} <span className="tag">estimado</span>
            </div>
          )}
        </div>
        <div className="card stat-card">
          <div className="l">Potência instalada</div>
          <div className="v">
            <span>{fmtNum(summary?.installed_power_kwp)}</span>
            <span className="unit"> kWp</span>
          </div>
        </div>
      </div>

      <AlertCenter alerts={alerts} />

      <div className="inv-row" style={invEntries.length === 1 ? { gridTemplateColumns: "1fr" } : undefined}>
        {invEntries.map(([inv, data]) => {
          const meta = INV_STATUS_META[data.status] ?? INV_STATUS_META.sem_comunicacao;
          return (
            <div className="card inv-card" key={inv}>
              <div className="inv-top">
                <div className="inv-id">
                  <span className="dot" style={{ background: inv === "huawei" ? "var(--accent-blue)" : "var(--accent-aqua)" }} />
                  <span className="name">{INV_LABELS[inv] ?? inv}</span>
                </div>
                <Tooltip text={meta.tip}>
                  <span className={`inv-status ${meta.cls}`}>
                    <i /> {meta.label}
                  </span>
                </Tooltip>
              </div>
              <div className="inv-body">
                <div>
                  <div className="model" />
                  <div className="temp">{data.temperature_c !== null && data.temperature_c !== undefined ? `${fmtNum(data.temperature_c, 0)}°C` : "--"}</div>
                </div>
                <div className="inv-vals">
                  <div className="p">
                    <span>{fmtNum(data.power_kw, 2)}</span> <span className="unit">kW</span>
                  </div>
                  <div className="k">
                    <span>{fmtNum(data.day_kwh)}</span> kWh hoje
                  </div>
                </div>
              </div>
            </div>
          );
        })}
      </div>

      <GeracaoChart plantId={plant.id} />

      <DayStatusCard plantId={plant.id} />
    </div>
  );
}
