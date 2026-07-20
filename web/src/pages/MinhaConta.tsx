import { useEffect, useState, type FormEvent } from "react";
import { api, ApiError, type Me } from "../lib/api";

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
      <div className="admin-section">
        <h3>Conta</h3>
        <ProfileForm name={me.name} email={me.email} onSaved={load} />
      </div>
      <div className="admin-section">
        <h3>Senha</h3>
        <PasswordForm />
      </div>
    </div>
  );
}

function ProfileForm({ name, email, onSaved }: { name: string; email: string; onSaved: () => Promise<void> }) {
  const [nameValue, setNameValue] = useState(name);
  const [emailValue, setEmailValue] = useState(email);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  useEffect(() => {
    setNameValue(name);
    setEmailValue(email);
  }, [name, email]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    setSuccess(false);
    try {
      await api.put("/api/me", { email: emailValue, name: nameValue });
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
