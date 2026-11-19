package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRutasConPatrones(t *testing.T) {
	h := NuevaAPI().Rutas()

	// POST /api/v1/tareas
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tareas", strings.NewReader(`{"titulo":"revisar PR"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("crear = %d; body=%s", rec.Code, rec.Body)
	}
	var creada Tarea
	if err := json.Unmarshal(rec.Body.Bytes(), &creada); err != nil {
		t.Fatal(err)
	}

	// GET /api/v1/tareas/{id}: PathValue llega al handler después del StripPrefix.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tareas/1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("obtener = %d; body=%s", rec.Code, rec.Body)
	}

	// ID inexistente
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tareas/999", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("inexistente = %d, esperaba 404", rec.Code)
	}

	// DELETE
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/tareas/1", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("borrar = %d, esperaba 204", rec.Code)
	}
}

func TestMetodoNoPermitido(t *testing.T) {
	h := NuevaAPI().Rutas()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/tareas", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PATCH = %d, esperaba 405", rec.Code)
	}
}

func TestRequestID(t *testing.T) {
	h := NuevaAPI().Rutas()

	// Sin header: se genera uno.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/salud", nil))
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("esperaba un X-Request-ID generado")
	}

	// Con header: se respeta el del cliente.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/salud", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-ID"); got != "abc-123" {
		t.Fatalf("X-Request-ID = %q, esperaba abc-123", got)
	}
}

func TestRecuperacionDePanic(t *testing.T) {
	h := NuevaAPI().Rutas()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/panic", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic = %d, esperaba 500", rec.Code)
	}

	// El servidor sigue vivo después del panic.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/salud", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("después del panic = %d, esperaba 200", rec.Code)
	}
}
