// Cliente HTTP único da API — sessão via cookie httpOnly (nunca lida
// diretamente pelo React), sempre com credentials:"include".
const API_URL = import.meta.env.VITE_API_URL ?? "http://localhost:8090";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
  });

  if (res.status === 204) {
    return undefined as T;
  }

  const contentType = res.headers.get("content-type") ?? "";
  const body = contentType.includes("application/json") ? await res.json() : undefined;

  if (!res.ok) {
    const message = body?.error ?? `Erro ${res.status}`;
    throw new ApiError(res.status, message);
  }
  return body as T;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: "POST", body: body ? JSON.stringify(body) : undefined }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: "PUT", body: body ? JSON.stringify(body) : undefined }),
  delete: <T>(path: string) => request<T>(path, { method: "DELETE" }),
};

// ---------- Tipos de resposta (espelham api-go/internal/httpapi) ----------

export interface Plant {
  id: string;
  name: string;
  lat: number | null;
  lon: number | null;
  installed_power_kwp: number;
  timezone: string;
}

export interface InverterCredential {
  id: string;
  brand: "huawei" | "foxess";
  enabled: boolean;
  configured: boolean;
}

export interface Summary {
  instantaneous_power_kw: number | null;
  installed_power_kwp: number | null;
  today_generated_kwh: number | null;
  today_economia_brl: number | null;
  today_vs_yesterday_pct: number | null;
  peak_power_kw: number | null;
  peak_power_at: string | null;
  status: "online" | "alerta" | "pendente";
  updated_at: string;
}

export type InverterStatus = "gerando" | "online_sem_geracao" | "sem_comunicacao";

export interface InverterEntry {
  power_kw: number | null;
  day_kwh: number | null;
  temperature_c: number | null;
  status: InverterStatus;
  consecutive_failures: number;
  last_error: string | null;
}

export type InvertersResponse = Record<string, InverterEntry>;

export interface CollectorHealthEntry {
  total_cycles: number;
  failed_cycles: number;
  reliability_pct: number | null;
}
export type CollectorHealthResponse = Record<string, CollectorHealthEntry>;

export interface HistoryRow {
  date: string;
  generated_kwh: number | null;
  valor_estimado_brl: number | null;
}
export interface HistoryResponse {
  rows: HistoryRow[];
  total_kwh: number;
  total_brl: number | null;
  previous_total_kwh: number;
  previous_total_brl: number | null;
}

export interface HistoryRecords {
  best_day_kwh: number | null;
  best_day_date: string | null;
  best_month_kwh: number | null;
  best_month_label: string | null;
  peak_power_kw: number | null;
  peak_power_at: string | null;
}

export interface HistoryInvertersRow {
  date: string;
  huawei_kwh: number | null;
  foxess_kwh: number | null;
}

export interface Annotation {
  date: string;
  note: string;
}

export interface DayStatus {
  date: string | null;
  generated_kwh: number | null;
  weather: string;
  weather_daylight?: string;
  solar_radiation_mj_m2: number;
  cloudcover_hourly?: number[];
  sunrise: string;
  sunset: string;
  has_alarm: boolean | null;
  alarm_detail: string | null;
  bandeira: string | null;
  bandeira_valor_kwh: number | null;
}

export interface ForecastDay {
  date: string;
  weather: string;
  rating: "bom" | "moderado" | "ruim";
  weather_daylight?: string;
  rating_daylight?: "bom" | "moderado" | "ruim";
  temp_max: number;
  temp_min: number;
  solar_radiation_mj_m2: number;
  precipitation_mm: number;
  precipitation_probability_pct: number;
  sunrise: string;
  sunset: string;
  cloudcover_hourly?: number[];
}
