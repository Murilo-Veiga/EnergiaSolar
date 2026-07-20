// Comando cmd/backfill-history: recupera a geração diária histórica das
// APIs Huawei/FoxESS pros dias em que o coletor não estava rodando ainda
// (ou estava mal configurado) e preenche inverter_status/daily_generation
// com esses dias — só o PASSADO, nunca hoje (isso o worker de tempo real
// já cobre).
//
// Uso:
//
//	go run ./cmd/backfill-history -days 30                # dry-run, só imprime
//	go run ./cmd/backfill-history -days 30 -write          # grava de verdade
//	go run ./cmd/backfill-history -days 30 -plant <uuid>   # só 1 usina
//
// Rode SEMPRE primeiro sem -write e confira os valores contra o app do
// fabricante (FusionSolar / FoxESS Cloud) antes de gravar — os nomes de
// campo das respostas históricas (day_power, todayYield) são os mesmos já
// usados pelo worker de tempo real, mas nunca foram confirmados contra uma
// resposta real do endpoint histórico.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"energiasolar-api/internal/brtime"
	"energiasolar-api/internal/collector"
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

func main() {
	days := flag.Int("days", 30, "quantos dias passados recuperar (excluindo hoje)")
	write := flag.Bool("write", false, "grava de verdade em inverter_status/daily_generation (default: dry-run, só imprime)")
	plantID := flag.String("plant", "", "filtra por 1 usina (plant_id); vazio = todas")
	debugHuawei := flag.String("debug-huawei", "", "não roda o backfill — só imprime a resposta CRUA de getKpiStationDay pra essa credential_id, pra conferir o nome real dos campos")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := mustEnv(log, "DATABASE_URL")
	encryptionKey := []byte(mustEnv(log, "CONFIG_ENCRYPTION_KEY"))
	if len(encryptionKey) != 32 {
		log.Error("CONFIG_ENCRYPTION_KEY precisa ter exatamente 32 bytes", "tamanho", len(encryptionKey))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		log.Error("falha ao conectar no postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	deps := collector.Deps{DB: pool, EncryptionKey: encryptionKey, Log: log}

	if *debugHuawei != "" {
		entries, err := collector.DebugHuaweiKpiStationDay(ctx, deps, *debugHuawei)
		if err != nil {
			log.Error("debug falhou", "error", err)
			os.Exit(1)
		}
		out, _ := json.MarshalIndent(entries, "", "  ")
		fmt.Println(string(out))
		return
	}

	results, err := collector.RunHistoryBackfill(ctx, deps, collector.BackfillOptions{
		Days:    *days,
		PlantID: *plantID,
		Write:   *write,
	})
	if err != nil {
		log.Error("backfill falhou", "error", err)
		os.Exit(1)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].PlantID != results[j].PlantID {
			return results[i].PlantID < results[j].PlantID
		}
		if !results[i].Day.Equal(results[j].Day) {
			return results[i].Day.Before(results[j].Day)
		}
		return results[i].Brand < results[j].Brand
	})

	fmt.Printf("%-38s %-8s %-12s %10s\n", "plant_id", "brand", "day", "kwh")
	var totalKWh float64
	for _, r := range results {
		fmt.Printf("%-38s %-8s %-12s %10.2f\n", r.PlantID, r.Brand, r.Day.In(brtime.Location).Format("2006-01-02"), r.DayKWh)
		totalKWh += r.DayKWh
	}
	fmt.Printf("\n%d dia(s)/credencial recuperados, total %.2f kWh\n", len(results), totalKWh)

	if *write {
		fmt.Println("Gravado em inverter_status e daily_generation.")
	} else {
		fmt.Println("Dry-run — nada foi gravado. Confira os valores acima contra o app do fabricante e rode de novo com -write pra persistir.")
	}
}
