package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func nuevoRepoTest(t *testing.T) *Repo {
	t.Helper()
	// Cada test tiene su propia DB: sin estado compartido entre tests.
	ruta := filepath.Join(t.TempDir(), "test.db")
	repo, err := Abrir(ruta)
	if err != nil {
		t.Fatalf("abrir: %v", err)
	}
	t.Cleanup(func() { repo.Cerrar() })

	if err := repo.Migrar(context.Background()); err != nil {
		t.Fatalf("migrar: %v", err)
	}
	return repo
}

func TestCRUD(t *testing.T) {
	repo := nuevoRepoTest(t)
	ctx := context.Background()

	t1, err := repo.Crear(ctx, "primera")
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if t1.ID == 0 || t1.Titulo != "primera" || t1.Hecha {
		t.Fatalf("tarea inesperada: %+v", t1)
	}

	if err := repo.Actualizar(ctx, t1.ID, "primera editada", true); err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	got, err := repo.Buscar(ctx, t1.ID)
	if err != nil {
		t.Fatalf("buscar: %v", err)
	}
	if got.Titulo != "primera editada" || !got.Hecha {
		t.Fatalf("no se actualizó: %+v", got)
	}

	if err := repo.Borrar(ctx, t1.ID); err != nil {
		t.Fatalf("borrar: %v", err)
	}
	if _, err := repo.Buscar(ctx, t1.ID); !errors.Is(err, ErrNoExiste) {
		t.Fatalf("después de borrar: %v, esperaba ErrNoExiste", err)
	}
}

func TestListarOrdenado(t *testing.T) {
	repo := nuevoRepoTest(t)
	ctx := context.Background()

	for _, titulo := range []string{"c", "a", "b"} {
		if _, err := repo.Crear(ctx, titulo); err != nil {
			t.Fatal(err)
		}
	}
	tareas, err := repo.Listar(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tareas) != 3 {
		t.Fatalf("esperaba 3, hay %d", len(tareas))
	}
	for i := 1; i < len(tareas); i++ {
		if tareas[i-1].ID > tareas[i].ID {
			t.Fatalf("listado desordenado: %+v", tareas)
		}
	}
}

func TestCrearVariasEsAtomico(t *testing.T) {
	repo := nuevoRepoTest(t)
	ctx := context.Background()

	if err := repo.CrearVarias(ctx, []string{"x", "y"}); err != nil {
		t.Fatal(err)
	}
	tareas, err := repo.Listar(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tareas) != 2 {
		t.Fatalf("esperaba 2, hay %d", len(tareas))
	}
}

func TestErroresEnIDsInexistentes(t *testing.T) {
	repo := nuevoRepoTest(t)
	ctx := context.Background()

	if err := repo.Actualizar(ctx, 42, "x", false); !errors.Is(err, ErrNoExiste) {
		t.Errorf("actualizar inexistente: %v", err)
	}
	if err := repo.Borrar(ctx, 42); !errors.Is(err, ErrNoExiste) {
		t.Errorf("borrar inexistente: %v", err)
	}
}
