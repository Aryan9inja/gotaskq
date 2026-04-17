package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port       string
	MaxRetries int
	BaseDelay  int
	MaxDelay   int
	NumWorkers int
}

func envToInt(v string, defaultValue int) int {
	if v != "" {
		val, err := strconv.Atoi(v)
		if err == nil {
			return val
		}
	}
	return defaultValue
}

func LoadConfig() *Config {
	// Load from env or file
	return &Config{
		Port: func() string {
			if port := os.Getenv("PORT"); port != "" {
				return port
			}
			return "8000"
		}(),

		MaxRetries: envToInt(os.Getenv("MAX_RETRIES"), 5),
		BaseDelay:  envToInt(os.Getenv("BASE_DELAY"), 100),
		MaxDelay:   envToInt(os.Getenv("MAX_DELAY"), 5000),
		NumWorkers: envToInt(os.Getenv("NUM_WORKERS"), 10),
	}
}
