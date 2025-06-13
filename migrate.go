package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// migrate runs all SQL migration files in migrations/ directory.
func migrate(db *sql.DB) error {
	entries, err := os.ReadDir("migrations")
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	for _, fname := range files {
		path := filepath.Join("migrations", fname)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(content)); err != nil {
			return err
		}
	}
	return nil
}
