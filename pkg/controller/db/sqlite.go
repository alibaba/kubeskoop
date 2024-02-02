package db

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
)

const defaultSQLitePath = "/var/lib/kubeskoop/controller_db.sqlite3"

//go:embed sqlite.ddl
var sqliteDDL string

func newSQLite3(config *Config) (string, *sqlx.DB, error) {
	path := config.Addr
	if path == "" {
		path = defaultSQLitePath
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", nil, fmt.Errorf("failed open sqlite db on path %s: %w", path, err)
		}
		f, err := os.Create(path)
		if err != nil {
			return "", nil, fmt.Errorf("failed open sqlite db on path %s: %w", path, err)
		}
		f.Close()
	}

	var err error
	db, err = sqlx.Connect("sqlite3", path)
	if err != nil {
		return "", nil, fmt.Errorf("failed open sqlite db on path %s: %w", path, err)
	}

	return sqliteDDL, db, nil
}
