import { useState } from "react";
import { IconBadge } from "./icons";
import { getReadAlertKeys, markAlertRead, type Alert } from "../lib/alerts";

const SEV_COLOR: Record<Alert["sev"], "red" | "gold" | "blue"> = { critical: "red", warning: "gold", info: "blue" };

// Espelha a Central de Alertas do Dashboard original: accordion fechado por
// padrão, badge com contador de não-lidos, "Marcar como lida" por item.
export function AlertCenter({ alerts }: { alerts: Alert[] }) {
  const [open, setOpen] = useState(false);
  const [, forceRender] = useState(0);

  const readKeys = getReadAlertKeys();
  const unread = alerts.filter((a) => !readKeys.has(a.key));

  function handleMarkRead(key: string) {
    markAlertRead(key);
    forceRender((n) => n + 1);
  }

  return (
    <div className={`card alert-card ${open ? "open" : ""}`}>
      <button className="alert-toggle" onClick={() => setOpen((o) => !o)}>
        <div className="alert-head">
          <h3>Central de alertas</h3>
        </div>
        <div className="alert-toggle-right">
          {unread.length > 0 && <span className="alert-badge">{unread.length}</span>}
          <span className="alert-chevron">▾</span>
        </div>
      </button>
      <div className="alert-body" hidden={!open}>
        {unread.length === 0 ? (
          <div className="alert-empty">
            {alerts.length > 0 ? "Todos os alertas foram marcados como lidos." : "Nenhum alerta ativo no momento."}
          </div>
        ) : (
          <div className="alert-list">
            {unread.map((a) => (
              <div className={`alert-item ${a.sev}`} key={a.key}>
                <div className="alert-icon">
                  <IconBadge name={a.icon} color={SEV_COLOR[a.sev]} size="alert" />
                </div>
                <div className="alert-item-body">
                  <div className="alert-title">{a.title}</div>
                  <div className="alert-desc">{a.desc}</div>
                </div>
                <button className="alert-mark-read" onClick={() => handleMarkRead(a.key)}>
                  Marcar como lida
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
