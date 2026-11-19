package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Puerto  int
	DBRuta  string
	Entorno string
	Timeout time.Duration
}

func Cargar() (Config, error) {
	cfg := Config{
		Puerto:  8080,
		DBRuta:  "tareas.db",
		Entorno: "dev",
		Timeout: 10 * time.Second,
	}
	var errs []error

	if v := os.Getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			errs = append(errs, fmt.Errorf("PORT inválido: %q", v))
		} else {
			cfg.Puerto = p
		}
	}
	if v := os.Getenv("DB_RUTA"); v != "" {
		cfg.DBRuta = v
	}
	switch v := os.Getenv("APP_ENV"); v {
	case "", "dev":
		cfg.Entorno = "dev"
	case "staging", "prod":
		cfg.Entorno = v
	default:
		errs = append(errs, fmt.Errorf("APP_ENV inválido: %q (usa dev, staging o prod)", v))
	}
	if v := os.Getenv("TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("TIMEOUT inválido: %q (ej: 10s)", v))
		} else {
			cfg.Timeout = d
		}
	}
	return cfg, errors.Join(errs...)
}
