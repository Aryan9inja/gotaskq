package config

type Config struct{
	Port string
	MaxRetries int
	BaseDelay int
	MaxDelay int
	NumWorkers int
}