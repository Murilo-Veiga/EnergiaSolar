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

var (
	referenciaLineRe = regexp.MustCompile(`^(\d{2})/(\d{4})\s+(\d{2}/\d{2}/\d{4})\s+R\$\s+([\d.,]+)$`)
	nomeLineRe       = regexp.MustCompile(`^NOME:\s*(.+)$`)
	diasLineRe       = regexp.MustCompile(`^\d{2}/\d{2}/\d{4}\s+\d{2}/\d{2}/\d{4}\s+(\d+)\s+\d{2}/\d{2}/\d{4}$`)
	medidorLineRe    = regexp.MustCompile(`Energia\s+Único\s+[\d.]+\s+[\d.]+\s+[\d,]+\s+[\d,]+\s+(\d+)`)
	bandeiraLineRe   = regexp.MustCompile(`^(Verde|Amarela|Vermelha ?\d?)\s+R\$\s+([\d,]+)\s+\d+$`)
	monthLabelRe     = regexp.MustCompile(`^([A-Z]{3})/(\d{2})$`)
	conGtpPairRe     = regexp.MustCompile(`^(\d+)\s+(\d+)$`)
	novoFormatoUCRe  = regexp.MustCompile(`passará a ser\s+([\d.\-]+)`)
	nonDigitRe       = regexp.MustCompile(`\D`)

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
	Ano       int
	Mes       int
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
// anterior a "Cliente: ...". Faturas em transição pro novo formato da
// ANEEL (REN 1095/2024) ainda mostram a UC antiga aqui — ver
// findUCNovoFormato, que tem prioridade quando presente.
func findUC(lines []string) string {
	for i, line := range lines {
		if strings.HasPrefix(line, "Cliente:") && i > 0 {
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

	// Depois dos labels vem a tabela de leitura do medidor (2 linhas) e só
	// então o cabeçalho "CON GTP" com 1 par de números por mês, na mesma
	// ordem dos labels.
	headerIdx := -1
	for j := i; j < len(lines); j++ {
		if strings.HasPrefix(lines[j], "CON") {
			headerIdx = j
			break
		}
	}
	if headerIdx == -1 {
		return nil
	}

	var historico []HistoricoMes
	for k, mo := range months {
		lineIdx := headerIdx + 1 + k
		if lineIdx >= len(lines) {
			return nil
		}
		pair := conGtpPairRe.FindStringSubmatch(lines[lineIdx])
		if pair == nil {
			return nil
		}
		kwh, err := strconv.Atoi(pair[1])
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

	for _, line := range lines {
		if bill.Titular == "" {
			if m := nomeLineRe.FindStringSubmatch(line); m != nil {
				bill.Titular = strings.TrimSpace(m[1])
				continue
			}
		}
		if !haveReferencia {
			if m := referenciaLineRe.FindStringSubmatch(line); m != nil {
				mes, _ := strconv.Atoi(m[1])
				ano, _ := strconv.Atoi(m[2])
				total, err := brlToFloat(m[4])
				if err != nil {
					return ParsedBill{}, &ParseError{"total a pagar em formato inválido"}
				}
				bill.ReferenciaMes = mes
				bill.ReferenciaAno = ano
				bill.Vencimento = m[3]
				bill.TotalPagarBRL = total
				haveReferencia = true
				continue
			}
		}
		if bill.DiasFaturados == nil {
			if m := diasLineRe.FindStringSubmatch(line); m != nil {
				dias, err := strconv.Atoi(m[1])
				if err == nil {
					bill.DiasFaturados = &dias
				}
				continue
			}
		}
		if !haveConsumo {
			if m := medidorLineRe.FindStringSubmatch(line); m != nil {
				kwh, err := strconv.ParseFloat(m[1], 64)
				if err == nil {
					bill.ConsumoKWh = kwh
					haveConsumo = true
				}
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

// ParseBill extrai o texto de um PDF (via github.com/ledongthuc/pdf, puro
// Go/sem cgo) e delega a extração de campos pra ParseBillText.
func ParseBill(pdfBytes []byte) (ParsedBill, error) {
	reader, err := pdf.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))
	if err != nil {
		return ParsedBill{}, fmt.Errorf("falha ao abrir PDF: %w", err)
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
		return ParsedBill{}, &ParseError{"não foi possível extrair texto do PDF"}
	}
	return ParseBillText(sb.String())
}
