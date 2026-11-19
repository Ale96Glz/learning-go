// Package main es un ejecutable: se compila a un binario y arranca en main().
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	// os.Args[0] es el nombre del programa; lo que sigue son los argumentos.
	args := os.Args[1:]

	nombre := "mundo"
	if len(args) > 0 {
		// Join permite "go run . Ana María" sin comillas.
		nombre = strings.Join(args, " ")
	}

	fmt.Printf("Hola, %s!\n", nombre)

	// Código de salida != 0 = fallo. Los scripts y CI lo usan.
	if nombre == "error" {
		fmt.Fprintln(os.Stderr, "pediste un error a propósito")
		os.Exit(1)
	}
}
