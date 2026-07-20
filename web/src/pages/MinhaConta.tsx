import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError, type Me } from "../lib/api";
import { IconBadge } from "../components/icons";

// Tela isolada de Administração — só ajustes da própria conta (e-mail e
// senha). Gestão de outros usuários fica em Administração > Gestão de
// usuários, restrita a admins.
export function MinhaConta() {
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
      <div className="card profile-hero">
        <IconBadge name="user" color="blue" size="lg" />
        <div>
          <div className="profile-hero-name">{me.name || me.email}</div>
          <div className="profile-hero-sub">
            {me.email}
            {me.username && ` · @${me.username}`}
          </div>
        </div>
      </div>

      <div className="card panel-card">
        <div className="panel-card-head">
          <IconBadge name="settings" color="blue" size="card" />
          <h3>Dados da conta</h3>
        </div>
        <ProfileForm name={me.name} email={me.email} username={me.username} onSaved={load} />
      </div>

      <div className="card panel-card">
        <div className="panel-card-head">
          <IconBadge name="shield" color="gold" size="card" />
          <h3>Senha</h3>
        </div>
        <PasswordForm />
      </div>
    </div>
  );
}

function ProfileForm({
  name,
  email,
  username,
  onSaved,
}: {
  name: string;
  email: string;
  username: string;
  onSaved: () => Promise<void>;
}) {
  const [nameValue, setNameValue] = useState(name);
  const [emailValue, setEmailValue] = useState(email);
  const [usernameValue, setUsernameValue] = useState(username);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  useEffect(() => {
    setNameValue(name);
    setEmailValue(email);
    setUsernameValue(username);
  }, [name, email, username]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    setSuccess(false);
    try {
      await api.put("/api/me", { email: emailValue, name: nameValue, username: usernameValue });
      setSuccess(true);
      await onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao salvar perfil");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="admin-form">
      <label>
        Nome
        <input value={nameValue} onChange={(e) => setNameValue(e.target.value)} required />
      </label>
      <label>
        E-mail
        <input type="email" value={emailValue} onChange={(e) => setEmailValue(e.target.value)} required />
      </label>
      <label>
        Nome de usuário (opcional)
        <input
          value={usernameValue}
          onChange={(e) => setUsernameValue(e.target.value)}
          placeholder="pra entrar sem digitar o e-mail"
        />
      </label>
      <div className="admin-form-full" style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <button className="btn" type="submit" disabled={submitting}>
          Salvar
        </button>
        {success && <span style={{ color: "var(--good)", fontSize: 13 }}>Perfil atualizado.</span>}
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
