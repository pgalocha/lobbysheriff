package migrations

import (
	"io/fs"
	"regexp"
	"testing"
)

var fileName = regexp.MustCompile(`^(\d{6})_[a-z0-9_]+\.(up|down)\.sql$`)

// Cada migration tem de ter o par up/down e seguir o formato do golang-migrate.
func TestMigrationFiles(t *testing.T) {
	files, err := fs.Glob(FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("nenhuma migration embebida")
	}

	directions := map[string]map[string]bool{}
	for _, f := range files {
		m := fileName.FindStringSubmatch(f)
		if m == nil {
			t.Errorf("nome inválido: %s (esperado NNNNNN_nome.up|down.sql)", f)
			continue
		}
		if directions[m[1]] == nil {
			directions[m[1]] = map[string]bool{}
		}
		directions[m[1]][m[2]] = true
	}

	for version, d := range directions {
		if !d["up"] || !d["down"] {
			t.Errorf("migration %s sem par up/down: %v", version, d)
		}
	}
}
