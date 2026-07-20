import type { ReactNode } from "react";

// Espelha o padrão .tt/.tip do CSS original (templates/index.html) — texto
// de ajuda que aparece no hover, sem precisar de lib de tooltip nova.
export function Tooltip({ text, wide = false, children }: { text: string; wide?: boolean; children: ReactNode }) {
  return (
    <span className="tt">
      {children}
      <span className={`tip ${wide ? "tip-wide" : ""}`}>{text}</span>
    </span>
  );
}
