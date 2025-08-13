package config

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                   string
	DBPath                 string
	MaxRequestSize         int64
	AllowDeleteSets        bool
	AllowDeleteCollections bool
	CORSOrigins            []string
	DevMode                bool
}

func Load() (*Config, error) {
	_ = godotenv.Load() // load .env if present
	cfg := &Config{
		Port:                   getEnv("PORT", "8080"),
		DBPath:                 getEnv("DB_PATH", "./data.db"),
		MaxRequestSize:         getEnvInt64("MAX_REQUEST_SIZE", 1048576),
		AllowDeleteSets:        getEnvBool("ALLOW_DELETE_SETS", false),
		AllowDeleteCollections: getEnvBool("ALLOW_DELETE_COLLECTIONS", false),
		CORSOrigins:            parseCSV(os.Getenv("CORS")),
		DevMode:                getEnvBool("DEV", false),
	}
	if cfg.Port == "" {
		return nil, errors.New("PORT cannot be empty")
	}
	if cfg.DBPath == "" {
		return nil, errors.New("DB_PATH cannot be empty")
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return def
}

func getEnvInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		i, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return i
		}
	}
	return def
}

func parseCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			res = append(res, p)
		}
	}
	return res
}
