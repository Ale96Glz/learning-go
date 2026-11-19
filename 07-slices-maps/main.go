package main

import (
	"fmt"
	"slices"
	"strings"
)

type Tarea struct {
	ID        int
	Titulo    string
	Prioridad int // 1 = alta
	Hecha     bool
	Etiquetas []string
}

func main() {
	tareas := []Tarea{
		{ID: 1, Titulo: "Desplegar", Prioridad: 1, Etiquetas: []string{"devops", "urgente"}},
		{ID: 2, Titulo: "Documentar", Prioridad: 3, Etiquetas: []string{"docs"}},
		{ID: 3, Titulo: "Code review", Prioridad: 2, Etiquetas: []string{"equipo", "devops"}},
		{ID: 4, Titulo: "Comprar café", Prioridad: 3, Hecha: true, Etiquetas: []string{"personal"}},
	}

	// --- append: el slice crece. Nunca ignores el retorno. ---
	tareas = append(tareas, Tarea{ID: 5, Titulo: "Tests", Prioridad: 1, Etiquetas: []string{"calidad"}})

	// --- filtrar: SIEMPRE crea un slice nuevo (no borres mientras iteras) ---
	pendientes := make([]Tarea, 0, len(tareas))
	for _, t := range tareas {
		if !t.Hecha {
			pendientes = append(pendientes, t)
		}
	}
	fmt.Printf("pendientes: %d/%d\n", len(pendientes), len(tareas))

	// --- slices.Contains, slices.Delete (Go 1.21+) ---
	if slices.ContainsFunc(tareas, func(t Tarea) bool { return t.Titulo == "Tests" }) {
		fmt.Println("hay una tarea de Tests")
	}
	original := len(tareas)
	tareas = slices.DeleteFunc(tareas, func(t Tarea) bool { return t.ID == 4 })
	fmt.Printf("borradas: %d\n", original-len(tareas))

	// --- ordenar sin escribir un algoritmo ---
	slices.SortFunc(pendientes, func(a, b Tarea) int {
		if a.Prioridad != b.Prioridad {
			return a.Prioridad - b.Prioridad // 1 (alta) primero
		}
		return strings.Compare(a.Titulo, b.Titulo)
	})
	fmt.Println("\npor prioridad:")
	for _, t := range pendientes {
		fmt.Printf("  P%d %s\n", t.Prioridad, t.Titulo)
	}

	// --- map: contar por etiqueta ---
	porEtiqueta := make(map[string][]string)
	for _, t := range tareas {
		for _, e := range t.Etiquetas {
			porEtiqueta[e] = append(porEtiqueta[e], t.Titulo)
		}
	}
	fmt.Println("\npor etiqueta:")
	// Para imprimir ordenado: extrae las claves y ordénalas.
	claves := make([]string, 0, len(porEtiqueta))
	for k := range porEtiqueta {
		claves = append(claves, k)
	}
	slices.Sort(claves)
	for _, k := range claves {
		fmt.Printf("  %-8s %v\n", k, porEtiqueta[k])
	}

	// --- mapa de contadores ---
	prioridad := map[int]int{}
	for _, t := range tareas {
		prioridad[t.Prioridad]++ // el zero value 0 permite ++ directo
	}
	fmt.Println("\nconteo por prioridad:", prioridad)
	delete(prioridad, 3)
	fmt.Println("sin prioridad 3:", prioridad)

	// --- trampa: map nil ---
	var nilMap map[string]int
	fmt.Println("leer map nil:", nilMap["x"], "len:", len(nilMap))
	// nilMap["x"] = 1 // panic: assignment to entry in nil map

	// --- trampa: dos slices comparten array ---
	base := []int{1, 2, 3}
	copia := slices.Clone(base) // Clone sí copia; `copia := base` NO
	copia[0] = 99
	fmt.Println("\nbase:", base, "copia:", copia)

	// --- deduplicar conservando orden ---
	entrada := []string{"a", "b", "a", "c", "b"}
	vistos := map[string]struct{}{} // struct{} ocupa 0 bytes: set idiomático
	unicos := make([]string, 0, len(entrada))
	for _, s := range entrada {
		if _, ok := vistos[s]; ok {
			continue
		}
		vistos[s] = struct{}{}
		unicos = append(unicos, s)
	}
	fmt.Println("únicos:", unicos)
}
