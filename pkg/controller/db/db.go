package db

import (
	"bytes"
	"database/sql"
	_ "database/sql"
	_ "embed"
	"fmt"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"text/template"
	"time"
)

//go:embed db_schema_v1.sql.tpl
var ddlTemplate string

// rendered ddl
var ddl string

var (
	db *sqlx.DB
)

func InitDB() {
	//TODO with config file?
}

func prepareDDL(engine string) {
	tpl := template.Must(template.New("ddl").Parse(ddlTemplate))
	buf := &bytes.Buffer{}
	ctx := map[string]interface{}{
		"engine": engine,
	}

	if err := tpl.Execute(buf, ctx); err != nil {
		panic(fmt.Sprintf("failed render ddl, err: %v", err))
	}

	ddl = buf.String()
}

func init() {

	engine := "sqlite3"
	prepareDDL(engine)

	//path := "/var/lib/kubeskoop/controller_db.sqlite3"
	path := ":memory:"

	var err error
	db, err = sqlx.Connect("sqlite3", path)
	if err != nil {
		panic(fmt.Sprintf("failed open sqlite db, err: %v", err))
	}

	db.DB.SetConnMaxLifetime(10 * time.Second)
	db.DB.SetMaxIdleConns(10)
	db.DB.SetMaxOpenConns(100)
	db.DB.SetConnMaxIdleTime(60 * time.Second)

	//TODO check weather this is a new database
	if err := CreateTables(); err != nil {
		panic(fmt.Sprintf("failed init database, err: %v", err))
	}
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

func CreateTables() error {
	_, err := db.Exec(ddl)
	return err
}
