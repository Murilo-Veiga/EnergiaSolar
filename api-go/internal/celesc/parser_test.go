package celesc

import "testing"

// billText é o texto plano extraído de uma fatura Celesc real (DANF3E) via
// github.com/ledongthuc/pdf (cmd/debug-celesc-pdf) — NÃO é a ordem de
// leitura visual: o parser da lib não preserva o layout em colunas, então
// cada célula de uma "linha" da fatura normalmente vira 1 linha de texto
// separada, e alguns pares rótulo/valor saem invertidos e colados (ex.:
// "63716073Cliente:", "MARCOS EDUARDO ENDLERNOME:"). Fixture travada contra
// esse formato real em vez de um texto "arrumado" à mão.
const billText = `RESIDENCIAL - RESIDENCIAL - B1 Residencial - MONOFÁSICO
55901236
63716073Cliente:
06/2026
27/07/2026
120,11 R$
MARCOS EDUARDO ENDLERNOME:
***.930.870-**CPF/CNPJ:
RUDINEI VIEIRA 526 CS 01 - PARQUE GUARANI - JVEENDERECO:
89230-158CEP:
JOINVILLE SCCIDADE:
BGrupo/Subgrupo Tensão:
B1/
07/05/2026
05/06/2026
29
07/07/2026
096039897NOTA FISCAL Nº
001SERIE:
21/06/2026DATA EMISSAO:
PIS
59,39
1,10
0,66
COFINS
59,39
5,06
3,00
ICMS
67,89
12,00
8,15
ICMS
117,63
17,00
20,00
(0D) Consumo TE
KWH
150,000
0,389867
58,48
3,17
58,48
12,00
7,02
0,321930
(0D) Consumo TE
KWH
174,000
0,413391
71,93
3,68
71,93
17,00
12,23
0,321930
(0E) Consumo TUSD
KWH
150,000
0,452600
67,89
3,68
67,89
12,00
8,15
0,373750
(0E) Consumo TUSD
KWH
174,000
0,479828
83,49
4,27
83,49
17,00
14,19
0,373750
(0R) Energia Injet. TE
KWH
150,000
-0,389867
-58,48
-3,17
-58,48
12,00
-7,02
0,321930
(0R) Energia Injet. TE
KWH
96,000
-0,413438
-39,69
-2,03
-39,69
17,00
-6,75
0,321930
(0S) Energia Inj. TUSD
KWH
246,000
-0,398293
-97,98
-6,04
0,00
0,00
0,00
0,373750
(2L) Bandeira Amarela
KWH
150,000
0,022733
3,41
0,18
3,41
12,00
0,41
0,018827
(2L) Bandeira Amarela
KWH
174,000
0,024253
4,22
0,22
4,22
17,00
0,72
0,018827
(2M) Band. Am. Injet.
KWH
150,000
-0,022733
-3,41
-0,18
-3,41
12,00
-0,41
-0,018821
(2M) Band. Am. Injet.
KWH
96,000
-0,024167
-2,32
-0,12
-2,32
17,00
-0,39
-0,018821
SUBTOTAL
87,54
(8H) Correção Monetária
0,000
0,000000
3,22
0,00
0,00
0,00
0,00
0,000000
(AH) Juros  03/2026
0,000
0,000000
0,21
0,00
0,00
0,00
0,00
0,000000
(AH) Juros  03/2026
0,000
0,000000
4,76
0,00
0,00
0,00
0,00
0,000000
(AM) Multa  03/2026
0,000
0,000000
7,37
0,00
0,00
0,00
0,00
0,000000
(C0) COSIP Municipal
0,000
0,000000
17,01
0,00
0,00
0,00
0,00
0,000000
SUBTOTAL
32,57
Iluminação pública: Joinville - 156
Comunicado importante
O número da sua unidade consumidora foi atualizado para um novo formato, conforme a REN 1095/2024 da ANEEL. Sua UC antiga era 55901236 e seu novo número e passará a ser 3.687.125.011-00. Atenção! Contas em atraso nas referência(s): 04/2026 R$370,30 - - = Totalizando R$370,30.
Consulte Chave de Acesso em:https://sat.sef.sc.gov.br/nf3e/consulta
Chave de Acesso:4226.0608.3367.8300.0190.6600.1096.0398.9710.5358.0001
3.422.600.023.436.416 - 22/06/2026 às 08:00Protocolo de Autorização:
04Etapa:
Amarela R$ 0,01885
29
MAI/26
ABR/26
MAR/26
FEV/26
JAN/26
DEZ/25
NOV/25
OUT/25
SET/25
AGO/25
JUL/25
JUN/25
MAI/25
Lida
5629677
Energia
Único
11
335
1,00000
0,00
324
5629677
Energia injetada
Único
0
246
1,00000
0,00
246
CON
GTP
359
0
422
0
439
0
391
0
539
0
360
0
396
0
465
0
426
0
352
0
426
0
357
0
0
0
LEGENDA:
Beneficiário: Celesc Distribuição SA - CNPJ 08336783/0001-90 Av. Itamarati, n 160 - Itacorubi - Florianópolis - SC CP: 88.034-900
PAGÁVEL EM QUALQUER AGÊNCIA BANCÁRIAMARCOS EDUARDO ENDLERPagador:
***.930.870-**CPF/CNPJ:
RUDINEI VIEIRA 526 CS 01 - PARQUE GUARANI - JVEEndereço:
89230-158CEP:
JOINVILLE SCCidade:
21/06/2026
Data DocumentoNúmero Referência202606-096039897
Agência / Código Cedente: 0348-4/0136136-8Unidade Consumidora0055901236
Nosso Número
Referência06/2026
Vencimento27/07/2026
Código para Cadastro em Débito Automático:Total a Pagar (R$)55901236
120,11
836400000011201101629008159050557705000559012364
BRADESCOSEGUNDA VIA
836400000011201101629008159050557705000559012364
  (0D) Consumo TE | (0E) Consumo TUSD | (0R) Energia Injetada TE | (0S) Energia Injetada TUSD | (2L) Bandeira Amarela | (2M) BandeiraAmarela da Energia Injetada | (8H) Correção Monetária | (AH) Juros | (AM) Multa | (C0) COSIP Municipal Joinville

TOTAL
120,11
SEGUNDA VIA  (0D) Consumo TE | (0E) Consumo TUSD | (0R) Energia Injetada TE | (0S) Energia Injetada TUSD | (2L) Bandeira Amarela | (2M) Bandeira Amarela da Energia Injetada | (8H) Correção Monetária | (AH) Juros | (AM) Multa | (C0) COSIP Municipal Joinville
LEGENDA:

JUN/26
MAI/26
Único
Único
Consumo Geradora no Período Atual
294
329
Injeção no Período Atual
246
0
Saldo Geradora Mês Anterior
0
0
Cobrança da Geradora
78
359
Injeção Restante Final
0
0
Injeção Distribuída Geradora
0
0
Injeção Distribuída Beneficiárias
0
0
Saldo Geradora Mês Anterior Restante
0
0
Saldo Final Geradora
0
0
Maiores informações, acesse seu demonstrativo na Agência WEB (https://agenciaweb.celesc.com.br/AgenciaWeb/
SEGUNDA VIA
`

