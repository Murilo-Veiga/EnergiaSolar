package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"energiasolar-api/internal/celesc"
)

const maxBillUploadBytes = 10 << 20 // 10MB

// tarifaEfetiva é valor pago ÷ kWh da fatura mais recente (source='fatura')
// entre todas as unidades consumidoras da usina — usada como estimativa de
// tarifa pra converter geração/consumo em R$ (summary.today_economia_brl,
// day-status.bandeira*, history.valor_estimado_brl). Nenhuma fatura cobre
// ainda um período com compensação solar, então essa tarifa efetiva não
// inclui nenhum crédito de geração. Mesma ideia de _tarifa_efetiva() em
// webapp/main.py (Python).
func (s *Server) tarifaEfetiva(ctx context.Context, plantID string) (*float64, error) {
	var kwh, brl float64
	err := s.DB.QueryRow(ctx,
		`SELECT b.consumo_kwh, b.total_pagar_brl
		 FROM celesc_bills b
		 JOIN consumer_units cu ON cu.id = b.consumer_unit_id
		 WHERE cu.plant_id = $1 AND b.source = 'fatura'
		   AND b.total_pagar_brl IS NOT NULL AND b.consumo_kwh > 0
		 ORDER BY b.referencia_ano DESC, b.referencia_mes DESC, b.created_at DESC
		 LIMIT 1`,
		plantID,
	).Scan(&kwh, &brl)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	tarifa := brl / kwh
	return &tarifa, nil
}

