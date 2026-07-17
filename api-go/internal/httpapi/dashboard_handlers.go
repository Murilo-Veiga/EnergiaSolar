package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type summaryResponse struct {
	InstantaneousPowerKW *float64 `json:"instantaneous_power_kw"`
	InstalledPowerKWp    *float64 `json:"installed_power_kwp"`
	TodayGeneratedKWh    *float64 `json:"today_generated_kwh"`
	TodayEconomiaBRL     *float64 `json:"today_economia_brl"`
	TodayVsYesterdayPct  *int     `json:"today_vs_yesterday_pct"`
	PeakPowerKW          *float64 `json:"peak_power_kw"`
	PeakPowerAt          *string  `json:"peak_power_at"`
	Status               string   `json:"status"`
	UpdatedAt            string   `json:"updated_at"`
}

// yesterdayGeneratedKWh é o penúltimo ponto de daily_generation (o último é
// hoje) — mesma lógica de _yesterday_generated_kwh() em webapp/main.py.
func (s *Server) yesterdayGeneratedKWh(ctx context.Context, plantID string) (*float64, error) {
	rows, err := s.DB.Query(ctx,
		`SELECT generated_kwh FROM daily_generation
		 WHERE plant_id = $1 AND day >= current_date - interval '4 days'
		 ORDER BY day DESC LIMIT 2`,
		plantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(values) < 2 {
		return nil, nil
	}
	return &values[1], nil
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}
	ctx := r.Context()

	instantaneous, err := s.scanOptionalFloat(ctx,
		`SELECT instantaneous_power_kw FROM plant_status
		 WHERE plant_id = $1 AND recorded_at >= now() - interval '6 hours'
		 ORDER BY recorded_at DESC LIMIT 1`, plantID)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar potência instantânea")
		return
	}

	installed, err := s.scanOptionalFloat(ctx,
		`SELECT installed_power_kwp FROM plant_status
		 WHERE plant_id = $1 AND recorded_at >= now() - interval '30 days'
		 ORDER BY recorded_at DESC LIMIT 1`, plantID)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar potência instalada")
		return
	}

	todayGenerated, err := s.scanOptionalFloat(ctx,
		`SELECT generated_kwh FROM daily_generation
		 WHERE plant_id = $1 AND day >= current_date - interval '3 days'
		 ORDER BY day DESC LIMIT 1`, plantID)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar geração de hoje")
		return
	}

	hasAlarm, err := s.scanOptionalBool(ctx,
		`SELECT has_alarm FROM plant_status
		 WHERE plant_id = $1 AND recorded_at >= now() - interval '1 hour'
		 ORDER BY recorded_at DESC LIMIT 1`, plantID)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar alarme")
		return
	}

	yesterdayGenerated, err := s.yesterdayGeneratedKWh(ctx, plantID)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar geração de ontem")
		return
	}

	var todayVsYesterdayPct *int
	if yesterdayGenerated != nil && *yesterdayGenerated != 0 && todayGenerated != nil {
		pct := int(roundHalfAwayFromZero((*todayGenerated - *yesterdayGenerated) / *yesterdayGenerated * 100))
		todayVsYesterdayPct = &pct
	}

	isOnline := instantaneous != nil && (*instantaneous > 0 || (todayGenerated != nil && *todayGenerated > 0))
	status := "pendente"
	if hasAlarm != nil && *hasAlarm {
		status = "alerta"
	} else if isOnline {
		status = "online"
	}

	var peakPowerKW *float64
	var peakPowerAt *string
	row := s.DB.QueryRow(ctx,
		`SELECT instantaneous_power_kw, recorded_at FROM plant_status
		 WHERE plant_id = $1 AND recorded_at >= $2
		 ORDER BY instantaneous_power_kw DESC LIMIT 1`,
		plantID, startOfDayBrazil())
	var peakVal float64
	var peakAt time.Time
	if err := row.Scan(&peakVal, &peakAt); err == nil {
		peakPowerKW = &peakVal
		peakAtStr := peakAt.Format(time.RFC3339)
		peakPowerAt = &peakAtStr
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeInternalError(w, err, "falha ao consultar pico do dia")
		return
	}

	// today_economia_brl depende da tarifa efetiva da última fatura Celesc
	// — ainda não portado (ver Fase 5 do plano, parser de fatura), então
	// fica sempre null por enquanto, igual ao comportamento real hoje pra
	// quem nunca enviou fatura nenhuma.
	writeJSON(w, http.StatusOK, summaryResponse{
		InstantaneousPowerKW: instantaneous,
		InstalledPowerKWp:    installed,
		TodayGeneratedKWh:    todayGenerated,
		TodayEconomiaBRL:     nil,
		TodayVsYesterdayPct:  todayVsYesterdayPct,
		PeakPowerKW:          peakPowerKW,
		PeakPowerAt:          peakPowerAt,
		Status:               status,
		UpdatedAt:            time.Now().UTC().Format(time.RFC3339),
	})
}

