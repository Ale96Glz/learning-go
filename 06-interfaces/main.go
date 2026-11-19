package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var ErrNoEncontrado = errors.New("no encontrado")

type Usuario struct {
	ID     int64  `json:"id"`
	Nombre string `json:"nombre"`
}

// Repositorio es una interfaz CHICA: solo lo que el consumidor necesita.
// No la definas junto a la implementación; defínela donde se usa.
type Repositorio interface {
	Guardar(u Usuario) error
	Buscar(id int64) (Usuario, error)
}

// --- Implementación 1: memoria (tests, prototipos) ---

type Memoria struct {
	datos map[int64]Usuario
}

func NuevaMemoria() *Memoria {
	return &Memoria{datos: make(map[int64]Usuario)}
}

func (m *Memoria) Guardar(u Usuario) error {
	if u.ID == 0 {
		return errors.New("id requerido")
	}
	m.datos[u.ID] = u
	return nil
}

func (m *Memoria) Buscar(id int64) (Usuario, error) {
	u, ok := m.datos[id]
	if !ok {
		return Usuario{}, ErrNoEncontrado
	}
	return u, nil
}

// --- Implementación 2: archivo JSON-lines (persistencia simple) ---

type Archivo struct {
	ruta string
}

func NuevoArchivo(ruta string) *Archivo { return &Archivo{ruta: ruta} }

func (a *Archivo) Guardar(u Usuario) error {
	f, err := os.OpenFile(a.ruta, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("abrir %s: %w", a.ruta, err)
	}
	defer f.Close()

	// json.Encoder escribe un objeto por línea. Ideal para logs/eventos.
	if err := json.NewEncoder(f).Encode(u); err != nil {
		return fmt.Errorf("escribir usuario: %w", err)
	}
	return nil
}

func (a *Archivo) Buscar(id int64) (Usuario, error) {
	f, err := os.Open(a.ruta)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Usuario{}, ErrNoEncontrado
		}
		return Usuario{}, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var u Usuario
		if err := json.Unmarshal(sc.Bytes(), &u); err != nil {
			return Usuario{}, fmt.Errorf("leer línea: %w", err)
		}
		if u.ID == id {
			return u, nil
		}
	}
	if err := sc.Err(); err != nil {
		return Usuario{}, err
	}
	return Usuario{}, ErrNoEncontrado
}

// --- Servicio: depende de la INTERFAZ, no de la implementación ---

type Servicio struct {
	repo Repositorio
	log  io.Writer // cualquier cosa que sepa escribir: consola, archivo, buffer
	seq  int64
}

func NuevoServicio(repo Repositorio, log io.Writer) *Servicio {
	return &Servicio{repo: repo, log: log}
}

func (s *Servicio) Crear(nombre string) (Usuario, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return Usuario{}, errors.New("nombre vacío")
	}
	s.seq++
	u := Usuario{ID: s.seq, Nombre: nombre}
	if err := s.repo.Guardar(u); err != nil {
		return Usuario{}, fmt.Errorf("guardar usuario: %w", err)
	}
	fmt.Fprintf(s.log, "usuario creado: %d %s\n", u.ID, u.Nombre)
	return u, nil
}

func main() {
	ruta := "usuarios.jsonl"
	// Limpieza para que el ejemplo sea repetible.
	_ = os.Remove(ruta)

	// main elige la implementación. El servicio no sabe cuál es.
	repos := []Repositorio{NuevaMemoria(), NuevoArchivo(ruta)}

	for i, repo := range repos {
		fmt.Printf("--- repositorio #%d (%T) ---\n", i+1, repo)
		svc := NuevoServicio(repo, os.Stdout)

		ana, _ := svc.Crear("Ana")
		_, _ = svc.Crear("Beto")

		encontrado, err := repo.Buscar(ana.ID)
		if err != nil {
			fmt.Println("error:", err)
		} else {
			fmt.Printf("encontrado: %+v\n", encontrado)
		}

		if _, err := repo.Buscar(999); errors.Is(err, ErrNoEncontrado) {
			fmt.Println("id 999: no encontrado (errors.Is funciona a través de capas)")
		}
	}

	// nil interface: trampa clásica.
	var r Repositorio
	if r == nil {
		fmt.Println("interfaz nil: no hay implementación")
	}
	// Un puntero nil DENTRO de una interfaz NO es igual a nil:
	// var m *Memoria = nil; var r Repositorio = m; r != nil  -> cuidado.
}
