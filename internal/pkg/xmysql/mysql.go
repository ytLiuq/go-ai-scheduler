package xmysql

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Open creates a shared MySQL connection pool for repository implementations.
func Open(dsn string) (*sql.DB, error) {
	// Use client-side interpolation to avoid MySQL server-side prepared statement
	// caching issues with recently added columns.
	if !strings.Contains(dsn, "interpolateParams=true") {
		if strings.Contains(dsn, "?") {
			dsn += "&interpolateParams=true"
		} else {
			dsn += "?interpolateParams=true"
		}
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return db, nil
}
