// Package migrations embebe os ficheiros SQL no binário, para a imagem não
// precisar de os copiar à parte.
//
// Criar uma nova migration (dentro do Dev Container):
//
//	migrate create -ext sql -dir migrations -seq <nome>
package migrations

import "embed"

// FS contém os ficheiros NNNNNN_<nome>.{up,down}.sql.
//
//go:embed *.sql
var FS embed.FS
