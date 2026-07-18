import { IconBadge } from "./icons";
import type { TabName } from "../App";
import { useAuth } from "../context/AuthContext";

const NAV_ITEMS: { tab: TabName; label: string; icon: string; color: "blue" | "green" | "aqua" }[] = [
  { tab: "dashboard", label: "Dashboard", icon: "layoutGrid", color: "blue" },
  { tab: "historico", label: "Histórico", icon: "trendingUp", color: "blue" },
  { tab: "saude", label: "Saúde da usina", icon: "activity", color: "green" },
  { tab: "consumo", label: "Consumo", icon: "wallet", color: "aqua" },
  { tab: "administracao", label: "Administração", icon: "settings", color: "blue" },
];

export function NavBar({ active, onSelect }: { active: TabName; onSelect: (tab: TabName) => void }) {
  const { logout } = useAuth();

  return (
    <aside className="sidebar" style={{ display: "flex", flexDirection: "column" }}>
      <div className="brand">
        <IconBadge name="sun" color="blue" size="nav" />
        <div>
          <div className="name">Solar Home</div>
          <div className="sub">Painel de Monitoramento</div>
        </div>
      </div>
      <nav className="nav">
        {NAV_ITEMS.map((item) => (
          <a
            key={item.tab}
            className={active === item.tab ? "active" : ""}
            onClick={() => onSelect(item.tab)}
            style={{ cursor: "pointer" }}
          >
            <IconBadge name={item.icon} color={item.color} size="nav" />
            {item.label}
          </a>
        ))}
      </nav>
      <div style={{ marginTop: "auto", paddingTop: 16 }}>
        <a onClick={() => void logout()} style={{ cursor: "pointer" }}>
          Sair
        </a>
      </div>
    </aside>
  );
}
