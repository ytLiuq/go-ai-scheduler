package config

import (
	"os"
	"strconv"
	"strings"
)

// Config contains bootstrap settings shared by all services.
type Config struct {
	ServiceName       string
	HTTPAddr          string
	GRPCAddr          string
	SchedulerURL      string
	SchedulerGRPCAddr string
	MySQLDSN          string
	RedisAddr         string
	AlertWebhookURL   string
	MaxPending        int
	EtcdAddrs         []string
	RepoBackend       string
	MigrationDir      string
	AutoMigrate       bool
	InternalProtocol  string
}

// Default returns bootstrap config for local development with MySQL-backed repositories.
func Default(serviceName string) Config {
	cfg := Config{
		ServiceName:  serviceName,
		HTTPAddr:     ":8080",
		GRPCAddr:     ":9090",
		SchedulerURL: "http://127.0.0.1:8080",
		SchedulerGRPCAddr: "127.0.0.1:9090",
		MySQLDSN:     "root:root@tcp(127.0.0.1:3306)/go_ai_scheduler?parseTime=true",
		RedisAddr:    "127.0.0.1:6379",
		EtcdAddrs:    []string{"127.0.0.1:2379"},
		RepoBackend:  "mysql",
		MigrationDir: "migrations",
		MaxPending:    1000,
		AutoMigrate:   false,
		InternalProtocol: "http",
	}
	switch serviceName {
	case "worker":
		cfg.HTTPAddr = ":8081"
		cfg.GRPCAddr = ":9091"
	case "api":
		cfg.HTTPAddr = ":8082"
	case "ai-service":
		cfg.HTTPAddr = ":8083"
	case "migrate":
		cfg.RepoBackend = "mysql"
	}
	if value := os.Getenv("APP_HTTP_ADDR"); value != "" {
		cfg.HTTPAddr = value
	}
	if value := os.Getenv("APP_GRPC_ADDR"); value != "" {
		cfg.GRPCAddr = value
	}
	if value := os.Getenv("SCHEDULER_URL"); value != "" {
		cfg.SchedulerURL = value
	}
	if value := os.Getenv("SCHEDULER_GRPC_ADDR"); value != "" {
		cfg.SchedulerGRPCAddr = value
	}
	if value := os.Getenv("MYSQL_DSN"); value != "" {
		cfg.MySQLDSN = value
	}
	if value := os.Getenv("REPO_BACKEND"); value != "" {
		cfg.RepoBackend = value
	}
	if value := os.Getenv("MIGRATION_DIR"); value != "" {
		cfg.MigrationDir = value
	}
	if value := os.Getenv("AUTO_MIGRATE"); value == "1" || value == "true" || value == "TRUE" {
		cfg.AutoMigrate = true
	}
	if value := os.Getenv("INTERNAL_PROTOCOL"); value != "" {
		cfg.InternalProtocol = value
	}
	if value := os.Getenv("ETCD_ENDPOINTS"); value != "" {
		cfg.EtcdAddrs = splitEnv(value)
	}
	if value := os.Getenv("REDIS_ADDR"); value != "" {
		cfg.RedisAddr = value
	}
	if value := os.Getenv("ALERT_WEBHOOK_URL"); value != "" {
		cfg.AlertWebhookURL = value
	}
	if value := os.Getenv("MAX_PENDING"); value != "" {
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			cfg.MaxPending = n
		}
	}
	return cfg
}

func splitEnv(value string) []string {
	parts := make([]string, 0)
	for _, s := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}
