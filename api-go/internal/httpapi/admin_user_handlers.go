package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"energiasolar-api/internal/auth"
)

type adminUser struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	IsAdmin     bool      `json:"is_admin"`
	CreatedAt   time.Time `json:"created_at"`
	PlantsCount int       `json:"plants_count"`
}

type adminCreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
}

type adminUpdateUserRequest struct {
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
}

type adminResetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// handleAdminListUsers lista todos os usuários do sistema — só admins acessam.
func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(), `
		SELECT u.id, u.email, u.is_admin, u.created_at, count(p.id)
		FROM users u
		LEFT JOIN plants p ON p.user_id = u.id
		GROUP BY u.id
		ORDER BY u.created_at
	`)
	if err != nil {
		writeInternalError(w, err, "falha ao listar usuários")
		return
	}
	defer rows.Close()

	users := []adminUser{}
	for rows.Next() {
		var u adminUser
		if err := rows.Scan(&u.ID, &u.Email, &u.IsAdmin, &u.CreatedAt, &u.PlantsCount); err != nil {
			writeInternalError(w, err, "falha ao listar usuários")
			return
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		writeInternalError(w, err, "falha ao listar usuários")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

// handleAdminCreateUser cria um novo usuário — só admins acessam.
func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var req adminCreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "email válido e senha com pelo menos 8 caracteres são obrigatórios")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeInternalError(w, err, "falha ao processar senha")
		return
	}

	var u adminUser
	err = s.DB.QueryRow(r.Context(),
		`INSERT INTO users (email, password_hash, is_admin) VALUES ($1, $2, $3)
		 RETURNING id, email, is_admin, created_at`,
		req.Email, hash, req.IsAdmin,
	).Scan(&u.ID, &u.Email, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			writeError(w, http.StatusConflict, "e-mail já cadastrado")
			return
		}
		writeInternalError(w, err, "falha ao criar usuário")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

// handleAdminGetUser devolve um usuário específico — só admins acessam.
func (s *Server) handleAdminGetUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")

	var u adminUser
	err := s.DB.QueryRow(r.Context(), `
		SELECT u.id, u.email, u.is_admin, u.created_at, count(p.id)
		FROM users u
		LEFT JOIN plants p ON p.user_id = u.id
		WHERE u.id = $1
		GROUP BY u.id
	`, userID).Scan(&u.ID, &u.Email, &u.IsAdmin, &u.CreatedAt, &u.PlantsCount)
	if isNoRows(err) {
		writeError(w, http.StatusNotFound, "usuário não encontrado")
		return
	}
	if err != nil {
		writeInternalError(w, err, "falha ao consultar usuário")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// handleAdminUpdateUser atualiza e-mail e status de admin de um usuário —
// só admins acessam.
func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")

	var req adminUpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		writeError(w, http.StatusBadRequest, "e-mail válido é obrigatório")
		return
	}

	// Impede o próprio admin de tirar seu privilégio via este endpoint —
	// evita ficar sem nenhum admin no sistema por engano.
	requesterID, _ := auth.UserIDFromContext(r.Context())
	if requesterID == userID && !req.IsAdmin {
		writeError(w, http.StatusConflict, "não é possível remover seu próprio privilégio de administrador")
		return
	}

	var u adminUser
	err := s.DB.QueryRow(r.Context(),
		`UPDATE users SET email = $1, is_admin = $2 WHERE id = $3
		 RETURNING id, email, is_admin, created_at`,
		req.Email, req.IsAdmin, userID,
	).Scan(&u.ID, &u.Email, &u.IsAdmin, &u.CreatedAt)
	if isNoRows(err) {
		writeError(w, http.StatusNotFound, "usuário não encontrado")
		return
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			writeError(w, http.StatusConflict, "e-mail já cadastrado")
			return
		}
		writeInternalError(w, err, "falha ao atualizar usuário")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// handleAdminResetPassword define uma nova senha pra um usuário sem exigir
// a senha atual (o admin está redefinindo em nome de outra pessoa) — só
// admins acessam.
func (s *Server) handleAdminResetPassword(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")

	var req adminResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "nova senha precisa ter pelo menos 8 caracteres")
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeInternalError(w, err, "falha ao processar senha")
		return
	}

	tag, err := s.DB.Exec(r.Context(), `UPDATE users SET password_hash = $1 WHERE id = $2`, hash, userID)
	if err != nil {
		writeInternalError(w, err, "falha ao atualizar senha")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "usuário não encontrado")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAdminDeleteUser remove um usuário (e suas plants em cascata) — só
// admins acessam. Um admin não pode apagar a própria conta por aqui.
func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")

	requesterID, _ := auth.UserIDFromContext(r.Context())
	if requesterID == userID {
		writeError(w, http.StatusConflict, "não é possível apagar a própria conta por aqui")
		return
	}

	tag, err := s.DB.Exec(r.Context(), `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		writeInternalError(w, err, "falha ao apagar usuário")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "usuário não encontrado")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
