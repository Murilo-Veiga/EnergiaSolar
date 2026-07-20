import { useState } from "react";
import { useAuth } from "./context/AuthContext";
import { PlantProvider } from "./context/PlantContext";
import { Login } from "./pages/Login";
import { Signup } from "./pages/Signup";
import { Administracao } from "./pages/Administracao";
import { MinhaConta } from "./pages/MinhaConta";
import { DashboardTab } from "./pages/Dashboard/DashboardTab";
import { HistoricoTab } from "./pages/Dashboard/HistoricoTab";
import { SaudeTab } from "./pages/Dashboard/SaudeTab";
import { ConsumoTab } from "./pages/Dashboard/ConsumoTab";
import { NavBar } from "./components/NavBar";

export type TabName = "dashboard" | "historico" | "saude" | "consumo" | "administracao" | "minha-conta";

const TITLES: Record<TabName, string> = {
  dashboard: "Dashboard",
  historico: "Histórico",
  saude: "Saúde da usina",
  consumo: "Consumo",
  administracao: "Administração",
  "minha-conta": "Minha conta",
};

function App() {
  const { authenticated, loading, plants } = useAuth();
  const [authView, setAuthView] = useState<"login" | "signup">("login");
  const [tab, setTab] = useState<TabName>("dashboard");
  const [selectedPlantId, setSelectedPlantId] = useState<string | null>(null);
  const [updatedAt, setUpdatedAt] = useState<string | null>(null);

  if (loading) {
    return <div className="app-loading">Carregando...</div>;
  }

  if (!authenticated) {
    return authView === "login" ? (
      <Login onSwitchToSignup={() => setAuthView("signup")} />
    ) : (
      <Signup onSwitchToLogin={() => setAuthView("login")} />
    );
  }

  const activePlant = plants.find((p) => p.id === selectedPlantId) ?? plants[0] ?? null;

  // Sem nenhuma usina cadastrada ainda: força a aba Administração — não dá
  // pra abrir Dashboard/Histórico/Saúde sem usina nenhuma selecionada.
  // "Minha conta" não depende de usina, então fica de fora dessa trava.
  const effectiveTab: TabName = !activePlant && tab !== "minha-conta" ? "administracao" : tab;

  return (
    <div className="app">
      <NavBar active={effectiveTab} onSelect={setTab} onMyAccount={() => setTab("minha-conta")} />
      <main className="main">
        <div className="topbar">
          <h2>{TITLES[effectiveTab]}</h2>
          {effectiveTab === "dashboard" && updatedAt && (
            <div className="updated">
              <span className="sw" /> Atualizado às {new Date(updatedAt).toLocaleTimeString("pt-BR")}
            </div>
          )}
        </div>

        {effectiveTab === "administracao" && (
          <Administracao plants={plants} activePlantId={activePlant?.id ?? null} onSelectPlant={setSelectedPlantId} />
        )}

        {effectiveTab === "minha-conta" && <MinhaConta />}

        {activePlant && effectiveTab !== "administracao" && effectiveTab !== "minha-conta" && (
          <PlantProvider plant={activePlant}>
            {effectiveTab === "dashboard" && <DashboardTab onUpdatedAt={setUpdatedAt} />}
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
