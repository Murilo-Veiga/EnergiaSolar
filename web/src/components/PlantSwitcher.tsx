import { useEffect, useRef, useState } from "react";
import { IconBadge } from "./icons";
import type { Plant } from "../lib/api";

export function PlantSwitcher({
  plants,
  activePlantId,
  onSelect,
}: {
  plants: Plant[];
  activePlantId: string | null;
  onSelect: (id: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function onClickOutside(e: MouseEvent) {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onClickOutside);
    return () => document.removeEventListener("mousedown", onClickOutside);
  }, [open]);

  if (plants.length === 0) return null;
  const active = plants.find((p) => p.id === activePlantId) ?? plants[0];
  const multiple = plants.length > 1;

  return (
    <div className="plant-switcher" ref={wrapRef}>
      <button
        className="plant-switcher-btn"
        type="button"
        onClick={() => multiple && setOpen((o) => !o)}
        aria-label="Trocar instalação"
      >
        <IconBadge name="mapPin" color="aqua" size="nav" />
        <span className="plant-switcher-name">{active.name}</span>
        {multiple && <span className="plant-switcher-chevron">▾</span>}
      </button>
      {open && multiple && (
        <div className="plant-switcher-dropdown">
          {plants.map((p) => (
            <button
              key={p.id}
              type="button"
              className={`plant-switcher-item ${p.id === active.id ? "active" : ""}`}
              onClick={() => {
                onSelect(p.id);
                setOpen(false);
              }}
            >
              <span>{p.name}</span>
              {!p.is_owner && <span className="badge">compartilhada</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