type inverterEntry struct {
	PowerKW             *float64 `json:"power_kw"`
	DayKWh              *float64 `json:"day_kwh"`
	TemperatureC        *float64 `json:"temperature_c"`
	Status              string   `json:"status"`
	ConsecutiveFailures int      `json:"consecutive_failures"`
	LastError           *string  `json:"last_error"`
}

func (s *Server) healthStatus(ctx context.Context, plantID, inverter string) (int, *string, error) {
	var failures int
	var lastError *string
	err := s.DB.QueryRow(ctx,
		`SELECT consecutive_failures, last_error FROM collector_health
		 WHERE plant_id = $1 AND inverter = $2 AND recorded_at >= now() - interval '1 hour'
		 ORDER BY recorded_at DESC LIMIT 1`,
		plantID, inverter,
	).Scan(&failures, &lastError)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, err
	}
	return failures, lastError, nil
}

func (s *Server) handleInverters(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}
	ctx := r.Context()

	brands, err := s.enabledInverters(ctx, plantID)
	if err != nil {
		writeInternalError(w, err, "falha ao listar inversores habilitados")
		return
	}

	result := map[string]inverterEntry{}
	for _, inverter := range brands {
		entry := inverterEntry{Status: "sem_comunicacao"}

		var powerKW, dayKWh, temperatureC *float64
		var recordedAt time.Time
		row := s.DB.QueryRow(ctx,
			`SELECT power_kw, day_kwh, temperature_c, recorded_at FROM inverter_status
			 WHERE plant_id = $1 AND inverter = $2 AND recorded_at >= now() - interval '1 hour'
			 ORDER BY recorded_at DESC LIMIT 1`,
			plantID, inverter,
		)
		err := row.Scan(&powerKW, &dayKWh, &temperatureC, &recordedAt)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeInternalError(w, err, "falha ao consultar status do inversor")
			return
		}
		if err == nil {
			ageMinutes := time.Since(recordedAt).Minutes()
			switch {
			case ageMinutes > commTimeoutMinutes:
				entry.Status = "sem_comunicacao"
			case powerKW != nil && *powerKW > 0:
				entry.Status = "gerando"
			default:
				entry.Status = "online_sem_geracao"
			}
			entry.PowerKW = powerKW
			entry.DayKWh = dayKWh
			entry.TemperatureC = temperatureC
		}

		failures, lastError, err := s.healthStatus(ctx, plantID, inverter)
		if err != nil {
			writeInternalError(w, err, "falha ao consultar saúde da coleta")
			return
		}
		entry.ConsecutiveFailures = failures
		entry.LastError = lastError

		result[inverter] = entry
	}
	writeJSON(w, http.StatusOK, result)
}

type collectorHealthEntry struct {
	TotalCycles    int      `json:"total_cycles"`
	FailedCycles   int      `json:"failed_cycles"`
	ReliabilityPct *float64 `json:"reliability_pct"`
}

func (s *Server) handleCollectorHealth(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}
	ctx := r.Context()

	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := parsePositiveInt(d); err == nil {
			days = parsed
		}
	}

	brands, err := s.enabledInverters(ctx, plantID)
	if err != nil {
		writeInternalError(w, err, "falha ao listar inversores habilitados")
		return
	}

	result := map[string]collectorHealthEntry{}
	for _, inverter := range brands {
		var total, failed int
		err := s.DB.QueryRow(ctx,
			`SELECT count(*), count(*) FILTER (WHERE consecutive_failures > 0)
			 FROM collector_health
			 WHERE plant_id = $1 AND inverter = $2 AND recorded_at >= now() - make_interval(days => $3)`,
			plantID, inverter, days,
		).Scan(&total, &failed)
		if err != nil {
			writeInternalError(w, err, "falha ao consultar confiabilidade da coleta")
			return
		}
		entry := collectorHealthEntry{TotalCycles: total, FailedCycles: failed}
		if total > 0 {
			pct := roundTo((float64(total-failed)/float64(total))*100, 1)
			entry.ReliabilityPct = &pct
		}
		result[inverter] = entry
	}
	writeJSON(w, http.StatusOK, result)
}
