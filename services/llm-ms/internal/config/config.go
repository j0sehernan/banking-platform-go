package config

import "github.com/caarlos0/env/v11"

type Config struct {
	HTTPAddr        string   `env:"HTTP_ADDR" envDefault:":8080"`
	DatabaseURL     string   `env:"DATABASE_URL,required"`
	KafkaBrokers    []string `env:"KAFKA_BROKERS,required" envSeparator:","`
	ServiceName     string   `env:"SERVICE_NAME" envDefault:"llm-ms"`
	AnthropicAPIKey string   `env:"ANTHROPIC_API_KEY"`
	LLMModel        string   `env:"LLM_MODEL" envDefault:"claude-haiku-4-5"`
}

func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
