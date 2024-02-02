package db

import (
	_ "embed"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
)

//go:embed mysql.ddl
var mysqlDDL string

func newMySQL(config *Config) (string, *sqlx.DB, error) {
	cfg := mysql.NewConfig()
	cfg.Addr = config.Addr
	cfg.User = config.Username
	cfg.Passwd = config.Password
	cfg.DBName = config.DBName
	cfg.Net = "tcp"
	dsn := cfg.FormatDSN()
	log.Infof("addr %s, dsn: %s", cfg.Addr, dsn)
	db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		return "", nil, fmt.Errorf("failed config mysql: %w", err)
	}

	return mysqlDDL, db, nil
}
