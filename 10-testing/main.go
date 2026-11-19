package main

import (
	"errors"
	"fmt"
	"strings"
)

var ErrDivisionPorCero = errors.New("división por cero")
var ErrEmailInvalido = errors.New("email inválido")

func Sumar(a, b int) int { return a + b }

func Dividir(a, b float64) (float64, error) {
	if b == 0 {
		return 0, ErrDivisionPorCero
	}
	return a / b, nil
}

// ValidarEmail es chica a propósito: fácil de testear, sin dependencias.
func ValidarEmail(s string) error {
	s = strings.TrimSpace(s)
	arroba := strings.Index(s, "@")
	if arroba <= 0 || arroba == len(s)-1 {
		return fmt.Errorf("%w: %q", ErrEmailInvalido, s)
	}
	return nil
}

func main() {
	fmt.Println("2+3 =", Sumar(2, 3))
	if r, err := Dividir(10, 4); err == nil {
		fmt.Println("10/4 =", r)
	}
	if _, err := Dividir(1, 0); errors.Is(err, ErrDivisionPorCero) {
		fmt.Println("error esperado:", err)
	}
	fmt.Println("email:", ValidarEmail("ana@example.com"))
}
