package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	OTelServiceName    string        `env:"OTEL_SERVICE_NAME" envDefault:"trip-service"`
	ServiceVersion     string        `env:"SERVICE_VERSION" envDefault:"local"`
	OTelEndpoint       string        `env:"OTEL_EXPORTER_OTLP_ENDPOINT,required"`
	OTelProtocol       string        `env:"OTEL_EXPORTER_OTLP_PROTOCOL" envDefault:"http/protobuf"`
	OTelSampler        string        `env:"OTEL_TRACES_SAMPLER" envDefault:"parentbased_always_on"`
	OTelSamplerArg     float64       `env:"OTEL_TRACES_SAMPLER_ARG" envDefault:"1"`
	OTelMetricInterval time.Duration `env:"OTEL_METRIC_EXPORT_INTERVAL_DURATION" envDefault:"5s"`

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

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	for name, value := range map[string]time.Duration{
		"HTTP_READ_TIMEOUT": c.HTTPReadTimeout, "HTTP_READ_HEADER_TIMEOUT": c.HTTPReadHeaderTimeout,
		"HTTP_WRITE_TIMEOUT": c.HTTPWriteTimeout, "HTTP_IDLE_TIMEOUT": c.HTTPIdleTimeout,
		"SHUTDOWN_TIMEOUT": c.ShutdownTimeout, "DATABASE_MAX_CONN_LIFETIME": c.DatabaseMaxConnLifetime,
		"DATABASE_CONNECT_TIMEOUT": c.DatabaseConnectTimeout, "DATABASE_QUERY_TIMEOUT": c.DatabaseQueryTimeout,
		"OTEL_METRIC_EXPORT_INTERVAL_DURATION": c.OTelMetricInterval,
	} {
		if value <= 0 {
			return fmt.Errorf("%s must be positive", name)
		}
	}
	if c.HTTPAddr == "" || c.DatabaseURL == "" || c.OTelServiceName == "" {
		return errors.New("HTTP_ADDR, DATABASE_URL and OTEL_SERVICE_NAME must not be empty")
	}
	if c.DatabaseMaxConns <= 0 || c.DatabaseMinConns < 0 || c.DatabaseMinConns > c.DatabaseMaxConns {
		return errors.New("invalid database pool limits")
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(c.LogLevel)); err != nil {
		return fmt.Errorf("LOG_LEVEL: %w", err)
	}
	if c.OTelProtocol != "grpc" && c.OTelProtocol != "http/protobuf" {
		return errors.New("OTEL_EXPORTER_OTLP_PROTOCOL must be grpc or http/protobuf")
	}
	endpoint, err := url.Parse(c.OTelEndpoint)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return errors.New("OTEL_EXPORTER_OTLP_ENDPOINT must be an HTTP(S) URL")
	}
	switch c.OTelSampler {
	case "always_on", "always_off", "traceidratio", "parentbased_always_on", "parentbased_always_off", "parentbased_traceidratio":
	default:
		return errors.New("unsupported OTEL_TRACES_SAMPLER")
	}
	if c.OTelSamplerArg < 0 || c.OTelSamplerArg > 1 {
		return errors.New("OTEL_TRACES_SAMPLER_ARG must be between 0 and 1")
	}
	return nil
}
