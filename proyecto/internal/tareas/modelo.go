package tareas

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNoExiste       = errors.New("tarea no existe")
	ErrTituloInvalido = errors.New("titulo inválido")
)

type Tarea struct {
	ID     int64     `json:"id"`
	Titulo string    `json:"titulo"`
	Hecha  bool      `json:"hecha"`
	Creada time.Time `json:"creada_en"`
}

func validarTitulo(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%w: no puede estar vacío", ErrTituloInvalido)
	}
	if len([]rune(s)) > 200 {
		return "", fmt.Errorf("%w: máximo 200 caracteres", ErrTituloInvalido)
	}
	return s, nil
}
