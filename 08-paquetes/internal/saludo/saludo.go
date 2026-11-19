// Package saludo demuestra un paquete interno: API pública mínima y estado privado.
package saludo

import "fmt"

// Saludo es exportado (mayúscula) y por eso visible desde main.
type Saludo struct {
	nombre string // privado: solo este paquete lo toca
	idioma string
	veces  int // estado que main no puede manipular directamente
}

// NuevoSaludo es el constructor. Valida y aplica defaults.
func NuevoSaludo(nombre, idioma string) *Saludo {
	if idioma == "" {
		idioma = "es"
	}
	return &Saludo{nombre: nombre, idioma: idioma}
}

// Texto es el único método exportado. Oculta la lógica de formateo.
func (s *Saludo) Texto() string {
	s.veces++
	return formatear(s.idioma, s.nombre) + fmt.Sprintf(" (llamada %d)", s.veces)
}

// Hola es una función de paquete, no un método.
func Hola(nombre string) string {
	return formatear("es", nombre)
}

// formatear es privado: no forma parte del contrato del paquete.
func formatear(idioma, nombre string) string {
	switch idioma {
	case "en":
		return "Hello, " + nombre
	case "pt":
		return "Olá, " + nombre
	default:
		return "Hola, " + nombre
	}
}
