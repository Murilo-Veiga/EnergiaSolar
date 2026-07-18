import { useEffect, useRef, useState } from "react";
import { Chart } from "chart.js/auto";
import { useActivePlant } from "../../context/PlantContext";
import { api, type DayStatus, type HistoryResponse, type InvertersResponse, type Summary } from "../../lib/api";
import { IconBadge } from "../../components/icons";

const COMM_TIMEOUT_MIN = 15;
const API_FAILURE_ALERT_THRESHOLD = 2;
const TEMP_THRESHOLD_C = 65; // ilustrativo — ver README > "Central de alertas"
const GERACAO_ABAIXO_MEDIA_PCT = 10;

const INV_LABELS: Record<string, string> = { huawei: "Huawei", foxess: "FoxESS" };
const INV_STATUS_META: Record<string, { cls: string; label: string; tip: string }> = {
  gerando: { cls: "st-good", label: "Gerando", tip: "Gerando energia agora." },
  online_sem_geracao: { cls: "st-idle", label: "Online, sem geração", tip: "Conectado à nuvem, sem geração agora — normal à noite ou com pouco sol." },
  sem_comunicacao: {
    cls: "st-critical",
    label: "Sem comunicação",
    tip: `Sem resposta da API há mais de ${COMM_TIMEOUT_MIN} min — pode indicar queda de energia, Wi-Fi ou disjuntor.`,
  },
};

interface Alert {
  key: string;
  sev: "critical" | "warning" | "info";
  icon: string;
  title: string;
  desc: string;
}

function fmtNum(v: number | null | undefined, decimals = 1): string {
  if (v === null || v === undefined) return "--";
  return v.toLocaleString("pt-BR", { minimumFractionDigits: decimals, maximumFractionDigits: decimals });
}

