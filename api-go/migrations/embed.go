// Package migrations embute os arquivos .sql no binário via go:embed, pra
// o servidor aplicar o schema sozinho na subida sem precisar de um
// container/CLI de migrate separado.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
