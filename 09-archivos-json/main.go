package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Tarea struct {
	ID     int    `json:"id"`
	Titulo string `json:"titulo"`
	Hecha  bool   `json:"hecha"`
}

// cargar devuelve una lista vacía si el archivo no existe: el primer arranque
// no es un error. Cualquier otro fallo sí se propaga con contexto.
func cargar(ruta string) ([]Tarea, error) {
	b, err := os.ReadFile(ruta)
	if errors.Is(err, os.ErrNotExist) {
		return []Tarea{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("leer %s: %w", ruta, err)
	}
	var tareas []Tarea
	if err := json.Unmarshal(b, &tareas); err != nil {
		return nil, fmt.Errorf("parsear %s: %w", ruta, err)
	}
	return tareas, nil
}

// guardar escribe a un temporal y renombra. Así un corte de luz a mitad de
// escritura no deja el archivo de datos corrupto: o está el viejo o el nuevo.
func guardar(ruta string, tareas []Tarea) error {
	f, err := os.CreateTemp(filepath.Dir(ruta), ".tareas-*.tmp")
	if err != nil {
		return fmt.Errorf("crear temporal: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op si el rename tuvo éxito

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(tareas); err != nil {
		f.Close()
		return fmt.Errorf("codificar json: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("cerrar temporal: %w", err)
	}
	if err := os.Rename(tmp, ruta); err != nil { // atómico en el mismo filesystem
		return fmt.Errorf("renombrar: %w", err)
	}
	return nil
}

func main() {
	ruta := "tareas.json"
	_ = os.Remove(ruta) // ejemplo repetible

	// 1. primer arranque: no existe -> lista vacía
	tareas, err := cargar(ruta)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("cargadas %d tareas\n", len(tareas))

	// 2. modificar y guardar
	tareas = append(tareas,
		Tarea{ID: 1, Titulo: "Aprender Go"},
		Tarea{ID: 2, Titulo: "Escribir tests", Hecha: true},
	)
	if err := guardar(ruta, tareas); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	// 3. releer y comprobar
	tareas, err = cargar(ruta)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	for _, t := range tareas {
		fmt.Printf("  #%d %-20s hecha=%t\n", t.ID, t.Titulo, t.Hecha)
	}

	// 4. JSON inválido: el error dice qué archivo y qué pasó.
	if err := os.WriteFile("roto.json", []byte("{no es json"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if _, err := cargar("roto.json"); err != nil {
		fmt.Println("error esperado:", err)
	}
	_ = os.Remove("roto.json")
	_ = os.Remove(ruta)
}
