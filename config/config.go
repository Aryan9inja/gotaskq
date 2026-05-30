package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port           string
	MaxDelay       int
	NumWorkers     int
	UseRedis       bool
	RedisUrl       string
	CompleteJobTTL int
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

		MaxDelay:       envToInt(os.Getenv("MAX_DELAY"), 5000),
		NumWorkers:     envToInt(os.Getenv("NUM_WORKERS"), 10),
		UseRedis:       os.Getenv("USE_REDIS") == "true",
		RedisUrl:       os.Getenv("REDIS_URL"),
		CompleteJobTTL: envToInt(os.Getenv("COMPLETE_JOB_TTL"), 5), // minutes will be converted in main
	}
}
