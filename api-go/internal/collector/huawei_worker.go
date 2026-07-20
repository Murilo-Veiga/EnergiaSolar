package collector

import (
	"context"
	"fmt"
	"time"

	"energiasolar-api/internal/huawei"
)

// ensureHuaweiDiscovery descobre stationCode/devDn 1x e persiste no
// Postgres — ciclos seguintes (e restarts do container) reaproveitam o
// cache em vez de rediscobrir, mesmo espírito de station_code/dev_dn
// descobertos 1x em collector/main.py::main() (Python), só que agora
// sobrevive a restart porque fica no banco, não em memória.
func ensureHuaweiDiscovery(ctx context.Context, cred CredentialRow, client *huawei.Client, cb func(stationCode, devDn string) error) (stationCode, devDn string, err error) {
	if cred.DiscoveredStationCode != nil && cred.DiscoveredDevDn != nil {
		return *cred.DiscoveredStationCode, *cred.DiscoveredDevDn, nil
	}
	if err := client.Login(ctx); err != nil {
		return "", "", fmt.Errorf("login pra descoberta: %w", err)
	}
	stations, err := client.GetStationList(ctx)
	if err != nil {
		return "", "", fmt.Errorf("getStationList: %w", err)
	}
	if len(stations) == 0 {
		return "", "", fmt.Errorf("conta huawei sem nenhuma usina cadastrada")
	}
	stationCode, _ = stations[0]["stationCode"].(string)

	devices, err := client.GetDevList(ctx, stationCode)
	if err != nil {
		return "", "", fmt.Errorf("getDevList: %w", err)
	}
	if len(devices) == 0 {
		return "", "", fmt.Errorf("usina huawei sem nenhum inversor cadastrado")
	}
	devDn, _ = devices[0]["devDn"].(string)

	if err := cb(stationCode, devDn); err != nil {
		return "", "", fmt.Errorf("gravando descoberta: %w", err)
	}
	return stationCode, devDn, nil
}

type huaweiPollResult struct {
	powerKW      float64
	dayKWh       float64
	temperatureC *float64
	hasAlarm     bool
	alarmDetail  *string
}

// pollHuawei é a porta de _collect_huawei em collector/main.py (Python).
// getAlarmList roda no mesmo ciclo que o resto (sem cadência separada):
// com 30min de intervalo, já ficamos acima do limite mais restrito medido
// no Python (~592-888s) — ver plano, seção "Collector: worker por
// credencial". Uma falha só no alarme não derruba o ciclo inteiro.
func pollHuawei(ctx context.Context, client *huawei.Client, stationCode, devDn string) (huaweiPollResult, error) {
	if err := client.Login(ctx); err != nil {
		return huaweiPollResult{}, fmt.Errorf("login: %w", err)
	}

	stationKpiList, err := client.GetStationRealKpi(ctx, stationCode)
	if err != nil {
		return huaweiPollResult{}, fmt.Errorf("getStationRealKpi: %w", err)
	}
	if len(stationKpiList) == 0 {
		return huaweiPollResult{}, fmt.Errorf("getStationRealKpi: resposta vazia")
	}
	stationKpi, _ := stationKpiList[0]["dataItemMap"].(map[string]any)

	devKpiList, err := client.GetDevRealKpi(ctx, devDn, huawei.DevTypeID)
	if err != nil {
		return huaweiPollResult{}, fmt.Errorf("getDevRealKpi: %w", err)
	}
	if len(devKpiList) == 0 {
		return huaweiPollResult{}, fmt.Errorf("getDevRealKpi: resposta vazia")
	}
	devKpi, _ := devKpiList[0]["dataItemMap"].(map[string]any)

	var alarms []map[string]any
	if list, err := client.GetAlarmList(ctx, stationCode); err == nil {
		alarms = list
	}
	// Uma falha só na consulta de alarme não derruba o ciclo — igual ao
	// Python, que mantém o último status conhecido nesse caso. Aqui,
	// simplesmente assume "sem alarme novo" neste ciclo em vez de manter
	// cache (o próximo ciclo, 30min depois, tenta de novo).

	dayKWh := floatFromMap(devKpi, "day_cap")
	if dayKWh == 0 {
		dayKWh = floatFromMap(stationKpi, "day_power")
	}

	return huaweiPollResult{
		powerKW:      floatFromMap(devKpi, "active_power"),
		dayKWh:       dayKWh,
		temperatureC: floatPtrFromMap(devKpi, "temperature"),
		hasAlarm:     len(alarms) > 0,
		alarmDetail:  extractAlarmDetail(alarms),
	}, nil
}

