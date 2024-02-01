package db

import (
	_ "embed"
	"fmt"
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

	var err error
	db, err = sqlx.Connect("sqlite3", path)
	if err != nil {
		return "", nil, fmt.Errorf("failed open sqlite db on path %s, err: %v", path, err)
	}

	return sqliteDDL, db, nil
}
