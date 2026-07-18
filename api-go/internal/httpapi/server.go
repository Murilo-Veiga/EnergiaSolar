// Package httpapi implementa a API JSON (chi router) consumida pelo
// frontend React — auth, cadastro de usinas/credenciais e (a partir da
// Fase 3 do plano) os endpoints de leitura hoje só em webapp/main.py
// (Python).
package httpapi

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server carrega as dependências compartilhadas por todos os handlers.
type Server struct {
	DB            *pgxpool.Pool
	JWTSecret     []byte
	EncryptionKey []byte
	// AllowedOrigin é a origem do frontend React (dev: localhost:5173,
	// produção: o serviço web-react) — precisa ser explícita (não "*")
	// porque a sessão usa cookie com credentials:"include".
	AllowedOrigin string
}
