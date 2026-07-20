package httpapi

import (
	"encoding/json"
	"net/http"
)

type updateSystemSettingsRequest struct {
	HuaweiBaseURL         string `json:"huawei_base_url"`
	FoxessBaseURL         string `json:"foxess_base_url"`
	WorkerIntervalMinutes int    `json:"worker_interval_minutes"`
}

// handleGetSystemSettings devolve a configuração global do sistema — só
// admins acessam.
func (s *Server) handleGetSystemSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.loadSystemSettings(r.Context())
	if err != nil {
		writeInternalError(w, err, "falha ao consultar configurações do sistema")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// handleUpdateSystemSettings atualiza a configuração global do sistema —
// URLs padrão das integrações Huawei/FoxESS (usadas quando uma credencial
// não tem URL própria) e o intervalo do worker de coleta. Só admins
// acessam. collector-go relê essa tabela a cada reconciliação (no máximo
// alguns minutos de atraso pra pegar a mudança — ver internal/collector/supervisor.go).
func (s *Server) handleUpdateSystemSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSystemSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}
	if req.WorkerIntervalMinutes < 1 || req.WorkerIntervalMinutes > 1440 {
		writeError(w, http.StatusBadRequest, "intervalo do worker precisa estar entre 1 e 1440 minutos")
		return
	}

	_, err := s.DB.Exec(r.Context(),
		`UPDATE system_settings
		 SET huawei_base_url = $1, foxess_base_url = $2, worker_interval_minutes = $3, updated_at = now()
		 WHERE id = true`,
		req.HuaweiBaseURL, req.FoxessBaseURL, req.WorkerIntervalMinutes,
	)
	if err != nil {
		writeInternalError(w, err, "falha ao atualizar configurações do sistema")
		return
	}
	writeJSON(w, http.StatusOK, systemSettings{
		HuaweiBaseURL:         req.HuaweiBaseURL,
		FoxessBaseURL:         req.FoxessBaseURL,
		WorkerIntervalMinutes: req.WorkerIntervalMinutes,
	})
}
