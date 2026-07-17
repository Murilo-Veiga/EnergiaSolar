package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const sessionDuration = 7 * 24 * time.Hour

// Claims é o payload do JWT de sessão — só carrega o UserID, sem dado
// sensível (nunca inclui e-mail/senha).
type Claims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// IssueToken assina um novo JWT de sessão pra um usuário, válido por
// sessionDuration a partir de agora.
func IssueToken(userID string, secret []byte) (string, error) {
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(sessionDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// ParseToken valida a assinatura e a expiração de um JWT de sessão e
// devolve as claims decodificadas.
func ParseToken(tokenString string, secret []byte) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("método de assinatura inesperado")
		}
		return secret, nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("token inválido")
	}
	return claims, nil
}
