import { IconBadge } from "../../components/icons";

// Placeholder desabilitado — depende do parser de fatura da Celesc (Fase
// 5 do plano, adiada a pedido do usuário). Ver
// /home/marcos/.claude/plans/polymorphic-humming-fog.md.
export function ConsumoTab() {
  return (
    <div className="card placeholder-tab">
      <IconBadge name="wallet" color="aqua" size="card" />
      <h3 style={{ margin: 0, color: "var(--ink-2)" }}>Em breve</h3>
      <p style={{ maxWidth: 380, fontSize: 13 }}>
        O upload de fatura da Celesc e o cálculo de economia ainda não foram portados pro novo backend — essa aba volta
        quando essa parte for implementada.
      </p>
    </div>
  );
}
