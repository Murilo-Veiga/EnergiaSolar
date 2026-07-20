package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"energiasolar-api/internal/auth"
	"energiasolar-api/internal/foxess"
	"energiasolar-api/internal/huawei"
)

// inverterCredentialIn cobre os campos de ambas as marcas — só os
// relevantes pra `brand` são exigidos (ver encodeCredentialSecrets).
type inverterCredentialIn struct {
	Brand      string `json:"brand"`
	Enabled    *bool  `json:"enabled,omitempty"`
	Username   string `json:"username,omitempty"`
	SystemCode string `json:"system_code,omitempty"`
	APIKey     string `json:"api_key,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
}

// encodeCredentialSecrets serializa os campos certos pra cada marca —
// mesmo formato JSON que collector-go espera decifrar (ver
// internal/collector/credential.go: huaweiSecrets/foxessSecrets).
func encodeCredentialSecrets(in inverterCredentialIn) (string, error) {
	switch in.Brand {
	case "huawei":
		if in.Username == "" || in.SystemCode == "" {
			return "", fmt.Errorf("username e system_code são obrigatórios pra huawei")
		}
		b, _ := json.Marshal(map[string]string{
			"username": in.Username, "system_code": in.SystemCode, "base_url": in.BaseURL,
		})
		return string(b), nil
	case "foxess":
		if in.APIKey == "" {
			return "", fmt.Errorf("api_key é obrigatória pra foxess")
		}
		b, _ := json.Marshal(map[string]string{"api_key": in.APIKey, "base_url": in.BaseURL})
		return string(b), nil
	default:
		return "", fmt.Errorf("brand precisa ser 'huawei' ou 'foxess'")
	}
}

type inverterCredentialResponse struct {
	ID         string              `json:"id"`
	Brand      string              `json:"brand"`
	Enabled    bool                `json:"enabled"`
	Configured bool                `json:"configured"`
	DeviceInfo *inverterDeviceInfo `json:"device_info,omitempty"`
}

// handleListInverterCredentials devolve o último retrato CONHECIDO de cada
// inversor (station_code/dev_dn/device_sn já descobertos + último ponto de
// inverter_status) — nunca chama a API do fabricante ao vivo, pra listar
// ficar rápido mesmo com N credenciais. Fica desatualizado só se o
// coletor/o backfill nunca rodaram ainda pra essa credencial (recém criada
// e ainda sem 1º ciclo) — POST/PUT preenchem isso na hora (ver abaixo).
func (s *Server) handleListInverterCredentials(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}

	rows, err := s.DB.Query(r.Context(), `
		SELECT c.id, c.brand, c.enabled,
		       c.discovered_station_code, c.discovered_dev_dn, c.discovered_device_sn,
		       latest.power_kw, latest.day_kwh, latest.temperature_c
		FROM inverter_credentials c
		LEFT JOIN LATERAL (
		  SELECT power_kw, day_kwh, temperature_c
		  FROM inverter_status
		  WHERE plant_id = c.plant_id AND inverter = c.brand
		  ORDER BY recorded_at DESC LIMIT 1
		) latest ON true
		WHERE c.plant_id = $1
		ORDER BY c.brand`, plantID)
	if err != nil {
		writeInternalError(w, err, "falha ao listar credenciais")
		return
	}
	defer rows.Close()

	result := []inverterCredentialResponse{}
	for rows.Next() {
		var c inverterCredentialResponse
		var info inverterDeviceInfo
		if err := rows.Scan(&c.ID, &c.Brand, &c.Enabled,
			&info.StationCode, &info.DevDn, &info.DeviceSN,
			&info.PowerKW, &info.DayKWh, &info.TemperatureC); err != nil {
			writeInternalError(w, err, "falha ao ler credenciais")
			return
		}
		c.Configured = true
		if info.StationCode != nil || info.DevDn != nil || info.DeviceSN != nil || info.PowerKW != nil {
			c.DeviceInfo = &info
		}
		result = append(result, c)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCreateInverterCredential(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}

	var in inverterCredentialIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}
	secretJSON, err := encodeCredentialSecrets(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	encrypted, err := auth.EncryptCredential(secretJSON, s.EncryptionKey)
	if err != nil {
		writeInternalError(w, err, "falha ao cifrar credencial")
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	var credID string
	err = s.DB.QueryRow(r.Context(),
		`INSERT INTO inverter_credentials (plant_id, brand, enabled, credentials_encrypted)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		plantID, in.Brand, enabled, encrypted,
	).Scan(&credID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			writeError(w, http.StatusConflict, "já existe uma credencial dessa marca pra essa usina — use PUT pra atualizar")
			return
		}
		writeInternalError(w, err, "falha ao salvar credencial")
		return
	}

	resp := inverterCredentialResponse{ID: credID, Brand: in.Brand, Enabled: enabled, Configured: true}
	resp.DeviceInfo = s.discoverAndPersistSnapshot(r.Context(), credID, in)
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleUpdateInverterCredential(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	credID := chi.URLParam(r, "credID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}

	var currentBrand string
	err := s.DB.QueryRow(r.Context(),
		`SELECT brand FROM inverter_credentials WHERE id = $1 AND plant_id = $2`, credID, plantID,
	).Scan(&currentBrand)
	if err != nil {
		if isNoRows(err) {
			writeError(w, http.StatusNotFound, "credencial não encontrada")
			return
		}
		writeInternalError(w, err, "falha ao consultar credencial")
		return
	}

	var in inverterCredentialIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}
	if in.Brand == "" {
		in.Brand = currentBrand
	}

	hasNewSecret := in.Username != "" || in.SystemCode != "" || in.APIKey != ""
	if hasNewSecret {
		// Usa "=" (não ":=") em cada atribuição de err abaixo — com ":="
		// aqui, o "err" ficaria restrito a este bloco (shadowing) e o
		// "if err != nil" depois do if/else nunca veria o resultado real
		// do Exec, checando sempre o err antigo (nil) da consulta de
		// currentBrand acima.
		var secretJSON string
		secretJSON, err = encodeCredentialSecrets(in)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var encrypted []byte
		encrypted, err = auth.EncryptCredential(secretJSON, s.EncryptionKey)
		if err != nil {
			writeInternalError(w, err, "falha ao cifrar credencial")
			return
		}
		enabled := true
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		// Zera a descoberta em cache: uma credencial nova pode apontar
		// pra uma conta/usina diferente, então station_code/dev_dn/
		// device_sn antigos não valem mais (collector-go redescobre no
		// próximo ciclo).
		_, err = s.DB.Exec(r.Context(),
			`UPDATE inverter_credentials
			 SET enabled = $1, credentials_encrypted = $2,
			     discovered_station_code = NULL, discovered_dev_dn = NULL, discovered_device_sn = NULL
			 WHERE id = $3 AND plant_id = $4`,
			enabled, encrypted, credID, plantID)
	} else if in.Enabled != nil {
		_, err = s.DB.Exec(r.Context(),
			`UPDATE inverter_credentials SET enabled = $1 WHERE id = $2 AND plant_id = $3`,
			*in.Enabled, credID, plantID)
	}
	if err != nil {
		writeInternalError(w, err, "falha ao atualizar credencial")
		return
	}

	var enabled bool
	if err := s.DB.QueryRow(r.Context(), `SELECT enabled FROM inverter_credentials WHERE id = $1`, credID).Scan(&enabled); err != nil {
		writeInternalError(w, err, "falha ao consultar credencial atualizada")
		return
	}

	resp := inverterCredentialResponse{ID: credID, Brand: in.Brand, Enabled: enabled, Configured: true}
	if hasNewSecret {
		// Credencial mudou de verdade — busca ao vivo pra usuário ver o
		// inversor certo na hora, sem esperar o coletor.
		resp.DeviceInfo = s.discoverAndPersistSnapshot(r.Context(), credID, in)
	} else {
		// Só enabled mudou — não vale a pena gastar uma chamada à API do
		// fabricante (a Huawei em particular tem rate limit apertado),
		// devolve o último retrato já em cache.
		resp.DeviceInfo = s.loadCachedDeviceInfo(r.Context(), credID)
	}
	writeJSON(w, http.StatusOK, resp)
}

