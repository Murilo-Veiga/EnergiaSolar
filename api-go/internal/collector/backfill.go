package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"energiasolar-api/internal/brtime"
	"energiasolar-api/internal/foxess"
	"energiasolar-api/internal/huawei"
)

// DebugHuaweiKpiStationDay chama getKpiStationDay pra 1 credencial e
// devolve a resposta CRUA (sem tentar extrair day_power) — usado só pra
// diagnosticar o nome real dos campos antes de confiar no backfill (ver
// cmd/backfill-history -debug-huawei).
func DebugHuaweiKpiStationDay(ctx context.Context, deps Deps, credentialID string) ([]map[string]any, error) {
	rows, err := FetchEnabledCredentials(ctx, deps.DB)
	if err != nil {
		return nil, err
	}
	var cred CredentialRow
	found := false
	for _, c := range rows {
		if c.ID == credentialID {
			cred = c
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("credencial %s não encontrada (ou não habilitada)", credentialID)
	}
	if cred.DiscoveredStationCode == nil {
		return nil, fmt.Errorf("station_code ainda não descoberto")
	}
	settings, err := loadSystemSettings(ctx, deps.DB)
	if err != nil {
		return nil, err
	}
	var secrets huaweiSecrets
	if err := decryptJSON(cred.CredentialsEncrypted, deps.EncryptionKey, &secrets); err != nil {
		return nil, err
	}
	baseURL := secrets.BaseURL
	if baseURL == "" {
		baseURL = settings.HuaweiBaseURL
	}
	client, err := huawei.NewClient(secrets.Username, secrets.SystemCode, baseURL)
	if err != nil {
		return nil, err
	}
	if err := client.Login(ctx); err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	return client.GetKpiStationDay(ctx, *cred.DiscoveredStationCode, time.Now().UnixMilli())
}

// DebugFoxessDeviceList chama device/list pra 1 credencial FoxESS e devolve
// a resposta CRUA (todos os campos de cada device, não só deviceSN) — usado
// só pra checar se a API expõe um campo de status/conectividade nativo que
// hoje o worker ignora (ver cmd/backfill-history -debug-foxess-devicelist).
func DebugFoxessDeviceList(ctx context.Context, deps Deps, credentialID string) ([]map[string]any, error) {
	rows, err := FetchEnabledCredentials(ctx, deps.DB)
	if err != nil {
		return nil, err
	}
	var cred CredentialRow
	found := false
	for _, c := range rows {
		if c.ID == credentialID {
			cred = c
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("credencial %s não encontrada (ou não habilitada)", credentialID)
	}
	settings, err := loadSystemSettings(ctx, deps.DB)
	if err != nil {
		return nil, err
	}
	var secrets foxessSecrets
	if err := decryptJSON(cred.CredentialsEncrypted, deps.EncryptionKey, &secrets); err != nil {
		return nil, err
	}
	baseURL := secrets.BaseURL
	if baseURL == "" {
		baseURL = settings.FoxessBaseURL
	}
	client := foxess.NewClient(secrets.APIKey, baseURL)
	return client.GetDeviceList(ctx)
}

// DebugFoxessRealQuery chama device/real/query pra 1 credencial FoxESS e
// devolve a resposta CRUA (todos os campos de cada item de "datas", não só
// variable/value) — debug temporário pra checar se existe timestamp por
// variável.
func DebugFoxessRealQuery(ctx context.Context, deps Deps, credentialID string) ([]map[string]any, error) {
	rows, err := FetchEnabledCredentials(ctx, deps.DB)
	if err != nil {
		return nil, err
	}
	var cred CredentialRow
	found := false
	for _, c := range rows {
		if c.ID == credentialID {
			cred = c
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("credencial %s não encontrada (ou não habilitada)", credentialID)
	}
	if cred.DiscoveredDeviceSN == nil {
		return nil, fmt.Errorf("device_sn ainda não descoberto")
	}
	settings, err := loadSystemSettings(ctx, deps.DB)
	if err != nil {
		return nil, err
	}
	var secrets foxessSecrets
	if err := decryptJSON(cred.CredentialsEncrypted, deps.EncryptionKey, &secrets); err != nil {
		return nil, err
	}
	baseURL := secrets.BaseURL
	if baseURL == "" {
		baseURL = settings.FoxessBaseURL
	}
	client := foxess.NewClient(secrets.APIKey, baseURL)
	return client.GetRealQuery(ctx, *cred.DiscoveredDeviceSN, []string{"generationPower", "todayYield", "invTemperation"})
}

// DebugHuaweiDevRealKpi chama getDevRealKpi pra 1 credencial Huawei e
// devolve a resposta CRUA (inclusive dataItemMap inteiro, não só
// active_power/temperature/day_cap) — usado só pra checar se a API expõe
// run_state (ou campo equivalente) que hoje o worker ignora (ver
// cmd/backfill-history -debug-huawei-devrealkpi).
func DebugHuaweiDevRealKpi(ctx context.Context, deps Deps, credentialID string) ([]map[string]any, error) {
	rows, err := FetchEnabledCredentials(ctx, deps.DB)
	if err != nil {
		return nil, err
	}
	var cred CredentialRow
	found := false
	for _, c := range rows {
		if c.ID == credentialID {
			cred = c
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("credencial %s não encontrada (ou não habilitada)", credentialID)
	}
	if cred.DiscoveredDevDn == nil {
		return nil, fmt.Errorf("dev_dn ainda não descoberto")
	}
	settings, err := loadSystemSettings(ctx, deps.DB)
	if err != nil {
		return nil, err
	}
	var secrets huaweiSecrets
	if err := decryptJSON(cred.CredentialsEncrypted, deps.EncryptionKey, &secrets); err != nil {
		return nil, err
	}
	baseURL := secrets.BaseURL
	if baseURL == "" {
		baseURL = settings.HuaweiBaseURL
	}
	client, err := huawei.NewClient(secrets.Username, secrets.SystemCode, baseURL)
	if err != nil {
		return nil, err
	}
	if err := client.Login(ctx); err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	return client.GetDevRealKpi(ctx, *cred.DiscoveredDevDn, huawei.DevTypeID)
}

// BackfillOptions parametriza uma execução de RunHistoryBackfill.
type BackfillOptions struct {
	// Days é quantos dias PASSADOS (sem contar hoje — hoje já é coberto
	// pelo worker de tempo real) tentar recuperar, contando de ontem pra
	// trás.
	Days int
	// PlantID filtra pra uma única usina; vazio = todas as usinas com
	// credencial habilitada.
	PlantID string
	// Write persiste em inverter_status/daily_generation; false só
	// calcula e devolve os valores (dry-run), sem tocar no banco.
	Write bool
}

// BackfillDayResult é 1 dia de geração recuperado pra 1 credencial.
type BackfillDayResult struct {
	PlantID string
	Brand   string
	Day     time.Time // meia-noite BRT do dia, já convertida pra UTC
	DayKWh  float64
}

// RunHistoryBackfill busca no histórico das APIs Huawei/FoxESS a geração
// diária dos últimos Days dias (excluindo hoje) pra cada credencial
// habilitada, e opcionalmente grava em inverter_status + daily_generation.
//
// Diferente do worker de tempo real (que só lê o ponto instantâneo), aqui
// usamos os endpoints de KPI HISTÓRICO de cada fabricante:
//   - Huawei: getKpiStationDay (1 chamada cobre o mês CALENDÁRIO inteiro
//     que contém a data pedida — se o intervalo cruzar 2 meses, chamamos
//     1x por mês).
//   - FoxESS: não existe um endpoint de relatório diário pronto no client
//     atual, então usamos a curva intradiária (device/history/query) e
//     pegamos o último ponto de "todayYield" (contador que já acumula o
//     dia inteiro) — 1 chamada por dia.
//
// Nome de campo do FoxESS ("todayYield") é o mesmo já usado e validado
// pelo worker de tempo real (pollFoxess). Já o campo do KPI diário da
// Huawei ("PVYield") teve que ser confirmado contra uma resposta real
// (cmd/backfill-history -debug-huawei) — o nome do worker de tempo real
// ("day_power") NÃO existe nesse endpoint histórico. Ainda assim, RODE
// PRIMEIRO COM Write=false E CONFIRA os valores contra o app do
// fabricante antes de persistir — ver cmd/backfill-history.
func RunHistoryBackfill(ctx context.Context, deps Deps, opts BackfillOptions) ([]BackfillDayResult, error) {
	if opts.Days <= 0 {
		opts.Days = 30
	}

	creds, err := FetchEnabledCredentials(ctx, deps.DB)
	if err != nil {
		return nil, fmt.Errorf("listando credenciais: %w", err)
	}
	settings, err := loadSystemSettings(ctx, deps.DB)
	if err != nil {
		return nil, fmt.Errorf("carregando configurações do sistema: %w", err)
	}

	// "Hoje" em BRT já é coberto pelo worker de tempo real — o backfill só
	// cobre o passado, pra não competir/sobrescrever o que o worker está
	// gravando agora.
	today := brtime.StartOfDay()
	endDay := today.AddDate(0, 0, -1)
	startDay := endDay.AddDate(0, 0, -(opts.Days - 1))

	var results []BackfillDayResult
	for _, cred := range creds {
		if opts.PlantID != "" && cred.PlantID != opts.PlantID {
			continue
		}
		log := deps.Log.With("brand", cred.Brand, "plant_id", cred.PlantID, "credential_id", cred.ID)

		var (
			days []BackfillDayResult
			err  error
		)
		switch cred.Brand {
		case "huawei":
			days, err = backfillHuawei(ctx, deps, cred, settings, startDay, endDay)
		case "foxess":
			days, err = backfillFoxess(ctx, deps, cred, settings, startDay, endDay)
		default:
			continue
		}
		if err != nil {
			log.Error("falha no backfill", "error", err)
			continue
		}
		log.Info("backfill calculado", "dias_encontrados", len(days))
		results = append(results, days...)
	}

	if opts.Write {
		if err := writeBackfillResults(ctx, deps.DB, results); err != nil {
			return results, fmt.Errorf("gravando backfill: %w", err)
		}
	}
	return results, nil
}

// backfillHuawei chama getKpiStationDay 1x por mês calendário cruzado pelo
// intervalo [startDay, endDay] e filtra pro intervalo pedido.
func backfillHuawei(ctx context.Context, deps Deps, cred CredentialRow, settings SystemSettings, startDay, endDay time.Time) ([]BackfillDayResult, error) {
	if cred.DiscoveredStationCode == nil {
		return nil, fmt.Errorf("station_code ainda não descoberto (o worker precisa ter rodado ao menos 1 ciclo antes)")
	}
	var secrets huaweiSecrets
	if err := decryptJSON(cred.CredentialsEncrypted, deps.EncryptionKey, &secrets); err != nil {
		return nil, err
	}
	baseURL := secrets.BaseURL
	if baseURL == "" {
		baseURL = settings.HuaweiBaseURL
	}
	client, err := huawei.NewClient(secrets.Username, secrets.SystemCode, baseURL)
	if err != nil {
		return nil, err
	}
	if err := client.Login(ctx); err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	var results []BackfillDayResult
	seen := map[string]bool{} // evita rechamar o mesmo mês 2x
	for day := startDay; !day.After(endDay); day = day.AddDate(0, 0, 1) {
		monthKey := day.In(brtime.Location).Format("2006-01")
		if seen[monthKey] {
			continue
		}
		seen[monthKey] = true

		// A NBI da Huawei tem limite de frequência entre chamadas à mesma
		// interface (ver comentário em internal/huawei/client.go); como
		// aqui é só 1-2 chamadas por credencial (1 por mês cruzado), não
		// precisa de espera entre elas.
		collectTimeMs := day.In(brtime.Location).UnixMilli()
		entries, err := client.GetKpiStationDay(ctx, *cred.DiscoveredStationCode, collectTimeMs)
		if err != nil {
			return nil, fmt.Errorf("getKpiStationDay (%s): %w", monthKey, err)
		}
		for _, entry := range entries {
			collectTime, ok := entry["collectTime"]
			if !ok {
				continue
			}
			ms := toInt64(collectTime)
			if ms == 0 {
				continue
			}
			entryDay := brtime.DayFromUnixMillis(ms)
			if entryDay.Before(startDay) || entryDay.After(endDay) {
				continue
			}
			dataItemMap, _ := entry["dataItemMap"].(map[string]any)
			// Confirmado contra resposta real (cmd/backfill-history
			// -debug-huawei): getKpiStationDay NÃO tem campo "day_power"
			// (esse é só do getStationRealKpi, tempo real) — o KPI diário
			// histórico usa "PVYield" (kWh do dia, igual a "inverterYield"
			// nos testes feitos).
			dayKWh := floatFromMap(dataItemMap, "PVYield")
			results = append(results, BackfillDayResult{
				PlantID: cred.PlantID,
				Brand:   "huawei",
				Day:     entryDay,
				DayKWh:  dayKWh,
			})
		}
	}
	return results, nil
}

// backfillFoxess não tem endpoint de relatório diário pronto — usa a curva
// intradiária (device/history/query, até 24h por chamada) e pega o último
// valor de "todayYield" do dia como o total do dia.
func backfillFoxess(ctx context.Context, deps Deps, cred CredentialRow, settings SystemSettings, startDay, endDay time.Time) ([]BackfillDayResult, error) {
	if cred.DiscoveredDeviceSN == nil {
		return nil, fmt.Errorf("device_sn ainda não descoberto (o worker precisa ter rodado ao menos 1 ciclo antes)")
	}
	var secrets foxessSecrets
	if err := decryptJSON(cred.CredentialsEncrypted, deps.EncryptionKey, &secrets); err != nil {
		return nil, err
	}
	baseURL := secrets.BaseURL
	if baseURL == "" {
		baseURL = settings.FoxessBaseURL
	}
	client := foxess.NewClient(secrets.APIKey, baseURL)

	var results []BackfillDayResult
	for day := startDay; !day.After(endDay); day = day.AddDate(0, 0, 1) {
		dayLocal := day.In(brtime.Location)
		beginMs := dayLocal.UnixMilli()
		endMs := dayLocal.Add(24*time.Hour - time.Second).UnixMilli()

		raw, err := client.GetHistoryQuery(ctx, *cred.DiscoveredDeviceSN, []string{"todayYield"}, beginMs, endMs)
		if err != nil {
			return nil, fmt.Errorf("device/history/query (%s): %w", dayLocal.Format("2006-01-02"), err)
		}
		kwh, ok := lastFoxessCurvePoint(raw, "todayYield")
		if !ok {
			// Sem dado nesse dia (ex.: credencial ainda não existia) — não
			// é erro fatal, só pula o dia.
			continue
		}
		results = append(results, BackfillDayResult{
			PlantID: cred.PlantID,
			Brand:   "foxess",
			Day:     day,
			DayKWh:  kwh,
		})
		// Espaça as chamadas pra não estourar rate limit da OpenAPI
		// FoxESS — 1 dia por vez é bem mais chamadas que o Huawei (que
		// cobre o mês inteiro numa tacada só).
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return results, nil
}

// lastFoxessCurvePoint extrai o último ponto (maior "time") da variável
// pedida na resposta de device/history/query. Formato esperado (mesmo
// padrão de device/real/query, só que com uma lista "data" de pontos no
// tempo em vez de 1 valor): result: [{deviceSN, datas: [{variable, data:
// [{time, value}, ...]}]}].
func lastFoxessCurvePoint(raw any, variable string) (float64, bool) {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return 0, false
	}
	entry, ok := list[0].(map[string]any)
	if !ok {
		return 0, false
	}
	datas, ok := entry["datas"].([]any)
	if !ok {
		return 0, false
	}
	for _, d := range datas {
		dm, ok := d.(map[string]any)
		if !ok {
			continue
		}
		if name, _ := dm["variable"].(string); name != variable {
			continue
		}
		points, ok := dm["data"].([]any)
		if !ok || len(points) == 0 {
			return 0, false
		}
		last, ok := points[len(points)-1].(map[string]any)
		if !ok {
			return 0, false
		}
		return floatFromMap(last, "value"), true
	}
	return 0, false
}

func toInt64(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	}
	return 0
}

// writeBackfillResults grava cada resultado em inverter_status (1 linha
// por plant/inverter/dia, ao meio-dia BRT pra cair sem ambiguidade dentro
// do dia certo) e depois recalcula daily_generation somando todas as
// marcas de cada usina/dia — mesmo princípio de recomputePlantTotals, só
// que pro passado em vez de "hoje".
//
// Roda DELETE antes de cada INSERT pra ficar idempotente (rodar o backfill
// 2x não duplica linha) — seguro porque só mexe em dias estritamente no
// passado, nunca no dia de hoje que o worker de tempo real está gravando.
func writeBackfillResults(ctx context.Context, db *pgxpool.Pool, results []BackfillDayResult) error {
	if len(results) == 0 {
		return nil
	}

	plantDays := map[string]map[time.Time]bool{} // plant_id -> dias tocados
	for _, r := range results {
		recordedAt := r.Day.In(brtime.Location).Add(12 * time.Hour).UTC()

		if _, err := db.Exec(ctx,
			`DELETE FROM inverter_status
			 WHERE plant_id = $1 AND inverter = $2
			   AND (recorded_at AT TIME ZONE 'America/Sao_Paulo')::date = $3`,
			r.PlantID, r.Brand, r.Day.In(brtime.Location).Format("2006-01-02"),
		); err != nil {
			return fmt.Errorf("limpando dia anterior (%s/%s/%s): %w", r.PlantID, r.Brand, r.Day, err)
		}
		if _, err := db.Exec(ctx,
			`		INSERT INTO inverter_status (plant_id, inverter, recorded_at, power_kw, day_kwh, temperature_c, fault)
			 VALUES ($1, $2, $3, NULL, $4, NULL, false)`,
			r.PlantID, r.Brand, recordedAt, r.DayKWh,
		); err != nil {
			return fmt.Errorf("gravando inverter_status (%s/%s/%s): %w", r.PlantID, r.Brand, r.Day, err)
		}

		if plantDays[r.PlantID] == nil {
			plantDays[r.PlantID] = map[time.Time]bool{}
		}
		plantDays[r.PlantID][r.Day] = true
	}

	for plantID, days := range plantDays {
		for day := range days {
			var total float64
			err := db.QueryRow(ctx,
				`SELECT COALESCE(SUM(day_kwh), 0) FROM (
				   SELECT DISTINCT ON (inverter) inverter, day_kwh
				   FROM inverter_status
				   WHERE plant_id = $1 AND day_kwh IS NOT NULL
				     AND (recorded_at AT TIME ZONE 'America/Sao_Paulo')::date = $2
				   ORDER BY inverter, recorded_at DESC
				 ) sub`,
				plantID, day.In(brtime.Location).Format("2006-01-02"),
			).Scan(&total)
			if err != nil {
				return fmt.Errorf("somando geração do dia (%s/%s): %w", plantID, day, err)
			}
			if _, err := db.Exec(ctx,
				`INSERT INTO daily_generation (plant_id, day, generated_kwh)
				 VALUES ($1, $2, $3)
				 ON CONFLICT (plant_id, day) DO UPDATE SET generated_kwh = EXCLUDED.generated_kwh`,
				plantID, day.In(brtime.Location).Format("2006-01-02"), total,
			); err != nil {
				return fmt.Errorf("gravando daily_generation (%s/%s): %w", plantID, day, err)
			}
		}
	}
	return nil
}