// latestBandeira devolve a bandeira tarifária/valor da fatura mais recente
// da usina — usada em GET /day-status.
func (s *Server) latestBandeira(ctx context.Context, plantID string) (bandeira *string, valorKWh *float64, err error) {
	err = s.DB.QueryRow(ctx,
		`SELECT b.bandeira, b.bandeira_valor_kwh
		 FROM celesc_bills b
		 JOIN consumer_units cu ON cu.id = b.consumer_unit_id
		 WHERE cu.plant_id = $1 AND b.source = 'fatura' AND b.bandeira IS NOT NULL
		 ORDER BY b.referencia_ano DESC, b.referencia_mes DESC, b.created_at DESC
		 LIMIT 1`,
		plantID,
	).Scan(&bandeira, &valorKWh)
	if isNoRows(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return bandeira, valorKWh, nil
}

type consumptionUploadResponse struct {
	UC                       string   `json:"uc"`
	UCLabel                  string   `json:"uc_label"`
	Titular                  *string  `json:"titular"`
	Referencia               string   `json:"referencia"`
	ConsumoKWh               float64  `json:"consumo_kwh"`
	TotalPagarBRL            *float64 `json:"total_pagar_brl"`
	MesesHistoricoImportados int      `json:"meses_historico_importados"`
}

// handleUploadConsumption recebe o PDF da fatura Celesc (multipart,
// campo "file"), extrai os campos via internal/celesc e grava: 1 linha
// "fatura" (mês de referência, com todo o detalhe) + 1 linha
// "backfill_historico" por mês do quadro de histórico já impresso na
// própria fatura (ignora o mês que já foi coberto pela linha "fatura").
func (s *Server) handleUploadConsumption(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBillUploadBytes)
	if err := r.ParseMultipartForm(maxBillUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "arquivo muito grande ou requisição inválida (máx. 10MB)")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "campo \"file\" (PDF da fatura) é obrigatório")
		return
	}
	defer file.Close()

	pdfBytes, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "falha ao ler o arquivo enviado")
		return
	}

	bill, err := celesc.ParseBill(pdfBytes)
	if err != nil {
		var parseErr *celesc.ParseError
		if errors.As(err, &parseErr) {
			writeError(w, http.StatusUnprocessableEntity, parseErr.Error())
			return
		}
		writeInternalError(w, err, "falha ao processar o PDF")
		return
	}

	ctx := r.Context()
	label := bill.UC
	var consumerUnitID string
	err = s.DB.QueryRow(ctx,
		`INSERT INTO consumer_units (plant_id, uc_number, label)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (plant_id, uc_number) DO UPDATE SET label = consumer_units.label
		 RETURNING id`,
		plantID, bill.UC, label,
	).Scan(&consumerUnitID)
	if err != nil {
		writeInternalError(w, err, "falha ao gravar unidade consumidora")
		return
	}

	var titular *string
	if bill.Titular != "" {
		titular = &bill.Titular
	}
	_, err = s.DB.Exec(ctx,
		`INSERT INTO celesc_bills
		   (consumer_unit_id, referencia_ano, referencia_mes, consumo_kwh, total_pagar_brl,
		    dias_faturados, bandeira, bandeira_valor_kwh, titular, source)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'fatura')
		 ON CONFLICT (consumer_unit_id, referencia_ano, referencia_mes) DO UPDATE SET
		   consumo_kwh = EXCLUDED.consumo_kwh,
		   total_pagar_brl = EXCLUDED.total_pagar_brl,
		   dias_faturados = EXCLUDED.dias_faturados,
		   bandeira = EXCLUDED.bandeira,
		   bandeira_valor_kwh = EXCLUDED.bandeira_valor_kwh,
		   titular = EXCLUDED.titular,
		   source = 'fatura'`,
		consumerUnitID, bill.ReferenciaAno, bill.ReferenciaMes, bill.ConsumoKWh, bill.TotalPagarBRL,
		bill.DiasFaturados, bill.Bandeira, bill.BandeiraValorKWh, titular,
	)
	if err != nil {
		writeInternalError(w, err, "falha ao gravar fatura")
		return
	}

	imported := 0
	for _, h := range bill.Historico {
		if h.Ano == bill.ReferenciaAno && h.Mes == bill.ReferenciaMes {
			continue // já coberto acima, com mais detalhe (R$, dias, bandeira)
		}
		tag, err := s.DB.Exec(ctx,
			`INSERT INTO celesc_bills (consumer_unit_id, referencia_ano, referencia_mes, consumo_kwh, source)
			 VALUES ($1, $2, $3, $4, 'backfill_historico')
			 ON CONFLICT (consumer_unit_id, referencia_ano, referencia_mes) DO NOTHING`,
			consumerUnitID, h.Ano, h.Mes, h.ConsumoKWh,
		)
		if err != nil {
			writeInternalError(w, err, "falha ao gravar histórico da fatura")
			return
		}
		imported += int(tag.RowsAffected())
	}

	totalPagar := &bill.TotalPagarBRL
	writeJSON(w, http.StatusOK, consumptionUploadResponse{
		UC:                       bill.UC,
		UCLabel:                  label,
		Titular:                  titular,
		Referencia:               fmt.Sprintf("%02d/%d", bill.ReferenciaMes, bill.ReferenciaAno),
		ConsumoKWh:               bill.ConsumoKWh,
		TotalPagarBRL:            totalPagar,
		MesesHistoricoImportados: imported,
	})
}

type consumptionLatest struct {
	Referencia    string   `json:"referencia"`
	ConsumoKWh    float64  `json:"consumed_kwh"`
	TotalValueBRL *float64 `json:"total_value_brl"`
}

type consumptionUnitSummary struct {
	UCNumber string             `json:"uc_number"`
	Label    string             `json:"label"`
	Latest   *consumptionLatest `json:"latest"`
}

type consumptionSummaryResponse struct {
	Unidades            []consumptionUnitSummary `json:"unidades"`
	EconomiaEstimadaBRL *float64                 `json:"economia_estimada_brl"`
}

