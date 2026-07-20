package httpapi

import (
	"context"
	"strconv"

	"energiasolar-api/internal/foxess"
	"energiasolar-api/internal/huawei"
)

// inverterDeviceInfo é o retrato mais recente conhecido de 1 inversor —
// devolvido tanto pelo GET de listagem (a partir do cache no banco) quanto
// por POST/PUT (a partir de uma busca ao vivo, feita na hora).
type inverterDeviceInfo struct {
	StationCode  *string  `json:"station_code,omitempty"`
	DevDn        *string  `json:"dev_dn,omitempty"`
	DeviceSN     *string  `json:"device_sn,omitempty"`
	PowerKW      *float64 `json:"power_kw,omitempty"`
	DayKWh       *float64 `json:"day_kwh,omitempty"`
	TemperatureC *float64 `json:"temperature_c,omitempty"`
	// Error vem preenchido quando a busca ao vivo (POST/PUT) falhou — a
	// credencial já foi salva mesmo assim, o coletor tenta de novo no
	// próximo ciclo. Nunca aparece no GET de listagem (esse só lê cache).
	Error string `json:"error,omitempty"`
}

// fetchLiveInverterSnapshot faz a MESMA descoberta que o worker de tempo
// real faria no primeiro ciclo (ensureHuaweiDiscovery/ensureFoxessDiscovery
// em internal/collector), só que síncrono e disparado na hora de
// cadastrar/editar a credencial, pra usuário ver o inversor de verdade
// (não só "salvo com sucesso") sem esperar o próximo ciclo do coletor (até
// alguns minutos de atraso).
//
// Pequena duplicação intencional da lógica de pollHuawei/pollFoxess de
// internal/collector — mesmo princípio já usado em outras duplicações
// entre httpapi/collector neste projeto (ver README > "Estrutura do
// projeto"), evita import cruzado entre os dois pacotes.
func fetchLiveInverterSnapshot(ctx context.Context, in inverterCredentialIn, baseURL string) inverterDeviceInfo {
	switch in.Brand {
	case "huawei":
		return fetchHuaweiSnapshot(ctx, in, baseURL)
	case "foxess":
		return fetchFoxessSnapshot(ctx, in, baseURL)
	default:
		return inverterDeviceInfo{Error: "brand precisa ser 'huawei' ou 'foxess'"}
	}
}

func fetchHuaweiSnapshot(ctx context.Context, in inverterCredentialIn, baseURL string) inverterDeviceInfo {
	client, err := huawei.NewClient(in.Username, in.SystemCode, baseURL)
	if err != nil {
		return inverterDeviceInfo{Error: err.Error()}
	}
	if err := client.Login(ctx); err != nil {
		return inverterDeviceInfo{Error: err.Error()}
	}
	stations, err := client.GetStationList(ctx)
	if err != nil {
		return inverterDeviceInfo{Error: err.Error()}
	}
	if len(stations) == 0 {
		return inverterDeviceInfo{Error: "login funcionou, mas nenhuma usina foi encontrada nessa conta"}
	}
	stationCode, _ := stations[0]["stationCode"].(string)

	devices, err := client.GetDevList(ctx, stationCode)
	if err != nil {
		return inverterDeviceInfo{StationCode: &stationCode, Error: err.Error()}
	}
	if len(devices) == 0 {
		return inverterDeviceInfo{StationCode: &stationCode, Error: "usina encontrada, mas sem nenhum inversor cadastrado"}
	}
	devDn, _ := devices[0]["devDn"].(string)

	info := inverterDeviceInfo{StationCode: &stationCode, DevDn: &devDn}

	stationKpiList, err := client.GetStationRealKpi(ctx, stationCode)
	var stationKpi map[string]any
	if err == nil && len(stationKpiList) > 0 {
		stationKpi, _ = stationKpiList[0]["dataItemMap"].(map[string]any)
	}
	devKpiList, err := client.GetDevRealKpi(ctx, devDn, huawei.DevTypeID)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	if len(devKpiList) == 0 {
		info.Error = "inversor encontrado, mas sem dado de telemetria disponível agora"
		return info
	}
	devKpi, _ := devKpiList[0]["dataItemMap"].(map[string]any)

	dayKWh := floatFromMap(devKpi, "day_cap")
	if dayKWh == 0 {
		dayKWh = floatFromMap(stationKpi, "day_power")
	}
	powerKW := floatFromMap(devKpi, "active_power")
	info.PowerKW = &powerKW
	info.DayKWh = &dayKWh
	info.TemperatureC = floatPtrFromMap(devKpi, "temperature")
	return info
}

func fetchFoxessSnapshot(ctx context.Context, in inverterCredentialIn, baseURL string) inverterDeviceInfo {
	client := foxess.NewClient(in.APIKey, baseURL)
	devices, err := client.GetDeviceList(ctx)
	if err != nil {
		return inverterDeviceInfo{Error: err.Error()}
	}
	if len(devices) == 0 {
		return inverterDeviceInfo{Error: "conectou, mas nenhum inversor foi encontrado nessa conta"}
	}
	deviceSN, _ := devices[0]["deviceSN"].(string)
	info := inverterDeviceInfo{DeviceSN: &deviceSN}

	entries, err := client.GetRealQuery(ctx, deviceSN, []string{"generationPower", "todayYield", "invTemperation"})
	if err != nil {
		info.Error = err.Error()
		return info
	}
	if len(entries) == 0 {
		info.Error = "inversor encontrado, mas sem dado de telemetria disponível agora"
		return info
	}
	datas, ok := entries[0]["datas"].([]any)
	if !ok {
		info.Error = "resposta em formato inesperado"
		return info
	}
	values := map[string]any{}
	for _, item := range datas {
		if m, ok := item.(map[string]any); ok {
			if name, _ := m["variable"].(string); name != "" {
				values[name] = m["value"]
			}
		}
	}
	powerKW := floatFromMap(values, "generationPower")
	dayKWh := floatFromMap(values, "todayYield")
	info.PowerKW = &powerKW
	info.DayKWh = &dayKWh
	info.TemperatureC = floatPtrFromMap(values, "invTemperation")
	return info
}

func floatFromMap(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case string:
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f
		}
	}
	return 0
}

func floatPtrFromMap(m map[string]any, key string) *float64 {
	v, ok := m[key]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case float64:
		return &t
	case string:
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return &f
		}
	}
	return nil
}
