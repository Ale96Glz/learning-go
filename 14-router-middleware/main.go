package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
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

type API struct {
	mu     sync.Mutex
	tareas map[int]Tarea
	nextID int
}

func NuevaAPI() *API {
	return &API{tareas: make(map[int]Tarea), nextID: 1}
}

// Rutas usa los patrones de ServeMux de Go 1.22+: método + path con {params}.
// Ya no hace falta un router de terceros para CRUD normal.
func (a *API) Rutas() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /tareas", a.listar)
	api.HandleFunc("POST /tareas", a.crear)
	api.HandleFunc("GET /tareas/{id}", a.obtener)
	api.HandleFunc("DELETE /tareas/{id}", a.borrar)

	// Versionado por prefijo: /api/v1/* se atiende con el mux interno.
	raiz := http.NewServeMux()
	raiz.Handle("/api/v1/", http.StripPrefix("/api/v1", api))
	raiz.HandleFunc("GET /salud", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	raiz.HandleFunc("GET /debug/panic", func(http.ResponseWriter, *http.Request) {
		panic("falla simulada") // solo demo: el middleware la convierte en 500
	})

	return Cadena(raiz, ConRequestID, ConLogging, ConRecuperacion)
}

func (a *API) listar(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	lista := make([]Tarea, 0, len(a.tareas))
	for _, t := range a.tareas {
		lista = append(lista, t)
	}
	a.mu.Unlock()
	responderJSON(w, http.StatusOK, lista)
}

func (a *API) crear(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var entrada struct {
		Titulo string `json:"titulo"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&entrada); err != nil {
		responderError(w, http.StatusBadRequest, "json inválido")
		return
	}
	entrada.Titulo = strings.TrimSpace(entrada.Titulo)
	if entrada.Titulo == "" {
		responderError(w, http.StatusUnprocessableEntity, "titulo es obligatorio")
		return
	}

	a.mu.Lock()
	t := Tarea{ID: a.nextID, Titulo: entrada.Titulo}
	a.tareas[t.ID] = t
	a.nextID++
	a.mu.Unlock()

	w.Header().Set("Location", fmt.Sprintf("/api/v1/tareas/%d", t.ID))
	responderJSON(w, http.StatusCreated, t)
}

func (a *API) obtener(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	a.mu.Lock()
	t, existe := a.tareas[id]
	a.mu.Unlock()
	if !existe {
		responderError(w, http.StatusNotFound, "tarea no encontrada")
		return
	}
	responderJSON(w, http.StatusOK, t)
}

func (a *API) borrar(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	a.mu.Lock()
	_, existe := a.tareas[id]
	delete(a.tareas, id)
	a.mu.Unlock()
	if !existe {
		responderError(w, http.StatusNotFound, "tarea no encontrada")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PathValue("id") es el parámetro capturado por el patrón /tareas/{id}.
func parseID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		responderError(w, http.StatusBadRequest, "id inválido")
		return 0, false
	}
	return id, true
}

// --- Middlewares ---

type Middleware func(http.Handler) http.Handler

// Cadena aplica en orden: Cadena(h, A, B) => A(B(h)).
func Cadena(h http.Handler, ms ...Middleware) http.Handler {
	for i := len(ms) - 1; i >= 0; i-- {
		h = ms[i](h)
	}
	return h
}

type contextoKey struct{}

func ConRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = nuevoID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), contextoKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ConLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		id, _ := r.Context().Value(contextoKey{}).(string)
		log.Printf("req=%s %s %s -> %d (%s)",
			id, r.Method, r.URL.Path, rec.status, time.Since(inicio).Round(time.Millisecond))
	})
}

func ConRecuperacion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recuperado: %v", rec)
				// Si el handler ya escribió headers, esto no cambia el status.
				responderError(w, http.StatusInternalServerError, "error interno")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func nuevoID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

// --- Respuestas ---

func responderJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func responderError(w http.ResponseWriter, status int, msg string) {
	responderJSON(w, status, map[string]string{"error": msg})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func main() {
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           NuevaAPI().Rutas(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Println("escuchando en http://localhost:8080/api/v1/tareas")
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
