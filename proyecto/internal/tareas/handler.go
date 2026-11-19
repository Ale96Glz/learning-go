package tareas

import (
	"errors"
	"net/http"
	"strconv"

	"tutorialgo/proyecto/internal/httpx"
)

type Handler struct {
	repo *Repo
}

func NuevoHandler(repo *Repo) *Handler { return &Handler{repo: repo} }

// Registrar deja que main decida qué middleware protege cada ruta.
func (h *Handler) Registrar(mux *http.ServeMux, proteger func(http.Handler) http.Handler) {
	mux.Handle("GET /tareas", proteger(http.HandlerFunc(h.listar)))
	mux.Handle("POST /tareas", proteger(http.HandlerFunc(h.crear)))
	mux.Handle("GET /tareas/{id}", proteger(http.HandlerFunc(h.obtener)))
	mux.Handle("PUT /tareas/{id}", proteger(http.HandlerFunc(h.actualizar)))
	mux.Handle("DELETE /tareas/{id}", proteger(http.HandlerFunc(h.borrar)))
}

func (h *Handler) listar(w http.ResponseWriter, r *http.Request) {
	tareas, err := h.repo.Listar(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "error interno")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tareas": tareas, "total": len(tareas)})
}

func (h *Handler) crear(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Titulo string `json:"titulo"`
	}
	if err := httpx.Decodificar(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "json inválido")
		return
	}
	t, err := h.repo.Crear(r.Context(), in.Titulo)
	switch {
	case errors.Is(err, ErrTituloInvalido):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, "error interno")
	default:
		httpx.JSON(w, http.StatusCreated, t)
	}
}

func (h *Handler) obtener(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	t, err := h.repo.Buscar(r.Context(), id)
	switch {
	case errors.Is(err, ErrNoExiste):
		httpx.Error(w, http.StatusNotFound, "tarea no encontrada")
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, "error interno")
	default:
		httpx.JSON(w, http.StatusOK, t)
	}
}

func (h *Handler) actualizar(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in struct {
		Titulo string `json:"titulo"`
		Hecha  bool   `json:"hecha"`
	}
	if err := httpx.Decodificar(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "json inválido")
		return
	}
	t, err := h.repo.Actualizar(r.Context(), id, in.Titulo, in.Hecha)
	switch {
	case errors.Is(err, ErrTituloInvalido):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrNoExiste):
		httpx.Error(w, http.StatusNotFound, "tarea no encontrada")
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, "error interno")
	default:
		httpx.JSON(w, http.StatusOK, t)
	}
}

func (h *Handler) borrar(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	err := h.repo.Borrar(r.Context(), id)
	switch {
	case errors.Is(err, ErrNoExiste):
		httpx.Error(w, http.StatusNotFound, "tarea no encontrada")
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, "error interno")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusBadRequest, "id inválido")
		return 0, false
	}
	return id, true
}
