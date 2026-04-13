package config

import (
	"log"
	"os"
	"github.com/joho/godotenv"
)

type Config struct {
	Env  string
	Port string
	Log  LogConfig
}

type LogConfig struct {
	Level string
	Format string
}

func Load() *Config {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "dev"
	}

	// Load env file if exists (only for local/dev)
	_ = godotenv.Load(".env." + env)

	cfg := &Config{
		Env:  env,
		Port: getEnv("PORT", "8080"),
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "text"),
		},
	}

	validate(cfg)
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func validate(cfg *Config) {
	if cfg.Port == "" {
		log.Fatal("PORT must be set")
	}
}
