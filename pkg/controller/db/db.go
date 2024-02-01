package db

import (
	"database/sql"

	// import for go:embed
	_ "embed"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	// import for sqlite
	_ "github.com/mattn/go-sqlite3"
)

var (
	db *sqlx.DB
)

type Driver func(config *Config) (ddl string, db *sqlx.DB, err error)

var drivers = map[string]Driver{
	"sqlite": newSQLite3,
	"mysql":  newMySQL,
}

func InitializeDB(config *Config) error {
	var (
		ddl string
		err error
	)

	dbType := config.Type
	if dbType == "" {
		dbType = "sqlite3"
	}

	driver := drivers[dbType]
	if driver == nil {
		return fmt.Errorf("unsupported database type %s", config.Type)
	}

	ddl, db, err = driver(config)

	if err != nil {
		return err
	}

	if config.Pool.MaxIdleConns > 0 {
		db.DB.SetMaxIdleConns(config.Pool.MaxIdleConns)
	}
	if config.Pool.MaxOpenConns > 0 {
		db.DB.SetMaxOpenConns(config.Pool.MaxOpenConns)
	}
	db.DB.SetConnMaxIdleTime(time.Duration(config.Pool.ConnMaxIdleTime) * time.Second)
	db.DB.SetConnMaxLifetime(time.Duration(config.Pool.ConnMaxLifetime) * time.Second)

	if err := CreateTables(ddl); err != nil {
		return fmt.Errorf("failed create tables, err: %w", err)
	}
	return nil
}

type ConnectionPool struct {
	MaxIdleConns    int `yaml:"maxIdleConns"`
	MaxOpenConns    int `yaml:"maxOpenConns"`
	ConnMaxLifetime int `yaml:"connMaxLifetime"`
	ConnMaxIdleTime int `yaml:"connMaxIdleTime"`
}

type Config struct {
	Type     string         `yaml:"type"`
	Addr     string         `yaml:"addr"`
	Username string         `yaml:"username"`
	Password string         `yaml:"password"`
	DBName   string         `yaml:"dbName"`
	Pool     ConnectionPool `yaml:"pool"`
}

func getDB() *sqlx.DB {
	return db
}

func MustExec(query string, args ...interface{}) sql.Result {
	return getDB().MustExec(query, args...)
}

func NamedExec(query string, arg interface{}) (sql.Result, error) {
	return getDB().NamedExec(query, arg)
}

// NamedInsert execute insert statement, return last insert id and error
func NamedInsert(query string, arg interface{}) (int64, error) {
	result, err := getDB().NamedExec(query, arg)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// NamedUpdate execute update statement, return affected row count and error
func NamedUpdate(query string, arg interface{}) (int64, error) {
	result, err := getDB().NamedExec(query, arg)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func Query(query string, args ...interface{}) (*sqlx.Rows, error) {
	return getDB().Queryx(query, args...)
}

func NamedQuery(query string, arg interface{}) (*sqlx.Rows, error) {
	return getDB().NamedQuery(query, arg)
}

func Select(dest interface{}, query string, args ...interface{}) error {
	return getDB().Select(dest, query, args...)
}

func Get(dest interface{}, query string, args ...interface{}) error {
	return getDB().Get(dest, query, args...)
}

func CreateTables(ddl string) error {
	_, err := db.Exec(ddl)
	return err
}
