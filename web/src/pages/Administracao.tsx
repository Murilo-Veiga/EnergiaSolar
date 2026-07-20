import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError, type AdminUser, type SystemSettings } from "../lib/api";
import { useAuth } from "../context/AuthContext";
import { Modal } from "../components/Modal";
import { IconBadge } from "../components/icons";

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

// Configurações que não dependem de usuário nem de instalação: URL padrão das
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
    <div className="card panel-card">
      <div className="panel-card-head">
        <IconBadge name="settings" color="blue" size="card" />
        <h3>Configuração do sistema</h3>
      </div>
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
        URLs vazias usam o padrão do sistema. Credenciais de instalação com URL própria continuam usando a URL delas,
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
    <div className="card panel-card">
      <div className="panel-card-head">
        <IconBadge name="user" color="aqua" size="card" />
        <h3>Gestão de usuários</h3>
      </div>
      <UserManagement currentUserId={userId} />
    </div>
  );
}

function UserManagement({ currentUserId }: { currentUserId: string }) {
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [editingUser, setEditingUser] = useState<AdminUser | null>(null);
  const [creating, setCreating] = useState(false);
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
    if (!confirm(`Apagar o usuário "${user.email}"? Isso também remove suas instalações e credenciais.`)) return;
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
          {users.map((u) => (
            <div className="plant-list-item" key={u.id} style={{ cursor: "default" }}>
              <span>
                {u.email}
                {u.username && (
                  <span style={{ color: "var(--ink-muted)", fontSize: 12, marginLeft: 8 }}>@{u.username}</span>
                )}
                {u.is_admin && (
                  <span className="badge on" style={{ marginLeft: 8 }}>
                    admin
                  </span>
                )}
              </span>
              <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
                <span style={{ color: "var(--ink-muted)", fontSize: 12 }}>
                  {u.plants_count} instalaç{u.plants_count === 1 ? "ão" : "ões"}
                </span>
                <button className="btn btn-secondary" onClick={() => setEditingUser(u)}>
                  Editar
                </button>
                {u.id !== currentUserId && (
                  <button className="btn btn-danger" onClick={() => remove(u)}>
                    Apagar
                  </button>
                )}
              </div>
            </div>
          ))}
          {users.length === 0 && <div style={{ color: "var(--ink-muted)", fontSize: 13 }}>Nenhum usuário cadastrado.</div>}
        </div>
      )}
      <button className="btn btn-secondary" style={{ marginTop: 8 }} onClick={() => setCreating(true)}>
        + Novo usuário
      </button>

      {creating && (
        <Modal title="Novo usuário" onClose={() => setCreating(false)}>
          <NewUserForm
            onCreated={async () => {
              setCreating(false);
              await load();
            }}
            onCancel={() => setCreating(false)}
          />
        </Modal>
      )}

      {editingUser && (
        <Modal title="Editar usuário" onClose={() => setEditingUser(null)}>
          <EditUserForm
            user={editingUser}
            isSelf={editingUser.id === currentUserId}
            onSaved={async () => {
              setEditingUser(null);
              await load();
            }}
            onCancel={() => setEditingUser(null)}
          />
        </Modal>
      )}
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
  const [name, setName] = useState(user.name);
  const [email, setEmail] = useState(user.email);
  const [username, setUsername] = useState(user.username);
  const [isAdminFlag, setIsAdminFlag] = useState(user.is_admin);
  const [newPassword, setNewPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.put(`/api/admin/users/${user.id}`, { name, email, username, is_admin: isAdminFlag });
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
    <form onSubmit={handleSubmit} className="admin-form">
      <label className="admin-form-full">
        Nome
        <input value={name} onChange={(e) => setName(e.target.value)} required />
      </label>
      <label className="admin-form-full">
        E-mail
        <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
      </label>
      <label className="admin-form-full">
        Nome de usuário (opcional)
        <input value={username} onChange={(e) => setUsername(e.target.value)} />
      </label>
      <label className="admin-form-full">
        Nova senha (opcional)
        <input
          type="password"
          value={newPassword}
          onChange={(e) => setNewPassword(e.target.value)}
          minLength={8}
          placeholder="deixe em branco para manter"
        />
      </label>
      <label className="admin-form-full" style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
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

function NewUserForm({ onCreated, onCancel }: { onCreated: () => Promise<void>; onCancel: () => void }) {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [isAdminFlag, setIsAdminFlag] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.post("/api/admin/users", { name, email, username, password, is_admin: isAdminFlag });
      await onCreated();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao criar usuário");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="admin-form">
      <label className="admin-form-full">
        Nome
        <input value={name} onChange={(e) => setName(e.target.value)} required />
      </label>
      <label className="admin-form-full">
        E-mail
        <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
      </label>
      <label className="admin-form-full">
        Nome de usuário (opcional)
        <input value={username} onChange={(e) => setUsername(e.target.value)} />
      </label>
      <label className="admin-form-full">
        Senha
        <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required />
      </label>
      <label className="admin-form-full" style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
        <input type="checkbox" checked={isAdminFlag} onChange={(e) => setIsAdminFlag(e.target.checked)} />
        Administrador
      </label>
      <div className="admin-form-full" style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <button className="btn" type="submit" disabled={submitting}>
          Criar
        </button>
        <button className="btn btn-secondary" type="button" onClick={onCancel}>
          Cancelar
        </button>
        {error && <span className="auth-error">{error}</span>}
      </div>
    </form>
  );
}

