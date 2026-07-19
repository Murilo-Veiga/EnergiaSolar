package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"energiasolar-api/internal/auth"
)

type updateEmailRequest struct {
	Email string `json:"email"`
}

type updatePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handleGetMe devolve os dados da conta logada — nunca inclui password_hash.
func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	var email string
	var isAdmin bool
	err := s.DB.QueryRow(r.Context(), `SELECT email, is_admin FROM users WHERE id = $1`, userID).Scan(&email, &isAdmin)
	if isNoRows(err) {
		writeError(w, http.StatusNotFound, "usuário não encontrado")
		return
	}
	if err != nil {
		writeInternalError(w, err, "falha ao consultar usuário")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{UserID: userID, Email: email, IsAdmin: isAdmin})
}

// handleUpdateEmail troca o e-mail da conta logada.
func (s *Server) handleUpdateEmail(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	var req updateEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		writeError(w, http.StatusBadRequest, "e-mail válido é obrigatório")
		return
	}

	_, err := s.DB.Exec(r.Context(), `UPDATE users SET email = $1 WHERE id = $2`, req.Email, userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			writeError(w, http.StatusConflict, "e-mail já cadastrado")
			return
		}
		writeInternalError(w, err, "falha ao atualizar e-mail")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{UserID: userID, Email: req.Email})
}

// handleUpdatePassword troca a senha da conta logada — exige a senha atual.
func (s *Server) handleUpdatePassword(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	var req updatePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "nova senha precisa ter pelo menos 8 caracteres")
		return
	}

	var currentHash string
	err := s.DB.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&currentHash)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "usuário não encontrado")
		return
	}
	if err != nil {
		writeInternalError(w, err, "falha ao consultar usuário")
		return
	}
	if !auth.CheckPassword(currentHash, req.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "senha atual incorreta")
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeInternalError(w, err, "falha ao processar senha")
		return
	}
	if _, err := s.DB.Exec(r.Context(), `UPDATE users SET password_hash = $1 WHERE id = $2`, newHash, userID); err != nil {
		writeInternalError(w, err, "falha ao atualizar senha")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
