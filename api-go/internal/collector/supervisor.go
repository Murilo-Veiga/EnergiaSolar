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

	reconcile := func() {
		creds, err := FetchEnabledCredentials(ctx, deps.DB)
		if err != nil {
			deps.Log.Error("supervisor: falha ao listar credenciais habilitadas", "error", err)
			return
		}

		seen := make(map[string]bool, len(creds))
		for _, cred := range creds {
			seen[cred.ID] = true
			if _, ok := running[cred.ID]; ok {
				continue // já rodando, nada a fazer
			}
			workerCtx, cancel := context.WithCancel(ctx)
			running[cred.ID] = &runningWorker{cancel: cancel}
			startWorker(workerCtx, deps, cred)
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

func startWorker(ctx context.Context, deps Deps, cred CredentialRow) {
	switch cred.Brand {
	case "huawei":
		go RunHuaweiWorker(ctx, deps, cred)
	case "foxess":
		go RunFoxessWorker(ctx, deps, cred)
	default:
		deps.Log.Error("marca de inversor desconhecida, ignorando credencial", "brand", cred.Brand, "credential_id", cred.ID)
	}
}