// discoverAndPersistSnapshot busca o inversor AO VIVO na API do
// fabricante (mesma descoberta que o coletor faria no 1º ciclo) e já
// grava station_code/dev_dn/device_sn no banco, pra o coletor reaproveitar
// em vez de descobrir de novo. Nunca falha a requisição por causa disso —
// se a busca ao vivo der erro, devolve o erro dentro de DeviceInfo.Error e
// deixa pro próximo ciclo do coletor tentar de novo.
func (s *Server) discoverAndPersistSnapshot(ctx context.Context, credID string, in inverterCredentialIn) *inverterDeviceInfo {
	baseURL := in.BaseURL
	if baseURL == "" {
		if settings, err := s.loadSystemSettings(ctx); err == nil {
			if in.Brand == "huawei" {
				baseURL = settings.HuaweiBaseURL
			} else if in.Brand == "foxess" {
				baseURL = settings.FoxessBaseURL
			}
		}
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	info := fetchLiveInverterSnapshot(fetchCtx, in, baseURL)

	if info.StationCode != nil || info.DevDn != nil || info.DeviceSN != nil {
		if _, err := s.DB.Exec(ctx,
			`UPDATE inverter_credentials
			 SET discovered_station_code = COALESCE($1, discovered_station_code),
			     discovered_dev_dn = COALESCE($2, discovered_dev_dn),
			     discovered_device_sn = COALESCE($3, discovered_device_sn)
			 WHERE id = $4`,
			info.StationCode, info.DevDn, info.DeviceSN, credID,
		); err != nil {
			slog.Default().Error("falha ao gravar descoberta imediata", "credential_id", credID, "error", err)
		}
	}
	return &info
}

func (s *Server) loadCachedDeviceInfo(ctx context.Context, credID string) *inverterDeviceInfo {
	var info inverterDeviceInfo
	err := s.DB.QueryRow(ctx, `
		SELECT c.discovered_station_code, c.discovered_dev_dn, c.discovered_device_sn,
		       latest.power_kw, latest.day_kwh, latest.temperature_c
		FROM inverter_credentials c
		LEFT JOIN LATERAL (
		  SELECT power_kw, day_kwh, temperature_c
		  FROM inverter_status
		  WHERE plant_id = c.plant_id AND inverter = c.brand
		  ORDER BY recorded_at DESC LIMIT 1
		) latest ON true
		WHERE c.id = $1`, credID,
	).Scan(&info.StationCode, &info.DevDn, &info.DeviceSN, &info.PowerKW, &info.DayKWh, &info.TemperatureC)
	if err != nil {
		return nil
	}
	if info.StationCode == nil && info.DevDn == nil && info.DeviceSN == nil && info.PowerKW == nil {
		return nil
	}
	return &info
}

func (s *Server) handleDeleteInverterCredential(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	credID := chi.URLParam(r, "credID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}
	if _, err := s.DB.Exec(r.Context(),
		`DELETE FROM inverter_credentials WHERE id = $1 AND plant_id = $2`, credID, plantID); err != nil {
		writeInternalError(w, err, "falha ao remover credencial")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type testConnectionResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// handleTestInverterCredential valida uma credencial ANTES de salvar —
// recebe o mesmo corpo do POST de criação, mas não grava nada no banco.
func (s *Server) handleTestInverterCredential(w http.ResponseWriter, r *http.Request) {
	plantID := chi.URLParam(r, "plantID")
	if _, err := s.authorizePlant(r.Context(), plantID); err != nil {
		respondPlantAuthError(w, err)
		return
	}

	var in inverterCredentialIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// Sem URL própria na credencial: usa o padrão global (Administração >
	// Configuração do sistema) antes de cair no default hardcoded do
	// client — mesmo fallback aplicado pelos workers em produção (ver
	// internal/collector/huawei_worker.go / foxess_worker.go), pra o teste
	// refletir a URL que vai ser usada de verdade.
	baseURL := in.BaseURL
	if baseURL == "" {
		if settings, err := s.loadSystemSettings(ctx); err == nil {
			if in.Brand == "huawei" {
				baseURL = settings.HuaweiBaseURL
			} else if in.Brand == "foxess" {
				baseURL = settings.FoxessBaseURL
			}
		}
	}

	switch in.Brand {
	case "huawei":
		if in.Username == "" || in.SystemCode == "" {
			writeError(w, http.StatusBadRequest, "username e system_code são obrigatórios")
			return
		}
		client, err := huawei.NewClient(in.Username, in.SystemCode, baseURL)
		if err != nil {
			writeJSON(w, http.StatusOK, testConnectionResult{Message: err.Error()})
			return
		}
		if err := client.Login(ctx); err != nil {
			writeJSON(w, http.StatusOK, testConnectionResult{Message: err.Error()})
			return
		}
		stations, err := client.GetStationList(ctx)
		if err != nil {
			writeJSON(w, http.StatusOK, testConnectionResult{Message: err.Error()})
			return
		}
		if len(stations) == 0 {
			writeJSON(w, http.StatusOK, testConnectionResult{Message: "login funcionou, mas nenhuma usina foi encontrada nessa conta"})
			return
		}
		writeJSON(w, http.StatusOK, testConnectionResult{Success: true, Message: fmt.Sprintf("conectado — %d usina(s) encontrada(s)", len(stations))})

	case "foxess":
		if in.APIKey == "" {
			writeError(w, http.StatusBadRequest, "api_key é obrigatória")
			return
		}
		client := foxess.NewClient(in.APIKey, baseURL)
		devices, err := client.GetDeviceList(ctx)
		if err != nil {
			writeJSON(w, http.StatusOK, testConnectionResult{Message: err.Error()})
			return
		}
		if len(devices) == 0 {
			writeJSON(w, http.StatusOK, testConnectionResult{Message: "conectou, mas nenhum inversor foi encontrado nessa conta"})
			return
		}
		writeJSON(w, http.StatusOK, testConnectionResult{Success: true, Message: fmt.Sprintf("conectado — %d inversor(es) encontrado(s)", len(devices))})

	default:
		writeError(w, http.StatusBadRequest, "brand precisa ser 'huawei' ou 'foxess'")
	}
}
