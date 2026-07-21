import { useState } from "react";
import { useAuth } from "./context/AuthContext";
import { PlantProvider } from "./context/PlantContext";
import { Login } from "./pages/Login";
import { Administracao } from "./pages/Administracao";
import { MinhasUsinas } from "./pages/MinhasUsinas";
import { MinhaConta } from "./pages/MinhaConta";
import { DashboardTab } from "./pages/Dashboard/DashboardTab";
import { HistoricoTab } from "./pages/Dashboard/HistoricoTab";
import { SaudeTab } from "./pages/Dashboard/SaudeTab";
import { ConsumoTab } from "./pages/Dashboard/ConsumoTab";
import { NavBar } from "./components/NavBar";
import { IconBadge } from "./components/icons";
import { TopbarAlerts } from "./components/TopbarAlerts";
import { PlantSwitcher } from "./components/PlantSwitcher";
import type { Alert } from "./lib/alerts";

export type TabName = "dashboard" | "historico" | "saude" | "consumo" | "minhas-usinas" | "administracao" | "minha-conta";

const TITLES: Record<TabName, string> = {
  dashboard: "Dashboard",
  historico: "Histórico",
  saude: "Saúde da instalação",
  consumo: "Consumo",
  "minhas-usinas": "Gerenciar usinas",
  administracao: "Administração",
  "minha-conta": "Minha conta",
};

function App() {
  const { authenticated, loading, plants } = useAuth();
  const [tab, setTab] = useState<TabName>("dashboard");
  const [selectedPlantId, setSelectedPlantId] = useState<string | null>(null);
  const [updatedAt, setUpdatedAt] = useState<string | null>(null);
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  if (loading) {
    return <div className="app-loading">Carregando...</div>;
  }

  if (!authenticated) {
    return <Login />;
  }

  const activePlant = plants.find((p) => p.id === selectedPlantId) ?? plants[0] ?? null;

  // Sem nenhuma instalação cadastrada ainda: força a aba Minhas
  // instalações — não dá pra abrir Dashboard/Histórico/Saúde sem
  // instalação nenhuma selecionada. "Minha conta" não depende de
  // instalação, então fica de fora dessa trava.
  const effectiveTab: TabName = !activePlant && tab !== "minha-conta" ? "minhas-usinas" : tab;

  return (
    <div className="app">
      <NavBar
        active={effectiveTab}
        onSelect={setTab}
        onMyAccount={() => setTab("minha-conta")}
        mobileOpen={mobileNavOpen}
        onCloseMobile={() => setMobileNavOpen(false)}
      />
      <main className="main">
        <div className="topbar">
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <button className="nav-hamburger" type="button" onClick={() => setMobileNavOpen(true)} aria-label="Abrir menu">
              <IconBadge name="menu" color="blue" size="nav" />
            </button>
            <h2>{TITLES[effectiveTab]}</h2>
          </div>
          <div className="topbar-actions">
            {effectiveTab === "dashboard" && updatedAt && (
              <div className="updated">
                <span className="sw" /> Atualizado às {new Date(updatedAt).toLocaleTimeString("pt-BR")}
              </div>
            )}
            <TopbarAlerts alerts={alerts} />
            <PlantSwitcher plants={plants} activePlantId={activePlant?.id ?? null} onSelect={setSelectedPlantId} />
          </div>
        </div>

        {effectiveTab === "minhas-usinas" && (
          <MinhasUsinas plants={plants} activePlantId={activePlant?.id ?? null} onSelectPlant={setSelectedPlantId} />
        )}

        {effectiveTab === "administracao" && <Administracao />}

        {effectiveTab === "minha-conta" && <MinhaConta />}

        {activePlant && effectiveTab !== "administracao" && effectiveTab !== "minha-conta" && effectiveTab !== "minhas-usinas" && (
          <PlantProvider plant={activePlant}>
            {effectiveTab === "dashboard" && <DashboardTab onUpdatedAt={setUpdatedAt} onAlerts={setAlerts} />}
            {effectiveTab === "historico" && <HistoricoTab />}
            {effectiveTab === "saude" && <SaudeTab />}
            {effectiveTab === "consumo" && <ConsumoTab />}
          </PlantProvider>
        )}
      </main>
    </div>
  );
}

export default App;
