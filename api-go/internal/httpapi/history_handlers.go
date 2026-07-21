package httpapi

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"

	"energiasolar-api/internal/brtime"
)

type historyRow struct {
	Date            string   `json:"date"`
	GeneratedKWh    *float64 `json:"generated_kwh"`
	ValorEstimadoBR *float64 `json:"valor_estimado_brl"`
}

type historyResponse struct {
	Rows             []historyRow `json:"rows"`
	TotalKWh         float64      `json:"total_kwh"`
	TotalBRL         *float64     `json:"total_brl"`
	PreviousTotalKWh float64      `json:"previous_total_kwh"`
	PreviousTotalBRL *float64     `json:"previous_total_brl"`
}

// periodTotalKWh soma generated_kwh num intervalo de `days` dias terminando
// `startOffsetDays` dias atrás — usado pra comparar o período atual com o
// imediatamente anterior. Mesma ideia de _period_total_kwh() em
// webapp/main.py (Python).
func (s *Server) periodTotalKWh(ctx context.Context, plantID string, days, startOffsetDays int) (float64, error) {
	var total *float64
	err := s.DB.QueryRow(ctx,
		`SELECT SUM(generated_kwh) FROM daily_generation
		 WHERE plant_id = $1
		   AND day >= current_date - make_interval(days => $2)
		   AND day < current_date - make_interval(days => $3)`,
		plantID, days+startOffsetDays, startOffsetDays,
	).Scan(&total)
	if err != nil {
		return 0, err
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlantView(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}
	ctx := r.Context()
	days := daysForRange(r.URL.Query().Get("range"))

	rows, err := s.DB.Query(ctx,
		`SELECT day, generated_kwh FROM daily_generation
		 WHERE plant_id = $1 AND day >= current_date - make_interval(days => $2)
		 ORDER BY day`,
		plantID, days,
	)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar histórico")
		return
	}
	defer rows.Close()

	// valor_estimado_brl/total_brl = geração × tarifa efetiva da última
	// fatura Celesc — ficam null até a 1a fatura ser importada.
	tarifa, err := s.tarifaEfetiva(ctx, plantID)
	if err != nil {
		writeInternalError(w, err, "falha ao calcular tarifa efetiva")
		return
	}

	result := historyResponse{Rows: []historyRow{}}
	for rows.Next() {
		var day time.Time
		var kwh float64
		if err := rows.Scan(&day, &kwh); err != nil {
			writeInternalError(w, err, "falha ao ler histórico")
			return
		}
		row := historyRow{Date: day.Format("2006-01-02"), GeneratedKWh: &kwh}
		if tarifa != nil {
			valor := roundTo(kwh*(*tarifa), 2)
			row.ValorEstimadoBR = &valor
		}
		result.Rows = append(result.Rows, row)
		result.TotalKWh += kwh
	}
	if err := rows.Err(); err != nil {
		writeInternalError(w, err, "falha ao ler histórico")
		return
	}
	result.TotalKWh = roundTo(result.TotalKWh, 1)
	if tarifa != nil {
		totalBRL := roundTo(result.TotalKWh*(*tarifa), 2)
		result.TotalBRL = &totalBRL
	}

	previousTotal, err := s.periodTotalKWh(ctx, plantID, days, days)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar período anterior")
		return
	}
	result.PreviousTotalKWh = roundTo(previousTotal, 1)
	if tarifa != nil {
		previousBRL := roundTo(result.PreviousTotalKWh*(*tarifa), 2)
		result.PreviousTotalBRL = &previousBRL
	}

	writeJSON(w, http.StatusOK, result)
}

type historyRecordsResponse struct {
	BestDayKWh     *float64 `json:"best_day_kwh"`
	BestDayDate    *string  `json:"best_day_date"`
	BestMonthKWh   *float64 `json:"best_month_kwh"`
	BestMonthLabel *string  `json:"best_month_label"`
	PeakPowerKW    *float64 `json:"peak_power_kw"`
	PeakPowerAt    *string  `json:"peak_power_at"`
}

