package main

import (
	"strings"
	"testing"
	"time"
)

// limpiarDeja solo el secreto válido: los tests no dependen del entorno real.
func limpiar(t *testing.T) {
	t.Helper()
	for _, k := range []string{"PORT", "APP_ENV", "DB_RUTA", "TIMEOUT", "LOG_NIVEL"} {
		t.Setenv(k, "")
	}
	t.Setenv("TOKEN_SECRETO", strings.Repeat("s", 32))
}

func TestDefaults(t *testing.T) {
	limpiar(t)
	cfg, err := Cargar()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if cfg.Puerto != 8080 || cfg.Entorno != "dev" || cfg.Timeout != 10*time.Second {
		t.Fatalf("defaults incorrectos: %+v", cfg)
	}
}

func TestValoresValidos(t *testing.T) {
	limpiar(t)
	t.Setenv("PORT", "3000")
	t.Setenv("APP_ENV", "prod")
	t.Setenv("DB_RUTA", "/data/app.db")
	t.Setenv("TIMEOUT", "250ms")
	t.Setenv("LOG_NIVEL", "warn")

	cfg, err := Cargar()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Puerto != 3000 || cfg.Entorno != "prod" || cfg.DBRuta != "/data/app.db" ||
		cfg.Timeout != 250*time.Millisecond || cfg.NivelLog != "warn" {
		t.Fatalf("config incorrecta: %+v", cfg)
	}
}

func TestInvalidos(t *testing.T) {
	casos := []struct {
		nombre string
		env    map[string]string
		quiere string
	}{
		{"puerto no numérico", map[string]string{"PORT": "abc"}, "PORT"},
		{"puerto fuera de rango", map[string]string{"PORT": "70000"}, "PORT"},
		{"entorno desconocido", map[string]string{"APP_ENV": "qa"}, "APP_ENV"},
		{"timeout negativo", map[string]string{"TIMEOUT": "-1s"}, "TIMEOUT"},
		{"timeout sin unidad", map[string]string{"TIMEOUT": "10"}, "TIMEOUT"},
		{"secreto corto", map[string]string{"TOKEN_SECRETO": "corto"}, "TOKEN_SECRETO"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			limpiar(t)
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			_, err := Cargar()
			if err == nil {
				t.Fatal("esperaba error")
			}
			if !strings.Contains(err.Error(), c.quiere) {
				t.Errorf("error %q no menciona %q", err, c.quiere)
			}
		})
	}
}

func TestSecretoNoSeFiltra(t *testing.T) {
	limpiar(t)
	secreto := strings.Repeat("x", 40)
	t.Setenv("TOKEN_SECRETO", secreto)

	cfg, err := Cargar()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cfg.String(), secreto) {
		t.Fatal("String() no debe incluir el secreto")
	}
}
