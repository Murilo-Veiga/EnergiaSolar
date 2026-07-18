// Package collector implementa a coleta de dados dos inversores — porta
// de collector/main.py (Python), redesenhada como 1 goroutine por
// credencial em vez de 1 loop fixo pra 2 inversores hardcoded (ver plano
// em /home/marcos/.claude/plans/polymorphic-humming-fog.md > "Collector:
// worker por credencial").
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"energiasolar-api/internal/auth"
	"energiasolar-api/internal/brtime"
)

// Deps são as dependências compartilhadas por todos os workers.
type Deps struct {
	DB            *pgxpool.Pool
	EncryptionKey []byte
	Log           *slog.Logger
}

// CredentialRow espelha 1 linha de inverter_credentials.
type CredentialRow struct {
	ID                    string
	PlantID               string
	Brand                 string
	CredentialsEncrypted  []byte
	DiscoveredStationCode *string
	DiscoveredDevDn       *string
	DiscoveredDeviceSN    *string
}

type huaweiSecrets struct {
	Username   string `json:"username"`
	SystemCode string `json:"system_code"`
	BaseURL    string `json:"base_url"`
}

type foxessSecrets struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

func decryptJSON(encrypted []byte, key []byte, out any) error {
	plain, err := auth.DecryptCredential(encrypted, key)
	if err != nil {
		return fmt.Errorf("decifrando credencial: %w", err)
	}
	if err := json.Unmarshal([]byte(plain), out); err != nil {
		return fmt.Errorf("decodificando credencial: %w", err)
	}
	return nil
}

// FetchEnabledCredentials lista todas as credenciais habilitadas de todos
// os usuários — o supervisor usa isso pra saber quais workers devem estar
// rodando.
func FetchEnabledCredentials(ctx context.Context, db *pgxpool.Pool) ([]CredentialRow, error) {
	rows, err := db.Query(ctx,
		`SELECT id, plant_id, brand, credentials_encrypted,
		        discovered_station_code, discovered_dev_dn, discovered_device_sn
		 FROM inverter_credentials WHERE enabled = true`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []CredentialRow
	for rows.Next() {
		var c CredentialRow
		if err := rows.Scan(&c.ID, &c.PlantID, &c.Brand, &c.CredentialsEncrypted,
			&c.DiscoveredStationCode, &c.DiscoveredDevDn, &c.DiscoveredDeviceSN); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// resetGuardState detecta "dia novo, contador do fabricante ainda não
// resetou" — mesma lógica de _apply_daily_reset_guard em
// collector/main.py (Python), agora por goroutine em vez de por chave de
// mapa global (cada worker só cuida do próprio inversor).
type resetGuardState struct {
	year, month, day int
	started          bool
}

// apply devolve dayKWh se o inversor já gerou hoje (power>0 alguma vez
// hoje), senão 0 — evita mostrar o total de ontem como se fosse de hoje
// entre a meia-noite e o inversor acordar.
//
// Compara ano/mês/dia diretamente em vez de usar time.Truncate: Truncate
// arredonda por tempo absoluto desde o zero-time (sempre alinhado a
// fronteiras UTC), não pelo calendário local — em BRT (UTC-3) isso faria
// o "dia novo" virar às 21h locais em vez da meia-noite.
func (s *resetGuardState) apply(now time.Time, powerKW, dayKWh float64) float64 {
	y, m, d := now.In(brtime.Location).Date()
	if s.year != y || s.month != int(m) || s.day != d {
		s.year, s.month, s.day = y, int(m), d
		s.started = false
	}
	if powerKW > 0 {
		s.started = true
	}
	if s.started {
		return dayKWh
	}
	return 0
}
