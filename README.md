# Solar Home — Painel de Monitoramento

Painel próprio de monitoramento para a usina solar residencial, rodando
100% local via Docker. A usina tem 2 inversores, cada um monitorado
diretamente pela **API oficial do seu fabricante** (Huawei FusionSolar e
FoxESS Cloud) — sem depender de nenhum acesso de usuário de terceiros.

**Sumário:** [Arquitetura](#arquitetura) · [Como rodar](#como-rodar) ·
[Detalhes da instalação](#detalhes-da-instalação) ·
[Fontes de dados oficiais](#fontes-de-dados-oficiais) ·
[Limitações conhecidas](#limitações-conhecidas) ·
[Design do painel](#design-do-painel) ·
[Consumo por unidade consumidora](#consumo-por-unidade-consumidora-celesc) ·
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
  pode ser menor que 300. Login não tem essa restrição.
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

## Design do painel

O layout foi redesenhado (mockup iterado e aprovado em Artifact antes de ir
pro código real): paleta validada pelo skill de dataviz (contraste + separação
CVD), números nunca "vestem" a cor da série (identidade vem de um indicador
ao lado, texto sempre em tinta neutra), tabular-nums pra alinhar dígitos, e
tooltips em vez de legendas fixas pra explicar estados. O CSS é um sistema de
tokens próprio (`--surface`, `--ink`, `--accent-*` etc. no `<style>` do
`index.html`), sem framework de UI; os gráficos são Chart.js estilizado pra
bater com os tokens.

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

### Outros campos

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
- **Pico de potência do dia + horário**, **bandeira tarifária vigente** (da
  fatura mais recente, qualquer uma das 2 UCs) e **nascer/pôr do sol**
  (Open-Meteo) — todos exibidos no card "Status do dia".

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

### Linha de média nos gráficos

Os gráficos "Geração diária" (Dashboard e Histórico) mostram uma linha
tracejada vermelha com a média do período selecionado (diária pra
semana/mês, mensal pro ano), num segundo dataset do Chart.js sobreposto às
barras. O valor fica sempre visível abaixo do total do período, e passar o
mouse em qualquer barra mostra a média junto no tooltip (mesmo índice do
eixo X, `interaction.mode: "index"`).

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
    └── templates/index.html     # dashboard (CSS próprio + Chart.js, dark theme)
```

### Endpoints do webapp

| Rota | Retorna |
|---|---|
| `GET /` | Página do dashboard |
| `GET /api/summary` | KPIs atuais: potência, geração/economia do dia, pico do dia + horário, status |
| `GET /api/inverters` | Potência/geração/temperatura/status (gerando, online sem geração, sem comunicação) por inversor |
| `GET /api/day-status` | Status do dia: clima, sol, alarme (+ detalhe), geração, bandeira vigente |
| `GET /api/history?range=semana\|mes\|ano` | Série de geração no período + `total_kwh`/`total_brl` (estimado) |
| `GET /api/forecast` | Previsão do tempo 5 dias (Open-Meteo, sem API key) |
| `POST /api/consumption/upload` | Recebe fatura PDF da Celesc (multipart), extrai e grava no InfluxDB |
| `GET /api/consumption/summary` | Resumo por UC (última fatura) + economia estimada |
| `GET /api/consumption/history?uc=X` | Série histórica de consumo (kWh/R$) de uma UC |

## Pendências / próximos passos

- [ ] Confirmar o formato real do `getAlarmList` da Huawei quando um alarme de verdade acontecer (ver "Design do painel")
- [ ] Confirmar se a temperatura do inversor Huawei sai de `0.0` durante geração de pico
- [ ] Validar o limiar de temperatura da Central de alertas (`65°C`, hoje ilustrativo) contra a doc oficial do SIW300H-3K/SIW200G-5K (ver "Central de alertas")
- [ ] Estender o parser da Celesc pra ler o crédito de compensação oficial quando a primeira fatura pós-13/07 chegar (ver "Consumo por unidade consumidora")
- [ ] Enviar a fatura da UC `298240601131` (Elizabeth Rech) todo mês também — hoje só a de Guanabara é gerada com facilidade pelo usuário
