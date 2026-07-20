# Solar Home — Painel de Monitoramento

Painel multi-tenant de monitoramento de usinas solares residenciais,
rodando 100% local via Docker. Cada usina tem 1 ou 2 inversores, cada um
monitorado diretamente pela **API oficial do seu fabricante** (Huawei
FusionSolar e FoxESS Cloud) — sem depender de nenhum acesso de usuário de
terceiros. Cada conta só enxerga as próprias usinas; um administrador
gerencia as contas do sistema.

**Sumário:** [Arquitetura](#arquitetura) · [Como rodar](#como-rodar) ·
[Contas, usinas e administração](#contas-usinas-e-administração) ·
[Fontes de dados oficiais](#fontes-de-dados-oficiais) ·
[Falhas de coleta e fallback seguro](#falhas-de-coleta-e-fallback-seguro) ·
[Backfill de histórico](#backfill-de-histórico) ·
[Limitações conhecidas e funcionalidades ainda não portadas](#limitações-conhecidas-e-funcionalidades-ainda-não-portadas) ·
[Design do painel](#design-do-painel) ·
[Aba Histórico](#aba-histórico) · [Menu Saúde da usina](#menu-saúde-da-usina) ·
[Consumo por unidade consumidora](#consumo-por-unidade-consumidora-celesc) ·
[Princípio de design](#princípio-de-design-fácil-de-ler-pra-qualquer-idade) ·
[Selo "novo"](#regra-selo-novo-por-5-dias) ·
[Estrutura do projeto](#estrutura-do-projeto) ·
[Endpoints da API](#endpoints-da-api) ·
[Pendências](#pendências--próximos-passos)

## Arquitetura

```
Huawei FusionSolar (NBI)  ─┐
                            ├─→  collector  →  Postgres  ←→  api  ←→  web
FoxESS Cloud (OpenAPI)   ──┘                       ↑
                                     Open-Meteo (previsão) — chamada por api
```

- **collector** (serviço `collector`, container `solar-collector`, código em
  `api-go/cmd/collector`): processo Go de longa duração, 1 goroutine por
  credencial de inversor habilitada. Um supervisor
  (`internal/collector/supervisor.go`) relê a cada 2 min quais credenciais
  estão habilitadas e a configuração global (`system_settings`), sobe/derruba
  workers sem precisar reiniciar o processo. Cada worker consulta a API da
  Huawei ou da FoxESS no intervalo configurado (padrão 30 min, ajustável pela
  UI de admin) e grava potência, geração do dia e temperatura no Postgres —
  falha de um inversor nunca afeta o outro.
- **Postgres** (serviço `postgres`, container `solar-postgres`): banco
  relacional único para tudo — usuários, usinas, credenciais de inversor
  (cifradas em repouso), série temporal de potência/geração, saúde da coleta
  e anotações. Substituiu o InfluxDB.
- **api** (serviço `api`, container `solar-api`, código em `api-go/cmd/api`):
  API JSON (chi router), autenticação por cookie de sessão (JWT),
  multi-tenant — toda rota de usina é validada contra o dono
  (`authorizePlant`), nunca vaza dado de uma usina de outro usuário.
- **web** (serviço `web`, container `solar-web`, código em `web/`): frontend
  React (Vite), consome a api via fetch, servido como estático por nginx em
  produção.

## Como rodar

```bash
cp .env.example .env   # preencher POSTGRES_PASSWORD, JWT_SECRET, CONFIG_ENCRYPTION_KEY
docker compose up -d --build
```

- Painel: http://localhost:8090
- API: http://localhost:8091

Primeiro acesso: não há mais cadastro público — crie o primeiro usuário
direto no banco ou peça pra um admin existente criar em Administração >
Gestão de usuários. Depois, cadastre sua usina em Minhas usinas e configure
as credenciais Huawei/FoxESS por lá (usuário/senha ou API key, cifradas em
repouso). Não existe seed automático de dados de exemplo.

`.env` nunca é commitado (está no `.gitignore`); use `.env.example` como
referência. As URLs padrão das integrações Huawei/FoxESS e o intervalo do
worker de coleta **não são mais variável de ambiente** — ficam na tela
Administração > Configuração do sistema (tabela `system_settings`,
editável só por admin).

## Contas, usinas e administração

O painel é multi-tenant: não existe cadastro público (a tela de "criar
conta" foi removida de propósito) — um admin cria cada conta em
Administração > Gestão de usuários, e cada conta só vê as próprias usinas
(`plants.user_id`). Um usuário marcado como
administrador (`users.is_admin`) ganha acesso a duas telas extras em
Administração:

- **Gestão de usuários**: CRUD completo de contas do sistema (criar,
  editar e-mail/privilégio de admin, redefinir senha de outra pessoa,
  apagar) — um admin não consegue remover o próprio privilégio nem apagar
  a própria conta por essa tela, pra nunca ficar sem nenhum admin no ar.
- **Configuração do sistema**: parâmetros globais que não dependem de
  usuário nem de usina — hoje, a URL padrão das integrações Huawei/FoxESS
  (usada quando uma credencial de usina não define a própria) e o
  intervalo do worker de coleta. O serviço `collector` relê essa tabela a
  cada reconciliação (2 min).

Ajustes da própria conta (nome, e-mail, senha) ficam numa tela separada,
"Minha conta", acessível a qualquer usuário logado — não depende de ser
admin.

Não existe promoção de admin pela própria UI, de propósito (evita
escalação de privilégio); o primeiro admin precisa ser marcado direto no
banco:

```sql
UPDATE users SET is_admin = true WHERE email = 'voce@exemplo.com';
```

## Fontes de dados oficiais

| Inversor | Fabricante/API | Identificador | Descoberto por |
|---|---|---|---|
| FoxESS | **FoxESS Cloud** | `deviceSN` | `device/list` (primeiro item da conta) |
| Huawei | **Huawei FusionSolar (NBI)** | `stationCode`/`devDn` | `getStationList`/`getDevList` (primeiro item da conta) |

(O nome comercial usado por instaladoras — ex. "SIW300H-3K"/"SIW200G-5K" — é
frequentemente um rebrand; o hardware/telemetria real por trás costuma ser
Huawei e/ou FoxESS, dependendo do instalador.)

### Huawei FusionSolar — Northbound Interface (NBI)

Documentação oficial: https://support.huawei.com/enterprise/en/doc/EDOC1100387404/9e1a18d2/login-interface
(SPA que não renderiza via fetch simples — o conteúdo abaixo foi confirmado
testando direto contra a API real, não lendo a doc renderizada).

- `POST /thirdData/login` com `{"userName", "systemCode"}` → sessão via
  cookie + `xsrf-token` retornado no **header** da resposta (não no corpo).
  Enviado de volta como header `XSRF-TOKEN` nas chamadas seguintes.
- Servidor regional: `https://la5.fusionsolar.huawei.com` (confirmado por
  teste — América Latina; `intl.fusionsolar.huawei.com` também respondeu
  igual, mas usamos `la5` por ser o esperado para a região da conta).
  Configurável globalmente em Administração > Configuração do sistema, ou
  por usina se uma credencial específica precisar de outro servidor.
- Endpoints usados: `getStationList`, `getDevList`, `getStationRealKpi`,
  `getDevRealKpi`, `getAlarmList` (cliente em `api-go/internal/huawei`).
- **Rate limit confirmado por teste**: cada interface só pode ser chamada 1x
  a cada 5 minutos (senão retorna `failCode 407
  ACCESS_FREQUENCY_IS_TOO_HIGH`) — por isso o intervalo do worker não deve
  ficar abaixo de 300s. Login não tem essa restrição. `getAlarmList` tem um
  limite próprio, mais restrito (medido empiricamente em ~592-888s) — hoje
  roda no mesmo ciclo do resto (ver "Falhas de coleta e fallback seguro").
- Não fornece curva de potência histórica — só o valor instantâneo do
  momento da chamada.

### FoxESS Cloud — OpenAPI

Documentação oficial: https://www.foxesscloud.com/public/i18n/en/OpenApiDocument.html

- Autenticação por **token privado**: headers `token` (a API key),
  `timestamp` (ms) e `signature` = MD5 de `path + "\r\n" + token + "\r\n" +
  timestamp`. Não usamos OAuth — o método de token privado já é suficiente e
  a doc não permite misturar os dois.
- Endpoints usados hoje: `device/list` (descoberta do `deviceSN`),
  `device/real/query` (potência/geração instantânea) — cliente em
  `api-go/internal/foxess`. `device/history/query` (curva intradiária de
  alta resolução) ainda não foi portado do coletor Python (ver
  "Limitações conhecidas").
- Rate limit: 1440 chamadas/dia por inversor, máx. 1 requisição/segundo —
  bem mais folgado que a Huawei.

## Falhas de coleta e fallback seguro

Cada credencial de inversor roda numa goroutine isolada — uma falha na
Huawei não impede a FoxESS de gravar no mesmo ciclo (e vice-versa), e uma
falha numa usina nunca afeta outra usina. Duas proteções:

- **Geração diária só cresce**: se a consulta de um inversor falha num
  ciclo, o total do dia usa o último valor bem-sucedido em vez de contar a
  contribuição como zero (`guard.apply` em
  `api-go/internal/collector/credential.go`). Sem sucesso ainda no dia,
  assume 0 — mesmo comportamento de antes do inversor acordar de manhã.
- **Alerta de falha constante**: cada ciclo grava uma linha em
  `collector_health` (`consecutive_failures`, `last_error`) — sucesso zera
  o contador, falha incrementa. A partir de 2 falhas seguidas, o painel
  mostra alerta na Central de Alertas (ver "Status por inversor").

## Backfill de histórico

O worker de tempo real (`collector`) só grava o ponto do momento em que
roda — se uma usina/credencial começou a ser monitorada pelo painel depois
da data real de comissionamento (ou o `collector` ficou fora do ar/mal
configurado por um tempo), os dias anteriores ficam com buraco em
`inverter_status`/`daily_generation`, e o gráfico "Histórico > mês"/"ano"
mostra dias faltando ou zerados.

`api-go/cmd/backfill-history` é um comando avulso (não roda em nenhum
container do `docker-compose.yml`, é sob demanda) que busca a geração
diária retroativa direto na API de cada fabricante e preenche esse buraco:

- **Huawei**: `getKpiStationDay` — 1 chamada já devolve o **mês
  calendário inteiro** que contém a data pedida (até 2 chamadas se o
  intervalo pedido cruzar 2 meses). Campo usado: `PVYield` (kWh do dia) —
  **diferente** do campo que o worker de tempo real usa (`day_power`, que
  não existe nesse endpoint histórico; confirmado testando contra uma
  resposta real, não documentação).
- **FoxESS**: não existe endpoint de relatório diário pronto na OpenAPI
  pública, então usamos a curva intradiária (`device/history/query`, até
  24h por chamada) e pegamos o último ponto de `todayYield` do dia — 1
  chamada por dia, com uma pequena pausa entre chamadas pra não estourar
  rate limit.

Só cobre o **passado** (nunca "hoje" — isso o worker de tempo real já
cobre) e é idempotente: rodar de novo pro mesmo período substitui os
valores em vez de duplicar.

```bash
cd api-go

# 1. Sempre rode em dry-run primeiro (não grava nada, só imprime a tabela)
#    e confira os valores contra o app FusionSolar/FoxESS Cloud.
go run ./cmd/backfill-history -days 30

# 2. Confirmado que os valores batem, grava de verdade.
go run ./cmd/backfill-history -days 30 -write

# Só 1 usina:
go run ./cmd/backfill-history -days 30 -plant <plant_id> -write

# Diagnóstico: imprime a resposta CRUA de getKpiStationDay pra 1
# credencial Huawei, sem tentar extrair nenhum campo — útil se a Huawei
# mudar o formato de resposta de novo e os valores voltarem a sair 0.
go run ./cmd/backfill-history -debug-huawei <credential_id>
```

Precisa das mesmas variáveis de ambiente do `collector`/`api`
(`DATABASE_URL`, `CONFIG_ENCRYPTION_KEY`) — rodando fora de container,
exporte as do `.env` antes, ou aponte `DATABASE_URL` pro Postgres exposto
em `localhost:5432`.

## Limitações conhecidas e funcionalidades ainda não portadas

Esta seção existe porque parte do painel foi reconstruída do zero (stack
Python → Go/React) e nem tudo que existia antes já foi trazido de volta.

1. **Sem previsão de geração**: nem Huawei nem FoxESS oferecem esse dado —
   só medição real. Uma estimativa própria (radiação solar do Open-Meteo ×
   potência instalada × fator de performance) foi avaliada e **descartada**
   por decisão do usuário.
2. **Economia estimada e bandeira tarifária sempre nulas**
   (`today_economia_brl` em `/summary`, `bandeira`/`bandeira_valor_kwh` em
   `/day-status`): dependiam da tarifa extraída da fatura da Celesc, cujo
   upload/parser ainda não foi portado (ver "Consumo por unidade
   consumidora" abaixo). Sem isso, a Central de Alertas nunca dispara o
   alerta de "bandeira amarela/vermelha".
3. **Relatório em PDF do Histórico**: existia no webapp Python
   (`reportlab`), ainda não foi portado — o botão aparece desabilitado na
   aba Histórico.
4. **Upload de fatura da Celesc / consumo por UC**: aba "Consumo" existe no
   frontend só como placeholder ("Em breve") — o parser de PDF
   (`pdfplumber`) e os endpoints de upload/resumo/histórico por unidade
   consumidora ainda não foram portados. A tabela `consumer_units` já existe
   no schema do Postgres, sem handler nenhum usando ela ainda.
5. **Capacidade por inversor, usada em "Contribuição real vs. esperada"**
   (menu Saúde da usina): ainda hardcoded no frontend
   (`INVERTER_CAPACITY_KWP` em `SaudeTab.tsx`), igual ao valor fixo que já
   existia no painel antigo — o schema novo ainda não guarda essa
   capacidade por credencial/inversor.
6. **FoxESS: curva intradiária de alta resolução** (`device/history/query`)
   não foi portada — hoje só grava o ponto instantâneo por ciclo, igual à
   Huawei.
7. **Histórico só existe a partir da data real em que a usina começou a
   gerar** nas nuvens do fabricante — não é limitação de coleta.
8. **"Gerado hoje" fica em 0 até o inversor acordar de manhã**: as nuvens da
   Huawei/FoxESS cacheiam o total do dia anterior e só atualizam quando o
   inversor manda telemetria nova. O worker detecta isso e trata como zero
   até ver potência real nesse dia local (ver "Falhas de coleta").
9. **Um inversor pode nunca chegar perto da capacidade nominal do
   fabricante** sem que isso seja defeito ou bug de coleta — o gargalo
   costuma ser físico (nº de módulos por MPPT/string sub-dimensionado em
   relação ao inversor).
10. **Limiar de temperatura da Central de Alertas é ilustrativo** (65°C) —
    não validado contra a doc oficial do modelo exato do inversor de cada
    instalação.

## Design do painel

O layout foi redesenhado (mockup iterado e aprovado em Artifact antes de ir
pro código real): paleta validada pelo skill de dataviz (contraste +
separação CVD), números nunca "vestem" a cor da série (identidade vem de um
indicador ao lado, texto sempre em tinta neutra), tabular-nums pra alinhar
dígitos, e tooltips em vez de legendas fixas pra explicar estados. O CSS é
um sistema de tokens próprio (`--surface`, `--ink`, `--accent-*` etc. em
`web/src/styles/global.css`), sem framework de UI; os gráficos são Chart.js
estilizado pra bater com os tokens.

### Ícones

Sistema próprio de ícones — 4 direções foram comparadas em mockup (linha,
preenchido, duotone, badge/flat) antes de escolher. Estilo escolhido:
**badge/flat** — ícone SVG dentro de um chip colorido arredondado.

- `iconBody(name, bg)` em `web/src/components/icons.tsx`: gera o SVG
  (24×24, `fill="currentColor"`) de cada ícone. O parâmetro `bg` é usado só
  pelos ícones que têm um "furo" interno (moeda da carteira, círculo do
  pin, "!" do alerta) — pintado com a cor de fundo do chip específico
  daquela chamada, não uma cor fixa.
- Componente `<IconBadge name color size />` com modificadores de cor
  (`blue`/`aqua`/`green`/`gold`/`red`) e de tamanho
  (`nav`/`card`/`alert`/`fc`) — cor de fundo sempre a mesma tinta suave
  (~14% opacidade).
- Ícones da Central de Alertas herdam a cor do badge da severidade do
  alerta (crítico→vermelho, atenção→dourado, informativo→azul).
- "Saúde da usina" usa só o traçado de pulso (`activity`), sem coração —
  ficou mais direto como ícone de "monitoramento".

### Status por inversor

Cada inversor (Huawei, FoxESS) mostra um selo de status, calculado por
`handleInverters` (`api-go/internal/httpapi/dashboard_handlers.go`)
puramente pela idade do último ponto gravado:

| Status | Condição |
|---|---|
| **Gerando** | ponto recente e potência > 0 |
| **Online, sem geração** | ponto recente e potência = 0 (normal à noite) |
| **Sem comunicação** | nenhum ponto dentro da janela de timeout (`commTimeoutMinutes`, 3x o intervalo do worker) |

"Sem comunicação" é o sinal mais próximo de "pode ter caído a
energia/Wi-Fi/disjuntor" que dá pra obter sem depender de código de status
do fabricante — os campos `run_state`/`inverter_state` (Huawei) e `status`
(FoxESS) não têm documentação oficial confiável, e um mapeamento encontrado
numa integração open-source pra FoxESS contradisse o que observamos ao
vivo.

### Central de alertas

Accordion no topo do Dashboard (fechado por padrão, com contador de alertas
ativos no badge) que resume tudo que merece atenção — `AlertCenter.tsx` +
`computeAlerts` em `web/src/lib/alerts.ts`. A lista é recalculada do zero a
cada atualização a partir dos mesmos endpoints que já alimentam o resto do
dashboard.

| Alerta | Origem | Severidade |
|---|---|---|
| Inversor sem comunicação | `/inverters` → `status == "sem_comunicacao"` | crítico |
| Falha constante ao consultar a API do inversor (≥2x seguidas) | `/inverters` → `consecutive_failures`/`last_error` | atenção |
| Temperatura do inversor acima do limiar | `/inverters` → `temperature_c` | atenção |
| Bandeira amarela/vermelha ativa | `/day-status` → `bandeira` | atenção (hoje nunca dispara — ver "Limitações conhecidas" #2) |
| Geração do dia ≥10% abaixo da média da semana | `/summary` (hoje) vs. `/history?range=semana` (média) | informativo |

Cada alerta tem um botão "Marcar como lida", que some com ele da lista e do
contador do badge. A marcação vive em `sessionStorage`
(`solarhome_read_alerts`), então dura só pro acesso atual — se fechar a
aba/navegador, alertas cuja condição ainda esteja ativa voltam a aparecer
como não lidos. Intencional: marcar como lida serve pra não repetir o mesmo
aviso na mesma visita, não pra silenciar a condição de vez.

### Status do dia: clima e previsão

- **Cartão único, em linha do tempo**: uma tira fina no topo só com os
  fatos da usina (situação, geração do dia, bandeira tarifária vigente,
  quando disponível), e uma linha do tempo de 5 dias (hoje + próximos 4),
  onde "hoje" vem destacado por ser o único recalculado só nas horas de
  sol.
- **Clima recalculado só nas horas de sol**: a Open-Meteo entrega um
  `weathercode` diário que resume as 24h do dia (madrugada e noite
  incluídas, onde nuvem não afeta geração nenhuma). `handleForecast`
  (`api-go/internal/httpapi/weather_handlers.go`) busca também
  `hourly=weathercode,cloudcover` na mesma chamada e, só pro dia de hoje,
  recalcula o resumo (`weather_daylight`) e a favorabilidade
  (`rating_daylight`) usando a moda do `weathercode` apenas entre nascer e
  pôr do sol.
- **Ícones por badge**: a linha do tempo usa o mesmo sistema de
  `IconBadge` do resto do painel, mapeado a partir do `rating`
  (`bom`→sol dourado, `moderado`/`ruim`→nuvem azul).
- **Cache de 2h por usina** na consulta à Open-Meteo — chave `lat,lon`, em
  memória no processo da api-go (reseta a cada restart do container).

### Linha de média nos gráficos

O gráfico "Geração diária" do Dashboard (`GeracaoChart.tsx`) mostra uma
linha tracejada vermelha com a média do período selecionado, num segundo
dataset do Chart.js sobreposto às barras — visível abaixo do total do
período, e junto no tooltip ao passar o mouse em qualquer barra.

## Aba Histórico

Cada métrica é uma seção recolhível independente (`Collapsible.tsx`),
implementada em `web/src/pages/Dashboard/HistoricoTab.tsx`:

| Seção | Fonte |
|---|---|
| Quanto sua usina gerou | `/history` — aberta por padrão |
| Quanto você economizou | `/history` (`valor_estimado_brl`) |
| Seus melhores dias e meses | `/history/records` — melhor dia, melhor mês, maior potência já vista, sempre all-time |
| Sequência de dias acima da média | Derivado de `/history` no cliente — nenhum endpoint dedicado |
| Rendimento comparado ao período anterior | `total_kwh`/`installed_power_kwp` do período atual vs. anterior |
| Anotações sobre eventos importantes | `GET`/`POST /annotations` — 1 nota por dia (gravar de novo no mesmo dia sobrescreve) |
| Relatório em PDF | **desabilitado** — ver "Limitações conhecidas" #3 |

O seletor de período (Dia/Semana/Mês/Ano, no topo da aba) atualiza os
blocos de geração/economia/sequência/rendimento e a tabela; Recordes e
Anotações são sempre all-time. `/history` também retorna
`previous_total_kwh`/`previous_total_brl` (mesma duração, período
imediatamente anterior) pra mostrar a variação percentual — quando o
período anterior é zero, a comparação não aparece.

## Menu "Saúde da usina"

Separado do Histórico porque "quanto gerou" (medida pura) e "a usina está
funcionando direito?" (diagnóstico) são duas naturezas diferentes de
pergunta — ver "Princípio de design". Implementado em
`web/src/pages/Dashboard/SaudeTab.tsx`:

| Seção | Fonte |
|---|---|
| Quanto cada inversor contribuiu | `/history/inverters` — gráfico empilhado |
| Contribuição real vs. esperada pela capacidade | `/history/inverters?range=X` vs. capacidade hardcoded no frontend (ver "Limitações conhecidas" #5) |
| Confiabilidade da coleta | `/collector-health?days=30` — % de ciclos sem falha, por inversor |

`/history/inverters` deriva do último `power_kw`/`day_kwh` de cada dia por
inversor, mesmo princípio do total da usina, só que por credencial.
`/collector-health` lê direto da tabela `collector_health`, já gravada a
cada ciclo do worker (ver "Falhas de coleta").

**Ainda não implementado** (depende de dado novo da Huawei — mexe
exatamente na área que já causou um incidente de rate limit no passado,
então vale implementar com cautela):

- **Eficiência da geração** (Performance Ratio) e **impacto ambiental**
  (CO₂/carvão/árvores): a resposta de `getKpiStationDay` já traz
  `perpower_ratio`/`reduction_total_co2`/`_coal`/`_tree` (confirmado — ver
  "Backfill de histórico"), mas só é chamado pelo `cmd/backfill-history`
  avulso; o worker de tempo real (`collector`) ainda não expõe isso no dia
  a dia do Dashboard.
- **Radiação medida vs. geração**: mesma dependência do item acima.
- **Comparativo ano a ano**: sem dado real possível enquanto uma usina não
  completar 1 ano de operação.
- **Diagnóstico por string** (tensão/corrente por entrada MPPT): a Huawei
  já retorna isso em `getDevRealKpi`, nunca foi gravado nem exposto.

## Consumo por unidade consumidora (Celesc)

**Ainda não portado** — ver "Limitações conhecidas" #4. No painel Python
anterior, essa aba processava o upload manual da fatura em PDF da Celesc
(formato DANF3E) inteiramente em memória (`pdfplumber`), extraindo UC,
consumo, valor, bandeira tarifária e o histórico de 12-13 meses que cada
fatura já traz. A decisão de não integrar direto com a Celesc (sem API
oficial pra terceiros, só scraping frágil do portal do cliente) continua
valendo — a reintegração planejada é reimplementar o mesmo fluxo de upload
manual, agora contra o Postgres multi-tenant (a tabela `consumer_units` já
existe no schema, associada a `plant_id`).

## Princípio de design: fácil de ler pra qualquer idade

**Toda melhoria futura no painel — nova métrica, novo menu, novo alerta —
precisa ser pensada pra alguém sem bagagem técnica (ex.: usuário idoso)
conseguir olhar e entender o que está vendo, sem precisar perguntar.** Isso
vale mais que qualquer preferência técnica de nomenclatura. Na prática:

- **Título em linguagem direta, não jargão técnico.** "A usina está
  aproveitando bem o sol?" em vez de "Performance Ratio". O termo técnico
  pode aparecer como legenda secundária menor, nunca como título principal.
- **Resumo ao passar o mouse** em qualquer título de seção/métrica nova,
  explicando em 1-2 frases o que aquele bloco analisa (`Tooltip.tsx`).
- **Progressive disclosure**: seções recolhidas por padrão
  (`Collapsible.tsx`, reaproveitado em vez de criar um componente novo por
  seção), pra tela abrir limpa em vez de uma parede de números. Só o
  resumo mais importante do período fica aberto de cara.
- **Nunca esconder atrás de um termo sem explicação**: todo número tem, no
  mínimo, uma legenda curta dizendo o que ele significa (ex.: "kWh pra cada
  kW instalado" em vez de só "kWh/kWp").
- **Nome de menu reflete a pergunta que a pessoa está fazendo**, não a
  origem técnica do dado. Foi por isso que "Saúde da usina" virou seu
  próprio menu, separado do Histórico.

## Regra: selo "novo" por 5 dias

**Toda aba, menu ou seção nova adicionada ao painel precisa ficar marcada
como "novo" por 5 dias corridos a partir da data em que foi ao ar**, pra
chamar atenção de quem já usa o painel no dia a dia. Depois desse prazo o
selo some sozinho.

Mecanismo em `web/src/components/NewBadge.tsx`:

- `NEW_FEATURES_SINCE`: mapa `{ "chave": "YYYY-MM-DD" }` com a data em que
  cada novidade foi ao ar. **Toda vez que uma aba/menu/seção nova for
  adicionada, registrar uma entrada aqui** com a data do dia.
- `<NewBadge featureKey="chave" />` no ponto exato onde o selo deve
  aparecer (ao lado do nome do menu na `NavBar`, ao lado do título da
  seção, etc.).
- O componente decide sozinho se o selo aparece, comparando a data
  registrada com a data atual (`NEW_FEATURE_DAYS = 5`) — sem precisar de
  nenhum lembrete manual pra tirar depois.

## Estrutura do projeto

```
.
├── .env / .env.example        # credenciais (gitignored) / template
├── docker-compose.yml         # serviços: postgres, api, collector, web
├── postman/                   # collections Postman (api-go + integrações de terceiro)
├── docs/                      # doc oficial de referência (Huawei NBI, PDF)
├── api-go/
│   ├── cmd/
│   │   ├── api/                # entrypoint HTTP (porta 8000 no container)
│   │   ├── collector/           # entrypoint do worker de coleta
│   │   └── backfill-history/    # comando avulso de backfill (ver seção acima)
│   ├── internal/
│   │   ├── httpapi/             # handlers + router (chi)
│   │   ├── collector/            # supervisor + workers por credencial
│   │   ├── auth/                 # JWT, hash de senha, cifra AES-256 de credenciais
│   │   ├── huawei/ foxess/       # clientes das APIs oficiais dos fabricantes
│   │   ├── models/               # structs compartilhadas
│   │   └── db/                   # conexão + aplicação de migrations
│   └── migrations/               # schema Postgres (golang-migrate)
└── web/
    └── src/
        ├── pages/                # telas (Dashboard/Histórico/Saúde/Consumo/Administração/Minha conta)
        ├── components/           # AlertCenter, GeracaoChart, IconBadge, NewBadge, etc.
        ├── context/              # AuthContext, PlantContext
        ├── lib/                  # cliente HTTP (api.ts), cálculo de alertas, formatação
        └── styles/               # tokens CSS (dark theme)
```

## Endpoints da API

Autenticação por cookie de sessão httpOnly (JWT) — toda rota abaixo de
`/api/auth/*` exige sessão válida. Rotas `/api/plants/{plantID}/*` sempre
validam que a usina pertence ao usuário logado (404, nunca 403, se não
for). Rotas `/api/admin/*` exigem `is_admin = true`. Ver a collection
Postman `EnergiaSolar-API-Go` em `postman/` pra exemplos prontos de cada
uma.

| Rota | Retorna |
|---|---|
| `POST /api/auth/login` `logout` | Login/logout (sem cadastro público — contas só via admin) |
| `GET`/`PUT /api/me`, `PUT /api/me/password` | Perfil e senha da própria conta |
| `GET /api/plants`, `POST /api/plants` | Lista/cria usinas do usuário logado |
| `GET`/`PUT`/`DELETE /api/plants/{id}` | Consulta/edita/apaga uma usina |
| `GET /api/plants/{id}/summary` | KPIs atuais: potência, geração/economia do dia, pico, status |
| `GET /api/plants/{id}/inverters` | Status por inversor (gerando/online sem geração/sem comunicação) |
| `GET /api/plants/{id}/collector-health?days=N` | % de ciclos de coleta sem falha, por inversor |
| `GET /api/plants/{id}/history?range=dia\|semana\|mes\|ano` | Série de geração no período + totais atual/anterior |
| `GET /api/plants/{id}/history/records` | Recordes all-time: melhor dia, melhor mês, maior potência |
| `GET /api/plants/{id}/history/inverters?range=X` | Geração diária por inversor |
| `GET`/`POST /api/plants/{id}/annotations` | Lista/grava anotação do dia |
| `GET /api/plants/{id}/day-status` | Clima do dia (recalculado nas horas de sol), irradiância, alarme |
| `GET /api/plants/{id}/forecast` | Previsão 5 dias (Open-Meteo) |
| `GET`/`POST /api/plants/{id}/inverters-config` | Lista/cria credencial de inversor da usina |
| `POST /api/plants/{id}/inverters-config/test` | Testa uma credencial sem gravar |
| `PUT`/`DELETE /api/plants/{id}/inverters-config/{credID}` | Atualiza/apaga uma credencial |
| `GET /api/admin/users`, `POST /api/admin/users` | Lista/cria usuários (admin) |
| `GET`/`PUT`/`DELETE /api/admin/users/{id}` | Consulta/edita/apaga um usuário (admin) |
| `PUT /api/admin/users/{id}/password` | Redefine a senha de um usuário (admin) |
| `GET`/`PUT /api/admin/system-settings` | Configuração global: URLs padrão e intervalo do worker (admin) |

## Pendências / próximos passos

- [ ] Portar o upload/parser de fatura da Celesc e o cálculo de economia
      real (ver "Consumo por unidade consumidora")
- [ ] Portar o relatório em PDF do Histórico
- [ ] Portar a curva intradiária de alta resolução da FoxESS
      (`device/history/query`)
- [ ] Guardar capacidade (kWp) por credencial/inversor no schema, em vez do
      valor hardcoded no frontend (`INVERTER_CAPACITY_KWP`)
- [ ] Confirmar o formato real do `getAlarmList` da Huawei quando um
      alarme de verdade acontecer
- [ ] Validar o limiar de temperatura da Central de Alertas (`65°C`, hoje
      ilustrativo) contra a doc oficial do modelo exato do inversor de
      cada instalação
- [ ] Estender "Saúde da usina" com Performance Ratio, real vs. teórico,
      radiação e impacto ambiental — `getKpiStationDay` já foi implementado
      (`internal/huawei/client.go`, usado hoje só pelo `cmd/backfill-history`);
      falta ligar isso no worker/dashboard e, se precisar de granularidade
      mensal/anual, implementar `getKpiStationMonth`/`Year`
- [ ] Diagnóstico por string — FoxESS precisa de variáveis novas
      (`pv1Volt`/`pv2Volt`); Huawei já tem tudo em `getDevRealKpi`, só
      falta gravar/expor
- [ ] Comparativo ano a ano — sem dado real possível ainda (usina não
      completou 1 ano)
