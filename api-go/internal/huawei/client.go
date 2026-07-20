// Package huawei é o cliente da Northbound Interface (NBI) oficial do
// FusionSolar — porta fiel de collector/huawei_client.py (Python).
package huawei

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
)

const DefaultBaseURL = "https://la5.fusionsolar.huawei.com"

// DevTypeID é fixo na taxonomia de tipos de dispositivo da NBI — 38 =
// inversor (HUAWEI_DEV_TYPE_ID em collector/main.py, Python).
const DevTypeID = 38

// Client é o cliente HTTP da NBI, com sessão (cookie) persistida entre
// chamadas — equivalente à requests.Session() do Python.
type Client struct {
	username   string
	systemCode string
	baseURL    string
	httpClient *http.Client
	xsrfToken  string
}

func NewClient(username, systemCode, baseURL string) (*Client, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("criando cookie jar: %w", err)
	}
	return &Client{
		username:   username,
		systemCode: systemCode,
		baseURL:    baseURL,
		httpClient: &http.Client{Jar: jar},
	}, nil
}

// Login autentica e guarda o xsrf-token (vem no HEADER da resposta, não no
// corpo) pras chamadas seguintes — sem rate limit, ao contrário das
// demais interfaces.
func (c *Client) Login(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{
		"userName":   c.username,
		"systemCode": c.systemCode,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/thirdData/login", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var data struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("login: decodificando resposta: %w", err)
	}
	if !data.Success {
		return fmt.Errorf("login falhou")
	}
	c.xsrfToken = resp.Header.Get("xsrf-token")
	return nil
}

func (c *Client) post(ctx context.Context, path string, body map[string]any) (any, error) {
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("XSRF-TOKEN", c.xsrfToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: lendo resposta: %w", path, err)
	}
	var data struct {
		Success bool `json:"success"`
		Data    any  `json:"data"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("%s: decodificando resposta: %w", path, err)
	}
	if !data.Success {
		return nil, fmt.Errorf("%s falhou: %s", path, string(raw))
	}
	return data.Data, nil
}

// asSliceOfMaps converte o "data" genérico (any) da resposta num slice de
// mapas — todas as chamadas GetX abaixo devolvem uma lista de objetos.
func asSliceOfMaps(data any) ([]map[string]any, error) {
	list, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("formato de resposta inesperado (esperava lista)")
	}
	result := make([]map[string]any, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("formato de resposta inesperado (esperava objeto na lista)")
		}
		result = append(result, m)
	}
	return result, nil
}

func (c *Client) GetStationList(ctx context.Context) ([]map[string]any, error) {
	data, err := c.post(ctx, "/thirdData/getStationList", map[string]any{})
	if err != nil {
		return nil, err
	}
	return asSliceOfMaps(data)
}

func (c *Client) GetDevList(ctx context.Context, stationCodes string) ([]map[string]any, error) {
	data, err := c.post(ctx, "/thirdData/getDevList", map[string]any{"stationCodes": stationCodes})
	if err != nil {
		return nil, err
	}
	return asSliceOfMaps(data)
}

func (c *Client) GetStationRealKpi(ctx context.Context, stationCodes string) ([]map[string]any, error) {
	data, err := c.post(ctx, "/thirdData/getStationRealKpi", map[string]any{"stationCodes": stationCodes})
	if err != nil {
		return nil, err
	}
	return asSliceOfMaps(data)
}

func (c *Client) GetDevRealKpi(ctx context.Context, devIDs string, devTypeID int) ([]map[string]any, error) {
	data, err := c.post(ctx, "/thirdData/getDevRealKpi", map[string]any{"devIds": devIDs, "devTypeId": devTypeID})
	if err != nil {
		return nil, err
	}
	return asSliceOfMaps(data)
}

// GetKpiStationDay consulta o KPI diário (day_power, em kWh) de uma usina
// — devolve 1 entrada por dia do mês CALENDÁRIO inteiro que contém
// collectTimeMs (não só o dia pedido; a NBI v2 sempre devolve o mês
// todo). Só usado pelo backfill histórico (cmd/backfill-history) — o
// worker de tempo real usa getStationRealKpi/getDevRealKpi, não este.
func (c *Client) GetKpiStationDay(ctx context.Context, stationCodes string, collectTimeMs int64) ([]map[string]any, error) {
	data, err := c.post(ctx, "/thirdData/getKpiStationDay", map[string]any{
		"stationCodes": stationCodes,
		"collectTime":  collectTimeMs,
	})
	if err != nil {
		return nil, err
	}
	return asSliceOfMaps(data)
}

func (c *Client) GetAlarmList(ctx context.Context, stationCodes string) ([]map[string]any, error) {
	data, err := c.post(ctx, "/thirdData/getAlarmList", map[string]any{"stationCodes": stationCodes})
	if err != nil {
		return nil, err
	}
	return asSliceOfMaps(data)
}
