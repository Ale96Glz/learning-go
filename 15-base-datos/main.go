package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	// Import en blanco: registra el driver "sqlite" en database/sql.
	_ "modernc.org/sqlite"
)

var ErrNoExiste = errors.New("tarea no existe")

type Tarea struct {
	ID     int64
	Titulo string
	Hecha  bool
	Creada time.Time
}

type Repo struct {
	db *sql.DB
}

func Abrir(ruta string) (*Repo, error) {
	db, err := sql.Open("sqlite", ruta)
	if err != nil {
		return nil, fmt.Errorf("abrir sqlite: %w", err)
	}
	// SQLite permite un solo escritor. Limitar el pool evita "database is locked".
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Repo{db: db}, nil
}

func (r *Repo) Cerrar() error { return r.db.Close() }

// Migrar aplica el esquema. En un proyecto grande se usa una herramienta de
// migraciones (goose, golang-migrate); acá alcanza con IF NOT EXISTS.
func (r *Repo) Migrar(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS tareas (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	titulo     TEXT    NOT NULL,
	hecha      INTEGER NOT NULL DEFAULT 0,
	creada_en  TEXT    NOT NULL
);`
	if _, err := r.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrar: %w", err)
	}
	return nil
}

func (r *Repo) Crear(ctx context.Context, titulo string) (Tarea, error) {
	ahora := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO tareas (titulo, creada_en) VALUES (?, ?)`,
		titulo, ahora.Format(time.RFC3339Nano))
	if err != nil {
		return Tarea{}, fmt.Errorf("insertar tarea: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Tarea{}, fmt.Errorf("last insert id: %w", err)
	}
	return r.Buscar(ctx, id)
}

func (r *Repo) Buscar(ctx context.Context, id int64) (Tarea, error) {
	var (
		t      Tarea
		hecha  int
		creada string
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT id, titulo, hecha, creada_en FROM tareas WHERE id = ?`, id).
		Scan(&t.ID, &t.Titulo, &hecha, &creada)
	if errors.Is(err, sql.ErrNoRows) {
		return Tarea{}, ErrNoExiste
	}
	if err != nil {
		return Tarea{}, fmt.Errorf("buscar tarea %d: %w", id, err)
	}
	t.Hecha = hecha == 1
	t.Creada, err = time.Parse(time.RFC3339Nano, creada)
	if err != nil {
		return Tarea{}, fmt.Errorf("fecha inválida: %w", err)
	}
	return t, nil
}

func (r *Repo) Listar(ctx context.Context) ([]Tarea, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, titulo, hecha, creada_en FROM tareas ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("listar: %w", err)
	}
	defer rows.Close() // SIEMPRE: libera la conexión

	var tareas []Tarea
	for rows.Next() {
		var (
			t      Tarea
			hecha  int
			creada string
		)
		if err := rows.Scan(&t.ID, &t.Titulo, &hecha, &creada); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		t.Hecha = hecha == 1
		t.Creada, _ = time.Parse(time.RFC3339Nano, creada)
		tareas = append(tareas, t)
	}
	// rows.Err() detecta errores cortados a mitad de la iteración.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterar: %w", err)
	}
	return tareas, nil
}

func (r *Repo) Actualizar(ctx context.Context, id int64, titulo string, hecha bool) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE tareas SET titulo = ?, hecha = ? WHERE id = ?`,
		titulo, boolAInt(hecha), id)
	if err != nil {
		return fmt.Errorf("actualizar: %w", err)
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

func (r *Repo) Borrar(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tareas WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("borrar: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNoExiste
	}
	return nil
}

// CrearVarias es atómica: o se insertan todas o ninguna.
func (r *Repo) CrearVarias(ctx context.Context, titulos []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// Rollback después del Commit es no-op: el defer es seguro.
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO tareas (titulo, creada_en) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	ahora := time.Now().UTC().Format(time.RFC3339Nano)
	for _, titulo := range titulos {
		if _, err := stmt.ExecContext(ctx, titulo, ahora); err != nil {
			return fmt.Errorf("insertar %q: %w", titulo, err)
		}
	}
	return tx.Commit()
}

func boolAInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func main() {
	ruta := "tareas.db"
	_ = os.Remove(ruta) // ejemplo repetible

	repo, err := Abrir(ruta)
	if err != nil {
		log.Fatal(err)
	}
	defer repo.Cerrar()

	// Timeout global del ejemplo: ningún query puede pasar de 3s.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := repo.Migrar(ctx); err != nil {
		log.Fatal(err)
	}

	t1, err := repo.Crear(ctx, "Aprender database/sql")
	if err != nil {
		log.Fatal(err)
	}
	t2, err := repo.Crear(ctx, "Escribir el proyecto final")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("creadas: %d %q | %d %q\n", t1.ID, t1.Titulo, t2.ID, t2.Titulo)

	if err := repo.Actualizar(ctx, t1.ID, t1.Titulo, true); err != nil {
		log.Fatal(err)
	}

	tareas, err := repo.Listar(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("\nlistado:")
	for _, t := range tareas {
		fmt.Printf("  #%d %-30s hecha=%-5t creada=%s\n",
			t.ID, t.Titulo, t.Hecha, t.Creada.Format(time.RFC3339))
	}

	if _, err := repo.Buscar(ctx, 999); errors.Is(err, ErrNoExiste) {
		fmt.Println("\nid 999:", err)
	}

	// Transacción: las tres o ninguna.
	if err := repo.CrearVarias(ctx, []string{"una", "dos", "tres"}); err != nil {
		log.Fatal(err)
	}
	if err := repo.Borrar(ctx, t2.ID); err != nil {
		log.Fatal(err)
	}

	// Placeholders: SQLite usa ?, Postgres usa $1. Nunca concatenes strings:
	// es la puerta de entrada a SQL injection.
	fmt.Println("\nOK: revisa tareas.db con cualquier visor de SQLite")
}
