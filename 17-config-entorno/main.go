package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Puerto       int
	Entorno      string
	DBRuta       string
	Timeout      time.Duration
	NivelLog     string
	TokenSecreto string
}

// String redacta el secreto: así es seguro loguear la config completa.
func (c Config) String() string {
	secreto := "***"
	if c.TokenSecreto == "" {
		secreto = "(vacío)"
	}
	return fmt.Sprintf(
		"Config{puerto:%d entorno:%s db:%s timeout:%s log:%s secreto:%s}",
		c.Puerto, c.Entorno, c.DBRuta, c.Timeout, c.NivelLog, secreto)
}

// Cargar lee variables de entorno, aplica defaults y valida todo junto.
// Devuelve TODOS los errores de una vez: nadie quiere arreglar uno, recompilar
// y descubrir el siguiente.
func Cargar() (Config, error) {
	cfg := Config{
		Puerto:   8080,
		Entorno:  "dev",
		DBRuta:   "tareas.db",
		Timeout:  10 * time.Second,
		NivelLog: "info",
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

	switch v := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))); v {
	case "", "dev":
		cfg.Entorno = "dev"
	case "staging", "prod":
		cfg.Entorno = v
	default:
		errs = append(errs, fmt.Errorf("APP_ENV inválido: %q (usa dev, staging o prod)", v))
	}

	if v := os.Getenv("DB_RUTA"); v != "" {
		cfg.DBRuta = v
	}

	if v := os.Getenv("TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("TIMEOUT inválido: %q (ej: 10s, 1m)", v))
		} else {
			cfg.Timeout = d
		}
	}

	if v := strings.ToLower(os.Getenv("LOG_NIVEL")); v != "" {
		switch v {
		case "debug", "info", "warn", "error":
			cfg.NivelLog = v
		default:
			errs = append(errs, fmt.Errorf("LOG_NIVEL inválido: %q", v))
		}
	}

	cfg.TokenSecreto = os.Getenv("TOKEN_SECRETO")
	if len(cfg.TokenSecreto) < 32 {
		errs = append(errs, errors.New("TOKEN_SECRETO es obligatorio (mínimo 32 caracteres)"))
	}

	return cfg, errors.Join(errs...)
}

func main() {
	cfg, err := Cargar()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config inválida:\n%v\n", err)
		os.Exit(1)
	}

	fmt.Println("config cargada:", cfg)
	fmt.Printf("escucharía en :%d durante %s (entorno %s)\n", cfg.Puerto, cfg.Timeout, cfg.Entorno)
}