func (s *Server) handleHistoryRecords(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlantView(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}
	ctx := r.Context()
	resp := historyRecordsResponse{}

	var bestDayKWh float64
	var bestDay time.Time
	err := s.DB.QueryRow(ctx,
		`SELECT generated_kwh, day FROM daily_generation
		 WHERE plant_id = $1 ORDER BY generated_kwh DESC LIMIT 1`, plantID,
	).Scan(&bestDayKWh, &bestDay)
	if err == nil {
		rounded := roundTo(bestDayKWh, 1)
		resp.BestDayKWh = &rounded
		dateStr := bestDay.Format("2006-01-02")
		resp.BestDayDate = &dateStr
	} else if !isNoRows(err) {
		writeInternalError(w, err, "falha ao consultar melhor dia")
		return
	}

	var bestMonthKWh float64
	var bestMonth time.Time
	err = s.DB.QueryRow(ctx,
		`SELECT SUM(generated_kwh) AS total, date_trunc('month', day) AS month
		 FROM daily_generation WHERE plant_id = $1
		 GROUP BY month ORDER BY total DESC LIMIT 1`, plantID,
	).Scan(&bestMonthKWh, &bestMonth)
	if err == nil {
		rounded := roundTo(bestMonthKWh, 1)
		resp.BestMonthKWh = &rounded
		label := bestMonth.Format("01/2006")
		resp.BestMonthLabel = &label
	} else if !isNoRows(err) {
		writeInternalError(w, err, "falha ao consultar melhor mês")
		return
	}

	var peakKW float64
	var peakAt time.Time
	err = s.DB.QueryRow(ctx,
		`SELECT instantaneous_power_kw, recorded_at FROM plant_status
		 WHERE plant_id = $1 ORDER BY instantaneous_power_kw DESC LIMIT 1`, plantID,
	).Scan(&peakKW, &peakAt)
	if err == nil {
		resp.PeakPowerKW = &peakKW
		atStr := peakAt.Format(time.RFC3339)
		resp.PeakPowerAt = &atStr
	} else if !isNoRows(err) {
		writeInternalError(w, err, "falha ao consultar pico histórico")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

type historyInvertersRow struct {
	Date      string   `json:"date"`
	HuaweiKWh *float64 `json:"huawei_kwh"`
	FoxessKWh *float64 `json:"foxess_kwh"`
}

// handleHistoryInverters deriva do último inverter_status.day_kwh de cada
// dia-calendário, por inversor — mesmo princípio de "o último valor do dia
// é o total do dia" que já vale pro daily_generation. "dia" é exceção:
// usa a meia-noite BRT de hoje como início em vez da janela rolante de
// -1d, pelo mesmo motivo documentado no README (senão o ponto de ontem
// ~23:55 entra na janela e dobra a soma).
func (s *Server) handleHistoryInverters(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlantView(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}
	ctx := r.Context()
	rangeParam := r.URL.Query().Get("range")

	var sinceUTC time.Time
	if rangeParam == "dia" {
		sinceUTC = brtime.StartOfDay()
	} else {
		days := daysForRange(rangeParam)
		sinceUTC = time.Now().UTC().AddDate(0, 0, -days)
	}

	rows, err := s.DB.Query(ctx,
		`SELECT DISTINCT ON (day, inverter) day, inverter, day_kwh
		 FROM (
		   SELECT (recorded_at AT TIME ZONE 'America/Sao_Paulo')::date AS day,
		          inverter, day_kwh, recorded_at
		   FROM inverter_status
		   WHERE plant_id = $1 AND day_kwh IS NOT NULL AND recorded_at >= $2
		 ) sub
		 ORDER BY day, inverter, recorded_at DESC`,
		plantID, sinceUTC,
	)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar geração por inversor")
		return
	}
	defer rows.Close()

	byDay := map[string]map[string]float64{}
	for rows.Next() {
		var day time.Time
		var inverter string
		var kwh float64
		if err := rows.Scan(&day, &inverter, &kwh); err != nil {
			writeInternalError(w, err, "falha ao ler geração por inversor")
			return
		}
		dateStr := day.Format("2006-01-02")
		if byDay[dateStr] == nil {
			byDay[dateStr] = map[string]float64{}
		}
		byDay[dateStr][inverter] = kwh
	}
	if err := rows.Err(); err != nil {
		writeInternalError(w, err, "falha ao ler geração por inversor")
		return
	}

	dates := make([]string, 0, len(byDay))
	for d := range byDay {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	result := make([]historyInvertersRow, 0, len(dates))
	for _, d := range dates {
		vals := byDay[d]
		row := historyInvertersRow{Date: d}
		if v, ok := vals["huawei"]; ok {
			row.HuaweiKWh = &v
		}
		if v, ok := vals["foxess"]; ok {
			row.FoxessKWh = &v
		}
		result = append(result, row)
	}

	writeJSON(w, http.StatusOK, map[string]any{"rows": result})
}
