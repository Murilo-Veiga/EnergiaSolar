// Package brtime centraliza o fuso horário usado por todo o stack —
// mesmo BRAZIL_TZ fixo (America/Sao_Paulo, sem DST hoje) usado em
// collector/main.py e webapp/main.py (Python). Compartilhado entre
// internal/httpapi e internal/collector pra não duplicar nem criar
// dependência de um pacote sobre o outro.
package brtime

import (
	"log/slog"
	"os"
	"time"

	// Embute o banco de fusos horários IANA no binário — evita depender do
	// pacote tzdata do SO dentro da imagem alpine, que não vem por padrão.
	_ "time/tzdata"
)

// Location é o fuso fixo carregado 1x no import do pacote.
var Location = mustLoadLocation("America/Sao_Paulo")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("falha ao carregar timezone", "name", name, "error", err)
		os.Exit(1)
	}
	return loc
}

// StartOfDay devolve a meia-noite de hoje em America/Sao_Paulo, convertida
// pra UTC — mesmo conceito de start_of_day_brazil/today_midnight_brt
// usado no Python.
func StartOfDay() time.Time {
	now := time.Now().In(Location)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, Location).UTC()
}
