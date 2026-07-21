package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// wmoCodes traduz o weathercode da Open-Meteo — mesma tabela de
// WMO_CODES em webapp/main.py (Python).
var wmoCodes = map[int]struct {
	description string
	rating      string
}{
	0:  {"Céu limpo", "bom"},
	1:  {"Principalmente limpo", "bom"},
	2:  {"Parcialmente nublado", "bom"},
	3:  {"Nublado", "moderado"},
	45: {"Nevoeiro", "moderado"},
	48: {"Nevoeiro com geada", "moderado"},
	51: {"Garoa fraca", "moderado"},
	53: {"Garoa moderada", "moderado"},
	55: {"Garoa forte", "ruim"},
	56: {"Garoa congelante fraca", "ruim"},
	57: {"Garoa congelante forte", "ruim"},
	61: {"Chuva fraca", "moderado"},
	63: {"Chuva moderada", "ruim"},
	65: {"Chuva forte", "ruim"},
	66: {"Chuva congelante fraca", "ruim"},
	67: {"Chuva congelante forte", "ruim"},
	71: {"Neve fraca", "ruim"},
	73: {"Neve moderada", "ruim"},
	75: {"Neve forte", "ruim"},
	77: {"Grãos de neve", "ruim"},
	80: {"Pancadas de chuva fracas", "moderado"},
	81: {"Pancadas de chuva moderadas", "ruim"},
	82: {"Pancadas de chuva fortes", "ruim"},
	85: {"Pancadas de neve fracas", "ruim"},
	86: {"Pancadas de neve fortes", "ruim"},
	95: {"Trovoada", "ruim"},
	96: {"Trovoada com granizo fraco", "ruim"},
	99: {"Trovoada com granizo forte", "ruim"},
}

func hourInDaylight(hourIndex int, sunriseHHMM, sunsetHHMM string) bool {
	sunriseH := parseHHMM(sunriseHHMM)
	sunsetH := parseHHMM(sunsetHHMM)
	h := float64(hourIndex) + 0.5
	return sunriseH <= h && h <= sunsetH
}

func parseHHMM(s string) float64 {
	if len(s) < 5 {
		return 0
	}
	hh := float64(s[0]-'0')*10 + float64(s[1]-'0')
	mm := float64(s[3]-'0')*10 + float64(s[4]-'0')
	return hh + mm/60
}

type forecastDay struct {
	Date                 string    `json:"date"`
	Weather              string    `json:"weather"`
	Rating               string    `json:"rating"`
	WeatherDaylight      *string   `json:"weather_daylight,omitempty"`
	RatingDaylight       *string   `json:"rating_daylight,omitempty"`
	TempMax              float64   `json:"temp_max"`
	TempMin              float64   `json:"temp_min"`
	SolarRadiationMJm2   float64   `json:"solar_radiation_mj_m2"`
	PrecipitationMM      float64   `json:"precipitation_mm"`
	PrecipitationProbPct float64   `json:"precipitation_probability_pct"`
	Sunrise              string    `json:"sunrise"`
	Sunset               string    `json:"sunset"`
	CloudcoverHourly     []float64 `json:"cloudcover_hourly,omitempty"`
}

// forecastCacheTTL espelha _FORECAST_CACHE_TTL em webapp/main.py — a
// Open-Meteo só atualiza o modelo a cada poucas horas, então cache de 2h
// evita bater na API a cada 30s/30min que o frontend consulta.
const forecastCacheTTL = 2 * time.Hour

type forecastCacheEntry struct {
	data      []forecastDay
	fetchedAt time.Time
}

// forecastCache é indexado por "lat,lon" — diferente do Python (1 única
// usina, 1 cache global), agora cada usina tem sua própria localização.
var (
	forecastCacheMu sync.Mutex
	forecastCache   = map[string]forecastCacheEntry{}
)

