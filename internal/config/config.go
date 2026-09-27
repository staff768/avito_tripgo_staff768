package config

import (
	"fmt"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr        string        `env:"HTTP_ADDR" envDefault:":8080"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`

	HTTPReadTimeout       time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"5s"`
	HTTPReadHeaderTimeout time.Duration `env:"HTTP_READ_HEADER_TIMEOUT" envDefault:"3s"`
	HTTPWriteTimeout      time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"10s"`
	HTTPIdleTimeout       time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"60s"`

	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	DatabaseURL             string        `env:"DATABASE_URL,required"`
	DatabaseMaxConns        int32         `env:"DATABASE_MAX_CONNS" envDefault:"10"`
	DatabaseMinConns        int32         `env:"DATABASE_MIN_CONNS" envDefault:"2"`
	DatabaseMaxConnLifetime time.Duration `env:"DATABASE_MAX_CONN_LIFETIME" envDefault:"30m"`
	DatabaseConnectTimeout  time.Duration `env:"DATABASE_CONNECT_TIMEOUT" envDefault:"5s"`
	DatabaseQueryTimeout    time.Duration `env:"DATABASE_QUERY_TIMEOUT" envDefault:"3s"`
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load .env: %w", err)
	}

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	return cfg, nil
}
