package collector

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// writeInverterStatus grava 1 ponto de coleta. online e lastOnlineAt vêm do
// status nativo do fabricante (FoxESS device/list.status, Huawei
// dataItemMap.run_state) — não são inferidos por timeout de coleta, ver
// pollFoxess/pollHuawei.
func writeInverterStatus(ctx context.Context, db *pgxpool.Pool, plantID, inverter string, powerKW, dayKWh float64, temperatureC *float64, online bool, lastOnlineAt *time.Time) error {
	_, err := db.Exec(ctx,
		`INSERT INTO inverter_status (plant_id, inverter, recorded_at, power_kw, day_kwh, temperature_c, online, last_online_at)
		 VALUES ($1, $2, now(), $3, $4, $5, $6, $7)`,
		plantID, inverter, powerKW, dayKWh, temperatureC, online, lastOnlineAt,
	)
	return err
}

// writeCollectorHealth grava 1 ponto por ciclo (sucesso ou falha), nunca
// sobrescrito — mesmo princípio de _health_point em collector/main.py
// (Python). lastError vazio vira NULL (sucesso não tem erro pra guardar).
func writeCollectorHealth(ctx context.Context, db *pgxpool.Pool, plantID, inverter string, consecutiveFailures int, lastError string) error {
	var lastErrPtr *string
	if lastError != "" {
		truncated := lastError
		if len(truncated) > 200 {
			truncated = truncated[:200]
		}
		lastErrPtr = &truncated
	}
	_, err := db.Exec(ctx,
		`INSERT INTO collector_health (plant_id, inverter, recorded_at, consecutive_failures, last_error)
		 VALUES ($1, $2, now(), $3, $4)`,
		plantID, inverter, consecutiveFailures, lastErrPtr,
	)
	return err
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

// extractAlarmDetail replica _extract_alarm_detail em collector/main.py:
// formato exato do getAlarmList nunca foi confirmado contra um alarme
// real — tenta os candidatos mais prováveis e cai pra uma mensagem
// genérica.
func extractAlarmDetail(alarms []map[string]any) *string {
	if len(alarms) == 0 {
		return nil
	}
	alarm := alarms[0]
	for _, key := range []string{"alarmName", "name", "desc", "alarmCause"} {
		if v, ok := alarm[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return &s
			}
		}
	}
	generic := "Alarme ativo"
	return &generic
}
