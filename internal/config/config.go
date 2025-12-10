package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Environment       string
	Port              string
	DatabaseURL       string
	RedisURL          string
	JWTSecret         string
	JWTExpiry         time.Duration
	JWTRefreshExpiry  time.Duration
	EncryptionKey     string
	AllowedOrigins    string
	RateLimitRequests int
	RateLimitWindow   time.Duration
	MiniMaxAPIKey     string
	MiniMaxGroupID    string
	StorageType       string
	UploadPath        string
	UploadMaxSize     int64
	MTLSEnabled       bool
	MTLSCAPath        string
}

func Load() *Config {
	jwtExpiry, _ := time.ParseDuration(getEnv("JWT_EXPIRY", "15m"))
	jwtRefreshExpiry, _ := time.ParseDuration(getEnv("JWT_REFRESH_EXPIRY", "168h"))
	rateLimitWindow, _ := time.ParseDuration(getEnv("RATE_LIMIT_WINDOW", "1m"))
	rateLimitRequests, _ := strconv.Atoi(getEnv("RATE_LIMIT_REQUESTS", "100"))
	uploadMaxSize, _ := strconv.ParseInt(getEnv("UPLOAD_MAX_SIZE", "52428800"), 10, 64)

	return &Config{
		Environment:       getEnv("ENVIRONMENT", "development"),
		Port:              getEnv("PORT", "8082"),
		DatabaseURL:       getEnv("DATABASE_URL", ""),
		RedisURL:          getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:         getEnv("JWT_SECRET", ""),
		JWTExpiry:         jwtExpiry,
		JWTRefreshExpiry:  jwtRefreshExpiry,
		EncryptionKey:     getEnv("ENCRYPTION_KEY", ""),
		AllowedOrigins:    getEnv("ALLOWED_ORIGINS", "*"),
		RateLimitRequests: rateLimitRequests,
		RateLimitWindow:   rateLimitWindow,
		MiniMaxAPIKey:     getEnv("MINIMAX_API_KEY", ""),
		MiniMaxGroupID:    getEnv("MINIMAX_GROUP_ID", ""),
		StorageType:       getEnv("STORAGE_TYPE", "local"),
		UploadPath:        getEnv("UPLOAD_PATH", "./uploads"),
		UploadMaxSize:     uploadMaxSize,
		MTLSEnabled:       getEnv("MTLS_ENABLED", "false") == "true",
		MTLSCAPath:        getEnv("MTLS_CA_PATH", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
