import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError, type AdminUser, type SystemSettings } from "../lib/api";
import { useAuth } from "../context/AuthContext";

type AdminTab = "sistema" | "usuarios";

export function Administracao() {
  const [tab, setTab] = useState<AdminTab>("sistema");

  const tabs: { key: AdminTab; label: string }[] = [
    { key: "sistema", label: "Configuração do sistema" },
    { key: "usuarios", label: "Gestão de usuários" },
  ];

  return (
    <div>
      <div className="chart-head" style={{ marginBottom: 16 }}>
        <div className="range-toggle">
          {tabs.map((t) => (
            <span key={t.key} className={tab === t.key ? "active" : ""} onClick={() => setTab(t.key)}>
              {t.label}
            </span>
          ))}
        </div>
      </div>

      {tab === "sistema" && <SistemaGlobalTab />}
      {tab === "usuarios" && <GestaoUsuariosTab />}
    </div>
  );
}

// Configurações que não dependem de usuário nem de usina: URL padrão das
// integrações Huawei/FoxESS (usada quando uma credencial não define a
// própria) e o intervalo do worker de coleta — ver
// api-go/internal/collector/supervisor.go, que relê essa config a cada
// reconciliação (no máx. alguns minutos de atraso pra aplicar).
function SistemaGlobalTab() {
  const [settings, setSettings] = useState<SystemSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    try {
      const data = await api.get<SystemSettings>("/api/admin/system-settings");
      setSettings(data);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao carregar configurações");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <div className="admin-section">
      <h3>Configuração do sistema</h3>
      {error && <div className="auth-error" style={{ marginBottom: 8 }}>{error}</div>}
      {loading ? (
        <div style={{ color: "var(--ink-muted)", fontSize: 13 }}>Carregando...</div>
      ) : settings ? (
        <SystemSettingsForm settings={settings} onSaved={load} />
      ) : null}
    </div>
  );
}

function SystemSettingsForm({ settings, onSaved }: { settings: SystemSettings; onSaved: () => Promise<void> }) {
  const [huaweiBaseUrl, setHuaweiBaseUrl] = useState(settings.huawei_base_url);
  const [foxessBaseUrl, setFoxessBaseUrl] = useState(settings.foxess_base_url);
  const [workerInterval, setWorkerInterval] = useState(settings.worker_interval_minutes.toString());
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  useEffect(() => {
    setHuaweiBaseUrl(settings.huawei_base_url);
    setFoxessBaseUrl(settings.foxess_base_url);
    setWorkerInterval(settings.worker_interval_minutes.toString());
  }, [settings]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    setSuccess(false);
    try {
      await api.put("/api/admin/system-settings", {
        huawei_base_url: huaweiBaseUrl,
        foxess_base_url: foxessBaseUrl,
        worker_interval_minutes: Number(workerInterval) || 30,
      });
      setSuccess(true);
      await onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao salvar configurações");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="admin-form">
      <label>
        URL base — Huawei FusionSolar
        <input
          value={huaweiBaseUrl}
          onChange={(e) => setHuaweiBaseUrl(e.target.value)}
          placeholder="https://la5.fusionsolar.huawei.com"
        />
      </label>
      <label>
        URL base — FoxESS Cloud
        <input
          value={foxessBaseUrl}
          onChange={(e) => setFoxessBaseUrl(e.target.value)}
          placeholder="https://www.foxesscloud.com"
        />
      </label>
      <label>
        Intervalo do worker de coleta (minutos)
        <input
          type="number"
          min={1}
          max={1440}
          value={workerInterval}
          onChange={(e) => setWorkerInterval(e.target.value)}
          required
        />
      </label>
      <div className="admin-form-full" style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <button className="btn" type="submit" disabled={submitting}>
          Salvar
        </button>
        {success && <span style={{ color: "var(--good)", fontSize: 13 }}>Configurações atualizadas.</span>}
        {error && <span className="auth-error">{error}</span>}
      </div>
      <div className="admin-form-full" style={{ color: "var(--ink-muted)", fontSize: 12 }}>
        URLs vazias usam o padrão do sistema. Credenciais de usina com URL própria continuam usando a URL delas,
        não a global. O worker pode levar até alguns minutos pra aplicar uma mudança.
      </div>
    </form>
  );
}

function GestaoUsuariosTab() {
  const { userId } = useAuth();

  if (!userId) {
    return <div style={{ color: "var(--ink-muted)", fontSize: 13 }}>Carregando...</div>;
  }

  return (
    <div className="admin-section">
      <h3>Gestão de usuários</h3>
      <UserManagement currentUserId={userId} />
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

