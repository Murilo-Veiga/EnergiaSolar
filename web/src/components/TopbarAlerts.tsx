import { useEffect, useRef, useState } from "react";
import { IconBadge } from "./icons";
import { getReadAlertKeys, markAlertRead, type Alert } from "../lib/alerts";

const SEV_COLOR: Record<Alert["sev"], "red" | "gold" | "blue"> = { critical: "red", warning: "gold", info: "blue" };

// Sino no topbar: mesma fonte de alertas do AlertCenter (computeAlerts +
// leitura via sessionStorage), só que num popover sempre acessível, fora da
// aba Dashboard.
export function TopbarAlerts({ alerts }: { alerts: Alert[] }) {
  const [open, setOpen] = useState(false);
  const [, forceRender] = useState(0);
  const wrapRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function onClickOutside(e: MouseEvent) {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onClickOutside);
    return () => document.removeEventListener("mousedown", onClickOutside);
  }, [open]);

  const readKeys = getReadAlertKeys();
  const unread = alerts.filter((a) => !readKeys.has(a.key));

  function handleMarkRead(key: string) {
    markAlertRead(key);
    forceRender((n) => n + 1);
  }

  return (
    <div className="bell-wrap" ref={wrapRef}>
      <button className="bell-btn" type="button" onClick={() => setOpen((o) => !o)} aria-label="Alertas">
        <IconBadge name="bell" color={unread.length > 0 ? "gold" : "blue"} size="nav" />
        {unread.length > 0 && <span className="bell-count">{unread.length}</span>}
      </button>
      {open && (
        <div className="bell-dropdown">
          <div className="bell-dropdown-head">Alertas</div>
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
      )}
    </div>
  );
}
