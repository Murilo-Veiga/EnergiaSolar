package collector

import (
	"context"
	"fmt"
	"time"

	"energiasolar-api/internal/foxess"
)

// ensureFoxessDiscovery é o equivalente de ensureHuaweiDiscovery pro
// deviceSN da FoxESS.
func ensureFoxessDiscovery(ctx context.Context, cred CredentialRow, client *foxess.Client, cb func(deviceSN string) error) (deviceSN string, err error) {
	if cred.DiscoveredDeviceSN != nil {
		return *cred.DiscoveredDeviceSN, nil
	}
	devices, err := client.GetDeviceList(ctx)
	if err != nil {
		return "", fmt.Errorf("device/list: %w", err)
	}
	if len(devices) == 0 {
		return "", fmt.Errorf("conta foxess sem nenhum inversor cadastrado")
	}
	deviceSN, _ = devices[0]["deviceSN"].(string)

	if err := cb(deviceSN); err != nil {
		return "", fmt.Errorf("gravando descoberta: %w", err)
	}
	return deviceSN, nil
}

type foxessPollResult struct {
	powerKW      float64
	dayKWh       float64
	temperatureC *float64
	online       bool
	lastOnlineAt *time.Time
}

// foxessTimeLayout é o formato do campo "time" de device/real/query, ex.
// "2026-07-20 18:00:44 BRT-0300" — o offset numérico é o que importa pro
// parse, o nome do fuso (BRT) é só decorativo e ignorado pelo Go.
const foxessTimeLayout = "2006-01-02 15:04:05 MST-0700"

// pollFoxess é a porta de _collect_foxess em collector/main.py (Python).
//
// Simplificação consciente vs. o Python original: não busca a curva
// intradiária de alta resolução via device/history/query (_fox_history_points)
// nesta primeira versão — só o ponto instantâneo a cada ciclo (30min),
// igual ao que a Huawei já tem. Fica como pendência se a curva mais fina
// da FoxESS fizer falta (ver README > "Limitações conhecidas").
//
// online vem do campo nativo "status" de device/list (1=online, 2=alarme,
// 3=offline — confirmado contra a API real, ver cmd/backfill-history
// -debug-foxess-devicelist), não de um timeout de coleta. Exige 1 chamada
// extra a device/list por ciclo, além do device/real/query já existente.
func pollFoxess(ctx context.Context, client *foxess.Client, sn string) (foxessPollResult, error) {
	devices, err := client.GetDeviceList(ctx)
	if err != nil {
		return foxessPollResult{}, fmt.Errorf("device/list: %w", err)
	}
	statusFound := false
	online := false
	for _, d := range devices {
		devSN, _ := d["deviceSN"].(string)
		if devSN != sn {
			continue
		}
		online = floatFromMap(d, "status") == 1
		statusFound = true
		break
	}
	if !statusFound {
		return foxessPollResult{}, fmt.Errorf("device/list: dispositivo %s não encontrado na lista", sn)
	}

	entries, err := client.GetRealQuery(ctx, sn, []string{"generationPower", "todayYield", "invTemperation"})
	if err != nil {
		return foxessPollResult{}, fmt.Errorf("device/real/query: %w", err)
	}
	if len(entries) == 0 {
		return foxessPollResult{}, fmt.Errorf("device/real/query: resposta vazia")
	}
	datas, ok := entries[0]["datas"].([]any)
	if !ok {
		return foxessPollResult{}, fmt.Errorf("device/real/query: campo 'datas' ausente ou em formato inesperado")
	}

	values := map[string]any{}
	for _, item := range datas {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["variable"].(string)
		values[name] = m["value"]
	}

	var lastOnlineAt *time.Time
	if raw, _ := entries[0]["time"].(string); raw != "" {
		if parsed, err := time.Parse(foxessTimeLayout, raw); err == nil {
			lastOnlineAt = &parsed
		}
	}

	return foxessPollResult{
		powerKW:      floatFromMap(values, "generationPower"),
		dayKWh:       floatFromMap(values, "todayYield"),
		temperatureC: floatPtrFromMap(values, "invTemperation"),
		online:       online,
		lastOnlineAt: lastOnlineAt,
	}, nil
}

// RunFoxessWorker é o equivalente de RunHuaweiWorker pra uma credencial
// FoxESS — sem alarme (getAlarmList é exclusivo da NBI da Huawei neste
// projeto).
func RunFoxessWorker(ctx context.Context, deps Deps, cred CredentialRow, settings SystemSettings) {
	log := deps.Log.With("brand", "foxess", "plant_id", cred.PlantID, "credential_id", cred.ID)

	var secrets foxessSecrets
	if err := decryptJSON(cred.CredentialsEncrypted, deps.EncryptionKey, &secrets); err != nil {
		log.Error("falha ao decifrar credencial, worker não vai iniciar", "error", err)
		return
	}
	baseURL := secrets.BaseURL
	if baseURL == "" {
		baseURL = settings.FoxessBaseURL
	}
	client := foxess.NewClient(secrets.APIKey, baseURL)

	deviceSN, err := ensureFoxessDiscovery(ctx, cred, client, func(sn string) error {
		_, err := deps.DB.Exec(ctx,
			`UPDATE inverter_credentials SET discovered_device_sn = $1 WHERE id = $2`,
			sn, cred.ID)
		return err
	})
	if err != nil {
		log.Error("falha na descoberta inicial, worker não vai iniciar", "error", err)
		return
	}
	log.Info("worker foxess iniciado", "device_sn", deviceSN)

	guard := &resetGuardState{}
	failures := 0

	poll := func() {
		now := time.Now()
		result, err := pollFoxess(ctx, client, deviceSN)
		if err != nil {
			failures++
			log.Warn("falha na coleta", "consecutive_failures", failures, "error", err)
			if werr := writeCollectorHealth(ctx, deps.DB, cred.PlantID, "foxess", failures, err.Error()); werr != nil {
				log.Error("falha ao gravar collector_health", "error", werr)
			}
			return
		}
		failures = 0
		dayKWh := guard.apply(now, result.powerKW, result.dayKWh)

		if err := writeInverterStatus(ctx, deps.DB, cred.PlantID, "foxess", result.powerKW, dayKWh, result.temperatureC, result.online, result.lastOnlineAt); err != nil {
			log.Error("falha ao gravar inverter_status", "error", err)
			return
		}
		if err := writeCollectorHealth(ctx, deps.DB, cred.PlantID, "foxess", 0, ""); err != nil {
			log.Error("falha ao gravar collector_health", "error", err)
		}
		if err := recomputePlantTotals(ctx, deps.DB, cred.PlantID, false, false, nil); err != nil {
			log.Error("falha ao recalcular totais da usina", "error", err)
		}
	}

	poll()
	ticker := time.NewTicker(workerInterval(settings))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("worker foxess encerrado")
			return
		case <-ticker.C:
			poll()
		}
	}
}
