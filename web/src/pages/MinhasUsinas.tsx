import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError, type InverterCredential, type InverterDeviceInfo, type Plant } from "../lib/api";
import { useAuth } from "../context/AuthContext";

interface Props {
  plants: Plant[];
  activePlantId: string | null;
  onSelectPlant: (id: string | null) => void;
}

export function MinhasUsinas({ plants, activePlantId, onSelectPlant }: Props) {
  const { refreshPlants } = useAuth();
  const activePlant = plants.find((p) => p.id === activePlantId) ?? null;

  return (
    <div>
      <div className="admin-section">
        <h3>Suas instalações</h3>
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
          {plants.length === 0 && <div style={{ color: "var(--ink-muted)", fontSize: 13 }}>Nenhuma instalação cadastrada ainda.</div>}
        </div>
        <NewPlantForm onCreated={refreshPlants} />
      </div>

      {activePlant && (
        <>
          <div className="admin-section">
            <h3>Dados da instalação</h3>
            <PlantForm plant={activePlant} onSaved={refreshPlants} onDeleted={() => { onSelectPlant(null); void refreshPlants(); }} />
          </div>
          <div className="admin-section">
            <h3>Inversores da instalação</h3>
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
      setError(err instanceof ApiError ? err.message : "Falha ao criar instalação");
    } finally {
      setSubmitting(false);
    }
  }

  if (!open) {
    return (
      <button className="btn btn-secondary" onClick={() => setOpen(true)}>
        + Nova instalação
      </button>
    );
  }

  return (
    <form onSubmit={handleSubmit} style={{ display: "flex", gap: 8, alignItems: "flex-start" }}>
      <input
        placeholder="Nome da instalação"
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
    if (!confirm(`Remover a instalação "${plant.name}"? Isso apaga também as credenciais associadas.`)) return;
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
          Remover instalação
        </button>
        {error && <span className="auth-error">{error}</span>}
      </div>
    </form>
  );
}

function CredentialsManager({ plantId }: { plantId: string }) {
  const [credentials, setCredentials] = useState<InverterCredential[]>([]);
  const [loading, setLoading] = useState(true);
  const [editingId, setEditingId] = useState<string | null>(null);

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
        credentials.map((cred) =>
          editingId === cred.id ? (
            <CredentialForm
              key={cred.id}
              plantId={plantId}
              brand={cred.brand}
              credId={cred.id}
              onSaved={async () => {
                setEditingId(null);
                await load();
              }}
              onCancel={() => setEditingId(null)}
            />
          ) : (
            <div
              className="credential-row"
              key={cred.id}
              style={{ flexDirection: "column", alignItems: "stretch", justifyContent: "flex-start", gap: 6 }}
            >
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <span style={{ textTransform: "capitalize" }}>{cred.brand}</span>
                <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
                  <span className={`badge ${cred.enabled ? "on" : ""}`}>{cred.enabled ? "Habilitado" : "Desabilitado"}</span>
                  <button className="btn btn-secondary" onClick={() => setEditingId(cred.id)}>
                    Editar
                  </button>
                  <button className="btn btn-secondary" onClick={() => toggleEnabled(cred)}>
                    {cred.enabled ? "Desabilitar" : "Habilitar"}
                  </button>
                  <button className="btn btn-danger" onClick={() => remove(cred)}>
                    Remover
                  </button>
                </div>
              </div>
              <DeviceInfoSummary info={cred.device_info} />
            </div>
          ),
        )
      )}

      {!existingBrands.has("huawei") && <CredentialForm plantId={plantId} brand="huawei" onSaved={load} />}
      {!existingBrands.has("foxess") && <CredentialForm plantId={plantId} brand="foxess" onSaved={load} />}
    </div>
  );
}

