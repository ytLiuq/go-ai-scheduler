package xmysql

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// Open creates a shared MySQL connection pool for repository implementations.
func Open(dsn string) (*sql.DB, error) {
	gdb, err := OpenGorm(dsn)
	if err != nil {
		return nil, err
	}
	db, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("get mysql sql db: %w", err)
	}
	return db, nil
}

// OpenGorm creates a shared GORM client with the same underlying sql.DB pool.
func OpenGorm(dsn string) (*gorm.DB, error) {
	// Use client-side interpolation to avoid MySQL server-side prepared statement
	// caching issues with recently added columns.
	if !strings.Contains(dsn, "interpolateParams=true") {
		if strings.Contains(dsn, "?") {
			dsn += "&interpolateParams=true"
		} else {
			dsn += "?interpolateParams=true"
		}
	}
	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	db, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("get mysql sql db: %w", err)
	}

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return gdb, nil
}
