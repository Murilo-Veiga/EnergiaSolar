import { useEffect, useState } from "react";
import { api, type ForecastDay } from "../lib/api";
import { fmtNum } from "../lib/fmt";
import { IconBadge } from "./icons";

const RATING_ICON_NAME: Record<string, string> = { bom: "sun", moderado: "cloud", ruim: "drizzle" };
const RATING_BADGE_COLOR: Record<string, "gold" | "blue"> = { bom: "gold", moderado: "blue", ruim: "blue" };
// Escala fixa até 20 MJ/m² (dia de céu limpo típico na região) — dá contexto
// comparável entre os dias em vez de reescalar pelo máximo da semana.
const radiationMeterPct = (mj: number) => Math.max(4, Math.min(100, Math.round((mj / 20) * 100)));

function renderCloudSparklinePath(cloudcover: number[], sunriseHHMM: string, sunsetHHMM: string) {
  const w = 900;
  const h = 38;
  const n = cloudcover.length;
  const stepX = w / n;
  const sunriseH = parseInt(sunriseHHMM.slice(0, 2), 10) + parseInt(sunriseHHMM.slice(3, 5), 10) / 60;
  const sunsetH = parseInt(sunsetHHMM.slice(0, 2), 10) + parseInt(sunsetHHMM.slice(3, 5), 10) / 60;
  const dayX0 = (sunriseH / 24) * w;
  const dayX1 = (sunsetH / 24) * w;
  const gap = 2;
  const bw = stepX - gap;

  const bars = cloudcover.map((v, i) => {
    const bh = (v / 100) * (h - 4);
    const x = i * stepX;
    const inDaylight = i + 0.5 >= sunriseH && i + 0.5 <= sunsetH;
    const color = inDaylight ? "var(--warning)" : "var(--ink-muted)";
    const op = inDaylight ? 1 : 0.5;
    return `<rect x="${x.toFixed(1)}" y="${(h - bh).toFixed(1)}" width="${bw.toFixed(1)}" height="${bh.toFixed(1)}" rx="1.5" fill="${color}" opacity="${op}"/>`;
  });

  return (
    `<svg viewBox="0 0 ${w} ${h}" style="width:100%;display:block" preserveAspectRatio="none">` +
    `<rect x="${dayX0.toFixed(1)}" y="0" width="${(dayX1 - dayX0).toFixed(1)}" height="${h}" fill="var(--warning)" opacity="0.08"/>` +
    `<line x1="0" y1="${h - 0.5}" x2="${w}" y2="${h - 0.5}" stroke="var(--line)" stroke-width="1"/>` +
    bars.join("") +
    `</svg>`
  );
}

export function DayStatusCard({ plantId }: { plantId: string }) {
  const [forecast, setForecast] = useState<ForecastDay[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      const fc = await api.get<ForecastDay[]>(`/api/plants/${plantId}/forecast`);
      if (cancelled) return;
      setForecast(fc);
    }
    void load();
    const id = setInterval(load, 30 * 60_000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [plantId]);

  const today = forecast?.[0];

  return (
    <div className="card day-card">
      <div className="head">
        <h3>Previsão do tempo</h3>
        <span className="date">{today?.date ?? "--"}</span>
      </div>

      {forecast && (
        <div className="day-timeline">
          {forecast.map((d, i) => {
            const isToday = i === 0;
            const rating = isToday ? d.rating_daylight ?? d.rating : d.rating;
            const color = RATING_BADGE_COLOR[rating] ?? "blue";
            const iconName = RATING_ICON_NAME[rating] ?? "cloud";
            const weekday = new Date(`${d.date}T12:00:00`).toLocaleDateString("pt-BR", { weekday: "short" }).replace(".", "");
            const label = isToday ? "Hoje" : weekday.charAt(0).toUpperCase() + weekday.slice(1);
            const condition = isToday ? d.weather_daylight ?? d.weather : d.weather;
            return (
              <div className={`tl-day${isToday ? " today" : ""}`} key={d.date}>
                <div className="d">{label}</div>
                <IconBadge name={iconName} color={color} size="fc" />
                <div className="cond">{condition}</div>
                {isToday && d.sunrise && d.sunset && (
                  <div className="sun-note">
                    {d.sunrise}–{d.sunset}
                  </div>
                )}
                <div className="fc-stats">
                  <div className="rad">
                    {fmtNum(d.solar_radiation_mj_m2)} <span className="unit">MJ/m²</span>
                    <div className="rad-meter">
                      <i
                        style={{
                          width: `${radiationMeterPct(d.solar_radiation_mj_m2)}%`,
                          ["--rad-color" as string]: color === "gold" ? "var(--warning)" : "var(--accent-blue)",
                        }}
                      />
                    </div>
                  </div>
                  <div className="t">
                    {fmtNum(d.temp_max, 0)}°/{fmtNum(d.temp_min, 0)}°
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      <div className="sparkline-box" hidden={!(today?.cloudcover_hourly && today.sunrise && today.sunset)}>
        <div className="sparkline-label">Nuvens ao longo do dia</div>
        {today?.cloudcover_hourly && today.sunrise && today.sunset && (
          <div dangerouslySetInnerHTML={{ __html: renderCloudSparklinePath(today.cloudcover_hourly, today.sunrise, today.sunset) }} />
        )}
        <div className="sparkline-caption">
          <span>00h</span>
          <span>06h</span>
          <span>12h</span>
          <span>18h</span>
          <span>23h</span>
        </div>
      </div>
    </div>
  );
}
