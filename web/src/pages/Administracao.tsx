import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError, type InverterCredential, type Plant } from "../lib/api";
import { useAuth } from "../context/AuthContext";

interface Props {
  plants: Plant[];
  activePlantId: string | null;
  onSelectPlant: (id: string | null) => void;
}

export function Administracao({ plants, activePlantId, onSelectPlant }: Props) {
  const { refreshPlants } = useAuth();
  const activePlant = plants.find((p) => p.id === activePlantId) ?? null;

  return (
    <div>
      <div className="admin-section">
        <h3>Suas usinas</h3>
        <div className="plant-list">
          {plants.map((p) => (
            <div
              key={p.id}
              className={`plant-list-item ${p.id === activePlantId ? "active" : ""}`}
              onClick={() => onSelectPlant(p.id)}
            >
              <span>{p.name}</span>
              <span style={{ color: "var(--ink-muted)", fontSize: 12 }}>{p.installed_power_kwp} kWp</span>
            </div>
          ))}
          {plants.length === 0 && <div style={{ color: "var(--ink-muted)", fontSize: 13 }}>Nenhuma usina cadastrada ainda.</div>}
        </div>
        <NewPlantForm onCreated={refreshPlants} />
      </div>

      {activePlant && (
        <>
          <div className="admin-section">
            <h3>Dados da usina</h3>
            <PlantForm plant={activePlant} onSaved={refreshPlants} onDeleted={() => { onSelectPlant(null); void refreshPlants(); }} />
          </div>
          <div className="admin-section">
            <h3>Credenciais de inversor</h3>
            <CredentialsManager plantId={activePlant.id} />
          </div>
        </>
      )}
    </div>
  );
}

function NewPlantForm({ onCreated }: { onCreated: () => Promise<void> }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.post("/api/plants", { name, installed_power_kwp: 0 });
      setName("");
      setOpen(false);
      await onCreated();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao criar usina");
    } finally {
      setSubmitting(false);
    }
  }

  if (!open) {
    return (
      <button className="btn btn-secondary" onClick={() => setOpen(true)}>
        + Nova usina
      </button>
    );
  }

  return (
    <form onSubmit={handleSubmit} style={{ display: "flex", gap: 8, alignItems: "flex-start" }}>
      <input
        placeholder="Nome da usina"
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
        style={{ background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 7, padding: "8px 9px", color: "var(--ink)" }}
      />
      <button className="btn" type="submit" disabled={submitting}>
        Criar
      </button>
      <button className="btn btn-secondary" type="button" onClick={() => setOpen(false)}>
        Cancelar
      </button>
      {error && <span className="auth-error">{error}</span>}
    </form>
  );
}

function PlantForm({ plant, onSaved, onDeleted }: { plant: Plant; onSaved: () => Promise<void>; onDeleted: () => void }) {
  const [name, setName] = useState(plant.name);
  const [lat, setLat] = useState(plant.lat?.toString() ?? "");
  const [lon, setLon] = useState(plant.lon?.toString() ?? "");
  const [installedKwp, setInstalledKwp] = useState(plant.installed_power_kwp.toString());
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setName(plant.name);
    setLat(plant.lat?.toString() ?? "");
    setLon(plant.lon?.toString() ?? "");
    setInstalledKwp(plant.installed_power_kwp.toString());
  }, [plant]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.put(`/api/plants/${plant.id}`, {
        name,
        lat: lat ? Number(lat) : null,
        lon: lon ? Number(lon) : null,
        installed_power_kwp: Number(installedKwp) || 0,
      });
      await onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao salvar");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete() {
    if (!confirm(`Remover a usina "${plant.name}"? Isso apaga também as credenciais associadas.`)) return;
    await api.delete(`/api/plants/${plant.id}`);
    onDeleted();
  }

  return (
    <form onSubmit={handleSubmit} className="admin-form">
      <label>
        Nome
        <input value={name} onChange={(e) => setName(e.target.value)} required />
      </label>
      <label>
        Potência instalada (kWp)
        <input type="number" step="0.01" value={installedKwp} onChange={(e) => setInstalledKwp(e.target.value)} />
      </label>
      <label>
        Latitude
        <input type="number" step="0.000001" value={lat} onChange={(e) => setLat(e.target.value)} placeholder="ex.: -26.356268" />
      </label>
      <label>
        Longitude
        <input type="number" step="0.000001" value={lon} onChange={(e) => setLon(e.target.value)} placeholder="ex.: -48.807868" />
      </label>
      <div className="admin-form-full" style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <button className="btn" type="submit" disabled={submitting}>
          Salvar
        </button>
        <button className="btn btn-danger" type="button" onClick={handleDelete}>
          Remover usina
        </button>
        {error && <span className="auth-error">{error}</span>}
      </div>
    </form>
  );
}

