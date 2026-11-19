package tareas

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Repo struct {
	db *sql.DB
}

func Abrir(ruta string) (*Repo, error) {
	db, err := sql.Open("sqlite", ruta)
	if err != nil {
		return nil, fmt.Errorf("abrir sqlite: %w", err)
	}
	// Un solo escritor en SQLite: evita "database is locked".
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Repo{db: db}, nil
}

func (r *Repo) Cerrar() error { return r.db.Close() }

// Listo es el chequeo de readiness.
func (r *Repo) Listo(ctx context.Context) error { return r.db.PingContext(ctx) }

func (r *Repo) Migrar(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS tareas (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	titulo    TEXT    NOT NULL,
	hecha     INTEGER NOT NULL DEFAULT 0,
	creada_en TEXT    NOT NULL
);`
	if _, err := r.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrar: %w", err)
	}
	return nil
}

func (r *Repo) Listar(ctx context.Context) ([]Tarea, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, titulo, hecha, creada_en FROM tareas ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("listar: %w", err)
	}
	defer rows.Close()

	tareas := make([]Tarea, 0)
	for rows.Next() {
		t, err := scanTarea(rows)
		if err != nil {
			return nil, err
		}
		tareas = append(tareas, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterar: %w", err)
	}
	return tareas, nil
}

func (r *Repo) Buscar(ctx context.Context, id int64) (Tarea, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, titulo, hecha, creada_en FROM tareas WHERE id = ?`, id)
	t, err := scanTarea(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Tarea{}, ErrNoExiste
	}
	return t, err
}

func (r *Repo) Crear(ctx context.Context, titulo string) (Tarea, error) {
	titulo, err := validarTitulo(titulo)
	if err != nil {
		return Tarea{}, err
	}
	ahora := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO tareas (titulo, creada_en) VALUES (?, ?)`,
		titulo, ahora.Format(time.RFC3339Nano))
	if err != nil {
		return Tarea{}, fmt.Errorf("insertar: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Tarea{}, fmt.Errorf("last insert id: %w", err)
	}
	return r.Buscar(ctx, id)
}

func (r *Repo) Actualizar(ctx context.Context, id int64, titulo string, hecha bool) (Tarea, error) {
	titulo, err := validarTitulo(titulo)
	if err != nil {
		return Tarea{}, err
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE tareas SET titulo = ?, hecha = ? WHERE id = ?`,
		titulo, hecha, id)
	if err != nil {
		return Tarea{}, fmt.Errorf("actualizar: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Tarea{}, err
	}
	if n == 0 {
		return Tarea{}, ErrNoExiste
	}
	return r.Buscar(ctx, id)
}

func (r *Repo) Borrar(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tareas WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("borrar: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNoExiste
	}
	return nil
}

// scanTarea sirve para *sql.Row y para *sql.Rows: ambos tienen Scan.
type scanner interface{ Scan(dest ...any) error }

func scanTarea(s scanner) (Tarea, error) {
	var (
		t      Tarea
		hecha  int // SQLite no tiene bool: guarda 0/1
		creada string
	)
	if err := s.Scan(&t.ID, &t.Titulo, &hecha, &creada); err != nil {
		return Tarea{}, err
	}
	t.Hecha = hecha == 1
	t.Creada, _ = time.Parse(time.RFC3339Nano, creada)
	return t, nil
}
