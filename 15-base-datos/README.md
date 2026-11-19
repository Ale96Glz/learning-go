# 15 — Base de datos

**Qué vas a construir:** un repositorio SQLite con `database/sql`: crear, leer, listar,
actualizar, borrar y una transacción. Es el capítulo que convierte una API en memoria en algo
que sobrevive a un reinicio.

**Qué necesitas antes:** capítulos 01 a 14.

---

## Paso 0 — `database/sql` no es un ORM

`database/sql` es una capa fina sobre los drivers de bases de datos. Vos escribís SQL y
mapeás filas a structs a mano. No hay `@Entity`, ni lazy loading, ni migraciones mágicas.

Comparado con Java (JDBC/Hibernate) o C# (ADO.NET/Entity Framework):

- Es más parecido a **JDBC/ADO.NET con SQL a mano** que a un ORM.
- El driver se registra con un import en blanco y se selecciona por nombre en `sql.Open`.
- No hay entidades gestionadas ni sesión: hay un **pool de conexiones** (`*sql.DB`) que se
  comparte entre goroutines.

¿Por qué no un ORM? Porque el SQL es el contrato con la base, y en Go la comunidad prefiere
verlo. Si querés uno, hay opciones (GORM, ent), pero no son necesarias para CRUD.

## Paso 1 — Abrir: el pool, no una conexión

```go
import (
	"database/sql"
	_ "modernc.org/sqlite"
)

db, err := sql.Open("sqlite", ruta)
```

- **`_ "modernc.org/sqlite"`** es un **import en blanco**: no usás ningún nombre del paquete,
  pero su `init()` registra el driver `"sqlite"` en `database/sql`. Sin este import,
  `sql.Open("sqlite", ...)` falla con "unknown driver".
- **`modernc.org/sqlite`** es SQLite puro en Go: no necesita CGO, así que compila en Windows
  sin instalar gcc.
- **`sql.Open` no conecta**: valida el DSN y prepara el pool. La conexión real ocurre en el
  primer uso o en `Ping`.
- **`db` es un pool**, no una conexión. Es seguro compartirlo entre goroutines.

```go
db.SetMaxOpenConns(1)
db.SetMaxIdleConns(1)
db.SetConnMaxLifetime(0)
```

- **SQLite permite un solo escritor.** Si dejás el pool abierto, dos escrituras concurrentes
  compiten y aparece `database is locked` (SQLITE_BUSY). Limitar a 1 serializa el acceso y
  evita el error. El costo es que las lecturas también esperan; para este tamaño, no importa.
