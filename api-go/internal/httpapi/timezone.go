package httpapi

import (
	"log/slog"
	"os"
	"time"

	// Embute o banco de fusos horários IANA no binário — evita depender do
	// pacote tzdata do SO dentro da imagem alpine, que não vem por padrão.
	_ "time/tzdata"
)

// BrazilLocation é o mesmo fuso fixo (America/Sao_Paulo, sem DST hoje) usado
// por todo o stack Python (BRAZIL_TZ em collector/main.py e webapp/main.py)
// — carregado 1x no import do pacote.
var BrazilLocation = mustLoadLocation("America/Sao_Paulo")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("falha ao carregar timezone", "name", name, "error", err)
		os.Exit(1)
	}
	return loc
}

// startOfDayBrazil devolve a meia-noite de hoje em America/Sao_Paulo,
// convertida pra UTC — mesmo conceito de start_of_day_brazil/
// today_midnight_brt usado em webapp/main.py e collector/main.py (Python).
func startOfDayBrazil() time.Time {
	now := time.Now().In(BrazilLocation)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, BrazilLocation).UTC()
}