function CredentialsManager({ plantId }: { plantId: string }) {
  const [credentials, setCredentials] = useState<InverterCredential[]>([]);
  const [loading, setLoading] = useState(true);

  async function load() {
    setLoading(true);
    const list = await api.get<InverterCredential[]>(`/api/plants/${plantId}/inverters-config`);
    setCredentials(list);
    setLoading(false);
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [plantId]);

  async function toggleEnabled(cred: InverterCredential) {
    await api.put(`/api/plants/${plantId}/inverters-config/${cred.id}`, { enabled: !cred.enabled });
    await load();
  }

  async function remove(cred: InverterCredential) {
    if (!confirm(`Remover a credencial ${cred.brand}?`)) return;
    await api.delete(`/api/plants/${plantId}/inverters-config/${cred.id}`);
    await load();
  }

  const existingBrands = new Set(credentials.map((c) => c.brand));

  return (
    <div>
      {loading ? (
        <div style={{ color: "var(--ink-muted)", fontSize: 13 }}>Carregando...</div>
      ) : (
        credentials.map((cred) => (
          <div className="credential-row" key={cred.id}>
            <span style={{ textTransform: "capitalize" }}>{cred.brand}</span>
            <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
              <span className={`badge ${cred.enabled ? "on" : ""}`}>{cred.enabled ? "Habilitado" : "Desabilitado"}</span>
              <button className="btn btn-secondary" onClick={() => toggleEnabled(cred)}>
                {cred.enabled ? "Desabilitar" : "Habilitar"}
              </button>
              <button className="btn btn-danger" onClick={() => remove(cred)}>
                Remover
              </button>
            </div>
          </div>
        ))
      )}

      {!existingBrands.has("huawei") && <AddCredentialForm plantId={plantId} brand="huawei" onSaved={load} />}
      {!existingBrands.has("foxess") && <AddCredentialForm plantId={plantId} brand="foxess" onSaved={load} />}
    </div>
  );
}

function AddCredentialForm({ plantId, brand, onSaved }: { plantId: string; brand: "huawei" | "foxess"; onSaved: () => Promise<void> }) {
  const [open, setOpen] = useState(false);
  const [username, setUsername] = useState("");
  const [systemCode, setSystemCode] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);
  const [testing, setTesting] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function body() {
    return brand === "huawei" ? { brand, username, system_code: systemCode } : { brand, api_key: apiKey };
  }

  async function handleTest() {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await api.post<{ success: boolean; message: string }>(`/api/plants/${plantId}/inverters-config/test`, body());
      setTestResult(result);
    } catch (err) {
      setTestResult({ success: false, message: err instanceof ApiError ? err.message : "Falha ao testar" });
    } finally {
      setTesting(false);
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.post(`/api/plants/${plantId}/inverters-config`, body());
      setOpen(false);
      setUsername("");
      setSystemCode("");
      setApiKey("");
      setTestResult(null);
      await onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao salvar credencial");
    } finally {
      setSubmitting(false);
    }
  }

  if (!open) {
    return (
      <button className="btn btn-secondary" style={{ marginTop: 8 }} onClick={() => setOpen(true)}>
        + Adicionar credencial {brand}
      </button>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="admin-form" style={{ marginTop: 12 }}>
      {brand === "huawei" ? (
        <>
          <label>
            Usuário Huawei
            <input value={username} onChange={(e) => setUsername(e.target.value)} required />
          </label>
          <label>
            System code
            <input value={systemCode} onChange={(e) => setSystemCode(e.target.value)} required />
          </label>
        </>
      ) : (
        <label className="admin-form-full">
          API key FoxESS
          <input value={apiKey} onChange={(e) => setApiKey(e.target.value)} required />
        </label>
      )}
      <div className="admin-form-full" style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <button className="btn btn-secondary" type="button" onClick={handleTest} disabled={testing}>
          {testing ? "Testando..." : "Testar conexão"}
        </button>
        <button className="btn" type="submit" disabled={submitting}>
          Salvar
        </button>
        <button className="btn btn-secondary" type="button" onClick={() => setOpen(false)}>
          Cancelar
        </button>
      </div>
      {testResult && (
        <div className={`test-result admin-form-full ${testResult.success ? "ok" : "fail"}`}>{testResult.message}</div>
      )}
      {error && <div className="auth-error admin-form-full">{error}</div>}
    </form>
  );
}
