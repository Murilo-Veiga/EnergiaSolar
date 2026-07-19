import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError, type AdminUser, type InverterCredential, type Me, type Plant } from "../lib/api";
import { useAuth } from "../context/AuthContext";

interface Props {
  plants: Plant[];
  activePlantId: string | null;
  onSelectPlant: (id: string | null) => void;
}

type AdminTab = "sistema" | "usuarios";

export function Administracao({ plants, activePlantId, onSelectPlant }: Props) {
  const [tab, setTab] = useState<AdminTab>("sistema");

  return (
    <div>
      <div className="chart-head" style={{ marginBottom: 16 }}>
        <div className="range-toggle">
          <span className={tab === "sistema" ? "active" : ""} onClick={() => setTab("sistema")}>
            Configuração do sistema
          </span>
          <span className={tab === "usuarios" ? "active" : ""} onClick={() => setTab("usuarios")}>
            Configuração de usuários
          </span>
        </div>
      </div>

      {tab === "sistema" ? (
        <SistemaTab plants={plants} activePlantId={activePlantId} onSelectPlant={onSelectPlant} />
      ) : (
        <UsuariosTab />
      )}
    </div>
  );
}

function SistemaTab({ plants, activePlantId, onSelectPlant }: Props) {
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

function UsuariosTab() {
  const { isAdmin } = useAuth();
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  async function load() {
    setLoading(true);
    const data = await api.get<Me>("/api/me");
    setMe(data);
    setLoading(false);
  }

  useEffect(() => {
    void load();
  }, []);

  if (loading || !me) {
    return <div style={{ color: "var(--ink-muted)", fontSize: 13 }}>Carregando...</div>;
  }

  return (
    <div>
      <div className="admin-section">
        <h3>Conta</h3>
        <EmailForm email={me.email} onSaved={load} />
      </div>
      <div className="admin-section">
        <h3>Senha</h3>
        <PasswordForm />
      </div>
      {isAdmin && (
        <div className="admin-section">
          <h3>Gerência de usuários</h3>
          <UserManagement currentUserId={me.user_id} />
        </div>
      )}
    </div>
  );
}

function UserManagement({ currentUserId }: { currentUserId: string }) {
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    try {
      const list = await api.get<AdminUser[]>("/api/admin/users");
      setUsers(list);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao listar usuários");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function remove(user: AdminUser) {
    if (!confirm(`Apagar o usuário "${user.email}"? Isso também remove suas usinas e credenciais.`)) return;
    try {
      await api.delete(`/api/admin/users/${user.id}`);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao apagar usuário");
    }
  }

  return (
    <div>
      {error && <div className="auth-error" style={{ marginBottom: 8 }}>{error}</div>}
      {loading ? (
        <div style={{ color: "var(--ink-muted)", fontSize: 13 }}>Carregando...</div>
      ) : (
        <div className="plant-list">
          {users.map((u) =>
            editingId === u.id ? (
              <EditUserForm
                key={u.id}
                user={u}
                isSelf={u.id === currentUserId}
                onSaved={async () => {
                  setEditingId(null);
                  await load();
                }}
                onCancel={() => setEditingId(null)}
              />
            ) : (
              <div className="plant-list-item" key={u.id} style={{ cursor: "default" }}>
                <span>
                  {u.email}
                  {u.is_admin && (
                    <span className="badge on" style={{ marginLeft: 8 }}>
                      admin
                    </span>
                  )}
                </span>
                <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
                  <span style={{ color: "var(--ink-muted)", fontSize: 12 }}>
                    {u.plants_count} usina{u.plants_count === 1 ? "" : "s"}
                  </span>
                  <button className="btn btn-secondary" onClick={() => setEditingId(u.id)}>
                    Editar
                  </button>
                  {u.id !== currentUserId && (
                    <button className="btn btn-danger" onClick={() => remove(u)}>
                      Apagar
                    </button>
                  )}
                </div>
              </div>
            ),
          )}
          {users.length === 0 && <div style={{ color: "var(--ink-muted)", fontSize: 13 }}>Nenhum usuário cadastrado.</div>}
        </div>
      )}
      <NewUserForm onCreated={load} />
    </div>
  );
}

function EditUserForm({
  user,
  isSelf,
  onSaved,
  onCancel,
}: {
  user: AdminUser;
  isSelf: boolean;
  onSaved: () => Promise<void>;
  onCancel: () => void;
}) {
  const [email, setEmail] = useState(user.email);
  const [isAdminFlag, setIsAdminFlag] = useState(user.is_admin);
  const [newPassword, setNewPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.put(`/api/admin/users/${user.id}`, { email, is_admin: isAdminFlag });
      if (newPassword) {
        await api.put(`/api/admin/users/${user.id}/password`, { new_password: newPassword });
      }
      await onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao salvar usuário");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="admin-form" style={{ marginBottom: 8 }}>
      <label>
        E-mail
        <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
      </label>
      <label>
        Nova senha (opcional)
        <input
          type="password"
          value={newPassword}
          onChange={(e) => setNewPassword(e.target.value)}
          minLength={8}
          placeholder="deixe em branco para manter"
        />
      </label>
      <label style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
        <input
          type="checkbox"
          checked={isAdminFlag}
          disabled={isSelf}
          onChange={(e) => setIsAdminFlag(e.target.checked)}
        />
        Administrador
      </label>
      <div className="admin-form-full" style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <button className="btn" type="submit" disabled={submitting}>
          Salvar
        </button>
        <button className="btn btn-secondary" type="button" onClick={onCancel}>
          Cancelar
        </button>
        {error && <span className="auth-error">{error}</span>}
      </div>
    </form>
  );
}

function NewUserForm({ onCreated }: { onCreated: () => Promise<void> }) {
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [isAdminFlag, setIsAdminFlag] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.post("/api/admin/users", { email, password, is_admin: isAdminFlag });
      setEmail("");
      setPassword("");
      setIsAdminFlag(false);
      setOpen(false);
      await onCreated();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao criar usuário");
    } finally {
      setSubmitting(false);
    }
  }

  if (!open) {
    return (
      <button className="btn btn-secondary" style={{ marginTop: 8 }} onClick={() => setOpen(true)}>
        + Novo usuário
      </button>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="admin-form" style={{ marginTop: 12 }}>
      <label>
        E-mail
        <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
      </label>
      <label>
        Senha
        <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required />
      </label>
      <label style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
        <input type="checkbox" checked={isAdminFlag} onChange={(e) => setIsAdminFlag(e.target.checked)} />
        Administrador
      </label>
      <div className="admin-form-full" style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <button className="btn" type="submit" disabled={submitting}>
          Criar
        </button>
        <button className="btn btn-secondary" type="button" onClick={() => setOpen(false)}>
          Cancelar
        </button>
        {error && <span className="auth-error">{error}</span>}
      </div>
    </form>
  );
}

