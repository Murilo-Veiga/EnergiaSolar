import { useEffect, useState } from "react";
import { api, type ForecastDay } from "../lib/api";
import { fmtNum } from "../lib/fmt";
import { IconBadge } from "./icons";

const RATING_ICON: Record<string, string> = { bom: "sun", moderado: "cloud", ruim: "drizzle" };
const RATING_COLOR: Record<string, "gold" | "blue" | "red"> = { bom: "gold", moderado: "blue", ruim: "red" };

const radPct = (mj: number) => Math.max(4, Math.min(100, Math.round((mj / 20) * 100)));

function CloudSparkline({ cloudcover, sunrise, sunset }: { cloudcover: number[]; sunrise: string; sunset: string }) {
  const w = 900;
  const h = 36;
  const n = cloudcover.length;
  const stepX = w / n;
  const gap = 2;
  const bw = stepX - gap;
  const sr = parseInt(sunrise.slice(0, 2), 10) + parseInt(sunrise.slice(3, 5), 10) / 60;
  const ss = parseInt(sunset.slice(0, 2), 10) + parseInt(sunset.slice(3, 5), 10) / 60;
  const dayX0 = (sr / 24) * w;
  const dayX1 = (ss / 24) * w;

  const bars = cloudcover.map((v, i) => {
    const bh = Math.max(1, (v / 100) * (h - 8));
    const x = i * stepX;
    const inDay = i + 0.5 >= sr && i + 0.5 <= ss;
    return `<rect x="${x.toFixed(1)}" y="${(h - bh - 4).toFixed(1)}" width="${bw.toFixed(1)}" height="${bh.toFixed(1)}" rx="1.5" fill="${inDay ? "var(--warning)" : "var(--ink-muted)"}" opacity="${inDay ? "1" : "0.35"}"/>`;
  });

  const html =
    `<svg viewBox="0 0 ${w} ${h}" style="width:100%;display:block" preserveAspectRatio="none">` +
    `<rect x="${dayX0.toFixed(1)}" y="0" width="${(dayX1 - dayX0).toFixed(1)}" height="${h}" fill="var(--warning)" opacity="0.06"/>` +
    bars.join("") +
    `</svg>`;

  return <div className="cs-line" dangerouslySetInnerHTML={{ __html: html }} />;
}

function WeatherIcon({ rating }: { rating: string }) {
  const name = RATING_ICON[rating] ?? "cloud";
  const color = RATING_COLOR[rating] ?? "blue";
  return <IconBadge name={name} color={color} size="card" />;
}

function DayCard({ day, isToday }: { day: ForecastDay; isToday: boolean }) {
  const rating = isToday ? day.rating_daylight ?? day.rating : day.rating;
  const condition = isToday ? day.weather_daylight ?? day.weather : day.weather;
  const date = new Date(`${day.date}T12:00:00`);
  const weekday = date.toLocaleDateString("pt-BR", { weekday: "short" }).replace(".", "");
  const label = isToday ? "Hoje" : weekday.charAt(0).toUpperCase() + weekday.slice(1);
  const dayNum = date.toLocaleDateString("pt-BR", { day: "2-digit", month: "2-digit" });

  return (
    <div className={`w-day${isToday ? " w-day--today" : ""}`}>
      <div className="w-day__head">
        <span className="w-day__label">{label}</span>
        <span className="w-day__date">{dayNum}</span>
      </div>

      <div className="w-day__body">
        <span className="w-day__icon">
          <WeatherIcon rating={rating} />
        </span>
        <span className="w-day__temps">
          <span className="w-day__temp-max">{fmtNum(day.temp_max, 0)}°</span>
          <span className="w-day__temp-min">{fmtNum(day.temp_min, 0)}°</span>
        </span>
      </div>

      <span className="w-day__cond" title={condition}>{condition}</span>

      <div className="w-day__sun">
        <span className="w-day__sun-item"><i className="w-day__sun-ico w-day__sun-ico--up" />{day.sunrise}</span>
        <span className="w-day__sun-item"><i className="w-day__sun-ico w-day__sun-ico--down" />{day.sunset}</span>
      </div>
    </div>
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
    return () => { cancelled = true; clearInterval(id); };
  }, [plantId]);

  const today = forecast?.[0];

  return (
    <div className="card w-card">
      <div className="w-head">
        <div className="w-head__left">
          <h3 className="w-head__title">Clima</h3>
          <span className="w-head__sub">Previsão 5 dias</span>
        </div>
        {today && (
          <div className="w-head__right">
            <span className="w-head__rad">
              <span className="w-head__rad-val">{fmtNum(today.solar_radiation_mj_m2)}</span>
              <span className="w-head__rad-unit">MJ/m²</span>
            </span>
          </div>
        )}
      </div>

      <div className="w-timeline">
        {forecast?.map((d, i) => <DayCard key={d.date} day={d} isToday={i === 0} />)}
      </div>

      {today?.cloudcover_hourly && today.sunrise && today.sunset && (
        <div className="w-clouds">
          <div className="w-clouds__head">
            <span className="w-clouds__label">Nebulosidade</span>
            <span className="w-clouds__sub">hora a hora</span>
          </div>
          <CloudSparkline cloudcover={today.cloudcover_hourly} sunrise={today.sunrise} sunset={today.sunset} />
          <div className="w-clouds__ticks">
            <span>00h</span>
            <span>06h</span>
            <span>12h</span>
            <span>18h</span>
            <span>23h</span>
          </div>
        </div>
      )}

      {today && (today.precipitation_mm > 0 || today.precipitation_probability_pct > 0) && (
        <div className="w-precip">
          <div className="w-precip__item">
            <span className="w-precip__label">Chuva</span>
            <span className="w-precip__value">{fmtNum(today.precipitation_mm, 1)} mm</span>
          </div>
          <div className="w-precip__divider" />
          <div className="w-precip__item">
            <span className="w-precip__label">Probabilidade</span>
            <span className="w-precip__value">{fmtNum(today.precipitation_probability_pct, 0)}%</span>
          </div>
          <div className="w-precip__divider" />
          <div className="w-precip__item">
            <span className="w-precip__label">Radiação solar</span>
            <span className="w-precip__value">{fmtNum(today.solar_radiation_mj_m2)} <span className="w-precip__unit">MJ/m²</span></span>
          </div>
        </div>
      )}
    </div>
  );
}
