package main

import (
	"errors"
	"testing"
)

// Test de tabla: el patrón estándar en Go. Un caso = una fila.
func TestDividir(t *testing.T) {
	casos := []struct {
		nombre    string
		a, b      float64
		esperado  float64
		esperaErr error
	}{
		{"positivos", 10, 4, 2.5, nil},
		{"negativos", -9, 3, -3, nil},
		{"cero arriba", 0, 5, 0, nil},
		{"división por cero", 1, 0, 0, ErrDivisionPorCero},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := Dividir(c.a, c.b)

			if !errors.Is(err, c.esperaErr) {
				t.Fatalf("error = %v, esperaba %v", err, c.esperaErr)
			}
			if err != nil {
				return // si esperábamos error, no comparamos el resultado
			}
			if got != c.esperado {
				t.Errorf("Dividir(%v, %v) = %v, esperaba %v", c.a, c.b, got, c.esperado)
			}
		})
	}
}

func TestValidarEmail(t *testing.T) {
	validos := []string{"ana@example.com", "a@b.co", "  x@y.com  "}
	for _, e := range validos {
		if err := ValidarEmail(e); err != nil {
			t.Errorf("ValidarEmail(%q) devolvió error: %v", e, err)
		}
	}

	invalidos := []string{"", "sin-arroba", "@ejemplo.com", "ana@"}
	for _, e := range invalidos {
		err := ValidarEmail(e)
		if !errors.Is(err, ErrEmailInvalido) {
			t.Errorf("ValidarEmail(%q) = %v, esperaba ErrEmailInvalido", e, err)
		}
	}
}

// Test de ejemplo con subtest y helper: imprime como documentación cuando
// corres `go test -v`.
func TestSumar(t *testing.T) {
	if got := Sumar(2, 3); got != 5 {
		t.Fatalf("Sumar(2,3) = %d, esperaba 5", got)
	}
}
