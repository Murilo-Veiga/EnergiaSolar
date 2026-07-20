package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"energiasolar-api/internal/auth"
)

// postgresUniqueViolation é o código SQLSTATE padrão pra violação de
// constraint UNIQUE (ver https://www.postgresql.org/docs/current/errcodes-appendix.html) —
// usado pra distinguir "e-mail duplicado" de qualquer outra falha de banco,
// que merece 500 em vez de 409.
const postgresUniqueViolation = "23505"

type signupRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponse struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Name     string `json:"name"`
	IsAdmin  bool   `json:"is_admin"`
}

// usernameConflictMessage distingue qual constraint UNIQUE foi violada (email
// ou username) pra devolver uma mensagem de erro específica em vez de sempre
// "e-mail já cadastrado".
func usernameConflictMessage(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != postgresUniqueViolation {
		return "", false
	}
	if pgErr.ConstraintName == "users_username_key" {
		return "nome de usuário já em uso", true
	}
	return "e-mail já cadastrado", true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, userID string) error {
	token, err := auth.IssueToken(userID, s.JWTSecret)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		// None (não Lax) porque o frontend costuma ser acessado num domínio
		// diferente do da api (ex.: túnel ngrok na frente do :8090 enquanto a
		// api segue em :8091) — Lax não é enviado em fetch/XHR cross-site,
		// só em navegação de topo. SameSite=None exige Secure=true.
		SameSite: http.SameSiteNoneMode,
		Expires:  time.Now().Add(7 * 24 * time.Hour),
	})
	return nil
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	// req.Email carrega o identificador digitado no login — pode ser o e-mail
	// ou o username cadastrado, então a busca tenta os dois.
	var userID, email, username, passwordHash, name string
	var isAdmin bool
	err := s.DB.QueryRow(r.Context(),
		`SELECT id, email, COALESCE(username, ''), password_hash, name, is_admin FROM users WHERE email = $1 OR username = $1`, req.Email,
	).Scan(&userID, &email, &username, &passwordHash, &name, &isAdmin)
	// Erro de banco de verdade (conexão etc.) precisa de 500 — checar antes
	// do caminho de credencial inválida, senão uma falha de infra vira
	// silenciosamente "senha errada" (CheckPassword só recebe um hash vazio).
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "falha ao consultar usuário")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) || !auth.CheckPassword(passwordHash, req.Password) {
		// Mesma mensagem pra e-mail/usuário inexistente e senha errada — não revela qual dos dois.
		writeError(w, http.StatusUnauthorized, "e-mail, usuário ou senha inválidos")
		return
	}

	if err := s.setSessionCookie(w, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "falha ao criar sessão")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{UserID: userID, Email: email, Username: username, Name: name, IsAdmin: isAdmin})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteNoneMode,
		Expires:  time.Unix(0, 0),
	})
	w.WriteHeader(http.StatusNoContent)
}