export function DashboardTab() {
  const plant = useActivePlant();
  const [summary, setSummary] = useState<Summary | null>(null);
  const [inverters, setInverters] = useState<InvertersResponse>({});
  const [dayStatus, setDayStatus] = useState<DayStatus | null>(null);
  const [weekHistory, setWeekHistory] = useState<HistoryResponse | null>(null);
  const [alertsOpen, setAlertsOpen] = useState(false);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const chartRef = useRef<Chart | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function refresh() {
      const [s, inv, ds, wh] = await Promise.all([
        api.get<Summary>(`/api/plants/${plant.id}/summary`),
        api.get<InvertersResponse>(`/api/plants/${plant.id}/inverters`),
        api.get<DayStatus>(`/api/plants/${plant.id}/day-status`),
        api.get<HistoryResponse>(`/api/plants/${plant.id}/history?range=semana`),
      ]);
      if (cancelled) return;
      setSummary(s);
      setInverters(inv);
      setDayStatus(ds);
      setWeekHistory(wh);
    }

    void refresh();
    const id = setInterval(refresh, 30_000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [plant.id]);

  useEffect(() => {
    let cancelled = false;
    async function loadChart() {
      const monthHistory = await api.get<HistoryResponse>(`/api/plants/${plant.id}/history?range=mes`);
      if (cancelled || !canvasRef.current) return;
      const labels = monthHistory.rows.map((r) => r.date.slice(5));
      const data = monthHistory.rows.map((r) => r.generated_kwh ?? 0);

      chartRef.current?.destroy();
      chartRef.current = new Chart(canvasRef.current, {
        type: "bar",
        data: { labels, datasets: [{ label: "Gerado (kWh)", data, backgroundColor: "#3987e5", borderRadius: 4, maxBarThickness: 22 }] },
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
    }
    void loadChart();
    return () => {
      cancelled = true;
    };
  }, [plant.id]);

  const alerts: Alert[] = [];
  if (summary && inverters && weekHistory) {
    for (const [inv, data] of Object.entries(inverters)) {
      if (data.status === "sem_comunicacao") {
        alerts.push({
          key: `comm:${inv}`,
          sev: "critical",
          icon: "alertTriangle",
          title: `${INV_LABELS[inv] ?? inv} sem comunicação`,
          desc: INV_STATUS_META.sem_comunicacao.tip,
        });
      }
      if (data.consecutive_failures >= API_FAILURE_ALERT_THRESHOLD) {
        alerts.push({
          key: `api_failure:${inv}`,
          sev: "warning",
          icon: "plug",
          title: `Falha ao consultar a API da ${INV_LABELS[inv] ?? inv} (${data.consecutive_failures}x seguidas)`,
          desc: data.last_error
            ? `Último erro: ${data.last_error} — "Gerado hoje" segue com o último valor bem-sucedido.`
            : `A coleta está falhando repetidamente — "Gerado hoje" segue com o último valor bem-sucedido.`,
        });
      }
      if (data.temperature_c !== null && data.temperature_c >= TEMP_THRESHOLD_C) {
        alerts.push({
          key: `temp:${inv}`,
          sev: "warning",
          icon: "thermometer",
          title: `Temperatura do inversor ${INV_LABELS[inv] ?? inv} acima do limiar (${fmtNum(data.temperature_c, 0)}°C)`,
          desc: "Limiar ilustrativo — ver README.",
        });
      }
    }
    if (summary.today_generated_kwh !== null && weekHistory.rows.length > 0) {
      const avg = weekHistory.total_kwh / weekHistory.rows.length;
      if (avg > 0 && summary.today_generated_kwh < avg * (1 - GERACAO_ABAIXO_MEDIA_PCT / 100)) {
        alerts.push({
          key: "geracao_abaixo_media",
          sev: "info",
          icon: "trendingDown",
          title: "Geração do dia abaixo da média da semana",
          desc: `${fmtNum(summary.today_generated_kwh)} kWh hoje vs. média de ${fmtNum(avg)} kWh/dia na semana.`,
        });
      }
    }
  }

  const statusLabel = summary?.status === "alerta" ? "Alerta" : summary?.status === "online" ? "Online" : "Pendente";

  return (
    <div>
      <div className="hero-row">
        <div className="card hero-card">
          <div className="l">Potência instantânea</div>
          <div className="v-row">
            <span className="v">{fmtNum(summary?.instantaneous_power_kw, 2)}</span>
            <span className="unit">kW</span>
          </div>
          <div className="foot">{statusLabel}{summary?.peak_power_kw ? ` · pico ${fmtNum(summary.peak_power_kw, 2)} kW` : ""}</div>
        </div>
        <div className="card stat-card">
          <div className="l">Gerado hoje</div>
          <div className="v">
            <span>{fmtNum(summary?.today_generated_kwh)}</span> <span className="unit"> kWh</span>
          </div>
          {summary?.today_vs_yesterday_pct !== null && summary?.today_vs_yesterday_pct !== undefined && (
            <div className="sub">
              {summary.today_vs_yesterday_pct >= 0 ? "▲" : "▼"} {Math.abs(summary.today_vs_yesterday_pct)}% vs. ontem
            </div>
          )}
        </div>
        <div className="card stat-card">
          <div className="l">Potência instalada</div>
          <div className="v">
            <span>{fmtNum(summary?.installed_power_kwp)}</span> <span className="unit"> kWp</span>
          </div>
        </div>
      </div>

      <div className="card alert-card">
        <button className="alert-toggle" onClick={() => setAlertsOpen((v) => !v)} style={{ width: "100%", cursor: "pointer" }}>
          <div className="alert-head">
            <h3>Central de alertas</h3>
          </div>
          <div className="alert-toggle-right">
            {alerts.length > 0 && <span className="alert-badge">{alerts.length}</span>}
            <span className="alert-chevron">{alertsOpen ? "▴" : "▾"}</span>
          </div>
        </button>
        {alertsOpen && (
          <div className="alert-body">
            {alerts.length === 0 ? (
              <div className="alert-empty">Nenhum alerta ativo no momento.</div>
            ) : (
              <div className="alert-list">
                {alerts.map((a) => (
                  <div className={`alert-item ${a.sev}`} key={a.key}>
                    <IconBadge name={a.icon} color={a.sev === "critical" ? "red" : a.sev === "warning" ? "gold" : "blue"} size="alert" />
                    <div className="alert-item-body">
                      <div className="alert-title">{a.title}</div>
                      <div className="alert-desc">{a.desc}</div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>

      <div className="inv-row" style={Object.keys(inverters).length === 1 ? { gridTemplateColumns: "1fr" } : undefined}>
        {Object.entries(inverters).map(([inv, data]) => {
          const meta = INV_STATUS_META[data.status] ?? INV_STATUS_META.sem_comunicacao;
          return (
            <div className="card inv-card" key={inv}>
              <div className="inv-top">
                <div className="inv-id">
                  <span className="dot" style={{ background: inv === "huawei" ? "var(--accent-blue)" : "var(--accent-aqua)" }} />
                  <span className="name">{INV_LABELS[inv] ?? inv}</span>
                </div>
                <span className="tt">
                  <span className={`inv-status ${meta.cls}`}>
                    <i /> {meta.label}
                  </span>
                </span>
              </div>
              <div className="inv-body">
                <div>
                  <div className="temp">{data.temperature_c !== null ? `${fmtNum(data.temperature_c, 0)}°C` : "--"}</div>
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

      <div className="card chart-card">
        <div className="chart-head">
          <h3>Gerado no mês</h3>
        </div>
        <div className="chart-box" style={{ height: 180 }}>
          <canvas ref={canvasRef} />
        </div>
      </div>

      {dayStatus && (
        <div className="card day-card">
          <div className="head">
            <h3>Status do dia</h3>
            <span className="date">{dayStatus.date ?? "--"}</span>
          </div>
          <div className="day-grid" style={{ gridTemplateColumns: "repeat(3,1fr)" }}>
            <div>
              <div className="l">Situação</div>
              <span className="status-pill">
                <span className="sw" style={{ background: dayStatus.has_alarm ? "var(--critical)" : "var(--good)" }} />
                <span className="label">{dayStatus.has_alarm ? "Alerta" : "Ok"}</span>
              </span>
              {dayStatus.alarm_detail && <div style={{ fontSize: 10.5, color: "var(--ink-muted)", marginTop: 4 }}>{dayStatus.alarm_detail}</div>}
            </div>
            <div>
              <div className="l">Gerado hoje</div>
              <div className="v">{dayStatus.generated_kwh !== null ? `${fmtNum(dayStatus.generated_kwh)} kWh` : "--"}</div>
            </div>
            <div>
              <div className="l">Clima (horas de sol)</div>
              <div className="v" style={{ fontSize: 13 }}>
                {dayStatus.weather_daylight ?? dayStatus.weather}
              </div>
              <div style={{ fontSize: 10.5, color: "var(--ink-muted)" }}>
                {dayStatus.sunrise} – {dayStatus.sunset} · {fmtNum(dayStatus.solar_radiation_mj_m2)} MJ/m²
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
