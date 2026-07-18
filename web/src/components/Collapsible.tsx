import { useState, type ReactNode } from "react";
import { IconBadge, type IconColor } from "./icons";
import { Tooltip } from "./Tooltip";
import { NewBadge } from "./NewBadge";

// Espelha o padrão hist-collapsible/alert-toggle/wireCollapsibles do
// original (templates/index.html) — reaproveita o card de alerta pra
// seções recolhíveis em Histórico/Saúde. Body sempre montado (só
// hidden/exibido via atributo `hidden`), pra não destruir/recriar
// instâncias de Chart.js a cada toggle.
export function Collapsible({
  icon,
  iconColor,
  title,
  tooltip,
  defaultOpen = false,
  newKey,
  children,
}: {
  icon: string;
  iconColor: IconColor;
  title: string;
  tooltip: string;
  defaultOpen?: boolean;
  newKey?: string;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <div className={`card alert-card hist-collapsible ${open ? "open" : ""}`}>
      <button className="alert-toggle" onClick={() => setOpen((o) => !o)}>
        <div className="alert-head">
          <IconBadge name={icon} color={iconColor} size="card" />
          <Tooltip text={tooltip} wide>
            <h3>
              {title}
              {newKey && <NewBadge featureKey={newKey} />}
            </h3>
          </Tooltip>
        </div>
        <div className="alert-toggle-right">
          <span className="alert-chevron">▾</span>
        </div>
      </button>
      <div className="alert-body" hidden={!open}>
        {children}
      </div>
    </div>
  );
}
