# Solar Home — Painel de Monitoramento

Painel próprio de monitoramento para a usina solar residencial, rodando
100% local via Docker. A usina tem 2 inversores, cada um monitorado
diretamente pela **API oficial do seu fabricante** (Huawei FusionSolar e
FoxESS Cloud) — sem depender de nenhum acesso de usuário de terceiros.

**Sumário:** [Arquitetura](#arquitetura) · [Como rodar](#como-rodar) ·
[Detalhes da instalação](#detalhes-da-instalação) ·
[Fontes de dados oficiais](#fontes-de-dados-oficiais) ·
[Falhas de coleta e fallback seguro](#falhas-de-coleta-e-fallback-seguro) ·
[Limitações conhecidas](#limitações-conhecidas) ·
[Design do painel](#design-do-painel) ·
[Aba Histórico](#aba-histórico) ·
[Arquivamento de clima/irradiância](#arquivamento-de-climairradiância) ·
[Menu Saúde da usina](#menu-saúde-da-usina) ·
[Consumo por unidade consumidora](#consumo-por-unidade-consumidora-celesc) ·
[Princípio de design](#princípio-de-design-fácil-de-ler-pra-qualquer-idade) ·
[Selo "novo"](#regra-selo-novo-por-5-dias) ·
[Auditoria (2026-07-16)](#auditoria-2026-07-16) ·
[Estrutura do projeto](#estrutura-do-projeto) ·
[Pendências](#pendências--próximos-passos)

## Arquitetura

```
Huawei FusionSolar (NBI)  ─┐
                            ├─→  collector (Python)  →  InfluxDB  →  webapp (FastAPI)
FoxESS Cloud (OpenAPI)   ──┘                                    ↑         ↑
                                                    Open-Meteo (previsão)  │
                                     Upload manual de fatura (PDF, Celesc) ┘
```

- **collector**: a cada `COLLECT_INTERVAL_SECONDS` (padrão 300s), consulta a
  API da Huawei e a da FoxESS **de forma independente uma da outra** (se uma
  falhar, a outra continua gravando normalmente nesse ciclo — ver
  "Status por inversor" abaixo), soma a potência/geração dos dois e grava no
  InfluxDB — tanto o total da usina quanto o detalhe por inversor.
- **InfluxDB**: banco de séries temporais, guarda o histórico (bucket
  `solar-home`).
- **webapp**: dashboard web (dark theme, paleta validada — ver "Design do
  painel"), serve a página e expõe endpoints JSON que ele mesmo consome via
  fetch. Também recebe upload de fatura da Celesc (PDF) e extrai
  consumo/valor por unidade consumidora — ver "Consumo por unidade
  consumidora" abaixo.

## Como rodar

```bash
cp .env.example .env   # preencher as credenciais Huawei/FoxESS e PLANT_LAT/LON
docker compose up -d --build
```

- Painel: http://localhost:8080
- InfluxDB: http://localhost:8086

`.env` nunca é commitado (está no `.gitignore`); use `.env.example` como referência.

## Detalhes da instalação

- **20 módulos de 610 Wp** = 12,2 kWp instalados (nameplate DC)
- **Inversor FoxESS** (SIW200G-5K, 5 kW) — 13 módulos
- **Inversor Huawei** (SIW300H-3K, 3 kW) — 7 módulos
- **Capacidade AC total: 8 kW**
- Endereço: R. Guanabara, 3787 — Fátima, Joinville-SC

## Fontes de dados oficiais

| Inversor | Fabricante/API | Identificador | Capacidade |
|---|---|---|---|
| FoxESS | **FoxESS Cloud** | `deviceSN=J0MF502056LD436`, modelo `SIW200G M050 W1` | 5 kW |
| Huawei | **Huawei FusionSolar (NBI)** | `stationCode=NE=56719752`, `devDn=NE=56719754`, modelo `SIW300H M030 W00` | 3 kW |

(O nome comercial "SIW200G-5K"/"SIW300H-3K" usado pela instaladora é o
rebrand — o hardware/telemetria real é Huawei e FoxESS.)

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
- Endpoints usados: `getStationList`, `getDevList`, `getStationRealKpi`,
  `getDevRealKpi`, `getAlarmList`.
- **Rate limit confirmado por teste**: cada interface só pode ser chamada 1x
  a cada 5 minutos (senão retorna `failCode 407
  ACCESS_FREQUENCY_IS_TOO_HIGH`) — por isso `COLLECT_INTERVAL_SECONDS` não
  pode ser menor que 300. Login não tem essa restrição. Vale só pra
  `getStationRealKpi`/`getDevRealKpi`, que nunca falharam nesse intervalo em
  produção — **`getAlarmList` tem um limite próprio, mais restrito** (medido
  empiricamente em ~592-888s; ver "Falhas de coleta e fallback seguro"), por
  isso roda numa cadência separada de 900s em vez de todo ciclo de 300s.
- Não fornece curva de potência histórica — só o valor instantâneo do
  momento da chamada (por isso a curva intradiária Huawei no painel tem
  resolução de 5 min, uma por ciclo de coleta).

### FoxESS Cloud — OpenAPI

Documentação oficial: https://www.foxesscloud.com/public/i18n/en/OpenApiDocument.html

- Autenticação por **token privado**: headers `token` (a API key),
  `timestamp` (ms) e `signature` = MD5 de `path + "\r\n" + token + "\r\n" +
  timestamp`. Não usamos OAuth — o método de token privado já é suficiente e
  a doc não permite misturar os dois.
- Endpoints usados: `device/list` (descoberta do `deviceSN`),
  `device/real/query` (potência/geração instantânea),
  `device/history/query` (histórico de até 24h, usado para a curva
  intradiária).
- Rate limit: 1440 chamadas/dia por inversor, máx. 1 requisição/segundo —
  bem mais folgado que a Huawei.

## Falhas de coleta e fallback seguro

Cada inversor é coletado num `try/except` isolado (ver "Status por
inversor"), mas uma falha pontual na API não pode fazer o painel mentir sobre
o que já foi gerado. Duas proteções em `collector/main.py`:

- **`_carry_forward_day_kwh`**: geração diária só cresce, então quando a
  consulta de um inversor falha nesse ciclo, o total "Gerado hoje"
  (`daily_generation.generated_kwh`) usa o último `day_kwh` bem-sucedido
  daquele inversor em vez de contar a contribuição dele como zero. Sem
  sucesso ainda no dia (ex.: primeira tentativa do dia já falha), assume 0 —
  mesmo comportamento de antes do inversor acordar. Estado em memória, por
  inversor, resetado por dia.
- **Alerta de falha constante**: cada ciclo grava um ponto
  `collector_health` (tag `inverter`, campos `consecutive_failures` e
  `last_error`) — sucesso zera o contador, falha incrementa. A partir de 2
  falhas seguidas (`FAILURE_ALERT_THRESHOLD`, ~10 min de falha real, não 1
  blip isolado) o coletor loga como `ERROR` e o webapp expõe o contador via
  `/api/inverters`, virando alerta na Central de Alertas (ver tabela em
  "Design do painel").
  Isso é gravado mesmo se os dois inversores falharem no mesmo ciclo — é
  justamente aí que o alerta mais importa.

**Caso real que motivou isso** (2026-07-16): `getAlarmList` da Huawei tem um
limite de taxa mais restrito que os outros endpoints (ver "Fontes de dados
oficiais"), e falhava de forma determinística em 2 a cada 3 ciclos — cada
falha zerava a contribuição da Huawei no total do dia, fazendo "Gerado hoje"
cair momentaneamente. A causa raiz foi resolvida desacoplando o polling de
alarme (roda a cada 900s, isolado — uma falha nele não derruba mais
`power_kw`/`day_kwh` do ciclo); o fallback acima é a camada de segurança
que garante que qualquer falha residual, nesse ou em qualquer outro endpoint,
nunca faz o total regredir.

## Limitações conhecidas

1. **Sem previsão de geração**: nem Huawei nem FoxESS oferecem esse dado —
   confirmado checando as 151 variáveis do `device/variable/get` da FoxESS e
   os campos de `getStationRealKpi`/`getKpiStationDay` da Huawei, nenhum tem
   previsão/estimativa, só medição real. Uma estimativa própria (radiação
   solar do Open-Meteo × potência instalada × fator de performance) foi
   avaliada e **descartada** por decisão do usuário — o painel fica de fato
   sem previsão.
2. **Curva intradiária da Huawei é grosseira** (1 ponto a cada 5 min, pelo
   rate limit do NBI) — a FoxESS tem resolução bem melhor pro mesmo período.
3. **Histórico começa em 13/07/2026**: confirmado via `getKpiStationDay`
   (Huawei) e `device/report/query` (FoxESS) que a usina só passou a gerar
   de fato nessa data — não há dado real anterior a recuperar.
4. **"Gerado hoje" fica em 0 até o inversor acordar de manhã**: as nuvens da
   Huawei/FoxESS cacheiam o total do dia anterior e só atualizam quando o
   inversor manda telemetria nova — de madrugada ele fica dormindo. O
   coletor detecta isso e trata como zero até ver potência real (`power_kw
   > 0`) nesse dia local (`_apply_daily_reset_guard` em `collector/main.py`),
   pra não mostrar o total de ontem como se fosse de hoje. Comportamento
   esperado, não indica falha de coleta.
5. **Huawei nunca chega perto dos 3 kW nominais** (máximo histórico até
   17/07/2026: 1,745 kW, ~58% da capacidade) enquanto a FoxESS regularmente
   satura nos 5 kW dela — **homologado em 17/07/2026** puxando o
   `getDevRealKpi` bruto direto da API (fora do coletor, só leitura) e
   comparando com o InfluxDB: o campo `active_power` bate exatamente com
   `pv1_u × pv1_i` do string ativo (menos a perda de eficiência do
   inversor, `efficiency` no próprio payload), sem nenhum clamp/arredondamento
   no nosso código (`collector/main.py` só faz `dev_kpi.get("active_power")
   or 0.0`, sem teto). Ou seja, **a coleta está correta** — o gargalo é físico,
   não de dado. **Causa confirmada pelo próprio projeto de instalação**
   (documento 54287, 12/06/2026, fornecido pelo usuário): o inversor B
   (Huawei/SIW300H-3K) foi projetado com **7 módulos no MPPT1/String1 e 0
   módulos no MPPT2/String2** — só 4,27 kWp DC ligados num inversor de 3 kW
   AC (1,42x de sobredimensionamento), contra os 13 módulos (7,93 kWp,
   7+6 nas 2 strings) do inversor A/FoxESS. `mppt_2_cap`/`mppt_3_cap`/
   `mppt_4_cap` zerados na API batem exatamente com isso — **não é defeito
   nem string desconectada, é o desenho do projeto**. Pra aproveitar o teto
   de 3 kW da Huawei, precisaria de mais módulos no MPPT2 (hoje vazio).

## Design do painel

O layout foi redesenhado (mockup iterado e aprovado em Artifact antes de ir
pro código real): paleta validada pelo skill de dataviz (contraste + separação
CVD), números nunca "vestem" a cor da série (identidade vem de um indicador
ao lado, texto sempre em tinta neutra), tabular-nums pra alinhar dígitos, e
tooltips em vez de legendas fixas pra explicar estados. O CSS é um sistema de
tokens próprio (`--surface`, `--ink`, `--accent-*` etc. no `<style>` do
`index.html`), sem framework de UI; os gráficos são Chart.js estilizado pra
bater com os tokens.

### Ícones

Trocados de emoji/símbolos Unicode (☀ ▤ 💰 🏆 ⚠ etc.) por um sistema próprio
de ícones — 4 direções foram comparadas em mockup (linha, preenchido,
duotone, badge/flat) antes de escolher. Estilo escolhido: **badge/flat** —
ícone SVG dentro de um chip colorido arredondado, o mesmo padrão que os
selos de status (`.inv-status`) já usavam antes disso existir como sistema.

- `iconSvg(name, size, bg)` em `templates/index.html`: gera o SVG (24×24,
  `fill="currentColor"`) de cada ícone. O parâmetro `bg` é usado só pelos
  ícones que têm um "furo" interno (moeda da carteira, círculo do pin, "!" do
  alerta) — como o chip ao redor muda de cor conforme o contexto, o furo é
  pintado com a cor de fundo do chip específico daquela chamada, não uma cor
  fixa.
- `.icon-badge` + modificadores de cor (`.blue`/`.aqua`/`.green`/`.gold`/`.red`)
  e de tamanho (`.size-nav`/`.size-card`/`.size-alert`) no CSS — cor de fundo
  sempre a mesma tinta suave (~14% opacidade) já usada nos selos de status.
- Ícones estáticos (marca, navegação, cabeçalhos de card) usam
  `<span class="icon-badge {cor} {tamanho}" data-icon="{nome}">`, pintados
  uma vez por `paintIconBadges()` no carregamento da página.
- Ícones da Central de Alertas são gerados dinamicamente em
  `renderAlertList()`, com a cor do badge herdada da severidade do alerta
  (`SEV_COLOR`: crítico→vermelho, atenção→dourado, informativo→azul) — reforço
  visual que não existia com emoji.
- "Saúde da usina" usa só o traçado de pulso (`activity`), sem coração — ficou
  mais direto como ícone de "monitoramento", sem o símbolo de coração que
  remetia mais a "saúde humana" do que "usina".

### Status por inversor

Cada inversor (Huawei, FoxESS) mostra um selo de status, calculado no
`webapp` (`/api/inverters`) puramente pela idade do último ponto gravado —
não existe campo dedicado no InfluxDB pra isso:

| Status | Condição |
|---|---|
| **Gerando** | ponto com menos de 15 min e potência > 0 |
| **Online, sem geração** | ponto com menos de 15 min e potência = 0 (normal à noite) |
| **Sem comunicação** | nenhum ponto nos últimos 15 min (3 ciclos de coleta de 5 min) |

"Sem comunicação" é o sinal mais próximo de "pode ter caído a energia/Wi-Fi/
disjuntor" que dá pra obter sem depender de código de status do fabricante —
pesquisamos os valores de `run_state`/`inverter_state` (Huawei) e `status`
(FoxESS) e não achamos documentação oficial confiável; um mapeamento
encontrado numa integração open-source pra FoxESS **contradisse** o que
observamos ao vivo (status `3` apareceu com o inversor gerando normalmente),
então preferimos não confiar nisso pra uma decisão real.

Isso só funciona porque `collector/main.py` coleta cada inversor em um
`try/except` isolado — uma falha na Huawei não impede a FoxESS de gravar no
mesmo ciclo (e vice-versa).

### Central de alertas

Accordion no topo do Dashboard (fechado por padrão, com contador de alertas
ativos no badge) que resume tudo que merece atenção. Quais alertas existem
não tem estado próprio: a lista é recalculada do zero a cada atualização
(30s) a partir dos mesmos endpoints que já alimentam o resto do dashboard —
se uma condição deixa de existir, o alerta correspondente some sozinho na
atualização seguinte (o único estado guardado no cliente é o de "lido", ver
abaixo):

| Alerta | Origem | Severidade |
|---|---|---|
| Inversor sem comunicação | `/api/inverters` → `status == "sem_comunicacao"` | crítico |
| Falha constante ao consultar a API do inversor (≥2x seguidas) | `/api/inverters` → `consecutive_failures`/`last_error` | atenção |
| Temperatura do inversor acima do limiar | `/api/inverters` → `temperature_c` | atenção |
| Bandeira amarela/vermelha ativa | `/api/day-status` → `bandeira` | atenção |
| Geração do dia ≥10% abaixo da média da semana | `/api/summary` (hoje) vs. `/api/history?range=semana` (média) | informativo |

O limiar de temperatura (`TEMP_THRESHOLD_C = 65`, em `templates/index.html`)
é **ilustrativo** — não validamos contra a doc oficial dos modelos exatos
(SIW300H-3K / SIW200G-5K), só pesquisa geral de mercado (ver pendências).

Cada alerta tem um botão "Marcar como lida", que some com ele da lista e do
contador do badge. A marcação vive em `sessionStorage` (chave por tipo de
alerta, ex. `bandeira:amarela`), então dura só pro acesso atual — se fechar a
aba/navegador e abrir de novo, alertas cuja condição ainda esteja ativa
voltam a aparecer como não lidos. Isso é intencional: marcar como lida serve
pra não ficar repetindo o mesmo aviso na mesma visita, não pra silenciar a
condição de vez.

### Status do dia: clima e previsão

- **Cartão único, em linha do tempo** (redesenhado em 2026-07-17: mockup com 4
  opções iterado e aprovado em Artifact, a pedido do usuário — o layout antigo
  dividia clima/situação/geração numa grade de 3 colunas e repetia o clima de
  hoje de novo numa coluna própria do card "Previsão", separado ao lado).
  Hoje o card "Status do dia" tem 2 partes:
  - uma tira fina no topo só com os fatos **da usina** (situação, geração do
    dia, bandeira tarifária vigente);
  - uma **linha do tempo** de 5 dias (hoje + próximos 4), onde "hoje" vem
    destacado (badge "HOJE", borda de cor) por ser o único recalculado só nas
    horas de sol — os outros 4 usam o resumo bruto do dia inteiro da
    Open-Meteo. Isso elimina a duplicação: o clima de hoje só aparece uma vez,
    como o primeiro item da própria linha do tempo.
- **Clima recalculado só nas horas de sol**: a Open-Meteo entrega um
  `weathercode` diário que resume as 24h do dia (madrugada e noite incluídas,
  onde nuvem não afeta geração nenhuma) — descoberto ao investigar por que o
  card mostrava "Nublado" num dia que, na prática, gerou o recorde histórico
  da usina. `_forecast_days()` em `webapp/main.py` busca também
  `hourly=weathercode,cloudcover` na mesma chamada e, só pro dia de hoje,
  recalcula o resumo (`weather_daylight`) usando a moda do `weathercode`
  **apenas entre nascer e pôr do sol** (`_hour_in_daylight`) — e, desde
  2026-07-17, também recalcula a **favorabilidade** (`rating_daylight`) do
  mesmo jeito, porque o ícone do tile "Hoje" usava o `rating` do dia inteiro e
  podia contradizer o texto recalculado (ex.: texto "Principalmente limpo"
  com ícone de nuvem) — mesma classe de bug que motivou o `weather_daylight`
  original. O tile "Hoje" mostra o valor recalculado (com o horário da janela
  de sol logo abaixo), a irradiância do dia (`solar_radiation_mj_m2`) e um
  gráfico de nuvens hora a hora com a janela de sol destacada — pra deixar
  visível *por que* o resumo pode divergir do clima de "senso comum" de quem
  olhou pra fora hoje.
- **Ícones por badge, não mais emoji**: a linha do tempo usa o mesmo sistema
  de `iconSvg`/`.icon-badge` do resto do painel (ver "Ícones" acima) em vez do
  emoji fixo (`☀️`/`⛅`/`🌧️`) que o card "Previsão" antigo usava — mapeado a
  partir do `rating` (`bom`→sol dourado, `moderado`/`ruim`→nuvem azul).
- **Cache de 2h na consulta à Open-Meteo**: o frontend chama `/api/day-status`
  a cada 30s (fatos da usina) e `/api/forecast` a cada 30min (linha do tempo
  inteira, incluindo o tile de hoje) — sem cache isso bateria na Open-Meteo ao
  vivo pra um dado que o modelo deles só atualiza a cada poucas horas.
  `_forecast_days()` guarda a resposta numa variável de módulo
  (`_forecast_cache`) por até `_FORECAST_CACHE_TTL` (2h) antes de consultar de
  novo — cache em memória simples, suficiente porque o webapp roda como 1
  processo uvicorn só (sem múltiplos workers). Reseta sozinho a cada restart
  do container (só implica 1 consulta a mais logo depois).

### Outros campos

- **"Gerado hoje" vs. ontem**: mockup iterado e aprovado em Artifact antes do
  código real — `▲`/`▼` colorido (verde/vermelho) com a variação percentual,
  logo abaixo do valor em kWh (a % é sobre produção, não sobre a estimativa
  em R$, por isso fica acima do "estimado"). `/api/summary` calcula
  `today_vs_yesterday_pct` comparando com o penúltimo ponto de
  `daily_generation` (`_yesterday_generated_kwh()` em `webapp/main.py`) — sem
  esse valor (ex.: ontem a usina ainda não tinha ligado), o indicador
  simplesmente não aparece.
- **Temperatura do inversor**: Huawei via `getDevRealKpi` (campo
  `temperature`), FoxESS via a variável `invTemperation`. A Huawei retornou
  `0.0` em todos os testes até agora — pode ser normal pra esse modelo fora
  de operação ativa, ou o campo pode não vir preenchido; ainda não
  confirmamos como se comporta durante geração de pico.
- **Detalhe do alarme**: quando `has_alarm` é `true`, tentamos extrair o
  nome/descrição do primeiro alarme (`alarmName`/`name`/`desc`/`alarmCause`,
  o que vier primeiro). **Não verificado contra um alarme real** — só
  testamos com a lista vazia, e não há documentação dos nomes de campo
  exatos. Ajustar quando um alarme de verdade aparecer nos logs.

### Linha de média nos gráficos

Os gráficos "Geração diária" (Dashboard e Histórico) mostram uma linha
tracejada vermelha com a média do período selecionado (diária pra
semana/mês, mensal pro ano), num segundo dataset do Chart.js sobreposto às
barras. O valor fica sempre visível abaixo do total do período, e passar o
mouse em qualquer barra mostra a média junto no tooltip (mesmo índice do
eixo X, `interaction.mode: "index"`).

## Aba Histórico

Reformulada (mockup iterado e aprovado em Artifact, com o usuário pedindo
explicitamente títulos legíveis pra qualquer idade — ver "Princípio de
design"). Cada métrica é uma seção recolhível independente (reaproveita
`.alert-card`/`.alert-toggle`/`.alert-body` da Central de Alertas, com
`overflow: visible` numa classe extra `.hist-collapsible` — o card de
alerta original usa `overflow: hidden`, que cortava o tooltip; ver
`initHistoricoTab()` em `templates/index.html`):

| Seção | Título exibido | Fonte |
|---|---|---|
| Quanto gerou | "☀ Quanto sua usina gerou" | `/api/history` — aberta por padrão |
| Quanto economizou | "💰 Quanto você economizou" | `/api/history` (mesmo campo `valor_estimado_brl` de sempre, agora com bloco próprio em vez de escondido atrás do toggle Gerado/Economia) |
| Recordes | "🏆 Seus melhores dias e meses" | `/api/history/records` — melhor dia, melhor mês (`aggregateWindow(every: 1mo)`), maior potência já vista — sempre desde que a usina ligou, independe do período selecionado |
| Sequência acima da média | "📈 Sequência de dias acima da média" | Deriva de `/api/history` no cliente — nenhum endpoint novo. Conta quantos dias mais recentes seguidos ficaram acima da média de geração do período selecionado, e mostra o melhor/pior dia de dentro desse mesmo período |
| Rendimento vs. período anterior | "☀ Rendimento comparado ao período anterior" | `total_kwh` / `installed_power_kwp` do período atual vs. `previous_total_kwh` / `installed_power_kwp` — kWh por kW instalado é mais justo que comparar só o total porque não depende do tamanho do período |
| Anotações | "📌 Anotações sobre eventos importantes" | `POST`/`GET /api/annotations` — 1 nota por dia (gravar de novo no mesmo dia sobrescreve), measurement `annotation` dedicado |

O seletor de período (Semana/Mês/Ano, no topo da aba) atualiza os blocos
de geração/economia/sequência/rendimento e a tabela; Recordes e Anotações
são sempre all-time. `/api/history` também retorna
`previous_total_kwh`/`previous_total_brl` (mesma duração, período
imediatamente anterior) pra mostrar "▲ 12% a mais que no período
anterior" — quando o período anterior é zero (ex.: antes da usina ligar),
a comparação não aparece em vez de mostrar um percentual sem sentido.

**Relatório em PDF**: botão único no topo da aba (`GET
/api/history/report.pdf?range=X`), gerado com `reportlab`
(`webapp/report_pdf.py`), desenhado direto via `canvas` (sem HTML→PDF) pra
evitar dependência de sistema tipo Cairo/Pango. Layout de página única,
sem paginação: gráficos de barra mostram só os últimos 60 dias do período
e a tabela só os últimos 10. Cabeçalho traz a data/hora exata de geração
do relatório em destaque; os 3 blocos de resumo (energia gerada, economia,
rendimento kWh/kWp) mostram o percentual de variação vs. o período
anterior, colorido em verde (▲) ou vermelho (▼); um painel de recordes
mostra melhor dia do período, melhor dia histórico e maior potência já
registrada; os gráficos de barra rotulam valor e data em cada barra
(desligado acima de 31/15 barras respectivamente, pra não poluir períodos
longos); uma seção mostra a contribuição real de cada inversor (kWh e %)
comparada ao esperado pela capacidade instalada; anotações do período
(até 6) aparecem antes da tabela.

## Arquivamento de clima/irradiância

**Corrigido em 2026-07-17** (ver "Auditoria" acima): a Open-Meteo já era
consultada pelo `webapp` (`_forecast_days()`) pra alimentar o card "Status do
dia", mas o resultado só vivia no cache de 2h em memória — nunca era salvo,
então o clima/irradiância de qualquer dia passado se perdia pra sempre assim
que o cache expirava. Sem esse histórico, não dá pra calcular Performance
Ratio nem comparar radiação vs. geração real de um dia específico (ver
"Menu Saúde da usina" abaixo — são métricas aprovadas no mockup mas ainda
não implementadas, e dependiam desse dado existir).

- A mesma chamada que já era feita ganhou o parâmetro `past_days=3`
  (`PAST_DAYS_TO_ARCHIVE` em `webapp/main.py`) — a Open-Meteo passa a
  devolver, além da janela de previsão de sempre (hoje + 4 dias), os 3 dias
  anteriores já **encerrados e observados** (não previsão). "Hoje" muda de
  índice (`today_index = PAST_DAYS_TO_ARCHIVE`) no array retornado pela API,
  mas a fatia devolvida pro resto do código (`/api/forecast`, `/api/day-status`)
  continua exatamente a mesma de antes (hoje + 4 dias) — o corte acontece
  antes de cachear/retornar, então nenhum consumidor existente precisou mudar.
- `_persist_past_weather()` grava 1 ponto por dia (measurement `weather_daily`,
  tag `plant_id`) pros dias já encerrados, com timestamp fixo na meia-noite
  BRT de cada dia — mesmo truque do `daily_generation` do collector, uma nova
  gravação do mesmo dia sobrescreve o ponto em vez de duplicar. Roda a cada
  refresh do cache (a cada 2h, só quando alguém tem o painel aberto — não há
  processo em background/agendado).
- **Janela de 3 dias, não 1**: como a gravação só acontece quando o painel é
  acessado, usar só "ontem" deixaria um buraco se o painel ficasse mais de 1
  dia sem ser aberto. Com 3 dias de folga, o próximo acesso recupera os dias
  perdidos sozinho (reescrever um dia já arquivado é seguro, ver acima) — mas
  ainda existe uma janela real de perda se o painel ficar **mais de 3 dias**
  sem ser aberto nenhuma vez; não há garantia absoluta sem um processo
  agendado próprio, que não foi implementado por ora (baixo custo de decidir
  depois, dado que o painel é usado diariamente).
- Falha ao gravar (`write_api.write`) é capturada e logada, sem derrubar
  `/api/forecast`/`/api/day-status` — é só arquivamento, a previsão em si já
  foi obtida com sucesso a essa altura.

## Menu "Saúde da usina"

Separado do Histórico depois que o usuário notou que "Histórico" estava
acumulando duas naturezas diferentes de conteúdo: medida pura (quanto
gerou) vs. diagnóstico (a usina está funcionando direito?). Ver "Princípio
de design".

| Seção | Título exibido | Fonte |
|---|---|---|
| Contribuição por inversor | "🔀 Quanto cada inversor contribuiu" | `/api/history/inverters` — gráfico empilhado, sempre no mês |
| Contribuição real vs. esperada | "🎯 Contribuição real vs. esperada pela capacidade" | `/api/history/inverters?range=X`, com seletor Dia/Semana/Mês/Ano — compara o % real gerado por cada inversor com o % esperado só pela capacidade instalada (Huawei 3 kW = 37,5%, FoxESS 5 kW = 62,5%), pra notar se um lado está rendendo menos que deveria |
| Confiabilidade da coleta | "🛡 Confiabilidade da coleta de dados" | `/api/collector-health?days=30` — % de ciclos de coleta sem falha nos últimos 30 dias, por inversor |

`/api/history/inverters` **não precisou de nenhum dado novo do coletor**:
deriva do último `inverter_status.day_kwh` de cada dia-calendário (fuso
`BRAZIL_TZ`, igual ao collector), por inversor — o mesmo princípio que já
vale pro `daily_generation` (o último valor do dia é o total do dia), só
que aplicado por inversor em vez do total da usina. Semana/Mês/Ano usam o
mesmo `RANGE_DAYS` (janela corrida, não alinhada ao calendário) do resto do
painel — com pouco histórico acumulado (usina ligou 13/07/2026), esses 3
períodos podem mostrar o mesmo resultado até existir mais de ~7 dias de
dado. **Dia é exceção** (corrigido em 2026-07-17): usa a meia-noite BRT de
hoje como início em vez de `-1d` rolante — com `-1d`, o último ponto de
ontem (gravado ~23:55) caía dentro da janela e virava um dia extra na soma,
fazendo "Contribuição real vs. esperada" (aba Saúde da usina) somar
ontem-inteiro + hoje-parcial como se fosse só hoje.

`/api/collector-health` também **não precisou de coleta nova**: o
measurement `collector_health` já grava 1 ponto por ciclo (sucesso ou
falha) desde sempre — ver "Falhas de coleta e fallback seguro" — só nunca
tinha sido exposto num endpoint.

**Ainda não implementado** (aprovado no mockup, mas depende de dado novo da
Huawei — mexe exatamente na área que já causou o incidente de rate limit
documentado em "Falhas de coleta e fallback seguro", então vale implementar
com cautela e medição, não de uma vez):

- **Eficiência da geração** (Performance Ratio, geração real vs. teórica) e
  **impacto ambiental** (CO₂/carvão/árvores): precisam de
  `getKpiStationDay`/`Month`/`Year` da Huawei, endpoints nunca chamados
  pelo coletor até hoje.
- **Radiação medida vs. geração**: mesma dependência do item acima
  (`radiation_intensity`).
- **Comparativo ano a ano**: sem dado real possível ainda de qualquer forma
  (usina ligou 13/07/2026, não completou 1 ano).
- **Diagnóstico por string** (tensão/corrente por entrada MPPT): precisa de
  variáveis novas no `device/history/query` da FoxESS (`pv1Volt`/`pv2Volt`),
  nunca usadas pelo coletor. Pro lado da Huawei, `getDevRealKpi` já retorna
  isso pronto (`pv1_u`/`pv1_i` ... `pv8_u`/`pv8_i`, `mppt_1_cap`...
  `mppt_4_cap`) — só nunca foi gravado no InfluxDB nem exposto no painel.
  Motivada pela homologação de 17/07/2026 (ver "Limitações conhecidas" #5):
  o MPPT2 da Huawei está zerado por projeto (0 módulos, confirmado no
  documento de instalação), não por defeito — mas gravar isso no InfluxDB
  ainda vale, pra monitorar se o MPPT1 (a única string ativa) degrada com o
  tempo.

## Consumo por unidade consumidora (Celesc)

A usina compensa energia em **2 unidades consumidoras (UC)** da Celesc, a
concessionária local — não é só a usina que "consome" a própria geração:

| UC (formato novo ANEEL) | Endereço | Titular |
|---|---|---|
| `19647901154` | Guanabara 3787 (onde a usina está) | Maria Terezinha da Veiga |
| `298240601131` | Elizabeth Rech 171 | Marcelo Romano da Veiga (portabilidade de créditos) |

**Por que não integramos direto com a Celesc**: avaliamos e descartamos —
não existe API oficial pra consulta de consumo/fatura de terceiros, só o
portal de login do cliente (`conecte.celesc.com.br`). Automatizar isso
seria scraping com credencial de usuário — frágil, e pior ainda tratando-se
de dado financeiro (fatura), não só telemetria, sujeito a CAPTCHA/bloqueio
anti-bot.

**Solução adotada**: upload manual da "2ª via" da fatura em PDF (documento
fiscal padronizado DANF3E) pela aba "Consumo" do painel. O PDF é processado
inteiramente em memória (`webapp/celesc_bill_parser.py`, via `pdfplumber`)
e **nunca é salvo em disco** — só os campos extraídos vão pro InfluxDB
(measurement `consumption`, tag `uc`). Testado com faturas reais, extrai:
UC, titular, referência, vencimento, valor total, consumo (kWh), bandeira
tarifária, e a tabela de **histórico de 12-13 meses** que cada fatura já
traz — ou seja, um único upload já faz backfill de quase 1 ano.

- **Formato de UC em transição**: a ANEEL mandou migrar pro formato novo
  (11-12 dígitos, REN 1095/2024). Faturas em transição ainda mostram o
  número antigo no topo — o parser lê a frase "seu novo número e passará a
  ser X" no comunicado da fatura e usa **sempre o formato novo** como tag,
  pra não quebrar a série no InfluxDB quando a próxima fatura já vier só
  com o número novo.
- **Economia**: até agora, nenhuma fatura enviada cobre um período com
  geração solar (a usina só ligou 13/07 — ver "Limitações conhecidas") —
  não tem como validar contra dado real ainda. O painel mostra uma
  **estimativa** (geração acumulada × tarifa efetiva da fatura mais
  recente), com selo "estimativa" visível. Quando uma fatura futura trouxer
  o crédito de compensação oficial da Celesc, o parser precisa ser
  estendido pra ler esse campo — os rótulos exatos que a Celesc usa pra
  isso ainda não são conhecidos, só vamos saber quando a fatura chegar.
- **Tarifa pública da Celesc** (`celesc.com.br/tarifas-de-energia`,
  `celesc.com.br/bandeiras-tarifarias`): existe e é pública, mas decidimos
  não consultar periodicamente por enquanto — cada fatura já traz a tarifa
  exata que valeu naquele período, o que é mais preciso que qualquer
  consulta externa. Fica como possível melhoria futura só se quisermos
  projetar o mês corrente antes da fatura chegar.

## Princípio de design: fácil de ler pra qualquer idade

**Toda melhoria futura no painel — nova métrica, novo menu, novo alerta —
precisa ser pensada pra alguém sem bagagem técnica (ex.: usuário idoso)
conseguir olhar e entender o que está vendo, sem precisar perguntar.** Isso
vale mais que qualquer preferência técnica de nomenclatura. Na prática:

- **Título em linguagem direta, não jargão técnico.** "A usina está
  aproveitando bem o sol?" em vez de "Performance Ratio". O termo técnico
  pode aparecer como legenda secundária menor, nunca como título principal.
- **Resumo ao passar o mouse** em qualquer título de seção/métrica nova,
  explicando em 1-2 frases o que aquele bloco analisa — antes mesmo de abrir
  (componente `.tt`/`.tip` já existente em `templates/index.html`; usar a
  variante `.tip-wide` quando o texto for mais longo que uma frase curta).
- **Progressive disclosure**: seções recolhidas por padrão (mesmo padrão
  visual da Central de Alertas — classes `.alert-card`/`.alert-toggle`/
  `.alert-body`, reaproveitadas em vez de criar um componente novo), pra
  tela abrir limpa em vez de uma parede de números. Só o resumo mais
  importante do período fica aberto de cara.
- **Nunca esconder atrás de um termo sem explicação**: todo número tem, no
  mínimo, uma legenda curta dizendo o que ele significa (ex.: "kWh pra cada
  kW instalado" em vez de só "kWh/kWp").
- **Nome de menu reflete a pergunta que a pessoa está fazendo**, não a
  origem técnica do dado. Foi por isso que "Qualidade da geração" virou seu
  próprio menu, "Saúde da usina" — ver "Menu Saúde da usina".

Esse princípio nasceu da revisão da aba Histórico (mockup iterado e aprovado
em Artifact — ver "Aba Histórico"), mas vale pra qualquer tela nova do
projeto daqui pra frente.

## Regra: selo "novo" por 5 dias

**Toda aba, menu ou seção nova adicionada ao painel precisa ficar marcada
como "novo" por 5 dias corridos a partir da data em que foi ao ar**, pra
chamar atenção de quem já usa o painel no dia a dia e não ia notar a
novidade sozinho. Depois desse prazo o selo some — não é permanente.

Mecanismo já implementado em `templates/index.html`, reaproveitar em vez de
inventar de novo a cada novidade:

- `NEW_FEATURES_SINCE`: objeto `{ "chave": "YYYY-MM-DD" }` com a data em que
  cada novidade foi ao ar. **Toda vez que uma aba/menu/seção nova for
  adicionada, registrar uma entrada aqui** com a data do dia.
- `<span data-new-key="chave">` no HTML, no ponto exato onde o selo deve
  aparecer (ao lado do nome do menu, ao lado do título da seção, etc.).
- `paintNewBadges()` varre esses elementos e decide sozinho se o selo
  aparece (chamado uma vez no carregamento, junto com `paintIconBadges()`)
  — comparando a data registrada com a data atual, sem precisar de nenhum
  lembrete manual pra tirar depois dos 5 dias.

Exemplos já em uso, todos adicionados em 2026-07-16: o menu "Saúde da
usina" (`nav-saude`), o clima recalculado nas horas de sol
(`clima-horas-de-sol`), e as 4 seções novas de Histórico/Saúde da usina
(`hist-streak`, `hist-yield`, `saude-reliability`, `saude-contrib-range`)
— ver "Aba Histórico", "Menu Saúde da usina" e "Status do dia: clima e
previsão".

## Estrutura do projeto

```
.
├── .env / .env.example        # credenciais (gitignored) / template
├── docker-compose.yml         # influxdb + collector + webapp
├── collector/
│   ├── huawei_client.py       # cliente da NBI oficial da Huawei
│   ├── foxess_client.py       # cliente da OpenAPI oficial da FoxESS
│   └── main.py                # loop de coleta: soma os 2 inversores → grava no InfluxDB
└── webapp/
    ├── main.py                  # FastAPI: serve o dashboard + endpoints JSON
    ├── celesc_bill_parser.py    # extrai dados da fatura da Celesc (PDF, em memória)
    ├── report_pdf.py            # monta o relatório de Histórico em PDF (reportlab)
    └── templates/index.html     # dashboard (CSS próprio + Chart.js, dark theme)
```

### Endpoints do webapp

| Rota | Retorna |
|---|---|
| `GET /` | Página do dashboard |
| `GET /api/summary` | KPIs atuais: potência, geração/economia do dia (+ variação % vs. ontem), pico do dia + horário, status |
| `GET /api/inverters` | Potência/geração/temperatura/status (gerando, online sem geração, sem comunicação) por inversor, + `consecutive_failures`/`last_error` da coleta |
| `GET /api/day-status` | Status do dia: clima (recalculado nas horas de sol), irradiância, nuvens por hora, sol, alarme (+ detalhe), geração, bandeira vigente |
| `GET /api/history?range=dia\|semana\|mes\|ano` | Série de geração no período + `total_kwh`/`total_brl` (estimado) e `previous_total_kwh`/`previous_total_brl` do período anterior |
| `GET /api/history/records` | Recordes all-time: melhor dia, melhor mês, maior potência já vista |
| `GET /api/history/inverters?range=dia\|semana\|mes\|ano` | Geração diária por inversor (Huawei/FoxESS), derivada do `inverter_status` já coletado |
| `GET /api/history/report.pdf?range=X` | Relatório em PDF enriquecido (recordes do período, contribuição por inversor, anotações, rendimento, deltas vs. período anterior) pra download |
| `GET /api/collector-health?days=30` | % de ciclos de coleta sem falha por inversor, derivado do `collector_health` já gravado a cada ciclo |
| `POST /api/annotations` | Grava uma anotação (`date`, `note`) — 1 por dia, sobrescreve se já existir |
| `GET /api/annotations?range=X` | Lista anotações do período, mais recente primeiro |
| `GET /api/forecast` | Previsão do tempo 5 dias (Open-Meteo, sem API key, mesmo cache de 2h do `/api/day-status`) |
| `POST /api/consumption/upload` | Recebe fatura PDF da Celesc (multipart), extrai e grava no InfluxDB |
| `GET /api/consumption/summary` | Resumo por UC (última fatura) + economia estimada |
| `GET /api/consumption/history?uc=X` | Série histórica de consumo (kWh/R$) de uma UC |

## Auditoria (2026-07-16)

Revisão ponta a ponta pedida pelo usuário: consultas a API, dados exibidos no
Dashboard, comportamento na virada do dia, coerência do Histórico com o banco,
e o que falta coletar hoje pra não faltar dado daqui 12 meses. Achados abaixo,
verificados direto no código e no InfluxDB rodando (não é uma estimativa).

### 1. Consultas a APIs — inventário e frequência

**Coletor → Huawei/FoxESS** (a cada `COLLECT_INTERVAL_SECONDS`, 300s/5min):

| Chamada | Cadência real | Observação |
|---|---|---|
| Huawei `login`, `getStationRealKpi`, `getDevRealKpi` | todo ciclo (300s) | nunca falhou nesse intervalo em produção |
| Huawei `getAlarmList` | a cada 900s (1 a cada 3 ciclos) | desacoplado depois do incidente de rate limit — ver "Falhas de coleta" |
| FoxESS `device/real/query` | todo ciclo (300s) | dentro do limite (1440/dia) com folga |
| FoxESS `device/history/query` (curva intradiária, 3h de janela) | todo ciclo (300s) | **sobreposição**: cada chamada busca 3h, rodando a cada 5min — ~97% do intervalo já foi buscado no ciclo anterior. Não é incorreto (InfluxDB dedup por timestamp), só busca mais dado do que precisaria. |

**Webapp → Open-Meteo**: cacheado por 2h desde a auditoria anterior (ver
"Status do dia: clima e previsão") — antes disso era ao vivo a cada chamada.

**Frontend → webapp** (`setInterval`, todos rodando em paralelo o tempo todo
que a aba estiver aberta):

| Timer | Intervalo | Endpoints chamados |
|---|---|---|
| `refreshSummary` | 30s | `/api/summary` |
| `refreshInverters` | 30s | `/api/inverters` |
| `refreshDayStatus` | 30s | `/api/day-status` |
| `refreshAlerts` | 30s | `/api/inverters` + `/api/day-status` + `/api/summary` + `/api/history?range=semana` |
| `refreshForecast` | 30min | `/api/forecast` |

**Achado**: `refreshAlerts` refaz sozinho 3 das 4 chamadas que os outros
timers já fazem (`/api/inverters`, `/api/day-status`, `/api/summary`) — no
mesmo intervalo de 30s, sem cache nem compartilhamento de resultado entre
timers. Ou seja, a cada 30s o browser faz ~7 requisições quando 4
bastariam, e a mais cara delas (`/api/history?range=semana`, que já embute
2 consultas ao InfluxDB — o período e o período anterior) roda inteira toda
vez só pra alimentar 1 alerta. Não é um bug — os números continuam corretos
— é desperdício de consulta ao banco que vale revisitar (dá pra fazer
`refreshAlerts` reaproveitar o resultado dos outros 3 fetches em vez de
buscar de novo).

### 2. Dados do Dashboard — fonte e comportamento na virada da meia-noite

| Dado exibido | Fonte / janela da consulta | Zera à meia-noite (BRT)? |
|---|---|---|
| Potência instantânea | último ponto de `plant_status` | não se aplica (é "agora", não "hoje") — vai pra ~0 sozinho à noite por não ter sol, não por lógica de data |
| **Gerado hoje** | último ponto de `daily_generation` (janela `-3d`) | **sim**, mas com até ~5min de atraso: o coletor só escreve o ponto zerado do novo dia no primeiro ciclo depois da virada — entre 00:00 e esse ciclo, o painel ainda mostra o total de ontem |
| **Gerado hoje vs. ontem (▲/▼)** | `today_generated_kwh` − penúltimo ponto de `daily_generation` | correto mas pouco útil logo após meia-noite: mostra **"▼ 100%"** todo dia de madrugada (0 gerado ainda vs. o total de ontem) até o sol nascer — matematicamente certo, mas pode ler como alarme falso às 3h da manhã |
| kWh/temperatura por inversor | último ponto de `inverter_status` (janela `-1h`) | sim, mesmo atraso de ~5min do item acima (mesmo guard de reset por inversor) |
| **Pico hoje (kW + horário)** | máximo de `plant_status.instantaneous_power_kw` desde a meia-noite local (`range(start: <meia-noite BRT de hoje>)`) | **sim** — corrigido em 2026-07-17 (era `range(start: -24h)`, janela rolante; um pico de ontem à tarde continuava aparecendo como "Pico hoje" até completar 24h) |
| Situação / alarme | `plant_status.has_alarm` (janela `-1h`) | não é por dia, é "última hora" — comportamento correto pro que se propõe |
| Bandeira tarifária | fatura mais recente | não é diário (é mensal), correto como está |
| Clima / irradiância / nuvens | Open-Meteo, recalculado só nas horas de sol de hoje | sim, muda de dia porque a própria Open-Meteo já entrega por data |

**Resumindo o achado principal deste item**: só o "Pico hoje" tinha uma
inconsistência real entre o nome e o comportamento — os demais "hoje" zeram
corretamente na virada (com uma folga de até ~5min, que é o próprio
intervalo de coleta, não um bug). Corrigido em 2026-07-17: `/api/summary`
agora calcula o range a partir da meia-noite BRT do dia corrente
(`start_of_day_brazil`, `webapp/main.py`) em vez de `range(start: -24h)`.

### 3. Histórico vs. banco de dados

Comparado direto: `/api/history` (o que a aba mostra) contra uma consulta
crua no InfluxDB pro mesmo measurement — **os 4 dias batem exatamente**,
sem lacuna nem duplicata:

```
2026-07-13   26.34 kWh
2026-07-14   30.88 kWh
2026-07-15   30.13 kWh
2026-07-16   39.52 kWh
```

Também cruzei a soma por inversor (`/api/history/inverters`, usada em
"Saúde da usina") contra o total do dia (`/api/history`) — bate exatamente
nos 2 dias em que os dois existem (15/07: 8,13+22,00=30,13 ✓; 16/07:
10,02+29,50=39,52 ✓), confirmando que a derivação por inversor não diverge
do total oficial.

**Achado**: os timestamps dos pontos de `daily_generation` não são
consistentes entre si. Os 3 primeiros dias (13, 14 e 15/07) foram gravados
às **12:00 UTC**; a partir de 16/07 os pontos passaram a usar **03:00 UTC**
(meia-noite BRT exata, que é a convenção documentada em `collector/main.py`
hoje). Isso sugere que os 3 primeiros pontos vieram de uma versão anterior
do coletor ou de uma carga inicial, antes da convenção de "sempre meia-noite
BRT" ser fixada. Não afeta nada visível hoje — toda leitura no painel só usa
a **data** do ponto, nunca a hora exata —, mas fica registrado porque uma
futura análise que dependa do horário exato do ponto (não só da data)
encontraria essa inconsistência nos primeiros 3 dias.

### 4. Retenção e lacunas pra daqui 12 meses

**Retenção do InfluxDB**: confirmado direto no banco (`influx bucket list`)
— o bucket `solar-home` está configurado com **retenção infinita**. Nada
que já foi gravado vai ser apagado sozinho; o que for coletado a partir de
hoje estará disponível daqui 12 meses.

**O que já está sendo guardado e vai acumular bem**:
`daily_generation` (total/dia), `inverter_status` (potência + energia do dia
por inversor, a cada 5min), `plant_status` (potência instantânea + alarme, a
cada 5min), `collector_health` (falha/sucesso de cada API por ciclo, nunca
sobrescrito — dá pra reconstruir um histórico de confiabilidade da coleta),
`consumption` (faturas) e `annotation` (suas anotações manuais).

**O que NÃO está sendo guardado, e devia se a ideia é ter mais material
daqui 12 meses**:

- **Clima/irradiância histórico**: `/api/day-status` e `/api/forecast`
  buscam radiação solar, cobertura de nuvem e código de tempo na Open-Meteo,
  mas **nenhum desses valores é gravado no InfluxDB** — ficam só em cache de
  2h e desaparecem. Daqui 12 meses, não vai dar pra responder "esse mês foi
  mais nublado que o mesmo mês do ano passado?" ou cruzar geração real com
  irradiância histórica, porque o dado de irradiância de hoje simplesmente
  não vai mais existir amanhã. É o maior buraco encontrado nesta auditoria —
  resolver é barato (1 `Point` novo por dia no coletor ou no webapp,
  reaproveitando a mesma chamada que já existe) e o custo de não resolver só
  cresce quanto mais tempo passar sem começar a gravar.
- **Performance Ratio, geração teórica, impacto ambiental** (`getKpiStationDay`
  da Huawei): já mapeado como pendência em "Menu Saúde da usina" — vale
  reforçar aqui que cada dia sem coletar isso é um dia que não dá mais pra
  recuperar depois (a Huawei não garante manter esse histórico disponível
  indefinidamente do lado dela).
- **Tensão por string** (`pv1Volt`/`pv2Volt` da FoxESS): mesma lógica —
  pendência conhecida, mas diagnóstico de sombra/sujeira por fileira só
  funciona com histórico acumulado, então quanto antes começar a coletar,
  mais cedo fica útil.

### Resumo priorizado

| Achado | Severidade | Ação sugerida |
|---|---|---|
| ~~Clima/irradiância não é persistido~~ | ~~Alto~~ | **Corrigido em 2026-07-17** — ver "Arquivamento de clima/irradiância" |
| ~~"Pico hoje" é janela rolante de 24h, não por dia-calendário~~ | ~~Médio~~ | **Corrigido em 2026-07-17** |
| `refreshAlerts` duplica 3 chamadas que já existem | Baixo (desperdício, não incorreção) | Reaproveitar os fetches dos outros timers em vez de refazer |
| "▼ 100% vs. ontem" todo início de manhã | Baixo (correto, mas pouco útil) | Considerar esconder o indicador antes do nascer do sol |
| Timestamp inconsistente nos 3 primeiros dias de `daily_generation` | Baixo (não afeta nada visível hoje) | Só documentar (feito) — não vale reescrever dado histórico por causa disso |

Nenhum desses itens foi corrigido nesta auditoria — ficou registrado pra
decidir prioridade com calma.

## Pendências / próximos passos

- [ ] Confirmar o formato real do `getAlarmList` da Huawei quando um alarme de verdade acontecer (ver "Outros campos")
- [ ] Confirmar se a temperatura do inversor Huawei sai de `0.0` durante geração de pico
- [ ] Validar o limiar de temperatura da Central de alertas (`65°C`, hoje ilustrativo) contra a doc oficial do SIW300H-3K/SIW200G-5K (ver "Central de alertas")
- [ ] Estender o parser da Celesc pra ler o crédito de compensação oficial quando a primeira fatura pós-13/07 chegar (ver "Consumo por unidade consumidora")
- [ ] Enviar a fatura da UC `298240601131` (Elizabeth Rech) todo mês também — hoje só a de Guanabara é gerada com facilidade pelo usuário
- [ ] Estender "Saúde da usina" com Performance Ratio, real vs. teórico, radiação e impacto ambiental — precisa de `getKpiStationDay`/`Month`/`Year` da Huawei, endpoints novos (ver "Menu Saúde da usina")
- [ ] Diagnóstico por string — FoxESS precisa de variáveis novas (`pv1Volt`/`pv2Volt`); Huawei já tem tudo em `getDevRealKpi`, só falta gravar/expor. Serve agora pra monitorar degradação do MPPT1 da Huawei ao longo do tempo (MPPT2 é vazio por projeto, não por defeito — ver "Limitações conhecidas" #5)
- [ ] Comparativo ano a ano — sem dado real possível ainda (usina não completou 1 ano)
