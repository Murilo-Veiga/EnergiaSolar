// Comando de migração única (Fase 2 do plano): lê o histórico já gravado
// no InfluxDB (measurements plant_status, inverter_status,
// daily_generation, collector_health, tag plant_id="casa" — a mesma
// constante PLANT_TAG usada pelo collector/webapp Python) e escreve nas
// tabelas equivalentes do Postgres, sob o plant_id (uuid) de uma usina já
// cadastrada (ver cmd/seed).
//
// Idempotente: apaga o que já existir pro TARGET_PLANT_ID antes de
// reinserir, então pode ser rodado de novo com segurança se o InfluxDB
// ganhar mais dado histórico depois.
//
// consumption/annotation ficam de fora desta migração: nenhuma fatura foi
// enviada ainda nesta instalação (nenhum dado real a migrar) — entram
// quando o parser de fatura for portado (Fase 5 do plano).
package main

import (
	"context"
	"log/slog"
	"os"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"

	"energiasolar-api/internal/db"
)

func mustEnv(log *slog.Logger, key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Error("variável de ambiente obrigatória não definida", "key", key)
		os.Exit(1)
	}
	return v
}

// legacyPlantTag é o PLANT_TAG fixo usado por todo o stack Python
// (collector/main.py, webapp/main.py) antes do multi-tenant — a origem de
// todo dado migrado por este comando.
const legacyPlantTag = "casa"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	influxURL := mustEnv(log, "INFLUXDB_URL")
	influxToken := mustEnv(log, "INFLUXDB_TOKEN")
	influxOrg := mustEnv(log, "INFLUXDB_ORG")
	influxBucket := mustEnv(log, "INFLUXDB_BUCKET")
	databaseURL := mustEnv(log, "DATABASE_URL")
	targetPlantID := mustEnv(log, "TARGET_PLANT_ID")

	influxClient := influxdb2.NewClient(influxURL, influxToken)
	defer influxClient.Close()
	queryAPI := influxClient.QueryAPI(influxOrg)

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		log.Error("falha ao conectar no postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Confirma que a usina alvo existe antes de mexer em qualquer dado —
	// evita migrar histórico pra um plant_id que não existe (FK falharia
	// no INSERT de qualquer forma, mas falhar cedo com mensagem clara é
	// melhor que um erro de constraint no meio da migração).
	var plantName string
	if err := pool.QueryRow(ctx, `SELECT name FROM plants WHERE id = $1`, targetPlantID).Scan(&plantName); err != nil {
		log.Error("TARGET_PLANT_ID não corresponde a nenhuma usina cadastrada", "plant_id", targetPlantID, "error", err)
		os.Exit(1)
	}
	log.Info("migrando histórico do InfluxDB", "plant_id", targetPlantID, "plant_name", plantName)

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error("falha ao iniciar transação", "error", err)
		os.Exit(1)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit bem-sucedido é no-op

	// Idempotente: limpa o que já existir pra essa usina antes de reinserir.
	for _, table := range []string{"plant_status", "inverter_status", "daily_generation", "collector_health"} {
		if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE plant_id = $1`, targetPlantID); err != nil {
			log.Error("falha ao limpar tabela antes de migrar", "table", table, "error", err)
			os.Exit(1)
		}
	}

	counts := map[string]int{}

	// --- plant_status ---
	{
		flux := `from(bucket: "` + influxBucket + `")
		  |> range(start: 0)
		  |> filter(fn: (r) => r._measurement == "plant_status" and r.plant_id == "` + legacyPlantTag + `")
		  |> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")`
		result, err := queryAPI.Query(ctx, flux)
		if err != nil {
			log.Error("falha ao consultar plant_status no influx", "error", err)
			os.Exit(1)
		}
		for result.Next() {
			r := result.Record()
			_, err := tx.Exec(ctx,
				`INSERT INTO plant_status (plant_id, recorded_at, instantaneous_power_kw, installed_power_kwp, has_alarm, alarm_detail)
				 VALUES ($1, $2, $3, $4, $5, $6)`,
				targetPlantID, r.Time(),
				asFloat(r.ValueByKey("instantaneous_power_kw")),
				asFloatPtr(r.ValueByKey("installed_power_kwp")),
				asBool(r.ValueByKey("has_alarm")),
				asStringPtr(r.ValueByKey("alarm_detail")),
			)
			if err != nil {
				log.Error("falha ao inserir plant_status", "error", err)
				os.Exit(1)
			}
			counts["plant_status"]++
		}
		if result.Err() != nil {
			log.Error("erro lendo resultado de plant_status", "error", result.Err())
			os.Exit(1)
		}
	}

	// --- inverter_status ---
	{
		flux := `from(bucket: "` + influxBucket + `")
		  |> range(start: 0)
		  |> filter(fn: (r) => r._measurement == "inverter_status" and r.plant_id == "` + legacyPlantTag + `")
		  |> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")`
		result, err := queryAPI.Query(ctx, flux)
		if err != nil {
			log.Error("falha ao consultar inverter_status no influx", "error", err)
			os.Exit(1)
		}
		for result.Next() {
			r := result.Record()
			_, err := tx.Exec(ctx,
				`INSERT INTO inverter_status (plant_id, inverter, recorded_at, power_kw, day_kwh, temperature_c)
				 VALUES ($1, $2, $3, $4, $5, $6)`,
				targetPlantID, asString(r.ValueByKey("inverter")), r.Time(),
				asFloatPtr(r.ValueByKey("power_kw")),
				asFloatPtr(r.ValueByKey("day_kwh")),
				asFloatPtr(r.ValueByKey("temperature_c")),
			)
			if err != nil {
				log.Error("falha ao inserir inverter_status", "error", err)
				os.Exit(1)
			}
			counts["inverter_status"]++
		}
		if result.Err() != nil {
			log.Error("erro lendo resultado de inverter_status", "error", result.Err())
			os.Exit(1)
		}
	}

	// --- daily_generation ---
	var influxTotalKWh float64
	{
		flux := `from(bucket: "` + influxBucket + `")
		  |> range(start: 0)
		  |> filter(fn: (r) => r._measurement == "daily_generation" and r._field == "generated_kwh" and r.plant_id == "` + legacyPlantTag + `")`
		result, err := queryAPI.Query(ctx, flux)
		if err != nil {
			log.Error("falha ao consultar daily_generation no influx", "error", err)
			os.Exit(1)
		}
		for result.Next() {
			r := result.Record()
			kwh := asFloat(r.Value())
			_, err := tx.Exec(ctx,
				`INSERT INTO daily_generation (plant_id, day, generated_kwh) VALUES ($1, $2, $3)
				 ON CONFLICT (plant_id, day) DO UPDATE SET generated_kwh = EXCLUDED.generated_kwh`,
				targetPlantID, r.Time(), kwh,
			)
			if err != nil {
				log.Error("falha ao inserir daily_generation", "error", err)
				os.Exit(1)
			}
			influxTotalKWh += kwh
			counts["daily_generation"]++
		}
		if result.Err() != nil {
			log.Error("erro lendo resultado de daily_generation", "error", result.Err())
			os.Exit(1)
		}
	}

	// --- collector_health ---
	{
		flux := `from(bucket: "` + influxBucket + `")
		  |> range(start: 0)
		  |> filter(fn: (r) => r._measurement == "collector_health" and r.plant_id == "` + legacyPlantTag + `")
		  |> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")`
		result, err := queryAPI.Query(ctx, flux)
		if err != nil {
			log.Error("falha ao consultar collector_health no influx", "error", err)
			os.Exit(1)
		}
		for result.Next() {
			r := result.Record()
			_, err := tx.Exec(ctx,
				`INSERT INTO collector_health (plant_id, inverter, recorded_at, consecutive_failures, last_error)
				 VALUES ($1, $2, $3, $4, $5)`,
				targetPlantID, asString(r.ValueByKey("inverter")), r.Time(),
				int(asFloat(r.ValueByKey("consecutive_failures"))),
				asStringPtr(r.ValueByKey("last_error")),
			)
			if err != nil {
				log.Error("falha ao inserir collector_health", "error", err)
				os.Exit(1)
			}
			counts["collector_health"]++
		}
		if result.Err() != nil {
			log.Error("erro lendo resultado de collector_health", "error", result.Err())
			os.Exit(1)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error("falha ao commitar migração", "error", err)
		os.Exit(1)
	}

	// Validação: soma gravada no Postgres precisa bater com a soma somada
	// direto do InfluxDB durante a leitura acima — mesmo princípio da
	// verificação manual já feita no README (referência conhecida: 787,4
	// kWh / 68 dias).
	var postgresTotalKWh float64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(generated_kwh), 0) FROM daily_generation WHERE plant_id = $1`, targetPlantID).Scan(&postgresTotalKWh); err != nil {
		log.Error("falha ao validar total pós-migração", "error", err)
		os.Exit(1)
	}

	log.Info("migração concluída",
		"plant_status", counts["plant_status"],
		"inverter_status", counts["inverter_status"],
		"daily_generation", counts["daily_generation"],
		"collector_health", counts["collector_health"],
		"total_kwh_influx", influxTotalKWh,
		"total_kwh_postgres", postgresTotalKWh,
	)
	if influxTotalKWh != postgresTotalKWh {
		log.Error("DIVERGÊNCIA: total migrado não bate com o total lido do influx — não confie neste resultado")
		os.Exit(1)
	}
	log.Info("validação ok: total do influx e do postgres batem exatamente")
}

func asFloat(v any) float64 {
	f, _ := v.(float64)
	return f
}

func asFloatPtr(v any) *float64 {
	if v == nil {
		return nil
	}
	f, ok := v.(float64)
	if !ok {
		return nil
	}
	return &f
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asStringPtr(v any) *string {
	if v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	return &s
}
