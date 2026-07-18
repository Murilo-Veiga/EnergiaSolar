// Package foxess é o cliente da FoxESS Cloud OpenAPI — porta fiel de
// collector/foxess_client.py (Python).
package foxess

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // exigido pelo esquema de assinatura da própria FoxESS, não é uso criptográfico nosso
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const DefaultBaseURL = "https://www.foxesscloud.com"

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewClient(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{apiKey: apiKey, baseURL: baseURL, httpClient: &http.Client{}}
}

// signature replica a assinatura da FoxESS: MD5 de path + "\r\n" literal
// (4 caracteres: barra-invertida, r, barra-invertida, n — não bytes CR+LF
// reais) + token + "\r\n" + timestamp. Confirmado testando contra a API
// real (ver comentário original em foxess_client.py).
func signature(path, apiKey, timestamp string) string {
	sum := md5.Sum([]byte(path + `\r\n` + apiKey + `\r\n` + timestamp))
	return hex.EncodeToString(sum[:])
}

func (c *Client) request(ctx context.Context, method, path string, body map[string]any) (any, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)

	var reqBody io.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("token", c.apiKey)
	req.Header.Set("timestamp", timestamp)
	req.Header.Set("signature", signature(path, c.apiKey, timestamp))
	req.Header.Set("lang", "en")
	req.Header.Set("Content-Type", "application/json")

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
		Errno  int `json:"errno"`
		Result any `json:"result"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("%s: decodificando resposta: %w", path, err)
	}
	if data.Errno != 0 {
		return nil, fmt.Errorf("%s falhou: %s", path, string(raw))
	}
	return data.Result, nil
}

// GetDeviceList descobre o deviceSN do inversor.
func (c *Client) GetDeviceList(ctx context.Context) ([]map[string]any, error) {
	result, err := c.request(ctx, http.MethodPost, "/op/v0/device/list", map[string]any{"currentPage": 1, "pageSize": 10})
	if err != nil {
		return nil, err
	}
	m, ok := result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("device/list: formato de resposta inesperado")
	}
	list, ok := m["data"].([]any)
	if !ok {
		return nil, fmt.Errorf("device/list: campo 'data' ausente ou em formato inesperado")
	}
	devices := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if dm, ok := item.(map[string]any); ok {
			devices = append(devices, dm)
		}
	}
	return devices, nil
}

// GetRealQuery consulta potência/geração/temperatura instantâneas.
func (c *Client) GetRealQuery(ctx context.Context, sn string, variables []string) ([]map[string]any, error) {
	result, err := c.request(ctx, http.MethodPost, "/op/v0/device/real/query", map[string]any{"sn": sn, "variables": variables})
	if err != nil {
		return nil, err
	}
	list, ok := result.([]any)
	if !ok {
		return nil, fmt.Errorf("device/real/query: formato de resposta inesperado")
	}
	entries := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			entries = append(entries, m)
		}
	}
	return entries, nil
}

// GetHistoryQuery é a curva histórica (até 24h), usada pra curva
// intradiária de maior resolução — só a FoxESS oferece esse endpoint.
func (c *Client) GetHistoryQuery(ctx context.Context, sn string, variables []string, beginMs, endMs int64) (any, error) {
	return c.request(ctx, http.MethodPost, "/op/v0/device/history/query", map[string]any{
		"sn": sn, "variables": variables, "begin": beginMs, "end": endMs,
	})
}
