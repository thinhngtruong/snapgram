package config

import "os"

type Config struct {
	AppEnv        string
	HTTPAddr      string
	JWTSecret     string
	DatabaseURL   string
	RedisAddr     string
	MinIOBucket   string
	CDNBaseURL    string
	MinIOEndpoint string
}

func Load() Config {
	return Config{
		AppEnv:        env("APP_ENV", "local"),
		HTTPAddr:      env("HTTP_ADDR", ":8080"),
		JWTSecret:     env("JWT_SECRET", "change-me-local-only"),
		DatabaseURL:   env("DATABASE_URL", "postgres://snapgram:snapgram@localhost:5432/snapgram?sslmode=disable"),
		RedisAddr:     env("REDIS_ADDR", "localhost:6379"),
		MinIOBucket:   env("MINIO_BUCKET", "snapgram-media"),
		CDNBaseURL:    env("CDN_BASE_URL", "http://localhost:8088/snapgram-media"),
		MinIOEndpoint: env("MINIO_ENDPOINT", "localhost:9000"),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
