package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"energiasolar-api/internal/db"
	"energiasolar-api/internal/httpapi"
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
	jwtSecret := []byte(mustEnv(log, "JWT_SECRET"))
	encryptionKey := []byte(mustEnv(log, "CONFIG_ENCRYPTION_KEY"))
	if len(encryptionKey) != 32 {
		log.Error("CONFIG_ENCRYPTION_KEY precisa ter exatamente 32 bytes", "tamanho", len(encryptionKey))
		os.Exit(1)
	}

	if err := db.Migrate(databaseURL); err != nil {
		log.Error("falha ao aplicar migrations", "error", err)
		os.Exit(1)
	}
	log.Info("migrations aplicadas com sucesso")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		log.Error("falha ao conectar no postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	allowedOrigins := strings.Split(os.Getenv("CORS_ALLOWED_ORIGIN"), ",")
	for i, origin := range allowedOrigins {
		allowedOrigins[i] = strings.TrimSpace(origin)
	}
	if len(allowedOrigins) == 1 && allowedOrigins[0] == "" {
		allowedOrigins = []string{"http://localhost:5173"} // servidor de dev do Vite
	}

	server := &httpapi.Server{DB: pool, JWTSecret: jwtSecret, EncryptionKey: encryptionKey, AllowedOrigins: allowedOrigins}
	httpServer := &http.Server{
		Addr:              ":8000",
		Handler:           httpapi.NewRouter(server),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("api-go escutando", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("servidor encerrado com erro", "error", err)
			os.Exit(1)
		}
	}()

	// Espera SIGTERM/SIGINT (ex.: "docker compose stop") e dá até 10s pras
	// requisições em andamento terminarem antes de fechar de vez — sem
	// isso, um `docker stop` corta conexões no meio, o que já causou
	// incidentes em outros serviços deste projeto (ver README do coletor
	// Python sobre restart em loop martelando rate limit de API externa).
	<-ctx.Done()
	log.Info("sinal de encerramento recebido, desligando com graça")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("falha ao desligar com graça, forçando", "error", err)
	}
}