func TestParseBillText(t *testing.T) {
	bill, err := ParseBillText(billText)
	if err != nil {
		t.Fatalf("ParseBillText falhou: %v", err)
	}

	if bill.UC != "368712501100" {
		t.Errorf("UC = %q, esperava %q (novo formato ANEEL, só dígitos)", bill.UC, "368712501100")
	}
	if bill.Titular != "MARCOS EDUARDO ENDLER" {
		t.Errorf("Titular = %q", bill.Titular)
	}
	if bill.ReferenciaMes != 6 || bill.ReferenciaAno != 2026 {
		t.Errorf("Referência = %02d/%d, esperava 06/2026", bill.ReferenciaMes, bill.ReferenciaAno)
	}
	if bill.Vencimento != "27/07/2026" {
		t.Errorf("Vencimento = %q", bill.Vencimento)
	}
	if bill.TotalPagarBRL != 120.11 {
		t.Errorf("TotalPagarBRL = %v, esperava 120.11", bill.TotalPagarBRL)
	}
	if bill.ConsumoKWh != 324 {
		t.Errorf("ConsumoKWh = %v, esperava 324", bill.ConsumoKWh)
	}
	if bill.DiasFaturados == nil || *bill.DiasFaturados != 29 {
		t.Errorf("DiasFaturados = %v, esperava 29", bill.DiasFaturados)
	}
	if bill.Bandeira == nil || *bill.Bandeira != "Amarela" {
		t.Errorf("Bandeira = %v, esperava Amarela", bill.Bandeira)
	}
	if bill.BandeiraValorKWh == nil || *bill.BandeiraValorKWh != 0.01885 {
		t.Errorf("BandeiraValorKWh = %v, esperava 0.01885", bill.BandeiraValorKWh)
	}

	if len(bill.Historico) != 13 {
		t.Fatalf("len(Historico) = %d, esperava 13", len(bill.Historico))
	}
	first := bill.Historico[0]
	if first.Mes != 5 || first.Ano != 2026 || first.ConsumoKWh != 359 {
		t.Errorf("Historico[0] = %+v, esperava {Mes:5 Ano:2026 ConsumoKWh:359}", first)
	}
	last := bill.Historico[12]
	if last.Mes != 5 || last.Ano != 2025 || last.ConsumoKWh != 0 {
		t.Errorf("Historico[12] = %+v, esperava {Mes:5 Ano:2025 ConsumoKWh:0}", last)
	}
}

func TestParseBillTextMissingUC(t *testing.T) {
	_, err := ParseBillText("linha qualquer\noutra linha\n")
	if err == nil {
		t.Fatal("esperava erro por falta de UC")
	}
}
