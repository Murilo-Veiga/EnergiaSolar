// Comando de migração única: cria o primeiro usuário + a primeira usina a
// partir dos valores hoje no .env (PLANT_LAT/LON/INSTALLED_POWER_KWP,
// credenciais Huawei/FoxESS) — ver plano em
// /home/marcos/.claude/plans/polymorphic-humming-fog.md > "Migração do
// dado atual". Roda 1x, é seguro rodar de novo (idempotente por e-mail).
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"

	"energiasolar-api/internal/auth"
	"energiasolar-api/internal/db"
)

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("variável de ambiente obrigatória não definida: %s", key)
	}
	return v
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

// huaweiCredential é serializado como JSON antes de ser cifrado — mesma
// estrutura que o collector-go vai decifrar e usar na Fase 4.
type huaweiCredential struct {
	Username   string `json:"username"`
	SystemCode string `json:"system_code"`
	BaseURL    string `json:"base_url"`
}

func main() {
	ctx := context.Background()
	databaseURL := mustEnv("DATABASE_URL")
	encryptionKey := []byte(mustEnv("CONFIG_ENCRYPTION_KEY"))
	if len(encryptionKey) != 32 {
		log.Fatalf("CONFIG_ENCRYPTION_KEY precisa ter exatamente 32 bytes (tem %d)", len(encryptionKey))
	}

	seedEmail := mustEnv("SEED_USER_EMAIL")
	seedPassword := mustEnv("SEED_USER_PASSWORD")

	if err := db.Migrate(databaseURL); err != nil {
		log.Fatalf("falha ao aplicar migrations: %v", err)
	}

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("falha ao conectar no postgres: %v", err)
	}
	defer pool.Close()

	// Idempotente: se o usuário já existe (rodou o seed antes), reaproveita.
	var userID string
	err = pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, seedEmail).Scan(&userID)
	if err != nil {
		hash, err := auth.HashPassword(seedPassword)
		if err != nil {
			log.Fatalf("falha ao gerar hash de senha: %v", err)
		}
		err = pool.QueryRow(ctx,
			`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`,
			seedEmail, hash,
		).Scan(&userID)
		if err != nil {
			log.Fatalf("falha ao criar usuário seed: %v", err)
		}
		log.Printf("usuário criado: %s (%s)", seedEmail, userID)
	} else {
		log.Printf("usuário já existia, reaproveitando: %s (%s)", seedEmail, userID)
	}

	lat := envFloat("PLANT_LAT", 0)
	lon := envFloat("PLANT_LON", 0)
	installedKWp := envFloat("PLANT_INSTALLED_POWER_KWP", 0)

	var plantID string
	err = pool.QueryRow(ctx, `SELECT id FROM plants WHERE user_id = $1 LIMIT 1`, userID).Scan(&plantID)
	if err != nil {
		err = pool.QueryRow(ctx,
			`INSERT INTO plants (user_id, name, lat, lon, installed_power_kwp)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			userID, "Usina principal", lat, lon, installedKWp,
		).Scan(&plantID)
		if err != nil {
			log.Fatalf("falha ao criar usina seed: %v", err)
		}
		log.Printf("usina criada: %s (installed_power_kwp=%.2f)", plantID, installedKWp)
	} else {
		log.Printf("usina já existia, reaproveitando: %s", plantID)
	}

	if envBool("HUAWEI_ENABLED", true) {
		cred := huaweiCredential{
			Username:   mustEnv("HUAWEI_USERNAME"),
			SystemCode: mustEnv("HUAWEI_SYSTEM_CODE"),
			BaseURL:    os.Getenv("HUAWEI_BASE_URL"),
		}
		plain, err := json.Marshal(cred)
		if err != nil {
			log.Fatalf("falha ao serializar credencial Huawei: %v", err)
		}
		encrypted, err := auth.EncryptCredential(string(plain), encryptionKey)
		if err != nil {
			log.Fatalf("falha ao cifrar credencial Huawei: %v", err)
		}
		_, err = pool.Exec(ctx,
			`INSERT INTO inverter_credentials (plant_id, brand, enabled, credentials_encrypted)
			 VALUES ($1, 'huawei', true, $2)
			 ON CONFLICT (plant_id, brand) DO UPDATE SET credentials_encrypted = EXCLUDED.credentials_encrypted`,
			plantID, encrypted,
		)
		if err != nil {
			log.Fatalf("falha ao gravar credencial Huawei: %v", err)
		}
		log.Println("credencial Huawei migrada e cifrada com sucesso")
	} else {
		log.Println("HUAWEI_ENABLED=false — pulando credencial Huawei")
	}

	log.Println("seed concluído.")
}