type openMeteoResponse struct {
	Daily struct {
		Time                        []string  `json:"time"`
		Weathercode                 []int     `json:"weathercode"`
		Temperature2mMax            []float64 `json:"temperature_2m_max"`
		Temperature2mMin            []float64 `json:"temperature_2m_min"`
		ShortwaveRadiationSum       []float64 `json:"shortwave_radiation_sum"`
		PrecipitationSum            []float64 `json:"precipitation_sum"`
		PrecipitationProbabilityMax []float64 `json:"precipitation_probability_max"`
		Sunrise                     []string  `json:"sunrise"`
		Sunset                      []string  `json:"sunset"`
	} `json:"daily"`
	Hourly struct {
		Weathercode []int     `json:"weathercode"`
		Cloudcover  []float64 `json:"cloudcover"`
	} `json:"hourly"`
}

func fetchForecastDays(lat, lon float64) ([]forecastDay, error) {
	key := fmt.Sprintf("%.6f,%.6f", lat, lon)

	forecastCacheMu.Lock()
	if entry, ok := forecastCache[key]; ok && time.Since(entry.fetchedAt) < forecastCacheTTL {
		forecastCacheMu.Unlock()
		return entry.data, nil
	}
	forecastCacheMu.Unlock()

	url := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f"+
			"&daily=weathercode,temperature_2m_max,temperature_2m_min,shortwave_radiation_sum,"+
			"precipitation_sum,precipitation_probability_max,sunrise,sunset"+
			"&hourly=weathercode,cloudcover&timezone=America/Sao_Paulo&forecast_days=5",
		lat, lon,
	)
	resp, err := http.Get(url) //nolint:gosec // URL montada só com float formatado, sem entrada de usuário livre
	if err != nil {
		return nil, fmt.Errorf("consultando open-meteo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var om openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&om); err != nil {
		return nil, fmt.Errorf("decodificando resposta da open-meteo: %w", err)
	}

	days := make([]forecastDay, 0, len(om.Daily.Time))
	for i, date := range om.Daily.Time {
		code := om.Daily.Weathercode[i]
		info, ok := wmoCodes[code]
		if !ok {
			info = struct {
				description string
				rating      string
			}{"Desconhecido", "moderado"}
		}
		sunrise := ""
		if len(om.Daily.Sunrise[i]) >= 16 {
			sunrise = om.Daily.Sunrise[i][11:16]
		}
		sunset := ""
		if len(om.Daily.Sunset[i]) >= 16 {
			sunset = om.Daily.Sunset[i][11:16]
		}

		day := forecastDay{
			Date:                 date,
			Weather:              info.description,
			Rating:               info.rating,
			TempMax:              om.Daily.Temperature2mMax[i],
			TempMin:              om.Daily.Temperature2mMin[i],
			SolarRadiationMJm2:   om.Daily.ShortwaveRadiationSum[i],
			PrecipitationMM:      om.Daily.PrecipitationSum[i],
			PrecipitationProbPct: om.Daily.PrecipitationProbabilityMax[i],
			Sunrise:              sunrise,
			Sunset:               sunset,
		}

		if i == 0 {
			start := i * 24
			end := start + 24
			if end > len(om.Hourly.Weathercode) {
				end = len(om.Hourly.Weathercode)
			}
			hourCodes := om.Hourly.Weathercode[start:end]
			hourClouds := om.Hourly.Cloudcover[start:end]

			counts := map[int]int{}
			for h, code := range hourCodes {
				if hourInDaylight(h, sunrise, sunset) {
					counts[code]++
				}
			}
			if len(counts) > 0 {
				bestCode, bestCount := 0, -1
				for code, count := range counts {
					if count > bestCount {
						bestCode, bestCount = code, count
					}
				}
				info, ok := wmoCodes[bestCode]
				if !ok {
					info = struct {
						description string
						rating      string
					}{"Desconhecido", "moderado"}
				}
				weatherDaylight := info.description
				ratingDaylight := info.rating
				day.WeatherDaylight = &weatherDaylight
				day.RatingDaylight = &ratingDaylight
			} else {
				weatherDaylight := day.Weather
				ratingDaylight := day.Rating
				day.WeatherDaylight = &weatherDaylight
				day.RatingDaylight = &ratingDaylight
			}
			day.CloudcoverHourly = hourClouds
		}

		days = append(days, day)
	}

	forecastCacheMu.Lock()
	forecastCache[key] = forecastCacheEntry{data: days, fetchedAt: time.Now()}
	forecastCacheMu.Unlock()

	return days, nil
}

