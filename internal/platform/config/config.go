package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env             string
	Port            string
	LogLevel        string
	DatabaseURL     string
	APIsPeruBaseURL string
	APIsPeruToken   string
	APIsPeruTimeout time.Duration
	// APIsPeruMonthlyLimit is our own cap on upstream calls per month, below the plan's quota.
	APIsPeruMonthlyLimit int
}

// Load reads .env when present (development) and then the process environment.
func Load() (Config, error) {
	_ = godotenv.Load()

	timeout, err := strconv.Atoi(get("APISPERU_TIMEOUT_SECONDS", "10"))
	if err != nil || timeout <= 0 {
		return Config{}, errors.New("APISPERU_TIMEOUT_SECONDS must be a positive integer")
	}
	limit, err := strconv.Atoi(get("APISPERU_MONTHLY_LIMIT", "900"))
	if err != nil || limit <= 0 {
		return Config{}, errors.New("APISPERU_MONTHLY_LIMIT must be a positive integer")
	}
	cfg := Config{
		APIsPeruMonthlyLimit: limit,
		Env:                  get("APP_ENV", "development"),
		Port:                 get("PORT", "8080"),
		LogLevel:             get("LOG_LEVEL", "info"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		APIsPeruBaseURL:      get("APISPERU_BASE_URL", "https://dniruc.apisperu.com/api/v1"),
		APIsPeruToken:        os.Getenv("APISPERU_TOKEN"),
		APIsPeruTimeout:      time.Duration(timeout) * time.Second,
	}
	for name, value := range map[string]string{"DATABASE_URL": cfg.DatabaseURL, "APISPERU_TOKEN": cfg.APIsPeruToken} {
		if value == "" {
			return Config{}, fmt.Errorf("%s is required", name)
		}
	}
	return cfg, nil
}

func get(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
