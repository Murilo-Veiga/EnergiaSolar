// Package celesc extrai os campos relevantes de uma fatura da Celesc
// (PDF/DANF3E) — porta pra Go do antigo webapp/celesc_bill_parser.py
// (Python), removido em 748cb61 quando o painel migrou pra Go/React (ver
// README > "Limitações conhecidas", item 4).
package celesc

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ledongthuc/pdf"
)

// A extração de texto do PDF (github.com/ledongthuc/pdf) NÃO preserva a
// ordem de leitura visual de um layout em colunas — cada valor de uma
// "linha" da fatura normalmente vira 1 linha de texto por célula, e alguns
// pares rótulo/valor saem invertidos e colados (ex.: "63716073Cliente:",
// "MARCOS EDUARDO ENDLERNOME:"). Por isso quase todo campo aqui é
// reconhecido por uma SEQUÊNCIA de linhas consecutivas (lookahead por
// índice) em vez de 1 regex por linha só — confirmado contra uma fatura
// real (ver cmd/debug-celesc-pdf).
var (
	nomeLineRe          = regexp.MustCompile(`^(.+)NOME:$`)
	referenciaMesAnoRe  = regexp.MustCompile(`^(\d{2})/(\d{4})$`)
	dateOnlyRe          = regexp.MustCompile(`^\d{2}/\d{2}/\d{4}$`)
	totalValueRe        = regexp.MustCompile(`^([\d.,]+)\s*R\$$`)
	integerLineRe       = regexp.MustCompile(`^(\d+)$`)
	bandeiraLineRe      = regexp.MustCompile(`^(Verde|Amarela|Vermelha ?\d?)\s+R\$\s+([\d,]+)$`)
	monthLabelRe        = regexp.MustCompile(`^([A-Z]{3})/(\d{2})$`)
	novoFormatoUCRe     = regexp.MustCompile(`passará a ser\s+([\d.\-]+)`)
	nonDigitRe          = regexp.MustCompile(`\D`)

	monthAbbr = map[string]int{
		"JAN": 1, "FEV": 2, "MAR": 3, "ABR": 4, "MAI": 5, "JUN": 6,
		"JUL": 7, "AGO": 8, "SET": 9, "OUT": 10, "NOV": 11, "DEZ": 12,
	}
)

// ParseError sinaliza que campos obrigatórios não foram encontrados no PDF
// — o handler HTTP mapeia isso pra 422 (ver internal/httpapi).
type ParseError struct{ msg string }

func (e *ParseError) Error() string { return e.msg }

// HistoricoMes é 1 mês do quadro "Histórico de consumo" já impresso na
// própria fatura (coluna CON) — permite importar até 13 meses de uma vez
// só, sem precisar de 1 upload por mês.
type HistoricoMes struct {
	Ano        int
	Mes        int
	ConsumoKWh int
}

// ParsedBill é o resultado de 1 fatura parseada.
type ParsedBill struct {
	UC               string
	Titular          string
	ReferenciaAno    int
	ReferenciaMes    int
	Vencimento       string
	TotalPagarBRL    float64
	ConsumoKWh       float64
	DiasFaturados    *int
	Bandeira         *string
	BandeiraValorKWh *float64
	Historico        []HistoricoMes
}

func brlToFloat(s string) (float64, error) {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	return strconv.ParseFloat(s, 64)
}

// findUC procura a UC exibida no topo da fatura — a linha imediatamente
// anterior à que contém "Cliente:" (o rótulo sai colado DEPOIS do valor na
// extração real, ex. "63716073Cliente:", por isso Contains em vez de
// HasPrefix). Faturas em transição pro novo formato da ANEEL (REN
// 1095/2024) ainda mostram a UC antiga aqui — ver findUCNovoFormato, que
// tem prioridade quando presente.
func findUC(lines []string) string {
	for i, line := range lines {
		if strings.Contains(line, "Cliente:") && i > 0 {
			digits := nonDigitRe.ReplaceAllString(strings.TrimSpace(lines[i-1]), "")
			if digits != "" {
				return digits
			}
		}
	}
	return ""
}

