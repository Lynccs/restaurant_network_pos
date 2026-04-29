package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

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

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(4 * time.Minute)

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	// Тримає мінімум одне idle-з'єднання живим, щоб SQL Server не переходив
	// у AUTO_CLOSE і не змушував чекати 30 с при першому запиті після паузи.
	go func() {
		ticker := time.NewTicker(3 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := db.PingContext(ctx); err != nil {
				log.Printf("[db] keepalive ping failed: %v", err)
			}
			cancel()
		}
	}()

	return db, nil
}
