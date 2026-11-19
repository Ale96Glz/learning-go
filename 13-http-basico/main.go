package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Tarea struct {
	ID     int    `json:"id"`
	Titulo string `json:"titulo"`
	Hecha  bool   `json:"hecha"`
}

type Server struct {
	mu     sync.Mutex
	tareas map[int]Tarea
	nextID int
}

func NuevoServer() *Server {
	return &Server{tareas: make(map[int]Tarea), nextID: 1}
}

// Handler arma el mux. Separarlo de main() es lo que permite testearlo con httptest.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/salud", s.salud)
	mux.HandleFunc("/tareas", s.coleccion)
	mux.HandleFunc("/tareas/", s.individual)
	return conLogging(mux)
}

func (s *Server) salud(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

// GET /tareas  |  POST /tareas
func (s *Server) coleccion(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		lista := make([]Tarea, 0, len(s.tareas))
		for _, t := range s.tareas {
			lista = append(lista, t)
		}
		s.mu.Unlock()

		// El map no tiene orden: ordenamos para que la API sea estable.
		sort.Slice(lista, func(i, j int) bool { return lista[i].ID < lista[j].ID })
		responderJSON(w, http.StatusOK, lista)

	case http.MethodPost:
		// Un body de usuario es input no confiable: límite de tamaño y sin campos raros.
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var entrada struct {
			Titulo string `json:"titulo"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&entrada); err != nil {
			responderError(w, http.StatusBadRequest, "json inválido: "+err.Error())
			return
		}
		entrada.Titulo = strings.TrimSpace(entrada.Titulo)
		if entrada.Titulo == "" {
			responderError(w, http.StatusUnprocessableEntity, "titulo es obligatorio")
			return
		}

		s.mu.Lock()
		t := Tarea{ID: s.nextID, Titulo: entrada.Titulo}
		s.tareas[t.ID] = t
		s.nextID++
		s.mu.Unlock()

		w.Header().Set("Location", fmt.Sprintf("/tareas/%d", t.ID))
		responderJSON(w, http.StatusCreated, t)

	default:
		w.Header().Set("Allow", "GET, POST")
		responderError(w, http.StatusMethodNotAllowed, "método no permitido")
	}
}

// GET /tareas/{id}  |  PUT /tareas/{id}  |  DELETE /tareas/{id}
func (s *Server) individual(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/tareas/"))
	if err != nil || id <= 0 {
		responderError(w, http.StatusBadRequest, "id inválido")
		return
	}

	s.mu.Lock()
	t, existe := s.tareas[id]
	s.mu.Unlock()

	switch r.Method {
	case http.MethodGet:
		if !existe {
			responderError(w, http.StatusNotFound, "tarea no encontrada")
			return
		}
		responderJSON(w, http.StatusOK, t)

	case http.MethodPut:
		if !existe {
			responderError(w, http.StatusNotFound, "tarea no encontrada")
			return
		}
		var entrada struct {
			Titulo string `json:"titulo"`
			Hecha  bool   `json:"hecha"`
		}
		if err := json.NewDecoder(r.Body).Decode(&entrada); err != nil {
			responderError(w, http.StatusBadRequest, "json inválido")
			return
		}
		entrada.Titulo = strings.TrimSpace(entrada.Titulo)
		if entrada.Titulo == "" {
			responderError(w, http.StatusUnprocessableEntity, "titulo es obligatorio")
			return
		}
		s.mu.Lock()
		s.tareas[id] = Tarea{ID: id, Titulo: entrada.Titulo, Hecha: entrada.Hecha}
		actualizada := s.tareas[id]
		s.mu.Unlock()
		responderJSON(w, http.StatusOK, actualizada)

	case http.MethodDelete:
		if !existe {
			responderError(w, http.StatusNotFound, "tarea no encontrada")
			return
		}
		s.mu.Lock()
		delete(s.tareas, id)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		responderError(w, http.StatusMethodNotAllowed, "método no permitido")
	}
}

func responderJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("codificar respuesta: %v", err)
	}
}

func responderError(w http.ResponseWriter, status int, msg string) {
	responderJSON(w, status, map[string]string{"error": msg})
}

// statusRecorder captura el código para poder loguearlo.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func conLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(inicio).Round(time.Millisecond))
	})
}

func main() {
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           NuevoServer().Handler(),
		ReadHeaderTimeout: 5 * time.Second, // evita clientes que nunca mandan headers
	}
	log.Println("escuchando en http://localhost:8080")
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
