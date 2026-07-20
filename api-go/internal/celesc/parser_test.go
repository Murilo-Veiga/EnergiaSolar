package celesc

import "testing"

// billText é o texto plano extraído (ordem de leitura) de uma fatura Celesc
// real (DANF3E), usada como fixture pra travar o comportamento das regexes
// sem depender de abrir um PDF de verdade.
const billText = `RESIDENCIAL - RESIDENCIAL - B1 Residencial - MONOFÁSICO
55901236
Cliente: 63716073
06/2026 27/07/2026 R$ 120,11
NOME: MARCOS EDUARDO ENDLER
CPF/CNPJ: ***.930.870-**
RUDINEI VIEIRA 526 CS 01 - PARQUE
GUARANI - JVE
ENDERECO:
CEP: 89230-158 CIDADE: JOINVILLE SC Grupo/Subgrupo Tensão:B/B1
07/05/2026 05/06/2026 29 07/07/2026
NOTA FISCAL Nº 096039897 SERIE:001 DATA EMISSAO: 21/06/2026
PIS 59,39 1,10 0,66
COFINS 59,39 5,06 3,00
ICMS 67,89 12,00 8,15
ICMS 117,63 17,00 20,00
(0D) Consumo TE KWH 150,000 0,389867 58,48 3,17 58,48 12,00 7,02 0,321930
(0D) Consumo TE KWH 174,000 0,413391 71,93 3,68 71,93 17,00 12,23 0,321930
(0E) Consumo TUSD KWH 150,000 0,452600 67,89 3,68 67,89 12,00 8,15 0,373750
(0E) Consumo TUSD KWH 174,000 0,479828 83,49 4,27 83,49 17,00 14,19 0,373750
(0R) Energia Injet. TE KWH 150,000 -0,389867 -58,48 -3,17 -58,48 12,00 -7,02 0,321930
(0R) Energia Injet. TE KWH 96,000 -0,413438 -39,69 -2,03 -39,69 17,00 -6,75 0,321930
(0S) Energia Inj. TUSD KWH 246,000 -0,398293 -97,98 -6,04 0,00 0,00 0,00 0,373750
(2L) Bandeira Amarela KWH 150,000 0,022733 3,41 0,18 3,41 12,00 0,41 0,018827
(2L) Bandeira Amarela KWH 174,000 0,024253 4,22 0,22 4,22 17,00 0,72 0,018827
(2M) Band. Am. Injet. KWH 150,000 -0,022733 -3,41 -0,18 -3,41 12,00 -0,41 -0,018821
(2M) Band. Am. Injet. KWH 96,000 -0,024167 -2,32 -0,12 -2,32 17,00 -0,39 -0,018821
SUBTOTAL 87,54
(8H) Correção Monetária 0,000 0,000000 3,22 0,00 0,00 0,00 0,00 0,000000
(AH) Juros 03/2026 0,000 0,000000 0,21 0,00 0,00 0,00 0,00 0,000000
(AH) Juros 03/2026 0,000 0,000000 4,76 0,00 0,00 0,00 0,00 0,000000
(AM) Multa 03/2026 0,000 0,000000 7,37 0,00 0,00 0,00 0,00 0,000000
(C0) COSIP Municipal 0,000 0,000000 17,01 0,00 0,00 0,00 0,00 0,000000
SUBTOTAL 32,57
Iluminação pública: Joinville - 156
Comunicado importante
O número da sua unidade consumidora foi atualizado para um novo formato, conforme a REN 1095/2024 da ANEEL. Sua UC antiga era 55901236 e seu novo
número e passará a ser 3.687.125.011-00.
Atenção! Contas em atraso nas referência(s): 04/2026 R$370,30 - - = Totalizando R$370,30.
Consulte Chave de Acesso em:
https://sat.sef.sc.gov.br/nf3e/consulta
Chave de Acesso:
4226.0608.3367.8300.0190.6600.1096.0398.9710.5358.0001
Protocolo de Autorização: 3.422.600.023.436.416 - 22/06/2026 às 08:00
Etapa: 04
Amarela R$ 0,01885 29
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
5629677 Energia Único 11 335 1,00000 0,00 324
5629677 Energia injetada Único 0 246 1,00000 0,00 246
CON GTP
359 0
422 0
439 0
391 0
539 0
360 0
396 0
465 0
426 0
352 0
426 0
357 0
0 0
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
