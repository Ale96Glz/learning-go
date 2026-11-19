package main

import (
	"fmt"

	"tutorialgo/08-paquetes/internal/saludo"
)

func main() {
	fmt.Println(saludo.Hola("Ana"))

	// saludo.formatear(...) NO compila: es privado (minúscula) del paquete.
	// Eso es intencional: el paquete controla su API pública.

	s := saludo.NuevoSaludo("Beto", "es")
	fmt.Println(s.Texto())
	fmt.Println(s.Texto()) // el contador interno solo lo ve el paquete
}
