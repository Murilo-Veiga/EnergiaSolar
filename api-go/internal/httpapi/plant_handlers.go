package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"energiasolar-api/internal/auth"
	"energiasolar-api/internal/models"
)

// ErrPlantNotAuthorized cobre tanto "plant_id inexistente" quanto "existe,
// mas é de outro usuário" — de propósito: quem chama nunca deve tratar os
// dois casos diferente (ver handleGetPlant, que sempre responde 404 pros
// dois, nunca 403 — não confirma pra quem tenta adivinhar um uuid que a
// usina "existe, só não é sua").
var ErrPlantNotAuthorized = errors.New("usina não encontrada ou não pertence ao usuário")

// authorizePlant é o único ponto de autorização multi-tenant do serviço:
// toda rota que recebe um plant_id passa por aqui antes de tocar em
// qualquer tabela de série temporal — garante que um usuário nunca lê o
// dado de uma usina de outro usuário, mesmo que adivinhe o uuid.
func (s *Server) authorizePlant(ctx context.Context, plantID string) (models.Plant, error) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return models.Plant{}, ErrPlantNotAuthorized
	}

	var p models.Plant
	err := s.DB.QueryRow(ctx,
		`SELECT id, user_id, name, lat, lon, installed_power_kwp, timezone, created_at
		   FROM plants WHERE id = $1 AND user_id = $2`,
		plantID, userID,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.Lat, &p.Lon, &p.InstalledPowerKWp, &p.Timezone, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Plant{}, ErrPlantNotAuthorized
	}
	if err != nil {
		return models.Plant{}, err
	}
	return p, nil
}

type plantResponse struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Lat               *float64 `json:"lat"`
	Lon               *float64 `json:"lon"`
	InstalledPowerKWp float64  `json:"installed_power_kwp"`
	Timezone          string   `json:"timezone"`
}

func (s *Server) handleGetPlant(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	p, err := s.authorizePlant(r.Context(), plantID)
	if errors.Is(err, ErrPlantNotAuthorized) {
		// 404, não 403: não confirma pra quem tenta adivinhar uuid que a
		// usina existe e é só "de outro usuário" — trata como inexistente.
		writeError(w, http.StatusNotFound, "usina não encontrada")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "falha ao consultar usina")
		return
	}
	writeJSON(w, http.StatusOK, plantResponse{
		ID: p.ID, Name: p.Name, Lat: p.Lat, Lon: p.Lon,
		InstalledPowerKWp: p.InstalledPowerKWp, Timezone: p.Timezone,
	})
}

func (s *Server) handleListPlants(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	rows, err := s.DB.Query(r.Context(),
		`SELECT id, name, lat, lon, installed_power_kwp, timezone
		   FROM plants WHERE user_id = $1 ORDER BY created_at`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "falha ao listar usinas")
		return
	}
	defer rows.Close()

	plants := []plantResponse{}
	for rows.Next() {
		var p plantResponse
		if err := rows.Scan(&p.ID, &p.Name, &p.Lat, &p.Lon, &p.InstalledPowerKWp, &p.Timezone); err != nil {
			writeError(w, http.StatusInternalServerError, "falha ao ler usinas")
			return
		}
		plants = append(plants, p)
	}
	writeJSON(w, http.StatusOK, plants)
}
