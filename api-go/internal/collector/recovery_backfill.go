package collector

import (
	"context"
	"sync"
)

// recoveryBackfillDays é a janela coberta quando um worker volta a
// responder depois de falhar — não usa os 30 dias default do backfill
// manual (cmd/backfill-history) porque aqui dispara sozinho e com muito
// mais frequência; 7 dias cobre folgado uma queda de conectividade comum
// (rede/VPN do site, credencial temporariamente inválida etc.).
const recoveryBackfillDays = 7

var (
	recoveryBackfillOnce    sync.Once
	recoveryBackfillPending = make(chan struct{}, 1)
)

// signalRecoveryBackfill agenda 1 rodada de RunHistoryBackfill pra TODAS as
// usinas (não só a credencial que reconectou) — queda de conectividade
// tende a ser geral (rede/VPN do site), então ao reconectar qualquer
// credencial já aproveita pra fechar o buraco de todo mundo de uma vez.
// Non-blocking e coalescente: se já tem uma rodada pendente ou em
// andamento, esse sinal é descartado (a que já vai rodar cobre a mesma
// janela de qualquer forma).
func signalRecoveryBackfill() {
	select {
	case recoveryBackfillPending <- struct{}{}:
	default:
	}
}

// startRecoveryBackfillLoop sobe (1x por processo) a goroutine que consome
// os sinais de signalRecoveryBackfill e roda o backfill de verdade — ver
// Supervisor.
func startRecoveryBackfillLoop(ctx context.Context, deps Deps) {
	recoveryBackfillOnce.Do(func() {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-recoveryBackfillPending:
					runRecoveryBackfill(ctx, deps)
				}
			}
		}()
	})
}

func runRecoveryBackfill(ctx context.Context, deps Deps) {
	log := deps.Log.With("trigger", "worker_reconnect")
	log.Info("worker reconectou depois de falhas consecutivas, rodando backfill histórico para todas as usinas", "days", recoveryBackfillDays)
	results, err := RunHistoryBackfill(ctx, deps, BackfillOptions{Days: recoveryBackfillDays, Write: true})
	if err != nil {
		log.Error("backfill automático de reconexão falhou", "error", err)
		return
	}
	var totalKWh float64
	for _, r := range results {
		totalKWh += r.DayKWh
	}
	log.Info("backfill automático de reconexão concluído", "dias_credencial_encontrados", len(results), "total_kwh", totalKWh)
}