function EmailForm({ email, onSaved }: { email: string; onSaved: () => Promise<void> }) {
  const [value, setValue] = useState(email);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  useEffect(() => setValue(email), [email]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    setSuccess(false);
    try {
      await api.put("/api/me", { email: value });
      setSuccess(true);
      await onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao salvar e-mail");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="admin-form">
      <label>
        E-mail
        <input type="email" value={value} onChange={(e) => setValue(e.target.value)} required />
      </label>
      <div className="admin-form-full" style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <button className="btn" type="submit" disabled={submitting}>
          Salvar
        </button>
        {success && <span style={{ color: "var(--good)", fontSize: 13 }}>E-mail atualizado.</span>}
        {error && <span className="auth-error">{error}</span>}
      </div>
    </form>
  );
}

function PasswordForm() {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    setSuccess(false);
    try {
      await api.put("/api/me/password", { current_password: currentPassword, new_password: newPassword });
      setCurrentPassword("");
      setNewPassword("");
      setSuccess(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao trocar senha");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="admin-form">
      <label>
        Senha atual
        <input type="password" value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} required />
      </label>
      <label>
        Nova senha
        <input
          type="password"
          value={newPassword}
          onChange={(e) => setNewPassword(e.target.value)}
          minLength={8}
          required
        />
      </label>
      <div className="admin-form-full" style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <button className="btn" type="submit" disabled={submitting}>
          Trocar senha
        </button>
        {success && <span style={{ color: "var(--good)", fontSize: 13 }}>Senha atualizada.</span>}
        {error && <span className="auth-error">{error}</span>}
      </div>
    </form>
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