// RunHuaweiWorker roda o ciclo de coleta de 1 credencial Huawei até o
// contexto ser cancelado (ver supervisor.go) — 1 goroutine por
// credencial, cada uma com seu próprio estado de reset diário e contador
// de falhas (nunca compartilhado entre inversores, diferente do Python
// original que usava mapas globais por nome de inversor).
func RunHuaweiWorker(ctx context.Context, deps Deps, cred CredentialRow, settings SystemSettings) {
	log := deps.Log.With("brand", "huawei", "plant_id", cred.PlantID, "credential_id", cred.ID)

	var secrets huaweiSecrets
	if err := decryptJSON(cred.CredentialsEncrypted, deps.EncryptionKey, &secrets); err != nil {
		log.Error("falha ao decifrar credencial, worker não vai iniciar", "error", err)
		return
	}
	// Credencial sem URL própria: cai na URL padrão configurada em
	// Administração > Configuração do sistema (se vazia também, o client
	// usa o próprio default hardcoded — ver huawei.DefaultBaseURL).
	baseURL := secrets.BaseURL
	if baseURL == "" {
		baseURL = settings.HuaweiBaseURL
	}
	client, err := huawei.NewClient(secrets.Username, secrets.SystemCode, baseURL)
	if err != nil {
		log.Error("falha ao criar cliente huawei, worker não vai iniciar", "error", err)
		return
	}

	stationCode, devDn, err := ensureHuaweiDiscovery(ctx, cred, client, func(sc, dd string) error {
		_, err := deps.DB.Exec(ctx,
			`UPDATE inverter_credentials SET discovered_station_code = $1, discovered_dev_dn = $2 WHERE id = $3`,
			sc, dd, cred.ID)
		return err
	})
	if err != nil {
		log.Error("falha na descoberta inicial, worker não vai iniciar", "error", err)
		return
	}
	log.Info("worker huawei iniciado", "station_code", stationCode, "dev_dn", devDn)

	guard := &resetGuardState{}
	failures := 0

	poll := func() {
		now := time.Now()
		result, err := pollHuawei(ctx, client, stationCode, devDn)
		if err != nil {
			failures++
			log.Warn("falha na coleta", "consecutive_failures", failures, "error", err)
			if werr := writeCollectorHealth(ctx, deps.DB, cred.PlantID, "huawei", failures, err.Error()); werr != nil {
				log.Error("falha ao gravar collector_health", "error", werr)
			}
			return
		}
		failures = 0
		dayKWh := guard.apply(now, result.powerKW, result.dayKWh)

		if err := writeInverterStatus(ctx, deps.DB, cred.PlantID, "huawei", result.powerKW, dayKWh, result.temperatureC); err != nil {
			log.Error("falha ao gravar inverter_status", "error", err)
			return
		}
		if err := writeCollectorHealth(ctx, deps.DB, cred.PlantID, "huawei", 0, ""); err != nil {
			log.Error("falha ao gravar collector_health", "error", err)
		}
		if err := recomputePlantTotals(ctx, deps.DB, cred.PlantID, true, result.hasAlarm, result.alarmDetail); err != nil {
			log.Error("falha ao recalcular totais da usina", "error", err)
		}
	}

	poll()
	ticker := time.NewTicker(workerInterval(settings))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("worker huawei encerrado")
			return
		case <-ticker.C:
			poll()
		}
	}
}