- En **Postgres** usarías `SetMaxOpenConns(10..25)`, no 1.
- **`SetConnMaxLifetime(0)`** significa "sin límite". Para Postgres conviene un valor (por
  ejemplo 30 min) para rotar conexiones detrás de un balanceador.

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := db.PingContext(ctx); err != nil {
	db.Close()
	return nil, fmt.Errorf("ping: %w", err)
}
```

- **`PingContext`** es la única forma de saber al arrancar que la base responde. Sin esto, el
  servicio arranca "sano" y falla en el primer request.
- Si el ping falla, se cierra el pool antes de devolver el error. No dejes recursos abiertos en
  el camino de error.
- El timeout de 5 s evita que el arranque se cuelgue si la base no está.

## Paso 2 — Migrar el esquema

```go
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
```

- **`CREATE TABLE IF NOT EXISTS`** es idempotente: se puede correr en cada arranque.
- **`INTEGER PRIMARY KEY AUTOINCREMENT`** en SQLite es un entero que se autoincrementa y es la
  clave primaria.
- **`hecha INTEGER`** porque SQLite no tiene `bool`: se guarda 0 o 1.
- **`creada_en TEXT`** porque SQLite no tiene un tipo fecha nativo. Se guarda ISO 8601
  (`2026-11-15T05:45:39.123Z`) y se parsea en Go.
- **`NOT NULL`** donde corresponde. La base también valida; no delegues todo al código.
- **`ExecContext`** ejecuta SQL que no devuelve filas. El `_` descarta el `sql.Result`.

En un proyecto real, las migraciones se versionan y se aplican en el deploy con
`goose`, `golang-migrate` o similar. Acá `IF NOT EXISTS` alcanza para el concepto.

## Paso 3 — Crear: insertar y releer

```go
func (r *Repo) Crear(ctx context.Context, titulo string) (Tarea, error) {
	ahora := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO tareas (titulo, creada_en) VALUES (?, ?)`,
		titulo, ahora.Format(time.RFC3339Nano))
	if err != nil {
		return Tarea{}, fmt.Errorf("insertar tarea: %w", err)
	}
	id, err := res.LastInsertId()
	...
	return r.Buscar(ctx, id)
}
```

- **`?`** son **placeholders**. El driver escapa los valores; no se concatenan strings.
  Concatenar es la puerta de entrada a **SQL injection**. Aunque el título venga de tu propia
  API, siempre placeholders.
- **`Format(time.RFC3339Nano)`** convierte la fecha a texto ISO 8601 con nanosegundos. Es
  ordenable lexicográficamente y parseable de vuelta.
- **`res.LastInsertId()`** devuelve el ID generado. Es de SQLite/MySQL; en Postgres se usa
  `INSERT ... RETURNING id` y `QueryRow().Scan`.
- **`return r.Buscar(ctx, id)`** relee la fila completa. Es un viaje extra, pero garantiza que
  la respuesta sea exactamente lo que quedó guardado (incluida la fecha que puso la base, si
  la pusiera ella).

## Paso 4 — Buscar: `sql.ErrNoRows` no es un 500

```go
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
```

- **`QueryRowContext`** devuelve una fila. **`Scan`** copia las columnas a las variables, en
  orden. La cantidad de destinos debe coincidir con la cantidad de columnas.
- **`hecha` es `int`, no `bool`**: se escanea como entero y después se convierte.
- **`creada` es `string`**: se escanea como texto y se parsea.
- **`sql.ErrNoRows`** es el error de "no encontré filas". Se traduce a `ErrNoExiste`, un error
  de dominio. El handler lo convierte en 404. Si lo dejaras pasar como error genérico,
  terminarías devolviendo 500 por algo normal.
- **`fmt.Errorf("buscar tarea %d: %w", id, err)`** para los errores reales, con el ID en el
  mensaje.

```go
t.Hecha = hecha == 1
t.Creada, err = time.Parse(time.RFC3339Nano, creada)
if err != nil {
	return Tarea{}, fmt.Errorf("fecha inválida: %w", err)
}
```

Esta es la parte de mapeo manual: la base devuelve tipos primitivos y vos construís el dominio.
`time.Parse` convierte el texto a `time.Time`; si el dato está corrupto, se devuelve error.

## Paso 5 — Listar: cerrar y revisar

```go
rows, err := r.db.QueryContext(ctx, `SELECT ... ORDER BY id`)
if err != nil {
	return nil, fmt.Errorf("listar: %w", err)
}
defer rows.Close()
```

- **`QueryContext`** devuelve `*sql.Rows`, un cursor.
- **`defer rows.Close()`** es obligatorio: libera la conexión al pool. Sin esto, después de
  unas cuantas consultas el pool se agota y el servicio se cuelga. `Close` es idempotente y
  seguro después de que `Next` terminó.

```go
for rows.Next() {
	...
	if err := rows.Scan(&t.ID, &t.Titulo, &hecha, &creada); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	...
}
if err := rows.Err(); err != nil {
	return nil, fmt.Errorf("iterar: %w", err)
}
```

- **`rows.Next()`** devuelve `false` al terminar **o** al fallar. Por eso hay que revisar
  **`rows.Err()`** después del loop: si no, un error a mitad de la iteración pasa como "lista
  vacía". Es el mismo patrón que `scanner.Err()` del capítulo 06.
- Acá `t.Creada, _ = time.Parse(...)` **ignora el error**, a diferencia de `Buscar`. Es una
  inconsistencia: si una fecha está corrupta, la tarea se lista con fecha cero. El ejercicio
  te pide corregirla.
- **`ORDER BY id`** es lo que hace estable la respuesta. Sin `ORDER BY`, SQLite puede devolver
  cualquier orden.

## Paso 6 — Actualizar y borrar: `RowsAffected`

```go
res, err := r.db.ExecContext(ctx,
	`UPDATE tareas SET titulo = ?, hecha = ? WHERE id = ?`,
	titulo, boolAInt(hecha), id)
...
n, err := res.RowsAffected()
if n == 0 {
	return ErrNoExiste
}
```

`UPDATE` y `DELETE` **no fallan si el ID no existe**: afectan 0 filas y devuelven éxito. Para
distinguir "no existe" de "se actualizó", se mira `RowsAffected`. Es el patrón estándar.

`boolAInt` traduce `true/false` a `1/0` para la columna `INTEGER`.

En `Borrar`, el ejemplo hace `n, _ := res.RowsAffected()` e ignora el error de `RowsAffected`.
También es una inconsistencia; en código real se chequea.

## Paso 7 — Transacciones: todo o nada

```go
tx, err := r.db.BeginTx(ctx, nil)
if err != nil {
	return err
}
defer tx.Rollback() // no-op después del Commit
```

- **`BeginTx`** abre una transacción. El segundo argumento son opciones (`*sql.TxOptions`), por
  ejemplo aislamiento o solo lectura.
- **`defer tx.Rollback()`** es la red de seguridad: si hay un `return` temprano, un error o un
  panic, la transacción se cancela y no queda bloqueando filas. Después de un `Commit`
  exitoso, `Rollback` no hace nada.

```go
stmt, err := tx.PrepareContext(ctx, `INSERT INTO tareas (titulo, creada_en) VALUES (?, ?)`)
if err != nil {
	return err
}
defer stmt.Close()

for _, titulo := range titulos {
	if _, err := stmt.ExecContext(ctx, titulo, ahora); err != nil {
		return fmt.Errorf("insertar %q: %w", titulo, err)
	}
}
return tx.Commit()
```

- **`PrepareContext`** compila la sentencia una vez; el loop la ejecuta N veces. Menos parsing,
  más rápido.
- **`defer stmt.Close()`** libera la sentencia preparada.
- **`%q`** en el error muestra el título entre comillas, así ves cuál falló.
- **`tx.Commit()`** confirma. Si falla, se devuelve el error y el `defer Rollback` corre (no
  hace nada si el commit ya se aplicó; si no, deshace).

O se insertan todas las filas o ninguna. Eso es **atomicidad**: la A de ACID. Si el proceso
muere a mitad del loop, la base queda como estaba.

## Paso 8 — `main`

```go
ruta := "tareas.db"
_ = os.Remove(ruta)
repo, err := Abrir(ruta)
...
defer repo.Cerrar()

ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()
```

- Se borra la base al inicio para que el ejemplo sea repetible. En producción **jamás**.
- **`defer repo.Cerrar()`** cierra el pool al salir.
- **`ctx` con 3 s** para todo el ejemplo: ningún query puede pasar de ahí. En un servidor
  real, el context viene del request y se propaga a cada consulta.

El flujo es: migrar, crear dos tareas, actualizar la primera, listar, buscar un ID inexistente,
crear tres en una transacción y borrar la segunda.

```text
creadas: 1 "Aprender database/sql" | 2 "Escribir el proyecto final"

listado:
  #1 Aprender database/sql          hecha=true  creada=2026-11-15T05:45:39Z
  #2 Escribir el proyecto final     hecha=false creada=2026-11-15T05:45:39Z

id 999: tarea no existe

OK: revisa tareas.db con cualquier visor de SQLite
```

- **`%d %q`**: el ID como número, el título entre comillas.
- **`%-30s`** alinea el título a la izquierda en 30; **`%-5t`** alinea el booleano en 5 para que
  la columna no baile; **`%s`** con una fecha ya formateada a segundos (el archivo guarda
  nanosegundos).
- `creada` se imprime con `Format(time.RFC3339)`, no con `%v`, para controlar el formato.

## Paso 9 — Placeholders: SQLite vs Postgres

```go
`INSERT INTO tareas (titulo, creada_en) VALUES (?, ?)`      // SQLite, MySQL
`INSERT INTO tareas (titulo, creada_en) VALUES ($1, $2)`    // Postgres
```

El resto del código cambia poco: driver (`pgx` o `lib/pq`), DSN y placeholders.
`database/sql` es el mismo. Eso es lo que permite migrar sin reescribir el dominio.

**Nunca** armes SQL concatenando:

```go
`SELECT * FROM tareas WHERE titulo = '` + titulo + `'` // SQL injection
```

## Correr

```bash
go run ./15-base-datos
```

Deja `tareas.db` en la **carpeta desde donde ejecutaste** (la raíz del módulo). Abrilo con
DB Browser for SQLite o `sqlite3 tareas.db ".tables"`.

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| `unknown driver "sqlite"` | Falta el import en blanco | `_ "modernc.org/sqlite"` |
| `database is locked` | Varias conexiones escribiendo | `SetMaxOpenConns(1)` |
| `sql: database is closed` | Usaste el pool después de `Close` | Revisá el ciclo de vida |
| `sql: no rows in result set` llega como 500 | No tradujiste `ErrNoRows` | Devolvé `ErrNoExiste` |
| El pool se agota | Falta `rows.Close()` | `defer rows.Close()` |
| Errores a mitad de lista invisibles | Falta `rows.Err()` | Chequealo después del loop |
| `UPDATE` "exitoso" con ID inexistente | No miraste `RowsAffected` | Compará con 0 |
| Fechas corridas | Guardaste hora local | `.UTC()` al escribir |

## Por qué importa en el trabajo

- **`database/sql` es el denominador común**: lo usan los drivers de Postgres, MySQL, SQLite y
  casi todas las librerías.
- **El pool** es el recurso más fácil de agotar y el más difícil de diagnosticar.
- **Traducir `ErrNoRows` a un error de dominio** es lo que separa un 404 de un 500.
- **Las transacciones** son obligatorias en cualquier operación que toque más de una tabla.
- **Placeholders** son la defensa número uno contra SQL injection.

## Ejercicio

1. Corregí la inconsistencia de `Listar`: si `time.Parse` falla, devolvé el error. Hacé lo
   mismo con `RowsAffected` en `Borrar`.
2. Agregá `BuscarPorTitulo(ctx, texto)` con `WHERE titulo LIKE ?` y el `%` agregado en Go, no en
   el SQL. Probá con un título que tenga `%` adentro y explicá qué pasa.
3. Agregá `ActualizarVarias` que actualice varias tareas en una transacción y devuelva cuántas
   se actualizaron.
4. Cambiá el driver a Postgres (sin ejecutarlo): anotá qué líneas cambian (import, DSN,
   placeholders, `LastInsertId`). Ese es el costo real de migrar.

Siguiente: [16 — Auth](../16-auth/).
