// Cliente HTTP único da API — sessão via cookie httpOnly (nunca lida
// diretamente pelo React), sempre com credentials:"include".
//
// URL sempre RELATIVA (mesma origem da página, nunca um host fixado em
// build-time) — de propósito: um bundle com "http://localhost:8091"
// embutido só funciona em quem abre o painel no MESMO PC do servidor
// (localhost ali resolve pro PC de quem está acessando, não pro
// servidor). Em produção o nginx do container web faz proxy de /api pra
// api-go (ver nginx.conf); em dev o Vite faz o mesmo (ver vite.config.ts)
// — em ambos os casos o browser só fala com a própria origem.
const API_URL = "";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const isFormData = init?.body instanceof FormData;
  const res = await fetch(`${API_URL}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      // FormData define seu próprio Content-Type (multipart + boundary) —
      // forçar application/json aqui quebraria o upload de arquivo.
      ...(isFormData ? {} : { "Content-Type": "application/json" }),
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
  upload: <T>(path: string, formData: FormData) => request<T>(path, { method: "POST", body: formData }),
};

// ---------- Tipos de resposta (espelham api-go/internal/httpapi) ----------

export interface Me {
  user_id: string;
  email: string;
  username: string;
  name: string;
  is_admin: boolean;
}

export interface AdminUser {
  id: string;
  name: string;
  email: string;
  username: string;
  is_admin: boolean;
  created_at: string;
  plants_count: number;
}

export interface SystemSettings {
  huawei_base_url: string;
  foxess_base_url: string;
  worker_interval_minutes: number;
}

export interface Plant {
  id: string;
  name: string;
  lat: number | null;
  lon: number | null;
  installed_power_kwp: number;
  timezone: string;
  is_owner: boolean;
}

export interface PlantAccessUser {
  id: string;
  name: string;
  email: string;
  username: string;
  granted_at: string;
}

export interface InverterDeviceInfo {
  station_code?: string;
  dev_dn?: string;
  device_sn?: string;
  power_kw?: number;
  day_kwh?: number;
  temperature_c?: number;
  error?: string;
}

export interface InverterCredential {
  id: string;
  brand: "huawei" | "foxess";
  enabled: boolean;
  configured: boolean;
  device_info?: InverterDeviceInfo;
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
  last_online_at: string | null;
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


export interface ConsumptionUploadResult {
  uc: string;
  uc_label: string;
  titular: string | null;
  referencia: string;
  consumo_kwh: number;
  total_pagar_brl: number | null;
  meses_historico_importados: number;
}

export interface ConsumptionLatest {
  referencia: string;
  consumed_kwh: number;
  total_value_brl: number | null;
}

export interface ConsumptionUnitSummary {
  uc_number: string;
  label: string;
  latest: ConsumptionLatest | null;
}

export interface ConsumptionSummary {
  unidades: ConsumptionUnitSummary[];
  economia_estimada_brl: number | null;
}

export interface ConsumptionHistoryRow {
  referencia: string;
  consumo_kwh: number;
  total_pagar_brl: number | null;
  bandeira: string | null;
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