// handleConsumptionSummary lista as unidades consumidoras da usina com a
// fatura mais recente de cada uma, mais uma estimativa de economia:
// geração acumulada da usina × tarifa efetiva (não é o valor oficial da
// Celesc, é aproximação — mesmo aviso do webapp/main.py original).
func (s *Server) handleConsumptionSummary(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}
	ctx := r.Context()

	rows, err := s.DB.Query(ctx,
		`SELECT cu.uc_number, cu.label, b.referencia_ano, b.referencia_mes, b.consumo_kwh, b.total_pagar_brl
		 FROM consumer_units cu
		 LEFT JOIN LATERAL (
		   SELECT referencia_ano, referencia_mes, consumo_kwh, total_pagar_brl
		   FROM celesc_bills
		   WHERE consumer_unit_id = cu.id AND source = 'fatura'
		   ORDER BY referencia_ano DESC, referencia_mes DESC LIMIT 1
		 ) b ON true
		 WHERE cu.plant_id = $1
		 ORDER BY cu.label`,
		plantID,
	)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar consumo")
		return
	}
	defer rows.Close()

	result := consumptionSummaryResponse{Unidades: []consumptionUnitSummary{}}
	for rows.Next() {
		var ucNumber, label string
		var ano, mes *int
		var kwh, total *float64
		if err := rows.Scan(&ucNumber, &label, &ano, &mes, &kwh, &total); err != nil {
			writeInternalError(w, err, "falha ao ler consumo")
			return
		}
		unit := consumptionUnitSummary{UCNumber: ucNumber, Label: label}
		if ano != nil && mes != nil && kwh != nil {
			unit.Latest = &consumptionLatest{
				Referencia:    fmt.Sprintf("%02d/%d", *mes, *ano),
				ConsumoKWh:    *kwh,
				TotalValueBRL: total,
			}
		}
		result.Unidades = append(result.Unidades, unit)
	}
	if err := rows.Err(); err != nil {
		writeInternalError(w, err, "falha ao ler consumo")
		return
	}

	tarifa, err := s.tarifaEfetiva(ctx, plantID)
	if err != nil {
		writeInternalError(w, err, "falha ao calcular tarifa efetiva")
		return
	}
	if tarifa != nil {
		geracaoTotal, err := s.scanOptionalFloat(ctx,
			`SELECT SUM(generated_kwh) FROM daily_generation WHERE plant_id = $1`, plantID)
		if err != nil {
			writeInternalError(w, err, "falha ao consultar geração acumulada")
			return
		}
		if geracaoTotal != nil {
			economia := roundTo(*geracaoTotal*(*tarifa), 2)
			result.EconomiaEstimadaBRL = &economia
		}
	}

	writeJSON(w, http.StatusOK, result)
}

type consumptionHistoryRow struct {
	Referencia    string   `json:"referencia"`
	ConsumoKWh    float64  `json:"consumo_kwh"`
	TotalPagarBRL *float64 `json:"total_pagar_brl"`
	Bandeira      *string  `json:"bandeira"`
}

// handleConsumptionHistory lista as faturas de 1 unidade consumidora
// (`uc` = uc_number), mais recente primeiro.
func (s *Server) handleConsumptionHistory(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}
	uc := r.URL.Query().Get("uc")
	if uc == "" {
		writeError(w, http.StatusBadRequest, "parâmetro \"uc\" é obrigatório")
		return
	}
	months := 13
	if m := r.URL.Query().Get("months"); m != "" {
		if v, err := parsePositiveInt(m); err == nil {
			months = v
		}
	}

	rows, err := s.DB.Query(r.Context(),
		`SELECT b.referencia_ano, b.referencia_mes, b.consumo_kwh, b.total_pagar_brl, b.bandeira
		 FROM celesc_bills b
		 JOIN consumer_units cu ON cu.id = b.consumer_unit_id
		 WHERE cu.plant_id = $1 AND cu.uc_number = $2
		 ORDER BY b.referencia_ano DESC, b.referencia_mes DESC
		 LIMIT $3`,
		plantID, uc, months,
	)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar histórico de consumo")
		return
	}
	defer rows.Close()

	result := []consumptionHistoryRow{}
	for rows.Next() {
		var ano, mes int
		var kwh float64
		var total *float64
		var bandeira *string
		if err := rows.Scan(&ano, &mes, &kwh, &total, &bandeira); err != nil {
			writeInternalError(w, err, "falha ao ler histórico de consumo")
			return
		}
		result = append(result, consumptionHistoryRow{
			Referencia:    fmt.Sprintf("%02d/%d", mes, ano),
			ConsumoKWh:    kwh,
			TotalPagarBRL: total,
			Bandeira:      bandeira,
		})
	}
	if err := rows.Err(); err != nil {
		writeInternalError(w, err, "falha ao ler histórico de consumo")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"rows": result})
}