func (s *Server) handleForecast(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	plant, err := s.authorizePlantView(r.Context(), plantID)
	if err != nil {
		respondPlantAuthError(w, err)
		return
	}
	if plant.Lat == nil || plant.Lon == nil {
		writeError(w, http.StatusUnprocessableEntity, "usina sem latitude/longitude cadastrada")
		return
	}

	days, err := fetchForecastDays(*plant.Lat, *plant.Lon)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar previsão do tempo")
		return
	}
	writeJSON(w, http.StatusOK, days)
}

type dayStatusResponse struct {
	Date               *string   `json:"date"`
	GeneratedKWh       *float64  `json:"generated_kwh"`
	Weather            string    `json:"weather"`
	WeatherDaylight    *string   `json:"weather_daylight,omitempty"`
	SolarRadiationMJm2 float64   `json:"solar_radiation_mj_m2"`
	CloudcoverHourly   []float64 `json:"cloudcover_hourly,omitempty"`
	Sunrise            string    `json:"sunrise"`
	Sunset             string    `json:"sunset"`
	HasAlarm           *bool     `json:"has_alarm"`
	AlarmDetail        *string   `json:"alarm_detail"`
	// bandeira/bandeira_valor_kwh vêm da fatura Celesc mais recente (ver
	// handleUploadConsumption) — ficam null até a 1a fatura ser importada.
	Bandeira         *string  `json:"bandeira"`
	BandeiraValorKWh *float64 `json:"bandeira_valor_kwh"`
}

func (s *Server) handleDayStatus(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	plant, err := s.authorizePlantView(r.Context(), plantID)
	if err != nil {
		respondPlantAuthError(w, err)
		return
	}
	if plant.Lat == nil || plant.Lon == nil {
		writeError(w, http.StatusUnprocessableEntity, "usina sem latitude/longitude cadastrada")
		return
	}
	ctx := r.Context()

	hasAlarm, err := s.scanOptionalBool(ctx,
		`SELECT has_alarm FROM plant_status WHERE plant_id = $1 AND recorded_at >= now() - interval '1 hour' ORDER BY recorded_at DESC LIMIT 1`,
		plantID)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar alarme")
		return
	}
	var alarmDetail *string
	if hasAlarm != nil && *hasAlarm {
		alarmDetail, err = s.scanOptionalString(ctx,
			`SELECT alarm_detail FROM plant_status WHERE plant_id = $1 AND recorded_at >= now() - interval '1 hour' ORDER BY recorded_at DESC LIMIT 1`,
			plantID)
		if err != nil {
			writeInternalError(w, err, "falha ao consultar detalhe do alarme")
			return
		}
	}

	days, err := fetchForecastDays(*plant.Lat, *plant.Lon)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar previsão do tempo")
		return
	}
	today := days[0]

	resp := dayStatusResponse{
		Weather:            today.Weather,
		WeatherDaylight:    today.WeatherDaylight,
		SolarRadiationMJm2: today.SolarRadiationMJm2,
		CloudcoverHourly:   today.CloudcoverHourly,
		Sunrise:            today.Sunrise,
		Sunset:             today.Sunset,
		HasAlarm:           hasAlarm,
		AlarmDetail:        alarmDetail,
	}

	var day time.Time
	var kwh float64
	err = s.DB.QueryRow(ctx,
		`SELECT day, generated_kwh FROM daily_generation
		 WHERE plant_id = $1 AND day >= current_date - interval '3 days'
		 ORDER BY day DESC LIMIT 1`, plantID,
	).Scan(&day, &kwh)
	if err == nil {
		dateStr := day.Format("2006-01-02")
		resp.Date = &dateStr
		resp.GeneratedKWh = &kwh
	} else if !isNoRows(err) {
		writeInternalError(w, err, "falha ao consultar geração de hoje")
		return
	}

	bandeira, bandeiraValorKWh, err := s.latestBandeira(ctx, plantID)
	if err != nil {
		writeInternalError(w, err, "falha ao consultar bandeira tarifária")
		return
	}
	resp.Bandeira = bandeira
	resp.BandeiraValorKWh = bandeiraValorKWh

	writeJSON(w, http.StatusOK, resp)
}
