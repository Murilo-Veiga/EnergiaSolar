// Comando cmd/debug-celesc-pdf: abre um PDF de fatura Celesc e imprime o
// texto puro extraído (github.com/ledongthuc/pdf), sem tentar reconhecer
// nenhum campo — usado só pra diagnosticar quando a ordem de leitura de um
// PDF real (layout em colunas) não bate com o que as regexes de
// internal/celesc/parser.go esperam.
//
// Uso:
//
//	go run ./cmd/debug-celesc-pdf caminho/para/fatura.pdf
package main

import (
	"fmt"
	"os"

	"energiasolar-api/internal/celesc"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "uso: debug-celesc-pdf <arquivo.pdf>")
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro lendo arquivo:", err)
		os.Exit(1)
	}
	text, err := celesc.ExtractText(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro extraindo texto:", err)
		os.Exit(1)
	}
	fmt.Print(text)
}
