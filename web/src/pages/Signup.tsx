import { useState, type FormEvent } from "react";
import { useAuth } from "../context/AuthContext";
import { ApiError } from "../lib/api";

export function Signup({ onSwitchToLogin }: { onSwitchToLogin: () => void }) {
  const { signup } = useAuth();
  const [email, setEmail] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (password.length < 8) {
      setError("A senha precisa ter pelo menos 8 caracteres");
      return;
    }
    setSubmitting(true);
    try {
      await signup(email, password, username);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao criar conta");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="auth-screen">
      <form className="card auth-card" onSubmit={handleSubmit}>
        <h1>Criar conta</h1>
        <label>
          E-mail
          <input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </label>
        <label>
          Nome de usuário (opcional)
          <input type="text" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="pra entrar sem digitar o e-mail" />
        </label>
        <label>
          Senha (mínimo 8 caracteres)
          <input type="password" required minLength={8} value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
        {error && <div className="auth-error">{error}</div>}
        <button type="submit" disabled={submitting}>
          {submitting ? "Criando..." : "Criar conta"}
        </button>
        <button type="button" className="auth-link" onClick={onSwitchToLogin}>
          Já tem conta? Entrar
        </button>
      </form>
    </div>
  );
}