// DeviceInfoSummary mostra o retrato mais recente do inversor (identificador
// + potência/geração/temperatura), atualizado na hora do cadastro/edição da
// credencial (ver discoverAndPersistSnapshot no backend) — ou o erro da
// última tentativa de busca, se não deu pra confirmar o inversor ainda.
function DeviceInfoSummary({ info }: { info?: InverterDeviceInfo }) {
  if (!info) {
    return <div style={{ color: "var(--ink-muted)", fontSize: 12 }}>Ainda sem dados do inversor — aguardando 1º ciclo de coleta.</div>;
  }
  if (info.error && info.power_kw === undefined) {
    return <div className="auth-error" style={{ fontSize: 12 }}>Não foi possível buscar os dados do inversor agora: {info.error}</div>;
  }

  const id = info.device_sn ?? info.dev_dn ?? info.station_code;
  return (
    <div style={{ display: "flex", gap: 14, flexWrap: "wrap", fontSize: 12, color: "var(--ink-muted)" }}>
      {id && <span>Inversor: {id}</span>}
      {info.power_kw !== undefined && <span>{info.power_kw.toFixed(2)} kW agora</span>}
      {info.day_kwh !== undefined && <span>{info.day_kwh.toFixed(2)} kWh hoje</span>}
      {info.temperature_c !== undefined && <span>{info.temperature_c.toFixed(1)} °C</span>}
      {info.error && <span style={{ color: "var(--critical)" }}>Última busca falhou: {info.error}</span>}
    </div>
  );
}

function CredentialForm({
  plantId,
  brand,
  credId,
  onSaved,
  onCancel,
}: {
  plantId: string;
  brand: "huawei" | "foxess";
  credId?: string;
  onSaved: () => Promise<void>;
  onCancel?: () => void;
}) {
  const isEdit = !!credId;
  const [open, setOpen] = useState(isEdit);
  const [username, setUsername] = useState("");
  const [systemCode, setSystemCode] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);
  const [testing, setTesting] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function body() {
    const common = { brand, base_url: baseUrl || undefined };
    return brand === "huawei"
      ? { ...common, username: username || undefined, system_code: systemCode || undefined }
      : { ...common, api_key: apiKey || undefined };
  }

  function close() {
    if (isEdit) {
      onCancel?.();
    } else {
      setOpen(false);
    }
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
      if (isEdit) {
        await api.put(`/api/plants/${plantId}/inverters-config/${credId}`, body());
      } else {
        await api.post(`/api/plants/${plantId}/inverters-config`, body());
      }
      setUsername("");
      setSystemCode("");
      setApiKey("");
      setBaseUrl("");
      setTestResult(null);
      setOpen(false);
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
        + Adicionar inversor {brand}
      </button>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="admin-form" style={{ marginTop: 12 }}>
      {isEdit && (
        <div className="admin-form-full" style={{ color: "var(--ink-muted)", fontSize: 12 }}>
          Por segurança, as credenciais salvas não são exibidas aqui. Preencha os campos abaixo apenas se quiser
          substituí-las — deixe em branco para manter os valores atuais.
        </div>
      )}
      {brand === "huawei" ? (
        <>
          <label>
            Usuário Huawei
            <input value={username} onChange={(e) => setUsername(e.target.value)} required={!isEdit} />
          </label>
          <label>
            System code
            <input value={systemCode} onChange={(e) => setSystemCode(e.target.value)} required={!isEdit} />
          </label>
        </>
      ) : (
        <label className="admin-form-full">
          API key FoxESS
          <input value={apiKey} onChange={(e) => setApiKey(e.target.value)} required={!isEdit} />
        </label>
      )}
      <label className="admin-form-full">
        URL base (opcional)
        <input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="usa o padrão do sistema se vazio" />
      </label>
      <div className="admin-form-full" style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <button className="btn btn-secondary" type="button" onClick={handleTest} disabled={testing}>
          {testing ? "Testando..." : "Testar conexão"}
        </button>
        <button className="btn" type="submit" disabled={submitting}>
          Salvar
        </button>
        <button className="btn btn-secondary" type="button" onClick={close}>
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
