package collector

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// enabledBrands lista as marcas habilitadas de uma usina — pequena
// duplicação intencional da mesma query em internal/httpapi (o Python
// original também duplicava PLANT_TAG/BRAZIL_TZ entre collector/webapp em
// vez de compartilhar módulo, ver README > "Estrutura do projeto").
func enabledBrands(ctx context.Context, db *pgxpool.Pool, plantID string) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT brand FROM inverter_credentials WHERE plant_id = $1 AND enabled = true ORDER BY brand`, plantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var brands []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		brands = append(brands, b)
	}
	return brands, rows.Err()
}

// recomputePlantTotals recalcula plant_status (potência instantânea total)
// e daily_generation (geração de hoje) somando o último dado conhecido de
// CADA inversor habilitado da usina — não só do inversor que acabou de
// ser lido neste ciclo. Isso é o que permite cada credencial pollar de
// forma independente (Fase 4 do plano) e ainda assim manter o total da
// usina correto, sem precisar de um ciclo conjunto como o
// collect_once() original em Python.
//
//   - power_kw: só conta se o inversor reportou nos últimos 45min (não
//     carrega valor antigo pra frente — um inversor que parou de responder
//     contribui 0 de potência instantânea, mesmo comportamento do Python:
//     "huawei_power_kw = huawei_data["power_kw"] if huawei_data else 0.0").
//   - day_kwh: carrega o último valor de HOJE (fuso BRT); se o inversor
//     ainda não reportou nada hoje, conta 0 — mesma ideia de
//     _carry_forward_day_kwh no Python, só que lida direto do Postgres em
//     vez de guardada em memória.
//
// has_alarm/alarm_detail: só a Huawei tem alarme neste projeto. Quem
// chama como Huawei passa o estado fresco deste ciclo; quem chama como
// outra marca preserva o último estado de alarme já gravado.
func recomputePlantTotals(ctx context.Context, db *pgxpool.Pool, plantID string, isHuawei bool, freshHasAlarm bool, freshAlarmDetail *string) error {
	brands, err := enabledBrands(ctx, db, plantID)
	if err != nil {
		return err
	}

	var totalPowerKW, totalDayKWh float64
	for _, inverter := range brands {
		var powerKW *float64
		err := db.QueryRow(ctx,
			`SELECT power_kw FROM inverter_status
			 WHERE plant_id = $1 AND inverter = $2 AND recorded_at >= now() - interval '45 minutes'
			 ORDER BY recorded_at DESC LIMIT 1`,
			plantID, inverter,
		).Scan(&powerKW)
		if err != nil && !isNoRows(err) {
			return err
		}
		if powerKW != nil {
			totalPowerKW += *powerKW
		}

		var dayKWh *float64
		err = db.QueryRow(ctx,
			`SELECT day_kwh FROM inverter_status
			 WHERE plant_id = $1 AND inverter = $2
			   AND (recorded_at AT TIME ZONE 'America/Sao_Paulo')::date = (now() AT TIME ZONE 'America/Sao_Paulo')::date
			 ORDER BY recorded_at DESC LIMIT 1`,
			plantID, inverter,
		).Scan(&dayKWh)
		if err != nil && !isNoRows(err) {
			return err
		}
		if dayKWh != nil {
			totalDayKWh += *dayKWh
		}
	}

	hasAlarm := freshHasAlarm
	alarmDetail := freshAlarmDetail
	if !isHuawei {
		// Preserva o último estado de alarme conhecido (só a Huawei
		// consulta getAlarmList neste projeto) — se nunca houve nenhum
		// ponto, fica em false/nil (os valores zero já passados).
		_ = db.QueryRow(ctx,
			`SELECT has_alarm, alarm_detail FROM plant_status WHERE plant_id = $1 ORDER BY recorded_at DESC LIMIT 1`,
			plantID,
		).Scan(&hasAlarm, &alarmDetail)
	}

	var installedKWp float64
	if err := db.QueryRow(ctx, `SELECT installed_power_kwp FROM plants WHERE id = $1`, plantID).Scan(&installedKWp); err != nil {
		return err
	}

	if _, err := db.Exec(ctx,
		`INSERT INTO plant_status (plant_id, recorded_at, instantaneous_power_kw, installed_power_kwp, has_alarm, alarm_detail)
		 VALUES ($1, now(), $2, $3, $4, $5)`,
		plantID, totalPowerKW, installedKWp, hasAlarm, alarmDetail,
	); err != nil {
		return err
	}

	_, err = db.Exec(ctx,
		`INSERT INTO daily_generation (plant_id, day, generated_kwh)
		 VALUES ($1, (now() AT TIME ZONE 'America/Sao_Paulo')::date, $2)
		 ON CONFLICT (plant_id, day) DO UPDATE SET generated_kwh = EXCLUDED.generated_kwh`,
		plantID, totalDayKWh,
	)
	return err
}
