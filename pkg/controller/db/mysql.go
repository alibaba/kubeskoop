package db

import (
	_ "embed"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

//go:embed mysql.ddl
var mysqlDDL string

func newMySQL(config *Config) (string, *sqlx.DB, error) {
	cfg := mysql.NewConfig()
	cfg.Addr = config.Addr
	cfg.User = config.Username
	cfg.Passwd = config.Password
	cfg.DBName = config.DBName
	dsn := cfg.FormatDSN()
	db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		return "", nil, fmt.Errorf("failed config mysql: %w", err)
	}

	return "", db, nil
}
