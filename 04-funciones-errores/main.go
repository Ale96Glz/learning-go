package main

import (
	"errors"
	"fmt"
	"strings"
)

// ErrEdadInvalida es un error "centinela": un valor conocido que el caller
// puede comparar con errors.Is sin mirar el texto.
var ErrEdadInvalida = errors.New("edad inválida")

// ValidationError es un error tipado: lleva datos, no solo un mensaje.
type ValidationError struct {
	Campo  string
	Motivo string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("campo %q: %s", e.Campo, e.Motivo)
}

type Usuario struct {
	Nombre string
	Edad   int
}

// NewUsuario valida y construye. Si devuelve error, el usuario es nil.
// Regla: un constructor no debe devolver un objeto a medio construir.
func NewUsuario(nombre string, edad int) (*Usuario, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return nil, &ValidationError{Campo: "nombre", Motivo: "no puede estar vacío"}
	}
	if len(nombre) > 80 {
		return nil, &ValidationError{Campo: "nombre", Motivo: "máximo 80 caracteres"}
	}
	if edad < 0 || edad > 120 {
		// %w envuelve el error: conserva la cadena para errors.Is.
		return nil, fmt.Errorf("crear usuario %q: %w", nombre, ErrEdadInvalida)
	}
	return &Usuario{Nombre: nombre, Edad: edad}, nil
}

func main() {
	casos := []struct {
		nombre string
		edad   int
	}{
		{"Ana", 30},
		{"   ", 30},
		{"Beto", -5},
		{strings.Repeat("x", 81), 40},
	}

	for _, c := range casos {
		u, err := NewUsuario(c.nombre, c.edad)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)

			// errors.As: ¿hay un ValidationError en la cadena? Extrae sus campos.
			var ve *ValidationError
			if errors.As(err, &ve) {
				fmt.Printf("  -> campo inválido: %s (%s)\n", ve.Campo, ve.Motivo)
			}

			// errors.Is: ¿la cadena contiene este error centinela?
			if errors.Is(err, ErrEdadInvalida) {
				fmt.Println("  -> el problema es la edad")
			}
			continue
		}
		fmt.Printf("OK: %+v\n", *u)
	}

	// defer: se ejecuta al salir de la función, en orden inverso (LIFO).
	// Acá se usa para garantizar limpieza aunque haya returns en el medio.
	func() {
		defer fmt.Println("3. cierro conexión")
		defer fmt.Println("2. hago rollback")
		fmt.Println("1. abro transacción")
	}()

	fmt.Println("fin")
}
