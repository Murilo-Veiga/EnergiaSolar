import { fmtNum } from "./fmt";
import type { InvertersResponse } from "./api";

export interface Alert {
  key: string;
  sev: "critical" | "warning" | "info";
  icon: string;
  title: string;
  desc: string;
}

const COMM_TIMEOUT_MIN = 15;
const TEMP_THRESHOLD_C = 65; // ILUSTRATIVO — não validado na doc dos fabricantes.
const GERACAO_ABAIXO_MEDIA_PCT = 10;
const API_FAILURE_ALERT_THRESHOLD = 2;

export const INV_LABELS: Record<string, string> = { huawei: "Huawei", foxess: "FoxESS" };

export const INV_STATUS_META: Record<string, { cls: string; label: string; tip: string }> = {
  gerando: { cls: "st-good", label: "Gerando", tip: "Gerando energia agora." },
  online_sem_geracao: {
    cls: "st-idle",
    label: "Online, sem geração",
    tip: "Conectado à nuvem, sem geração agora — normal à noite ou com pouco sol.",
  },
  sem_comunicacao: {
    cls: "st-critical",
    label: "Sem comunicação",
    tip: `Sem resposta da API há mais de ${COMM_TIMEOUT_MIN} min (3 ciclos de coleta) — pode indicar queda de energia, Wi-Fi ou disjuntor.`,
  },
};

// Espelha computeAlerts() em templates/index.html EXATAMENTE.
export function computeAlerts(c: {
  inverters: InvertersResponse;
  bandeira: string | null;
  bandeiraValorKwh: number | null;
  todayKwh: number | null;
  weekAvgKwh: number | null;
}): Alert[] {
  const alerts: Alert[] = [];

  for (const inv of Object.keys(c.inverters)) {
    const d = c.inverters[inv];
    if (d.status === "sem_comunicacao") {
      alerts.push({
        key: `comm:${inv}`,
        sev: "critical",
        icon: "alertTriangle",
        title: `${INV_LABELS[inv] ?? inv} sem comunicação`,
        desc: INV_STATUS_META.sem_comunicacao.tip,
      });
    }
    if (d.consecutive_failures >= API_FAILURE_ALERT_THRESHOLD) {
      alerts.push({
        key: `api_failure:${inv}`,
        sev: "warning",
        icon: "plug",
        title: `Falha ao consultar a API da ${INV_LABELS[inv] ?? inv} (${d.consecutive_failures}x seguidas)`,
        desc: d.last_error
          ? `Último erro: ${d.last_error} — "Gerado hoje" segue com o último valor bem-sucedido enquanto isso não normalizar.`
          : `A coleta está falhando repetidamente — "Gerado hoje" segue com o último valor bem-sucedido enquanto isso não normalizar.`,
      });
    }
    if (d.temperature_c !== null && d.temperature_c >= TEMP_THRESHOLD_C) {
      alerts.push({
        key: `temp:${inv}`,
        sev: "warning",
        icon: "thermometer",
        title: `Temperatura do inversor ${INV_LABELS[inv] ?? inv} em ${fmtNum(d.temperature_c, 0)}°C`,
        desc: `Acima do limiar ilustrativo de ${TEMP_THRESHOLD_C}°C (não validado com o fabricante ainda) — vale checar ventilação/sombra no local de instalação.`,
      });
    }
  }

  if (c.bandeira && c.bandeira.toLowerCase() !== "verde") {
    alerts.push({
      key: `bandeira:${c.bandeira.toLowerCase()}`,
      sev: "warning",
      icon: "flag",
      title: `Bandeira ${c.bandeira.toLowerCase()} ativa esse mês`,
      desc:
        c.bandeiraValorKwh !== null
          ? `Acréscimo de R$ ${fmtNum(c.bandeiraValorKwh, 5).replace(".", ",")}/kWh — tarifa mais alta que o normal enquanto durar.`
          : "Tarifa mais alta que o normal enquanto durar.",
    });
  }

  if (c.todayKwh !== null && c.weekAvgKwh) {
    const diffPct = Math.round((1 - c.todayKwh / c.weekAvgKwh) * 100);
    if (diffPct >= GERACAO_ABAIXO_MEDIA_PCT) {
      alerts.push({
        key: "geracao_baixa",
        sev: "info",
        icon: "trendingDown",
        title: `Geração de hoje ${diffPct}% abaixo da média da semana`,
        desc: `${fmtNum(c.todayKwh)} kWh hoje vs. média de ${fmtNum(c.weekAvgKwh)} kWh nos últimos 7 dias — pode ser só o clima, não é necessariamente um problema.`,
      });
    }
  }

  return alerts;
}

// "Marcar como lida" esconde o alerta pelo resto desse acesso
// (sessionStorage — some ao fechar a aba/navegador) sem apagar a condição
// em si — mesmo mecanismo do original.
const READ_ALERTS_KEY = "solarhome_read_alerts";

export function getReadAlertKeys(): Set<string> {
  try {
    return new Set(JSON.parse(sessionStorage.getItem(READ_ALERTS_KEY) ?? "[]"));
  } catch {
    return new Set();
  }
}

export function markAlertRead(key: string) {
  const read = getReadAlertKeys();
  read.add(key);
  sessionStorage.setItem(READ_ALERTS_KEY, JSON.stringify([...read]));
}
