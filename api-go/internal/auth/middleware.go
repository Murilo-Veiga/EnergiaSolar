package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const userIDContextKey contextKey = "user_id"

// Middleware valida o JWT enviado no header "Authorization: Bearer <token>"
// e injeta o user_id no contexto da requisição. Protege todo /api/* exceto
// /api/auth/*.
func Middleware(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			tokenString, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || tokenString == "" {
				http.Error(w, `{"error":"não autenticado"}`, http.StatusUnauthorized)
				return
			}
			claims, err := ParseToken(tokenString, secret)
			if err != nil {
				http.Error(w, `{"error":"sessão inválida ou expirada"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), userIDContextKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext lê o user_id injetado pelo Middleware. Só é chamado
// depois do Middleware, então o segundo valor sempre é true nas rotas
// protegidas — mas devolve o ok pra quem quiser checar defensivamente.
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDContextKey).(string)
	return id, ok
}
