package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type annotationIn struct {
	Date string `json:"date"` // "YYYY-MM-DD"
	Note string `json:"note"`
}

type annotationRow struct {
	Date string `json:"date"`
	Note string `json:"note"`
}

// handleCreateAnnotation grava 1 anotação por dia — gravar de novo no
// mesmo dia sobrescreve a anterior, via upsert explícito na PK
// (plant_id, day) da tabela annotation.
func (s *Server) handleCreateAnnotation(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}

	var in annotationIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}
	day, err := time.Parse("2006-01-02", in.Date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "date precisa estar no formato YYYY-MM-DD")
		return
	}
	note := strings.TrimSpace(in.Note)
	if note == "" || len(note) > 280 {
		writeError(w, http.StatusBadRequest, "note é obrigatória e precisa ter até 280 caracteres")
		return
	}

	_, err = s.DB.Exec(r.Context(),
		`INSERT INTO annotation (plant_id, day, note) VALUES ($1, $2, $3)
		 ON CONFLICT (plant_id, day) DO UPDATE SET note = EXCLUDED.note`,
		plantID, day, note,
	)
	if err != nil {
		writeInternalError(w, err, "falha ao gravar anotação")
		return
	}
	writeJSON(w, http.StatusOK, annotationRow{Date: in.Date, Note: note})
}

func (s *Server) handleListAnnotations(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}
	days := daysForRange(r.URL.Query().Get("range"))

	rows, err := s.DB.Query(r.Context(),
		`SELECT day, note FROM annotation
		 WHERE plant_id = $1 AND day >= current_date - make_interval(days => $2)
		 ORDER BY day DESC`,
		plantID, days,
	)
	if err != nil {
		writeInternalError(w, err, "falha ao listar anotações")
		return
	}
	defer rows.Close()

	result := []annotationRow{}
	for rows.Next() {
		var day time.Time
		var note string
		if err := rows.Scan(&day, &note); err != nil {
			writeInternalError(w, err, "falha ao ler anotações")
			return
		}
		result = append(result, annotationRow{Date: day.Format("2006-01-02"), Note: note})
	}
	if err := rows.Err(); err != nil {
		writeInternalError(w, err, "falha ao ler anotações")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": result})
}
