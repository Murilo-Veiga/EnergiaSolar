package auth

import (
	"context"
	"net/http"
)

const CookieName = "session"

type contextKey string

const userIDContextKey contextKey = "user_id"

// Middleware valida o cookie de sessão (JWT) e injeta o user_id no
// contexto da requisição. Protege todo /api/* exceto /api/auth/*.
func Middleware(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CookieName)
			if err != nil {
				http.Error(w, `{"error":"não autenticado"}`, http.StatusUnauthorized)
				return
			}
			claims, err := ParseToken(cookie.Value, secret)
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
