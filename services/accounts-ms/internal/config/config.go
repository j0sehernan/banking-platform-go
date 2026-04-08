// Package config carga las variables de entorno en un struct tipado.
// Usar caarlos0/env mantiene el código limpio y declarativo.
package config

import "github.com/caarlos0/env/v11"

type Config struct {
	HTTPAddr     string   `env:"HTTP_ADDR" envDefault:":8080"`
	DatabaseURL  string   `env:"DATABASE_URL,required"`
	KafkaBrokers []string `env:"KAFKA_BROKERS,required" envSeparator:","`
	ServiceName  string   `env:"SERVICE_NAME" envDefault:"accounts-ms"`
}

func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
