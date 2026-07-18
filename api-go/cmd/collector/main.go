package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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

	log.Info("collector-go iniciado")
	collector.Supervisor(ctx, deps)
	log.Info("collector-go encerrado")
}
