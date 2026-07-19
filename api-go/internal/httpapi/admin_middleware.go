package httpapi

import (
	"net/http"

	"energiasolar-api/internal/auth"
)

// RequireAdmin bloqueia rotas /api/admin/* pra quem não tem is_admin=true —
// roda depois de auth.Middleware, então userID já está no contexto.
func (s *Server) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := auth.UserIDFromContext(r.Context())

		var isAdmin bool
		err := s.DB.QueryRow(r.Context(), `SELECT is_admin FROM users WHERE id = $1`, userID).Scan(&isAdmin)
		if isNoRows(err) || !isAdmin {
			writeError(w, http.StatusForbidden, "acesso restrito a administradores")
			return
		}
		if err != nil {
			writeInternalError(w, err, "falha ao verificar permissão")
			return
		}
		next.ServeHTTP(w, r)
	})
}
