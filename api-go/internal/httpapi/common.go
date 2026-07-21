package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// writeInternalError loga o erro real (nunca exposto ao cliente) e
// responde com uma mensagem genérica — evita vazar detalhe de driver/SQL
// na resposta, mas sem perder o motivo real nos logs do servidor.
func writeInternalError(w http.ResponseWriter, err error, publicMsg string) {
	slog.Default().Error(publicMsg, "error", err)
	writeError(w, http.StatusInternalServerError, publicMsg)
}

// respondPlantAuthError traduz o erro de authorizePlant pra resposta HTTP —
// sempre 404 pra "não autorizado" (nunca 403, ver ErrPlantNotAuthorized),
// 500 pra qualquer outra falha real de banco.
func respondPlantAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrPlantNotAuthorized) {
		writeError(w, http.StatusNotFound, "usina não encontrada")
		return
	}
	writeInternalError(w, err, "falha ao consultar usina")
}

func roundTo(v float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))
	return math.Round(v*factor) / factor
}

func roundHalfAwayFromZero(v float64) float64 {
	return math.Round(v)
}

// isNoRows evita repetir errors.Is(err, pgx.ErrNoRows) em todo handler que
// trata "nenhuma linha encontrada" como um resultado nulo válido, em vez
// de erro de verdade.
func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func parsePositiveInt(s string) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return 0, strconv.ErrSyntax
	}
	return v, nil
}

// scanOptionalFloat roda uma query que devolve no máximo 1 float (ex.:
// "último valor dentro de uma janela") e trata a ausência de linha como
// nil em vez de erro — mesmo padrão do _last_value() em webapp/main.py
// (Python), que retorna None quando a janela não tem nenhum ponto.
func (s *Server) scanOptionalFloat(ctx context.Context, query string, args ...any) (*float64, error) {
	var v float64
	err := s.DB.QueryRow(ctx, query, args...).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// scanOptionalString é o equivalente pra colunas text (ex.: alarm_detail).
func (s *Server) scanOptionalString(ctx context.Context, query string, args ...any) (*string, error) {
	var v string
	err := s.DB.QueryRow(ctx, query, args...).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// scanOptionalBool é o equivalente pra colunas boolean (ex.: has_alarm).
func (s *Server) scanOptionalBool(ctx context.Context, query string, args ...any) (*bool, error) {
	var v bool
	err := s.DB.QueryRow(ctx, query, args...).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// rangeDays espelha RANGE_DAYS em webapp/main.py (Python).
var rangeDays = map[string]int{"dia": 1, "semana": 7, "mes": 30, "ano": 365}

func daysForRange(r string) int {
	if d, ok := rangeDays[r]; ok {
		return d
	}
	return 30
}

// enabledInverters lista as marcas (huawei/foxess) habilitadas pra uma
// usina — substitui o ENABLED_INVERTERS calculado a partir de
// HUAWEI_ENABLED/FOXESS_ENABLED (env vars fixas) no Python: agora cada
// usina tem seu próprio cadastro em inverter_credentials.
func (s *Server) enabledInverters(ctx context.Context, plantID string) ([]string, error) {
	rows, err := s.DB.Query(ctx,
		`SELECT brand FROM inverter_credentials WHERE plant_id = $1 AND enabled = true ORDER BY brand`,
		plantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var brands []string
	for rows.Next() {
		var brand string
		if err := rows.Scan(&brand); err != nil {
			return nil, err
		}
		brands = append(brands, brand)
	}
	return brands, rows.Err()
}
