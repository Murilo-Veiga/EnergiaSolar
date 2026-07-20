package collector

import (
	"context"
	"time"
)

// reloadInterval é a cadência com que o supervisor relê inverter_credentials
// pra pegar cadastro novo/alterado pela tela de admin — não precisa ser
// rápido (com só 2 usuários, poucas trocas de credencial no dia a dia).
const reloadInterval = 2 * time.Minute

type runningWorker struct {
	cancel context.CancelFunc
}

// Supervisor sobe/derruba 1 goroutine por credencial habilitada,
// reagindo a cadastro novo, removido ou (des)habilitado sem precisar
// reiniciar o processo — diferente do collector/main.py (Python), que lia
// a config 1x do .env na subida e nunca mais mudava em runtime.
func Supervisor(ctx context.Context, deps Deps) {
	running := map[string]*runningWorker{}
	var lastSettings SystemSettings

	reconcile := func() {
		settings, err := loadSystemSettings(ctx, deps.DB)
		if err != nil {
			deps.Log.Error("supervisor: falha ao carregar configurações do sistema, mantendo última conhecida", "error", err)
			settings = lastSettings
		}

		creds, err := FetchEnabledCredentials(ctx, deps.DB)
		if err != nil {
			deps.Log.Error("supervisor: falha ao listar credenciais habilitadas", "error", err)
			return
		}

		// URL padrão ou intervalo do worker mudou na tela de admin: derruba
		// todo mundo pra já subir de novo com a config nova (mais simples
		// que só reconfigurar o ticker de cada goroutine em voo).
		if settings != lastSettings && lastSettings != (SystemSettings{}) {
			deps.Log.Info("configurações do sistema mudaram, reiniciando workers",
				"worker_interval_minutes", settings.WorkerIntervalMinutes)
			for id, w := range running {
				w.cancel()
				delete(running, id)
			}
		}
		lastSettings = settings

		seen := make(map[string]bool, len(creds))
		for _, cred := range creds {
			seen[cred.ID] = true
			if _, ok := running[cred.ID]; ok {
				continue // já rodando, nada a fazer
			}
			workerCtx, cancel := context.WithCancel(ctx)
			running[cred.ID] = &runningWorker{cancel: cancel}
			startWorker(workerCtx, deps, cred, settings)
		}

		// Credencial removida ou desabilitada desde a última reconciliação:
		// cancela o contexto do worker correspondente, que sai do próprio
		// loop de ticks na próxima iteração (ver RunHuaweiWorker/RunFoxessWorker).
		for id, w := range running {
			if !seen[id] {
				w.cancel()
				delete(running, id)
			}
		}
	}

	reconcile()
	ticker := time.NewTicker(reloadInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

func startWorker(ctx context.Context, deps Deps, cred CredentialRow, settings SystemSettings) {
	switch cred.Brand {
	case "huawei":
		go RunHuaweiWorker(ctx, deps, cred, settings)
	case "foxess":
		go RunFoxessWorker(ctx, deps, cred, settings)
	default:
		deps.Log.Error("marca de inversor desconhecida, ignorando credencial", "brand", cred.Brand, "credential_id", cred.ID)
	}
}
