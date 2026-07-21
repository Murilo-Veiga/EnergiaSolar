package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type plantAccessUser struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	GrantedAt time.Time `json:"granted_at"`
}

// handleListPlantAccess lista as contas com acesso de visualização à
// usina — só o dono acessa (authorizePlant, não a variante de leitura).
func (s *Server) handleListPlantAccess(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}

	rows, err := s.DB.Query(r.Context(), `
		SELECT u.id, u.name, u.email, COALESCE(u.username, ''), a.created_at
		  FROM plant_access a
		  JOIN users u ON u.id = a.user_id
		 WHERE a.plant_id = $1
		 ORDER BY a.created_at
	`, plantID)
	if err != nil {
		writeInternalError(w, err, "falha ao listar acessos")
		return
	}
	defer rows.Close()

	users := []plantAccessUser{}
	for rows.Next() {
		var u plantAccessUser
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Username, &u.GrantedAt); err != nil {
			writeInternalError(w, err, "falha ao listar acessos")
			return
		}
		users = append(users, u)
	}
	writeJSON(w, http.StatusOK, users)
}

type grantPlantAccessRequest struct {
	Identifier string `json:"identifier"` // e-mail ou username da conta a compartilhar
}

// handleGrantPlantAccess dá acesso de visualização à usina pra uma conta
// já existente (identificada por e-mail ou username, igual ao login) — só
// o dono acessa. Não cria conta nova: quem recebe o acesso já precisa
// existir (criada por um admin em Administração > Gestão de usuários).
func (s *Server) handleGrantPlantAccess(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	owner, err := s.authorizePlant(r.Context(), plantID)
	if err != nil {
		respondPlantAuthError(w, err)
		return
	}

	var req grantPlantAccessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}
	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" {
		writeError(w, http.StatusBadRequest, "informe o e-mail ou nome de usuário da conta")
		return
	}

	var targetUserID string
	err = s.DB.QueryRow(r.Context(),
		`SELECT id FROM users WHERE email = $1 OR username = $1`, identifier,
	).Scan(&targetUserID)
	if isNoRows(err) {
		writeError(w, http.StatusNotFound, "nenhuma conta encontrada com esse e-mail ou usuário")
		return
	}
	if err != nil {
		writeInternalError(w, err, "falha ao consultar usuário")
		return
	}
	if targetUserID == owner.UserID {
		writeError(w, http.StatusConflict, "você já é o dono desta usina")
		return
	}

	_, err = s.DB.Exec(r.Context(),
		`INSERT INTO plant_access (plant_id, user_id) VALUES ($1, $2)
		 ON CONFLICT (plant_id, user_id) DO NOTHING`,
		plantID, targetUserID,
	)
	if err != nil {
		writeInternalError(w, err, "falha ao conceder acesso")
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// handleRevokePlantAccess remove o acesso de visualização de uma conta —
// só o dono acessa.
func (s *Server) handleRevokePlantAccess(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}

	userID := chi.URLParam(r, "userID")
	if _, err := s.DB.Exec(r.Context(),
		`DELETE FROM plant_access WHERE plant_id = $1 AND user_id = $2`, plantID, userID,
	); err != nil {
		writeInternalError(w, err, "falha ao remover acesso")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
