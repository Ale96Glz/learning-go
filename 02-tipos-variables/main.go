package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Las constantes se evalúan en compilación y no se pueden reasignar.
const (
	appName     = "tutorial-go"
	maxIntentos = 3
)

// iota genera valores consecutivos: 0, 1, 2.
type Nivel int

const (
	Debug Nivel = iota
	Info
	Error
)

func (n Nivel) String() string {
	niveles := [...]string{"DEBUG", "INFO", "ERROR"}
	if n < 0 || int(n) >= len(niveles) {
		return fmt.Sprintf("Nivel(%d)", int(n))
	}
	return niveles[n]
}

// Config es un ejemplo real: parámetros que vienen del entorno con defaults.
type Config struct {
	Puerto     int
	Debug      bool
	Timeout    time.Duration
	Directorio string
}

func main() {
	// Declaración corta (la más usada dentro de funciones).
	nombre := "Ana"
	edad := 30

	// var sirve cuando quieres el zero value o declarar sin asignar.
	var activo bool   // false
	var saldo float64 // 0
	var alias string  // ""
	var puntero *int  // nil

	fmt.Printf("%s (%d) activo=%t saldo=%.2f alias=%q puntero=%v\n",
		nombre, edad, activo, saldo, alias, puntero)

	// No hay conversión implícita: hay que ser explícito.
	var precio int = 100
	impuesto := 0.21
	total := float64(precio) * (1 + impuesto)
	fmt.Printf("total: %.2f\n", total)

	// Constantes tipadas vs no tipadas.
	const segundos = 30
	timeout := segundos * time.Second // constante sin tipo se adapta
	fmt.Printf("timeout: %v\n", timeout)

	nivel := Info
	fmt.Printf("nivel %d = %s\n", nivel, nivel)

	fmt.Println("app:", appName, "intentos:", maxIntentos)

	// Entorno: string siempre; la conversión y el default son tuyos.
	puerto := 8080
	if v := os.Getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "PORT inválido (%q): %v\n", v, err)
			os.Exit(1)
		}
		puerto = p
	}

	cfg := Config{
		Puerto:     puerto,
		Debug:      os.Getenv("DEBUG") == "true", // sin librería: comparación simple
		Timeout:    timeout,
		Directorio: "./datos",
	}
	fmt.Printf("config: %+v\n", cfg)
}