// findUCNovoFormato usa o número novo anunciado no comunicado da ANEEL
// ("sua UC antiga era X e seu novo número e passará a ser Y") como
// identificador canônico, pra não quebrar a série quando a fatura seguinte
// já vier só com o número novo.
func findUCNovoFormato(lines []string) string {
	for _, line := range lines {
		m := novoFormatoUCRe.FindStringSubmatch(line)
		if m != nil {
			digits := nonDigitRe.ReplaceAllString(m[1], "")
			if digits != "" {
				return digits
			}
		}
	}
	return ""
}

// parseHistorico extrai o quadro "Histórico de consumo" (colunas CON/GTP —
// consumo e geração injetada, mês a mês) impresso na própria fatura.
// Best-effort: se o layout não bater com o esperado, devolve lista vazia
// em vez de derrubar o parse dos campos principais.
func parseHistorico(lines []string) []HistoricoMes {
	var months []struct{ mes, ano int }
	i := 0
	// Acha o primeiro bloco contíguo de labels tipo "MAI/26".
	start := -1
	for j, line := range lines {
		if monthLabelRe.MatchString(line) {
			start = j
			break
		}
	}
	if start == -1 {
		return nil
	}
	i = start
	for i < len(lines) {
		m := monthLabelRe.FindStringSubmatch(lines[i])
		if m == nil {
			break
		}
		mes, ok := monthAbbr[m[1]]
		if !ok {
			break
		}
		ano, err := strconv.Atoi(m[2])
		if err != nil {
			break
		}
		months = append(months, struct{ mes, ano int }{mes, 2000 + ano})
		i++
	}
	if len(months) == 0 {
		return nil
	}

	// Depois dos labels vem a tabela de leitura do medidor e só então o
	// cabeçalho "CON"/"GTP" (2 linhas separadas, não "CON GTP" numa linha
	// só) com 2 números por mês (consumo, geração injetada), cada 1 na sua
	// própria linha, na mesma ordem dos labels.
	headerIdx := -1
	for j := i; j < len(lines)-1; j++ {
		if lines[j] == "CON" && lines[j+1] == "GTP" {
			headerIdx = j
			break
		}
	}
	if headerIdx == -1 {
		return nil
	}

	var historico []HistoricoMes
	for k, mo := range months {
		conIdx := headerIdx + 2 + k*2
		if conIdx >= len(lines) {
			return nil
		}
		kwh, err := strconv.Atoi(lines[conIdx])
		if err != nil {
			return nil
		}
		historico = append(historico, HistoricoMes{Ano: mo.ano, Mes: mo.mes, ConsumoKWh: kwh})
	}
	return historico
}

