package app

import "os"

type Config struct {
	Addr         string
	MLServiceURL string
	DatabaseURL  string
}

func LoadConfig() Config {
	port := envOrDefault("BACKEND_PORT", "8000")
	return Config{
		Addr:         ":" + port,
		MLServiceURL: envOrDefault("ML_SERVICE_URL", "http://localhost:8001"),
		DatabaseURL:  envOrDefault("DATABASE_URL", ""),
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
