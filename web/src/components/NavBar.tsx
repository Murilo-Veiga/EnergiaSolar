import { IconBadge } from "./icons";
import { NewBadge } from "./NewBadge";
import type { TabName } from "../App";
import { useAuth } from "../context/AuthContext";

const NAV_ITEMS: { tab: TabName; label: string; icon: string; color: "blue" | "green" | "aqua"; newKey?: string; adminOnly?: boolean }[] = [
  { tab: "dashboard", label: "Dashboard", icon: "layoutGrid", color: "blue" },
  { tab: "historico", label: "Histórico", icon: "trendingUp", color: "blue" },
  { tab: "saude", label: "Saúde", icon: "activity", color: "green" },
  { tab: "consumo", label: "Consumo", icon: "wallet", color: "aqua" },
  { tab: "minhas-usinas", label: "Gerenciar usinas", icon: "sun", color: "green" },
  { tab: "administracao", label: "Administração", icon: "settings", color: "blue", adminOnly: true },
];

export function NavBar({
  active,
  onSelect,
  onMyAccount,
  mobileOpen,
  onCloseMobile,
}: {
  active: TabName;
  onSelect: (tab: TabName) => void;
  onMyAccount: () => void;
  mobileOpen: boolean;
  onCloseMobile: () => void;
}) {
  const { logout, isAdmin } = useAuth();
  const items = NAV_ITEMS.filter((item) => !item.adminOnly || isAdmin);

  // No mobile o menu é um drawer — qualquer navegação (item, conta, sair)
  // já fecha ele sozinho, sem precisar tocar de novo no overlay/X.
  function select(tab: TabName) {
    onSelect(tab);
    onCloseMobile();
  }

  return (
    <>
      {mobileOpen && <div className="sidebar-overlay" onClick={onCloseMobile} />}
      <aside className={`sidebar ${mobileOpen ? "open" : ""}`} style={{ display: "flex", flexDirection: "column" }}>
        <div className="brand">
          <IconBadge name="sun" color="blue" size="nav" />
          <div>
            <div className="name">Solar Home</div>
            <div className="sub">Painel de Monitoramento</div>
          </div>
          <button className="sidebar-close" type="button" onClick={onCloseMobile} aria-label="Fechar menu">
            ×
          </button>
        </div>
        <nav className="nav">
          {items.map((item) => (
            <a
              key={item.tab}
              className={active === item.tab ? "active" : ""}
              onClick={() => select(item.tab)}
              style={{ cursor: "pointer" }}
            >
              <IconBadge name={item.icon} color={item.color} size="nav" />
              {" " + item.label}
              {item.newKey && <NewBadge featureKey={item.newKey} />}
            </a>
          ))}
        </nav>
        <div className="nav" style={{ marginTop: "auto", paddingTop: 16, borderTop: "1px solid var(--border)" }}>
          <a
            className={active === "minha-conta" ? "active" : ""}
            onClick={() => {
              onMyAccount();
              onCloseMobile();
            }}
            style={{ cursor: "pointer" }}
          >
            <IconBadge name="user" color="blue" size="nav" />
            {" Minha conta"}
          </a>
          <a onClick={() => void logout()} style={{ cursor: "pointer" }}>
            <IconBadge name="logout" color="red" size="nav" />
            {" Sair"}
          </a>
        </div>
      </aside>
    </>
  );
}
