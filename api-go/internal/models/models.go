// Package models contém as structs que espelham as tabelas de config do
// Postgres (users, plants, inverter_credentials, consumer_units) — ver
// migrations em api-go/migrations.
package models

import "time"

// User é uma conta com login próprio no painel.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

// Plant é uma usina cadastrada por um User — equivalente ao PLANT_TAG fixo
// ("casa") que existia no coletor/webapp Python, agora um registro por
// usina em vez de uma constante única.
type Plant struct {
	ID                string
	UserID            string
	Name              string
	Lat               *float64
	Lon               *float64
	InstalledPowerKWp float64
	Timezone          string
	CreatedAt         time.Time
}

// InverterCredential é a credencial (cifrada) de 1 inversor (Huawei ou
// FoxESS) de uma Plant — no máximo 1 linha por (plant_id, brand).
type InverterCredential struct {
	ID                    string
	PlantID               string
	Brand                 string // "huawei" | "foxess"
	Enabled               bool
	CredentialsEncrypted  []byte
	DiscoveredStationCode *string
	DiscoveredDevDn       *string
	DiscoveredDeviceSN    *string
	LastSuccessAt         *time.Time
	CreatedAt             time.Time
}

// ConsumerUnit é uma unidade consumidora da Celesc associada a uma Plant —
// substitui o UC_LABELS hardcoded que existia em webapp/main.py (Python).
type ConsumerUnit struct {
	ID        string
	PlantID   string
	UCNumber  string
	Label     string
	CreatedAt time.Time
}
