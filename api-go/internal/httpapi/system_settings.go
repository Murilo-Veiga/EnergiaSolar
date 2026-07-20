package httpapi

import "context"

// systemSettings espelha a linha singleton de system_settings — config
// global que não depende de usuário nem de usina (ver migration 000004).
type systemSettings struct {
	HuaweiBaseURL         string `json:"huawei_base_url"`
	FoxessBaseURL         string `json:"foxess_base_url"`
	WorkerIntervalMinutes int    `json:"worker_interval_minutes"`
}

func (s *Server) loadSystemSettings(ctx context.Context) (systemSettings, error) {
	var settings systemSettings
	err := s.DB.QueryRow(ctx,
		`SELECT huawei_base_url, foxess_base_url, worker_interval_minutes FROM system_settings WHERE id = true`,
	).Scan(&settings.HuaweiBaseURL, &settings.FoxessBaseURL, &settings.WorkerIntervalMinutes)
	return settings, err
}
