package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func main() {
	// --- if con declaración inicial ---
	// err solo existe dentro del if/else. Evita variables sueltas.
	if edad, err := parseEdad("30"); err != nil {
		fmt.Println("entrada inválida:", err)
	} else if edad < 18 {
		fmt.Println("menor de edad")
	} else {
		fmt.Printf("adulto de %d\n", edad)
	}

	// --- switch ---
	for _, nota := range []int{95, 80, 61, 40} {
		fmt.Printf("%d -> %s\n", nota, clasificar(nota))
	}

	// switch sin expresión: reemplaza cadenas de if/else if.
	estado := "activo"
	switch {
	case estado == "activo":
		fmt.Println("puede operar")
	case estado == "bloqueado":
		fmt.Println("no puede operar")
	default:
		fmt.Println("estado desconocido")
	}

	// --- for: la única estructura de repetición ---
	suma := 0
	for i := 1; i <= 5; i++ { // clásico
		suma += i
	}
	fmt.Println("suma:", suma)

	intentos := 0
	for intentos < 3 { // "while"
		intentos++
	}
	fmt.Println("intentos:", intentos)

	// --- range ---
	frutas := []string{"manzana", "pera", "uva"}
	for i, f := range frutas {
		fmt.Printf("%d: %s\n", i, f)
	}

	stock := map[string]int{"pera": 3, "uva": 10}
	// Ojo: el orden de un map NO es estable.
	for nombre, cantidad := range stock {
		fmt.Printf("%s=%d ", nombre, cantidad)
	}
	fmt.Println()

	// --- label: salir de loops anidados ---
	var encontrado bool
buscar:
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			if i*j == 6 {
				encontrado = true
				break buscar
			}
		}
	}
	fmt.Println("encontrado:", encontrado)

	// --- Ejemplo real: reintentar una operación que falla ---
	if err := conReintentos(3, func(intento int) error {
		if intento < 3 {
			return fmt.Errorf("servicio no disponible (intento %d)", intento)
		}
		fmt.Println("servicio respondió en el intento", intento)
		return nil
	}); err != nil {
		fmt.Println("se agotaron los intentos:", err)
	}
}

func parseEdad(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("edad %q no es un número: %w", s, err)
	}
	if n < 0 || n > 120 {
		return 0, errors.New("edad fuera de rango")
	}
	return n, nil
}

func clasificar(nota int) string {
	switch {
	case nota >= 90:
		return "excelente"
	case nota >= 70:
		return "aprobado"
	default:
		return "reprobado"
	}
}

// conReintentos ejecuta fn hasta max veces y devuelve el último error.
// Es el patrón de toda llamada a un servicio externo inestable.
func conReintentos(max int, fn func(intento int) error) error {
	var err error
	for i := 1; i <= max; i++ {
		if err = fn(i); err == nil {
			return nil
		}
	}
	return fmt.Errorf("después de %d intentos: %w", max, err)
}
