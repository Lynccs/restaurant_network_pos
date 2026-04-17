package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/microsoft/go-mssqldb"
)

func New() (*sql.DB, error) {
	server := os.Getenv("DB_SERVER")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	if port == "" {
		port = "1433"
	}

	dsn := fmt.Sprintf(
		"sqlserver://%s:%s@%s:%s?database=%s&connection+timeout=30&encrypt=disable",
		user, password, server, port, dbName,
	)

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return db, nil
}