// ParseBillText extrai os campos de uma fatura já convertida em texto
// plano (1 linha por elemento). Exportada separada de ParseBill pra
// permitir testar a lógica de extração sem depender do parser de PDF.
func ParseBillText(text string) (ParsedBill, error) {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, l)
		}
	}

	uc := findUCNovoFormato(lines)
	if uc == "" {
		uc = findUC(lines)
	}
	if uc == "" {
		return ParsedBill{}, &ParseError{"não encontrei a Unidade Consumidora no PDF"}
	}

	var bill ParsedBill
	bill.UC = uc

	haveReferencia := false
	haveConsumo := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if bill.Titular == "" {
			if m := nomeLineRe.FindStringSubmatch(line); m != nil {
				bill.Titular = strings.TrimSpace(m[1])
				continue
			}
		}

		// Referência/vencimento/total: 3 linhas seguidas — "MM/AAAA",
		// "DD/MM/AAAA", "<valor> R$" (valor antes do símbolo, invertido).
		if !haveReferencia {
			if m := referenciaMesAnoRe.FindStringSubmatch(line); m != nil && i+2 < len(lines) &&
				dateOnlyRe.MatchString(lines[i+1]) {
				if tm := totalValueRe.FindStringSubmatch(lines[i+2]); tm != nil {
					total, err := brlToFloat(tm[1])
					if err != nil {
						return ParsedBill{}, &ParseError{"total a pagar em formato inválido"}
					}
					mes, _ := strconv.Atoi(m[1])
					ano, _ := strconv.Atoi(m[2])
					bill.ReferenciaMes = mes
					bill.ReferenciaAno = ano
					bill.Vencimento = lines[i+1]
					bill.TotalPagarBRL = total
					haveReferencia = true
					i += 2
					continue
				}
			}
		}

		// Dias faturados: 4 linhas seguidas — data, data, dias (inteiro
		// puro), data (leitura anterior, leitura atual, dias, próxima
		// leitura).
		if bill.DiasFaturados == nil {
			if dateOnlyRe.MatchString(line) && i+3 < len(lines) &&
				dateOnlyRe.MatchString(lines[i+1]) &&
				integerLineRe.MatchString(lines[i+2]) &&
				dateOnlyRe.MatchString(lines[i+3]) {
				dias, err := strconv.Atoi(lines[i+2])
				if err == nil {
					bill.DiasFaturados = &dias
				}
				i += 3
				continue
			}
		}

		// Consumo: linha exata "Energia" (não "Energia injetada") seguida
		// de "Único", depois 5 números — leitura anterior, leitura atual,
		// constante, perdas(%), total apurado (o que queremos, o último).
		if !haveConsumo {
			if line == "Energia" && i+6 < len(lines) && lines[i+1] == "Único" {
				kwh, err := strconv.ParseFloat(lines[i+6], 64)
				if err == nil {
					bill.ConsumoKWh = kwh
					haveConsumo = true
				}
				i += 6
				continue
			}
		}

		if bill.Bandeira == nil {
			if m := bandeiraLineRe.FindStringSubmatch(line); m != nil {
				bandeira := m[1]
				valor, err := brlToFloat(m[2])
				if err == nil {
					bill.Bandeira = &bandeira
					bill.BandeiraValorKWh = &valor
				}
				continue
			}
		}
	}

	var faltando []string
	if !haveReferencia {
		faltando = append(faltando, "referência/vencimento/total")
	}
	if !haveConsumo {
		faltando = append(faltando, "consumo (kWh)")
	}
	if len(faltando) > 0 {
		return ParsedBill{}, &ParseError{fmt.Sprintf("campos obrigatórios não encontrados no PDF: %s", strings.Join(faltando, ", "))}
	}

	bill.Historico = parseHistorico(lines)
	return bill, nil
}

// trimAfterLastEOF corta qualquer byte depois do marcador "%%EOF" final do
// PDF. Alguns geradores (inclusive o da Celesc) deixam espaço em
// branco/quebra de linha ou até uma revisão incremental extra depois do
// %%EOF real — o parser da ledongthuc/pdf só procura esse marcador numa
// janela fixa perto do fim do arquivo e falha com "missing %%EOF" quando
// esse lixo extra empurra o marcador real pra fora dessa janela. Cortar até
// o último %%EOF é inofensivo pra PDFs que já terminam nele.
func trimAfterLastEOF(b []byte) []byte {
	marker := []byte("%%EOF")
	idx := bytes.LastIndex(b, marker)
	if idx == -1 {
		return b
	}
	end := idx + len(marker)
	if end >= len(b) {
		return b
	}
	return b[:end]
}

// ExtractText abre o PDF (via github.com/ledongthuc/pdf, puro Go/sem cgo) e
// devolve o texto puro extraído, sem tentar reconhecer nenhum campo — usado
// por ParseBill e também como debug isolado (ver cmd/debug-celesc-pdf)
// quando o texto extraído não bate com o que as regexes de ParseBillText
// esperam (a ordem de leitura de um PDF real com layout em colunas pode
// diferir do texto "arrumado" usado nos testes).
func ExtractText(pdfBytes []byte) (string, error) {
	pdfBytes = trimAfterLastEOF(pdfBytes)
	reader, err := pdf.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))
	if err != nil {
		return "", fmt.Errorf("falha ao abrir PDF: %w", err)
	}

	var sb strings.Builder
	numPages := reader.NumPage()
	for i := 1; i <= numPages; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		content, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	if sb.Len() == 0 {
		return "", &ParseError{"não foi possível extrair texto do PDF"}
	}
	return sb.String(), nil
}

// ParseBill extrai o texto de um PDF e delega a extração de campos pra
// ParseBillText.
func ParseBill(pdfBytes []byte) (ParsedBill, error) {
	text, err := ExtractText(pdfBytes)
	if err != nil {
		return ParsedBill{}, err
	}
	return ParseBillText(text)
}
