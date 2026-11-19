package tareas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func nuevoMux(t *testing.T) http.Handler {
	t.Helper()
	repo, err := Abrir(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("abrir: %v", err)
	}
	t.Cleanup(func() { repo.Cerrar() })
	if err := repo.Migrar(context.Background()); err != nil {
		t.Fatalf("migrar: %v", err)
	}

	mux := http.NewServeMux()
	sinAuth := func(h http.Handler) http.Handler { return h }
	NuevoHandler(repo).Registrar(mux, sinAuth)
	return mux
}

func pedir(t *testing.T, h http.Handler, metodo, ruta, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestFlujoCRUD(t *testing.T) {
	h := nuevoMux(t)

	rec := pedir(t, h, http.MethodPost, "/tareas", `{"titulo":"  desplegar  "}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("crear = %d; body=%s", rec.Code, rec.Body)
	}
	var creada Tarea
	if err := json.Unmarshal(rec.Body.Bytes(), &creada); err != nil {
		t.Fatal(err)
	}
	if creada.Titulo != "desplegar" {
		t.Errorf("titulo = %q, esperaba normalizado", creada.Titulo)
	}

	rec = pedir(t, h, http.MethodGet, "/tareas", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("listar = %d", rec.Code)
	}
	var lista struct {
		Tareas []Tarea `json:"tareas"`
		Total  int     `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &lista); err != nil {
		t.Fatal(err)
	}
	if lista.Total != 1 || len(lista.Tareas) != 1 {
		t.Fatalf("lista inesperada: %+v", lista)
	}

	rutaID := "/tareas/1"
	rec = pedir(t, h, http.MethodPut, rutaID, `{"titulo":"desplegar","hecha":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("actualizar = %d; body=%s", rec.Code, rec.Body)
	}
	var actualizada Tarea
	json.Unmarshal(rec.Body.Bytes(), &actualizada)
	if !actualizada.Hecha {
		t.Error("hecha debería ser true")
	}

	rec = pedir(t, h, http.MethodDelete, rutaID, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("borrar = %d", rec.Code)
	}
	rec = pedir(t, h, http.MethodGet, rutaID, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("después de borrar = %d, esperaba 404", rec.Code)
	}
}

func TestErrores(t *testing.T) {
	h := nuevoMux(t)

	casos := []struct {
		nombre string
		metodo string
		ruta   string
		body   string
		status int
	}{
		{"titulo vacío", http.MethodPost, "/tareas", `{"titulo":"   "}`, http.StatusUnprocessableEntity},
		{"json roto", http.MethodPost, "/tareas", `{`, http.StatusBadRequest},
		{"campo desconocido", http.MethodPost, "/tareas", `{"titulo":"x","otro":1}`, http.StatusBadRequest},
		{"id inválido", http.MethodGet, "/tareas/abc", "", http.StatusBadRequest},
		{"no existe", http.MethodGet, "/tareas/77", "", http.StatusNotFound},
		{"actualizar no existe", http.MethodPut, "/tareas/77", `{"titulo":"x"}`, http.StatusNotFound},
		{"borrar no existe", http.MethodDelete, "/tareas/77", "", http.StatusNotFound},
		{"método no permitido", http.MethodPatch, "/tareas", "", http.StatusMethodNotAllowed},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			rec := pedir(t, h, c.metodo, c.ruta, c.body)
			if rec.Code != c.status {
				t.Errorf("%s %s = %d, esperaba %d; body=%s",
					c.metodo, c.ruta, rec.Code, c.status, rec.Body)
			}
		})
	}
}

func TestTituloLargo(t *testing.T) {
	h := nuevoMux(t)
	largo := strings.Repeat("a", 201)
	rec := pedir(t, h, http.MethodPost, "/tareas", `{"titulo":"`+largo+`"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("titulo largo = %d, esperaba 422", rec.Code)
	}
}
