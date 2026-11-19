package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// httptest.ServeHTTP ejecuta el handler sin abrir un puerto.
// Es el test de integración más barato que existe en Go.
func TestCRUDTareas(t *testing.T) {
	h := NuevoServer().Handler()

	// POST /tareas -> 201
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tareas", strings.NewReader(`{"titulo":"comprar café"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, esperaba 201; body=%s", rec.Code, rec.Body)
	}

	var creada Tarea
	if err := json.Unmarshal(rec.Body.Bytes(), &creada); err != nil {
		t.Fatal(err)
	}
	if creada.ID != 1 || creada.Titulo != "comprar café" {
		t.Fatalf("tarea creada inesperada: %+v", creada)
	}

	// GET /tareas -> lista con 1
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tareas", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}
	var lista []Tarea
	if err := json.Unmarshal(rec.Body.Bytes(), &lista); err != nil {
		t.Fatal(err)
	}
	if len(lista) != 1 {
		t.Fatalf("esperaba 1 tarea, hay %d", len(lista))
	}

	// GET /tareas/1 -> 200
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tareas/1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET por id status = %d", rec.Code)
	}

	// DELETE /tareas/1 -> 204 y luego 404
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/tareas/1", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tareas/1", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET borrada status = %d, esperaba 404", rec.Code)
	}
}

func TestErroresDeEntrada(t *testing.T) {
	h := NuevoServer().Handler()

	casos := []struct {
		nombre string
		metodo string
		ruta   string
		body   string
		status int
	}{
		{"json roto", http.MethodPost, "/tareas", `{`, http.StatusBadRequest},
		{"titulo vacío", http.MethodPost, "/tareas", `{"titulo":"   "}`, http.StatusUnprocessableEntity},
		{"campo desconocido", http.MethodPost, "/tareas", `{"titulo":"x","precio":3}`, http.StatusBadRequest},
		{"id no numérico", http.MethodGet, "/tareas/abc", "", http.StatusBadRequest},
		{"id inexistente", http.MethodGet, "/tareas/99", "", http.StatusNotFound},
		{"método no permitido", http.MethodPatch, "/tareas", "", http.StatusMethodNotAllowed},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var body *strings.Reader
			if c.body == "" {
				body = strings.NewReader("")
			} else {
				body = strings.NewReader(c.body)
			}
			req := httptest.NewRequest(c.metodo, c.ruta, body)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.status {
				t.Errorf("%s %s = %d, esperaba %d; body=%s", c.metodo, c.ruta, rec.Code, c.status, rec.Body)
			}
		})
	}
}
