package collector

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultWorkerInterval é usado se a config global ainda não carregou (ex.:
// falha passageira no primeiro reconcile) ou vier com valor inválido.
const defaultWorkerInterval = 30 * time.Minute

// workerInterval converte o intervalo configurado (minutos) em Duration —
// mesmo valor pra huawei e foxess, já que hoje não há motivo pra ter
// cadências diferentes por marca.
func workerInterval(settings SystemSettings) time.Duration {
	if settings.WorkerIntervalMinutes <= 0 {
		return defaultWorkerInterval
	}
	return time.Duration(settings.WorkerIntervalMinutes) * time.Minute
}

// SystemSettings espelha a linha singleton de system_settings (ver
// migration 000004) — config global aplicada como fallback quando uma
// credencial não tem URL própria, e como intervalo do ticker de todo
// worker (huawei e foxess).
type SystemSettings struct {
	HuaweiBaseURL         string
	FoxessBaseURL         string
	WorkerIntervalMinutes int
}

func loadSystemSettings(ctx context.Context, db *pgxpool.Pool) (SystemSettings, error) {
	var s SystemSettings
	err := db.QueryRow(ctx,
		`SELECT huawei_base_url, foxess_base_url, worker_interval_minutes FROM system_settings WHERE id = true`,
	).Scan(&s.HuaweiBaseURL, &s.FoxessBaseURL, &s.WorkerIntervalMinutes)
	return s, err
}
