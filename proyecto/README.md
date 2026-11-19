# Proyecto final — API de tareas

**Qué vas a construir:** el servicio completo que junta los 18 capítulos. Routing con
parámetros, middlewares, auth con bcrypt y tokens Bearer, SQLite con `database/sql`,
configuración por entorno, logs estructurados, apagado ordenado y tests sin abrir puertos.

**Qué necesitas antes:** haber leído los capítulos 01 a 18. Acá no se enseña un tema nuevo:
se ve cómo se combinan, y qué decisiones hay detrás de cada archivo.

---

## Paso 0 — El mapa

```text
proyecto/
├── main.go                     # cablea todo; no tiene lógica de negocio
├── Dockerfile
└── internal/
    ├── config/config.go        # env + defaults + validación
    ├── httpx/httpx.go          # JSON, errores, request ID, logging, recover
    ├── auth/auth.go            # bcrypt, tokens, middleware
    └── tareas/
        ├── modelo.go           # entidad + validación
        ├── repo.go             # SQL
        ├── handler.go          # HTTP
        └── handler_test.go
```

La regla que hace mantenible esto es una sola: **las dependencias apuntan hacia adentro**.

```text
main ──> config
main ──> httpx
main ──> auth ──> httpx
main ──> tareas ──> httpx
```

`repo.go` no importa `net/http`: no sabe que existe un request. `handler.go` no importa
`auth`: no sabe que existe un token. `main` es el único que conoce a todos. Por eso podés
cambiar SQLite por Postgres tocando un archivo, y testear los handlers sin montar auth.

En Java/C# esa separación se logra con capas (`@Repository`, `@Service`, `@Controller`) y el
contenedor las inyecta. En Go las capas son paquetes y la inyección es pasar un puntero en el
constructor. El compilador es el que hace cumplir la regla: si `repo` importa `net/http`, se ve
en el `import`, no en un XML.

### Correr

```bash
# desde la raíz "tutorial go"
go run ./proyecto
```

Crea `tareas.db` en el directorio desde donde corrés (la raíz del módulo, no `proyecto/`).
Variables:

| Variable | Default | Ejemplo |
|---|---|---|
| `PORT` | `8080` | `3000` |
| `DB_RUTA` | `tareas.db` | `./datos/tareas.db` |
| `APP_ENV` | `dev` | `prod` |
| `TIMEOUT` | `10s` | `30s` |

## Paso 1 — `internal/config/config.go`

```go
type Config struct {
	Puerto  int
	DBRuta  string
	Entorno string
	Timeout time.Duration
}
```

Los valores llegan como string desde `os.Getenv` y salen **ya convertidos**. El resto del
programa usa `cfg.Timeout` sin volver a parsear. Es la misma idea del capítulo 17, recortada a
cuatro campos.

```go
cfg := Config{
	Puerto:  8080,
	DBRuta:  "tareas.db",
	Entorno: "dev",
	Timeout: 10 * time.Second,
}
var errs []error
```

- **Defaults primero.** El servicio arranca sin configurar nada. El entorno solo pisa lo que
  existe.
- **`10 * time.Second`**: `time.Duration` es un `int64` de nanosegundos; multiplicar por la
  constante lo hace legible.
- **`errs []error`**: se acumulan los errores en vez de cortar en el primero, para que un
  deploy mal configurado se arregle en una pasada (capítulo 17).

```go
if v := os.Getenv("PORT"); v != "" {
	p, err := strconv.Atoi(v)
	if err != nil || p < 1 || p > 65535 {
		errs = append(errs, fmt.Errorf("PORT inválido: %q", v))
	} else {
		cfg.Puerto = p
	}
}
```

- **`os.Getenv` devuelve `""`** si la variable no existe. Para config, "no existe" y "vacía"
  son el mismo caso: usá el default.
- **`strconv.Atoi`** convierte y devuelve error; no hay coerción implícita entre string e `int`.
- **`p < 1 || p > 65535`** valida el rango de puerto TCP además de que sea número. `"0"`,
  `"70000"` y `"abc"` fallan por el mismo camino.
- **`%q`** muestra `"abc"` entre comillas: deja ver un espacio o un carácter invisible que un
  `%s` esconde.

```go
switch v := os.Getenv("APP_ENV"); v {
case "", "dev":
	cfg.Entorno = "dev"
case "staging", "prod":
	cfg.Entorno = v
default:
	errs = append(errs, fmt.Errorf("APP_ENV inválido: %q (usa dev, staging o prod)", v))
}
```

- **Allowlist, no fallback.** `"production"` es error, no "prod" silencioso. Si el código
  compara `== "prod"` y el deploy escribe otra cosa, querés enterarte al arrancar.
- **Acá no hay `ToLower`/`TrimSpace`**, a diferencia del capítulo 17. `APP_ENV=PROD` falla. Es
  una inconsistencia real del proyecto; el ejercicio 1 la arregla.

```go
if v := os.Getenv("TIMEOUT"); v != "" {
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		errs = append(errs, fmt.Errorf("TIMEOUT inválido: %q (ej: 10s)", v))
	} else {
		cfg.Timeout = d
	}
}
return cfg, errors.Join(errs...)
```

- **`time.ParseDuration("45s")`** acepta `300ms`, `1m30s`, `2h`. `"10"` sin unidad falla.
- **`d <= 0`** rechaza negativos y cero. Un timeout negativo hace que todo venza antes de
  empezar.
- **`errors.Join(errs...)`** con slice vacío devuelve `nil`: ese es el camino feliz, sin
  `if len(errs) > 0`. Con errores, imprime uno por línea.

Contrato: **si `err != nil`, `cfg` no sirve** aunque venga a medio llenar. `main` lo respeta.

## Paso 2 — `httpx`: respuestas y entradas

```go
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("codificar respuesta", "error", err)
	}
}
```

- **El header va antes de `WriteHeader`.** Después ya se enviaron los headers y el
  `Content-Type` no cambia. Es el error clásico de "el JSON sale como `text/plain`".
- **`WriteHeader(status)`** manda el status; si nunca lo llamás, Go asume 200.
- **`json.NewEncoder(w).Encode(v)`** serializa directo al socket, sin buffer intermedio. Si
  falla a mitad de escritura ya no podés cambiar el status: solo loguear.
- **`any`** es `interface{}`. Acepta un struct, un map o un slice.

```go
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}
```

Todos los errores de la API tienen la misma forma: `{"error":"..."}`. Un cliente escribe un
solo parser. El mensaje es para el que depura, no para el usuario final.

```go
func Decodificar(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
```

- **`MaxBytesReader`** corta el body a 1 MB (`1<<20`). Sin límite, un cliente manda 2 GB y te
  quedás sin memoria.
- **El `w` del parámetro** no se usa para escribir la respuesta: `MaxBytesReader` lo necesita
  para marcar que la conexión se cierre después de responder. Verificado con un body de 2 MB:
  llega `400 {"error":"json inválido"}` con `Connection: close`. O sea: el límite funciona, pero
  el status es 400 y no el 413 que correspondería.
- **`DisallowUnknownFields`** convierte `{"titulo":"x","admin":true}` en error. El capítulo 13
  lo introduce; acá es obligatorio para todas las rutas.
- **`Decode(v)`** lee **un** valor JSON del stream y lo deja en el puntero. Un body con dos
  objetos pegados no da error: se procesa el primero y se ignora el resto.
- **Un body vacío es error.** `curl -X POST` sin `-d` devuelve 400, no un struct con ceros.

## Paso 3 — `httpx`: la cadena de middlewares

```go
type Middleware func(http.Handler) http.Handler

// Cadena(h, A, B) => A(B(h)).
func Cadena(h http.Handler, ms ...Middleware) http.Handler {
	for i := len(ms) - 1; i >= 0; i-- {
		h = ms[i](h)
	}
	return h
}
```

- **Un middleware es una función que envuelve un handler y devuelve otro.** Nada más. En
  Java es un `Filter`; en ASP.NET, un middleware en el pipeline. Acá no hay interfaz que
  implementar ni registro global.
- **Recorre al revés** para que el primero de la lista quede **afuera**. `Cadena(h, A, B)` da
  `A(B(h))`: el request entra por A, después B, después el handler; la respuesta sale al revés.
- **`ms ...Middleware`** es variádica: cero o más. Con cero devuelve `h` sin tocar.

### `RequestID`

```go
type claveID struct{}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			b := make([]byte, 8)
			if _, err := rand.Read(b); err != nil {
				id = strconv.FormatInt(time.Now().UnixNano(), 16)
			} else {
				id = hex.EncodeToString(b)
			}
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claveID{}, id)))
	})
}
```

- **Si el cliente manda `X-Request-ID`, se respeta.** Así se correlaciona un request que
  atraviesa varios servicios: el borde genera el ID y los demás lo arrastran.
- **`rand.Read` llena 8 bytes** = 16 caracteres hex. No es un secreto, es un identificador:
  `crypto/rand` acá es más de lo necesario, pero es la función que ya está importada.
- **El fallback con `UnixNano`** solo cubre el caso raro de que falle la entropía. Dos
  requests en el mismo nanosegundo colisionarían; es aceptable para un ID de log.
- **`claveID{}`** es un tipo propio, no un string. Si dos paquetes usan `"id"` se pisan
  (capítulo 12). Un tipo vacío no ocupa memoria y es imposible de colisionar desde afuera.
- **`r.WithContext`** devuelve una copia del request; el original no se muta.

### `Logging`

```go
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
```

- **`http.ResponseWriter` no tiene getter de status.** Para loguearlo hay que interceptar
  `WriteHeader`, y para eso se envuelve el writer.
- **El embed** (`http.ResponseWriter` sin nombre) delega automáticamente `Header()` y
  `Write()`. Solo se sobreescribe lo que hace falta. En Java sería una clase que implementa la
  interfaz y delega a mano.
- **Se guarda antes de delegar.** Si el writer original hiciera panic, el dato ya está.

```go
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"request_id", ID(r.Context()),
			"metodo", r.Method,
			"ruta", r.URL.Path,
			"status", rec.status,
			"duracion_ms", time.Since(inicio).Milliseconds(),
		)
	})
}
```

- **`status: http.StatusOK`** es el default correcto: si el handler nunca llama `WriteHeader`,
  la respuesta es 200 y el log no debe mentir con 0.
- **El log se emite después** de `next.ServeHTTP`, por eso conoce el status final y la
  duración. Es lo que te deja responder "¿qué requests dieron 500 en la última hora?".
- **`status` y `duracion_ms` son números.** En JSON se consultan con `status >= 500`; si fueran
  texto, no.
- **`ID(r.Context())`** lee lo que puso `RequestID`. Por eso `RequestID` está **afuera** de
  `Logging`: el context ya tiene el ID cuando el logger lo necesita.

### `Recuperacion`

```go
func Recuperacion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recuperado", "request_id", ID(r.Context()), "panic", rec)
				Error(w, http.StatusInternalServerError, "error interno")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
```

- **`defer` + `recover`** es el único `try/catch` que existe en Go. Sin este `defer`, el panic
  sube a `net/http`, que corta la conexión y loguea el stack (capítulo 14).
- **`rec`** puede ser un string, un error o cualquier cosa; `slog` lo serializa.
- **El 500 solo funciona si todavía no se escribieron headers.** Si el handler ya mandó un 200
  y después hace panic, `WriteHeader(500)` se ignora y el cliente recibe un body truncado. No
  hay forma de arreglarlo desde acá: la respuesta ya salió.

### El orden final

```go
return httpx.Cadena(raiz, httpx.RequestID, httpx.Logging, httpx.Recuperacion)
```

Es `RequestID(Logging(Recuperacion(raiz)))`:

```text
request  ->  RequestID  ->  Logging  ->  Recuperacion  ->  handler
response <-  RequestID  <-  Logging  <-  Recuperacion  <-  handler
```

- **`RequestID` primero** porque todos los demás lo usan para loguear.
- **`Recuperacion` adentro de `Logging`** para que el 500 que escribe quede registrado con su
  status real. Si estuviera afuera, el logger nunca vería ese request.
- **`Recuperacion` fuera del handler** es lo que captura el panic. Cualquier cosa que esté más
  adentro queda protegida.

## Paso 4 — `tareas/modelo.go`

```go
var (
	ErrNoExiste       = errors.New("tarea no existe")
	ErrTituloInvalido = errors.New("titulo inválido")
)

type Tarea struct {
	ID     int64     `json:"id"`
	Titulo string    `json:"titulo"`
	Hecha  bool      `json:"hecha"`
	Creada time.Time `json:"creada_en"`
}
```

- **Dos centinelas** que el handler traduce a 404 y 422 con `errors.Is`. Ninguna capa compara
  strings de error.
- **Los tags JSON son el contrato con el cliente.** Sin tags, `encoding/json` usa el nombre del
  campo (`Creada`) y el cliente recibe otra cosa. En Java/C# el nombre del campo importa poco
  porque hay anotaciones o configuración de serialización; en Go el tag es la única fuente.
- **`time.Time`** se serializa como RFC 3339 con nanosegundos
  (`"2026-11-15T05:55:37.1408674Z"`). Es lo que espera cualquier cliente.

```go
func validarTitulo(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%w: no puede estar vacío", ErrTituloInvalido)
	}
	if len([]rune(s)) > 200 {
		return "", fmt.Errorf("%w: máximo 200 caracteres", ErrTituloInvalido)
	}
	return s, nil
}
```

- **`TrimSpace`** normaliza. `"  preparar el deploy  "` se guarda como `"preparar el deploy"`;
  el test lo verifica.
- **`%w` envuelve el centinela**: `errors.Is(err, ErrTituloInvalido)` sigue siendo verdadero, y
  el mensaje conserva el detalle. Ese detalle es el que el handler le manda al cliente.
- **`len([]rune(s))` cuenta caracteres, no bytes.** `"añ"` son 3 bytes y 2 caracteres. Usar
  `len(s)` rechazaría títulos válidos con acentos. El costo es una copia a un slice; para 200
  caracteres, irrelevante. (`utf8.RuneCountInString(s)` hace lo mismo sin alocar.)
- **Se devuelve el título normalizado**, no el original. El caller tiene que usar el valor
  devuelto.

## Paso 5 — `tareas/repo.go`

```go
type Repo struct {
	db *sql.DB
}
```

`*sql.DB` **no es una conexión**: es un pool. Se crea una vez y se comparte entre goroutines;
es seguro para uso concurrente. En Java sería el `DataSource`; en .NET, el pool detrás de
`SqlConnection`.

```go
func Abrir(ruta string) (*Repo, error) {
	db, err := sql.Open("sqlite", ruta)
	if err != nil {
		return nil, fmt.Errorf("abrir sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	...
}
```

- **`sql.Open` no conecta.** Solo valida el driver y el DSN. La conexión real ocurre en el
  primer query o en el `Ping`.
- **`"sqlite"`** es el nombre que registra `modernc.org/sqlite` con el import `_`. Sin ese
  import en blanco, el driver no existe y `sql.Open` falla con `unknown driver`.
- **`MaxOpenConns(1)`** fuerza un solo escritor. SQLite no soporta escrituras concurrentes:
  dos conexiones escribiendo dan `database is locked`. El costo es que también serializa las
  lecturas. Para este servicio, correcto; para carga alta, un pool real o Postgres.
- **`MaxIdleConns(1)`** mantiene esa conexión abierta en vez de cerrarla y reabrirla en cada
  request.
- **`fmt.Errorf("...: %w", err)`** agrega contexto sin perder el error original. `errors.Is`
  sigue funcionando.

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := db.PingContext(ctx); err != nil {
	db.Close()
	return nil, fmt.Errorf("ping: %w", err)
}
```

- **`PingContext` es la primera conexión real.** Si la ruta es inválida o el archivo está
  bloqueado, falla acá y no en el primer request.
- **Si falla, `db.Close()`** para no dejar el pool abierto.
- **`cancel()` va en `defer`** y no suelto: si el ping falla con `return`, igual se libera el
  timer. Crear un context con timeout y no cancelarlo es una fuga.

```go
func (r *Repo) Cerrar() error { return r.db.Close() }

// Listo es el chequeo de readiness.
func (r *Repo) Listo(ctx context.Context) error { return r.db.PingContext(ctx) }
```

`Listo` es el readiness de `/listo`: un ping barato que responde "puedo hablar con la base".
No es lo mismo que `/salud`, que solo dice "el proceso está vivo" (capítulo 18).

```go
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
```

- **`IF NOT EXISTS`** hace la migración idempotente: correrla dos veces no rompe. Es una
  migración de un solo paso, no versionada. Con cambios de esquema necesitás `goose` o
  `golang-migrate` (ejercicio 5).
- **`INTEGER PRIMARY KEY AUTOINCREMENT`**: en SQLite, `INTEGER PRIMARY KEY` ya es el rowid y se
  autoincrementa. `AUTOINCREMENT` además garantiza que un ID borrado no se reutilice.
- **`hecha INTEGER`**: SQLite no tiene `BOOLEAN`. Guarda 0/1 y el escaneo lo traduce.
- **`creada_en TEXT`**: se guarda como RFC 3339. Se podría guardar como entero Unix; texto es
  legible en cualquier visor de SQLite y ordena bien porque el formato es fijo.
- **`ExecContext`** no devuelve filas; `QueryContext` sí (`QueryRowContext` para una).

### `Listar`

```go
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
```

- **`defer rows.Close()`** es obligatorio. Un `*sql.Rows` abierto retiene la conexión; con
  `MaxOpenConns(1)`, olvidarlo cuelga el servicio entero en el siguiente request.
- **`make([]Tarea, 0)` y no `var tareas []Tarea`.** Un slice nil se serializa como `null`; uno
  vacío, como `[]`. El cliente espera un array siempre. Es la diferencia entre
  `{"tareas":null,"total":0}` y `{"tareas":[],"total":0}`.
- **`rows.Next()`** avanza y devuelve false al final. **`rows.Err()`** después del loop es el
  error de iteración (conexión cortada, escaneo fallido). Si no lo chequeás, un error a mitad
  de camino devuelve una lista incompleta como si fuera válida.
- **`ctx`** viene del request. Si el cliente se desconecta, la query se cancela y no seguís
  gastando CPU para nadie.

### `Buscar`

```go
func (r *Repo) Buscar(ctx context.Context, id int64) (Tarea, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, titulo, hecha, creada_en FROM tareas WHERE id = ?`, id)
	t, err := scanTarea(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Tarea{}, ErrNoExiste
	}
	return t, err
}
```

- **`?` es el placeholder de SQLite** (y de MySQL). Postgres usa `$1`. Nunca concatenes el ID
  en el SQL: eso es inyección SQL. El driver manda el valor aparte del texto de la query.
- **`QueryRowContext` devuelve `*sql.Row`**, que difiere el error al `Scan`.
- **`sql.ErrNoRows` se traduce a `ErrNoExiste`** en el borde del repo. Hacia arriba no sale un
  error de base: sale un error de dominio. Así el handler no importa `database/sql`.
- **`Tarea{}`** es el zero value, el "no hay nada" idiomático.

### `Crear`

```go
func (r *Repo) Crear(ctx context.Context, titulo string) (Tarea, error) {
	titulo, err := validarTitulo(titulo)
	if err != nil {
		return Tarea{}, err
	}
	ahora := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO tareas (titulo, creada_en) VALUES (?, ?)`,
		titulo, ahora.Format(time.RFC3339Nano))
	...
	id, err := res.LastInsertId()
	...
	return r.Buscar(ctx, id)
}
```

- **La validación vive en el repo, no en el handler.** Así cualquier caller (un CLI, un job)
  pasa por la misma regla. El handler igual devuelve 422 porque reconoce el centinela.
- **`time.Now().UTC()`** guarda en UTC. La zona local se aplica al mostrar, no al persistir.
  Con dos servidores en zonas distintas, UTC es lo único comparable.
- **`res.LastInsertId()`** es específico de SQLite/MySQL. Postgres necesita `RETURNING id` con
  `QueryRow`. Es la diferencia más común al cambiar de motor.
- **Se vuelve a leer la fila** en vez de armar el struct a mano. Así la respuesta incluye el
  `id` y la fecha exactamente como quedaron en la base, no como creés que quedaron.
- **No hay transacción** porque es un solo `INSERT`. Si necesitaras insertar tarea + historial,
  ahí sí: `BeginTx`, `defer tx.Rollback()`, `tx.Commit()`.

### `Actualizar`

```go
res, err := r.db.ExecContext(ctx,
	`UPDATE tareas SET titulo = ?, hecha = ? WHERE id = ?`, titulo, hecha, id)
...
n, err := res.RowsAffected()
...
if n == 0 {
	return Tarea{}, ErrNoExiste
}
return r.Buscar(ctx, id)
```

- **`RowsAffected()` es cómo distinguís "actualicé" de "no existía".** Un `UPDATE` sobre un ID
  inexistente no es error para SQL: afecta cero filas.
- **Ojo con el matiz**: SQLite cuenta las filas que **matchean** el `WHERE`, aunque los valores
  sean idénticos. Por eso el mismo PUT dos veces devuelve 200 las dos veces, no 404.
- **`PUT` reemplaza todo**: `titulo` y `hecha` son obligatorios. Si el cliente manda solo
  `titulo`, `hecha` queda en `false`. Para actualización parcial hace falta `PATCH` con punteros
  (ejercicio 4).

### `Borrar`

```go
res, err := r.db.ExecContext(ctx, `DELETE FROM tareas WHERE id = ?`, id)
...
if n == 0 {
	return ErrNoExiste
}
return nil
```

Mismo patrón: cero filas afectadas es 404. El handler responde `204 No Content` y **no escribe
body**; un 204 con body es un error de protocolo.

### `scanTarea`

```go
// scanTarea sirve para *sql.Row y para *sql.Rows: ambos tienen Scan.
type scanner interface{ Scan(dest ...any) error }

func scanTarea(s scanner) (Tarea, error) {
	var (
		t      Tarea
		hecha  int
		creada string
	)
	if err := s.Scan(&t.ID, &t.Titulo, &hecha, &creada); err != nil {
		return Tarea{}, err
	}
	t.Hecha = hecha == 1
	t.Creada, _ = time.Parse(time.RFC3339Nano, creada)
	return t, nil
}
```

- **La interfaz `scanner`** tiene un solo método, y tanto `*sql.Row` como `*sql.Rows` la
  cumplen. Sin eso, necesitarías dos funciones idénticas. Es la interfaz chica del capítulo 06
  aplicada.
- **`&hecha` es `*int`** aunque la columna se llame booleana: SQLite devuelve 0/1. Escanear
  directo a `*bool` falla con `unsupported Scan`.
- **`hecha == 1`** convierte a bool. Cualquier otro valor (incluido 2) da `false`.
- **`t.Creada, _ = time.Parse(...)` ignora el error.** Si alguien escribe basura en esa columna,
  la tarea sale con fecha cero (`0001-01-01`) en vez de fallar. Es un atajo: la fecha la escribe
  este mismo programa, así que el caso es improbable; si la base se toca a mano, preferir
  devolver el error.

## Paso 6 — `tareas/handler.go`

```go
type Handler struct{ repo *Repo }

func NuevoHandler(repo *Repo) *Handler { return &Handler{repo: repo} }

func (h *Handler) Registrar(mux *http.ServeMux, proteger func(http.Handler) http.Handler) {
	mux.Handle("GET /tareas", proteger(http.HandlerFunc(h.listar)))
	mux.Handle("POST /tareas", proteger(http.HandlerFunc(h.crear)))
	mux.Handle("GET /tareas/{id}", proteger(http.HandlerFunc(h.obtener)))
	mux.Handle("PUT /tareas/{id}", proteger(http.HandlerFunc(h.actualizar)))
	mux.Handle("DELETE /tareas/{id}", proteger(http.HandlerFunc(h.borrar)))
}
```

- **`Registrar` recibe el middleware como parámetro.** El paquete `tareas` no importa `auth`;
  `main` decide qué protege cada ruta. En Spring sería `@PreAuthorize` o el `HttpSecurity`; acá
  es una función, y se puede testear pasando `func(h) h { return h }`.
- **`"GET /tareas/{id}"`** es el patrón de Go 1.22: método + path con comodín. El valor se lee
  con `r.PathValue("id")` (capítulo 14). Sin esto habría que parsear la URL a mano.
- **`mux.Handle`** pide un `http.Handler`. `http.HandlerFunc` convierte un método con la firma
  correcta en handler. Es el adapter de Go: no hay herencia.
- **El orden de registro no importa.** El mux elige el patrón más específico, no el primero que
  coincide.

```go
func (h *Handler) listar(w http.ResponseWriter, r *http.Request) {
	tareas, err := h.repo.Listar(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "error interno")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tareas": tareas, "total": len(tareas)})
}
```

- **`r.Context()`** se pasa al repo: si el cliente se va, la query se cancela.
- **El error real no va al cliente.** `err.Error()` puede tener rutas, SQL o nombres de tablas.
  Se loguea (o se pierde, en este ejemplo) y al cliente le llega `"error interno"`.
- **`map[string]any`** arma un objeto con dos claves. `total` ahorra que el cliente cuente el
  array. `any` porque los valores son de tipos distintos.

```go
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
```

- **Struct anónimo solo con lo que el cliente puede mandar.** Aunque `Tarea` tuviera `ID`, acá
  no se acepta: el servidor decide el ID. Aceptar el struct completo es cómo se regalan
  permisos.
- **`switch` sin expresión** evalúa los `case` en orden y entra al primero verdadero. Se lee
  mejor que tres `if` con `return`.
- **`errors.Is`** funciona porque el repo devuelve el centinela envuelto con `%w`.
- **422 con `err.Error()`** es la excepción a "no filtres detalles": el mensaje
  (`titulo inválido: no puede estar vacío`) es para el usuario, no un detalle interno. Si
  agregás un error de dominio nuevo, decidí explícitamente cuál se muestra y cuál no.
- **201 Created** al crear. El `Location` header no se manda; sería lo correcto en REST estricto.

```go
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
```

- **`ok bool`** en vez de devolver `(id, error)`: el helper ya escribió la respuesta de error, y
  el handler solo tiene que cortar. Es un patrón común para evitar que el handler decida dos
  veces el mismo status.
- **404 y 500 son distintos a propósito.** Un ID que no existe es culpa del cliente; una base
  caída es culpa tuya. Devolver 404 para todo esconde incidentes.

```go
func (h *Handler) actualizar(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in struct {
		Titulo string `json:"titulo"`
		Hecha  bool   `json:"hecha"`
	}
	...
}
```

- **Dos campos, sin punteros**: es un reemplazo completo. `{"titulo":"x"}` deja `hecha` en
  `false` aunque antes fuera `true`. Documentarlo en la API es obligatorio; el cliente que no
  lo sabe pierde datos.
- **El repo valida de nuevo** aunque el handler ya haya parseado. La validación no es del
  protocolo, es de la entidad.

```go
func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusBadRequest, "id inválido")
		return 0, false
	}
	return id, true
}
```

- **`r.PathValue("id")`** devuelve el comodín `{id}` del patrón. Es un string aunque en SQL sea
  un número.
- **`ParseInt(..., 10, 64)`**: base 10, 64 bits. Se usa `int64` porque es lo que devuelve
  `LastInsertId()` y el ancho natural de `INTEGER` en SQLite.
- **`id <= 0`**: los IDs empiezan en 1. Un `0` o un negativo no existen y merecen 400, no 404.
- **`/tareas/abc` da 400, no 404**: el recurso no está mal formado, el identificador sí.

## Paso 7 — `auth/auth.go`

El capítulo 16 explica bcrypt, el timing attack y las sesiones en detalle. Acá importan las
diferencias con ese ejemplo:

```go
type Servicio struct {
	mu       sync.Mutex
	porEmail map[string]*Usuario
	porID    map[int64]*Usuario
	sesiones map[string]sesion
	nextID   int64
	ttl      time.Duration
}

func Nuevo(ttl time.Duration) *Servicio { ... }
```

- **`ttl` es un parámetro**, no una constante: `main` pasa `24*time.Hour`. Así los tests pueden
  usar un TTL de un segundo sin tocar el código.
- **`sesion` es minúscula**: nadie fuera del paquete la necesita. En Go el exportado es la
  mayúscula; no hay `private` aparte.
- **`Usuario` tiene `Nombre`** y el `Hash` sigue con `json:"-"`. Ese tag es lo único que impide
  que el hash salga en `/auth/yo`.

```go
var hashFalso, _ = bcrypt.GenerateFromPassword([]byte("relleno-para-igualar-tiempos"), bcrypt.DefaultCost)
```

Se calcula **una vez al arrancar**. En el login, un email inexistente compara contra este hash
para tardar lo mismo que uno real. Sin eso, el tiempo de respuesta delata qué cuentas existen.

```go
func (s *Servicio) ConAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			httpx.Error(w, http.StatusUnauthorized, "falta el token Bearer")
			return
		}
		u, err := s.Validar(token)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "token inválido o expirado")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claveUsuario{}, u)))
	})
}
```

- **`CutPrefix`** devuelve el resto y un bool. No hace falta `Split` ni índices.
- **Dos mensajes distintos** pero el mismo 401: "falta el token" es un cliente que no lo mandó;
  "inválido o expirado" no distingue cuál es, a propósito.
- **El usuario viaja en el context**, con una clave de tipo propio. El handler de tareas no
  recibe el token: recibe el request y, si lo necesita, `auth.UsuarioDesde(r.Context())`.
- **`ConAuth` usa `httpx.Error`**, así el 401 tiene el mismo formato JSON que todo lo demás. Es
  la razón por la que `auth` importa `httpx`.

```go
func UsuarioDesde(ctx context.Context) *Usuario {
	u, _ := ctx.Value(claveUsuario{}).(*Usuario)
	return u
}
```

El `ok` se descarta y puede devolver `nil`. En `/auth/yo` la ruta está protegida, así que
nunca pasa; si alguien registra una ruta sin `ConAuth`, recibe `null` con 200 en vez de un
panic. Es más silencioso y más difícil de detectar: si agregás rutas, revisá que estén
envueltas.

**`Logout` está implementado y no se usa.** No hay ruta que lo llame. Es código muerto que
además sugiere una feature que no existe; el ejercicio 2 lo conecta.

## Paso 8 — `main.go`: el arranque

```go
func main() {
	cfg, err := config.Cargar()
	if err != nil {
		slog.Error("config inválida", "error", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
```

- **La config va primero.** Si falta una variable, no hay nada que arrancar.
- **`os.Exit(1)`** corta con código de fallo. Los `defer` pendientes **no** corren (capítulo
  01); acá no hay ninguno todavía.
- **Problema real: el logger se configura después.** El error de config sale por el handler
  por defecto, en **texto** y a stderr, no en JSON:

  ```text
  2026/11/14 23:56:24 ERROR config inválida error="PORT inválido: \"abc\""
  ```

  El resto de los logs sí es JSON. Un sistema que parsea logs va a tener un formato inesperado
  justo en el arranque fallido. Se arregla moviendo `slog.SetDefault` **antes** de
  `config.Cargar()`; es el ejercicio 3.

```go
	repo, err := tareas.Abrir(cfg.DBRuta)
	if err != nil {
		slog.Error("abrir base de datos", "error", err)
		os.Exit(1)
	}
	defer repo.Cerrar()
```

- **`defer repo.Cerrar()`** se ejecuta al salir de `main` por el camino normal. **No** se ejecuta
  si más abajo llamás `os.Exit`: ese es el precio de `os.Exit`. El sistema operativo cierra el
  descriptor igual, pero te quedás sin el cierre ordenado del pool. En el camino de error,
  preferí devolver el error hasta `main` y cerrar ahí.
- **La ruta de la base** viene de la config: `DB_RUTA` o el default.

```go
	migrarCtx, cancelar := context.WithTimeout(context.Background(), cfg.Timeout)
	err = repo.Migrar(migrarCtx)
	cancelar()
	if err != nil { ... }
```

`cancelar()` va explícito y no en `defer` porque el context se usa una sola vez y no querés
arrastrarlo hasta el final de `main`. Un timeout en la migración evita que el arranque se
cuelgue si el archivo está bloqueado por otro proceso.

```go
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Puerto),
		Handler:           construirRutas(repo, auth.Nuevo(24*time.Hour)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
```

- **`":8080"`** escucha en todas las interfaces. `"127.0.0.1:8080"` solo en localhost.
- **`fmt.Sprintf(":%d", cfg.Puerto)`** arma la dirección desde el puerto configurado.
- **Los cuatro timeouts** son la defensa contra clientes lentos (capítulo 18). Sin
  `ReadHeaderTimeout` el default es cero: sin límite.
- **`auth.Nuevo(24*time.Hour)`** crea el servicio acá y lo pasa a las rutas. Nadie más lo
  instancia: una sola fuente de usuarios y sesiones.

## Paso 9 — `main.go`: las rutas

```go
func construirRutas(repo *tareas.Repo, authSvc *auth.Servicio) http.Handler {
	api := http.NewServeMux()
	...
	raiz := http.NewServeMux()
	raiz.HandleFunc("GET /salud", ...)
	raiz.HandleFunc("GET /listo", ...)
	raiz.Handle("/api/v1/", http.StripPrefix("/api/v1", api))
	return httpx.Cadena(raiz, httpx.RequestID, httpx.Logging, httpx.Recuperacion)
}
```

- **Dos mux.** `raiz` atiende salud y el prefijo versionado; `api` tiene las rutas de negocio
  **sin** el `/api/v1`. Separarlos permite montar la API bajo otro prefijo sin tocar los
  handlers, y dejar `/salud` fuera del volumen versionado.
- **`http.StripPrefix("/api/v1", api)`** quita el prefijo antes de delegar. `api` ve
  `/auth/login`, no `/api/v1/auth/login`. Los patrones quedan cortos y estables.
- **`/api/v1/` con barra final** es un patrón de subárbol: matchea todo lo que empiece así. Sin
  la barra, solo matchearía exactamente `/api/v1`.
- **`raiz` no valida método para `/api/v1/`**, así que un `PATCH` llega al `api` y es el `api`
  el que responde 405. Un solo lugar decide qué métodos existen.
- **El orden de la cadena** ya se explicó: `RequestID(Logging(Recuperacion(raiz)))`.

```go
	api.HandleFunc("POST /auth/registro", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Nombre   string `json:"nombre"`
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := httpx.Decodificar(w, r, &in); err != nil {
			httpx.Error(w, http.StatusBadRequest, "json inválido")
			return
		}
		u, err := authSvc.Registrar(in.Nombre, in.Email, in.Password)
		switch {
		case errors.Is(err, auth.ErrDuplicado):
			httpx.Error(w, http.StatusConflict, "email ya registrado")
		case errors.Is(err, auth.ErrDatos):
			httpx.Error(w, http.StatusUnprocessableEntity, "nombre, email o contraseña (8-72) inválidos")
		case err != nil:
			httpx.Error(w, http.StatusInternalServerError, "error interno")
		default:
			httpx.JSON(w, http.StatusCreated, u)
		}
	})
```

- **El handler de auth vive en `main`**, no en el paquete `auth`. Es deliberado: `auth` expone
  el servicio y el middleware, no las rutas; `main` es el único que conoce el mux. La contra es
  que `main.go` crece; con diez rutas más, conviene un `auth.Handler` como el de `tareas`.
- **La traducción de errores es idéntica a la de tareas.** Tres `case` y un `default`:
  duplicado → 409, datos → 422, cualquier otro → 500, éxito → 201.
- **422 y no 400** para datos inválidos: el JSON estaba bien, los valores no.
- **`u` sale con `json:"-"` en el hash**, así que la respuesta es
  `{"id":1,"nombre":"Ana","email":"ana@example.com"}`.

```go
	api.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		...
		token, expira, err := authSvc.Login(in.Email, in.Password)
		if errors.Is(err, auth.ErrCredenciales) {
			httpx.Error(w, http.StatusUnauthorized, "credenciales inválidas")
			return
		}
		...
		httpx.JSON(w, http.StatusOK, map[string]any{
			"token":  token,
			"expira": expira.UTC().Format(time.RFC3339),
			"tipo":   "Bearer",
		})
	})
```

- **Mismo 401 para email inexistente y contraseña incorrecta.** No se enumera usuarios.
- **`expira` en UTC con RFC 3339** (`2026-11-16T05:55:37Z`), 24 horas adelante porque ese es el
  TTL que `main` le pasa a `auth.Nuevo`.
- **`map[string]any`** porque la respuesta es ad hoc. Las claves salen ordenadas
  alfabéticamente al serializar: `expira`, `tipo`, `token`.

```go
	api.Handle("GET /auth/yo", authSvc.ConAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, auth.UsuarioDesde(r.Context()))
	})))
```

- **`Handle` y no `HandleFunc`** porque `ConAuth` devuelve un `http.Handler`.
- **No hay lookup en la base**: el usuario vino en el context. El handler no sabe de tokens.
- **El hash sigue oculto** por el mismo tag del struct.

```go
	tareas.NuevoHandler(repo).Registrar(api, authSvc.ConAuth)
```

Una línea conecta el paquete `tareas` con el middleware de `auth` sin que ninguno de los dos
se conozca. Eso es inyección de dependencias sin framework.

```go
	raiz.HandleFunc("GET /listo", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := repo.Listo(ctx); err != nil {
			httpx.Error(w, http.StatusServiceUnavailable, "no listo")
			return
		}
		w.Write([]byte("listo"))
	})
```

- **`repo.Listo` hace un ping real.** Si la base no responde en 2 s, 503 y el balanceador deja
  de mandar tráfico, pero **no** reinicia el proceso. Eso es readiness, no liveness.
- **2 s es más corto que `cfg.Timeout`** a propósito: un readiness colgado es peor que un
  readiness que falla rápido.
- **`w.Write`** escribe texto plano; un probe no necesita JSON.

## Paso 10 — `main.go`: el apagado

```go
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("servidor iniciado", "puerto", cfg.Puerto, "entorno", cfg.Entorno, "db", cfg.DBRuta)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("servidor", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	stop()
```

- **`signal.NotifyContext`** devuelve un context que se cancela con Ctrl+C (`os.Interrupt`) o con
  SIGTERM (lo que manda Docker/Kubernetes). Es el reemplazo moderno de un channel de señales.
- **`ListenAndServe` en una goroutine** porque bloquea. `main` queda esperando la señal.
- **`http.ErrServerClosed` no es un error**: es lo que devuelve `ListenAndServe` cuando alguien
  llamó a `Shutdown`. Tratarlo como fallo haría que cada deploy saliera con error.
- **`stop()`** devuelve el manejo de señales al default: un segundo Ctrl+C mata el proceso de
  inmediato. Es la salida de emergencia si el apagado se cuelga.

**Bug real de este `main`:** si el puerto está ocupado, el `ListenAndServe` falla, se loguea el
error… y se llama `stop()`. `main` sigue, hace `Shutdown` (que no tiene nada que apagar),
loguea `"apagado completo"` y **sale con código 0**. Verificado:

```json
{"time":"2026-11-14T23:56:09.4473788-06:00","level":"INFO","msg":"servidor iniciado","puerto":8091,"entorno":"dev","db":"...\\a.db"}
{"time":"2026-11-14T23:56:09.4540101-06:00","level":"ERROR","msg":"servidor","error":"listen tcp :8091: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted."}
{"time":"2026-11-14T23:56:09.4540101-06:00","level":"INFO","msg":"apagando: esperando requests en curso"}
{"time":"2026-11-14T23:56:09.4545363-06:00","level":"INFO","msg":"apagado completo"}
```

Exit code: **0**. Un orquestador que solo mira el código de salida cree que el deploy salió
bien mientras el servicio no escucha. El capítulo 18 usa `os.Exit(1)` justamente por esto; acá
la corrección se perdió. Es el ejercicio 3. (En el bloque de arriba recorté el path de la base.)

```go
	slog.Info("apagando: esperando requests en curso")

	apagadoCtx, cancelar := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancelar()
	if err := srv.Shutdown(apagadoCtx); err != nil {
		slog.Error("shutdown forzado", "error", err)
		os.Exit(1)
	}
	slog.Info("apagado completo")
```

- **`Shutdown` deja de aceptar conexiones nuevas, cierra las ociosas y espera las que están en
  curso.** Es distinto de `Close`, que las corta.
- **`context.Background()` y no el `ctx` de la señal**: ese ya está cancelado, y `Shutdown` con
  un context cancelado devuelve inmediatamente sin esperar nada.
- **El techo es `cfg.Timeout` (10 s por default).** Si un handler se pasa, `Shutdown` devuelve
  `context deadline exceeded` y el proceso sale con 1. En Kubernetes,
  `terminationGracePeriodSeconds` tiene que ser mayor que este valor.
- **`os.Exit(1)` acá también saltea `defer repo.Cerrar()` y `defer cancelar()`.** En el camino
  de error, aceptable; en el camino feliz, los defers corren.

## La API

| Método | Ruta | Auth | Respuesta |
|---|---|---|---|
| GET | `/salud` | no | `200 ok` (texto) |
| GET | `/listo` | no | `200 listo` / `503` |
| POST | `/api/v1/auth/registro` | no | `201` usuario / `409` / `422` / `400` |
| POST | `/api/v1/auth/login` | no | `200` token / `401` |
| GET | `/api/v1/auth/yo` | sí | `200` usuario / `401` |
| GET | `/api/v1/tareas` | sí | `200 {"tareas":[...],"total":n}` |
| POST | `/api/v1/tareas` | sí | `201` tarea / `422` |
| GET | `/api/v1/tareas/{id}` | sí | `200` tarea / `404` / `400` |
| PUT | `/api/v1/tareas/{id}` | sí | `200` tarea / `404` / `400` / `422` |
| DELETE | `/api/v1/tareas/{id}` | sí | `204` / `404` / `400` |

Errores de la API: `{"error":"..."}`. Códigos: 400 JSON o id inválido, 401 sin token o token
inválido, 404 no existe, 409 email duplicado, 422 datos inválidos, 500 error interno.

**Excepción real:** los 404 y 405 que genera el `ServeMux` (ruta inexistente, método no
permitido) salen en **texto plano**, no en JSON. Verificado:

```text
GET /api/v1/nada      -> 404 "404 page not found"   (text/plain)
PATCH /api/v1/tareas  -> 405 "Method Not Allowed"   (text/plain, Allow: GET, HEAD, POST)
```

Un cliente que hace `json.Unmarshal` sobre el body de error explota. Si el contrato dice
"siempre JSON", hay que registrar un `NotFound` propio o envolver el mux. Es el ejercicio 6.

## Recorrido de un request

El camino completo, que es lo que te permite depurar sin adivinar:

```text
cliente
  -> :8080                        server (4 timeouts)
  -> RequestID                    genera o hereda X-Request-ID
  -> Logging                      arranca el cronómetro
  -> Recuperacion                 convierte un panic en 500
  -> raiz (ServeMux)              /salud, /listo o /api/v1/
  -> StripPrefix("/api/v1")       quita el prefijo
  -> api (ServeMux)               método + patrón, y si no hay, 405
  -> ConAuth                      valida Bearer y pone el usuario en context
  -> handler                      decodifica JSON, llama al repo, elige status
  -> repo                         SQL con el context del request
  -> httpx.JSON                   escribe status + body
  -> Logging                      registra status y duración
  -> cliente
```

Si el request falla, el status lo decide la capa más cercana que sepa traducirlo:

| Error de dominio | HTTP | Dónde se decide |
|---|---|---|
| JSON malformado o campo desconocido | 400 | handler |
| `/tareas/abc` | 400 | `parseID` |
| falta token / token inválido o vencido | 401 | `auth.ConAuth` |
| tarea inexistente | 404 | handler, desde `ErrNoExiste` |
| ruta inexistente | 404 | `ServeMux` (texto plano) |
| método no permitido | 405 | `ServeMux` (texto plano) |
| email duplicado | 409 | `main`, en registro |
| título vacío o de más de 200 caracteres | 422 | handler, desde `ErrTituloInvalido` |
| base caída, panic | 500 | handler / `Recuperacion` |

La regla es que **ninguna capa inventa un status**: cada una devuelve un error de dominio y la
de arriba traduce. `repo` no importa `net/http`; `ConAuth` no sabe qué es una tarea.

## Flujo real

Salida real del servidor con una base temporal (recorté `Date`, `Content-Length` y el valor
completo del token):

```text
GET /salud
HTTP/1.1 200 OK
X-Request-Id: 6a604c7aaaee3a62
ok

GET /listo
HTTP/1.1 200 OK
X-Request-Id: c4d06f592b2f37a2
listo

POST /api/v1/auth/registro  {"nombre":"Ana","email":"ana@example.com","password":"secreto123"}
HTTP/1.1 201 Created
X-Request-Id: ed858610543f64c7
{"id":1,"nombre":"Ana","email":"ana@example.com"}

POST /api/v1/auth/registro  (el mismo email otra vez)
HTTP/1.1 409 Conflict
{"error":"email ya registrado"}

POST /api/v1/auth/login  {"email":"ana@example.com","password":"secreto123"}
HTTP/1.1 200 OK
{"expira":"2026-11-16T05:55:37Z","tipo":"Bearer","token":"6bc86267910e93760ee4e70990f509cb5d681759328f05a45f29ac58fb46d3b3"}

GET /api/v1/auth/yo  Authorization: Bearer <token>
HTTP/1.1 200 OK
{"id":1,"nombre":"Ana","email":"ana@example.com"}

POST /api/v1/tareas  {"titulo":"  preparar el deploy  "}
HTTP/1.1 201 Created
{"id":1,"titulo":"preparar el deploy","hecha":false,"creada_en":"2026-11-15T05:55:37.1408674Z"}

GET /api/v1/tareas
HTTP/1.1 200 OK
{"tareas":[{"id":1,"titulo":"preparar el deploy","hecha":false,"creada_en":"2026-11-15T05:55:37.1408674Z"}],"total":1}

GET /api/v1/tareas/1
HTTP/1.1 200 OK
{"id":1,"titulo":"preparar el deploy","hecha":false,"creada_en":"2026-11-15T05:55:37.1408674Z"}

PUT /api/v1/tareas/1  {"titulo":"preparar el deploy","hecha":true}
HTTP/1.1 200 OK
{"id":1,"titulo":"preparar el deploy","hecha":true,"creada_en":"2026-11-15T05:55:37.1408674Z"}

DELETE /api/v1/tareas/1
HTTP/1.1 204 No Content

GET /api/v1/tareas/1
HTTP/1.1 404 Not Found
{"error":"tarea no encontrada"}

GET /api/v1/tareas  (sin header)
HTTP/1.1 401 Unauthorized
{"error":"falta el token Bearer"}

GET /api/v1/auth/yo  Authorization: Bearer basura
HTTP/1.1 401 Unauthorized
{"error":"token inválido o expirado"}

POST /api/v1/tareas  {"titulo":"   "}
HTTP/1.1 422 Unprocessable Entity
{"error":"titulo inválido: no puede estar vacío"}

POST /api/v1/tareas  {"titulo":"x","otro":1}
HTTP/1.1 400 Bad Request
{"error":"json inválido"}

GET /api/v1/tareas/abc
HTTP/1.1 400 Bad Request
{"error":"id inválido"}
```

Cuatro cosas para mirar:

- **El título se guardó sin los espacios** (`"  preparar el deploy  "` → `"preparar el deploy"`).
- **La respuesta de registro y de `/auth/yo` no tiene `hash`.** Lo garantiza el tag `json:"-"`
  del struct. No hay un test que lo cubra; el capítulo 16 muestra lo fácil que es escribir uno
  que pase siempre sin verificar nada.
- **`X-Request-Id`** aparece en todas las respuestas, incluso en las de error. Go canonicaliza
  el nombre: lo escribís `X-Request-ID` y viaja `X-Request-Id`.
- **El `creada_en` no cambia** entre el POST y el PUT: la actualización no toca esa columna.

En PowerShell, con el servidor corriendo:

```powershell
$reg = @{ nombre="Ana"; email="ana@example.com"; password="secreto123" } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri localhost:8080/api/v1/auth/registro -Body $reg -ContentType "application/json"

$login = @{ email="ana@example.com"; password="secreto123" } | ConvertTo-Json
$r = Invoke-RestMethod -Method Post -Uri localhost:8080/api/v1/auth/login -Body $login -ContentType "application/json"
$h = @{ Authorization = "Bearer $($r.token)" }

Invoke-RestMethod -Method Get -Uri localhost:8080/api/v1/tareas -Headers $h
Invoke-RestMethod -Method Post -Uri localhost:8080/api/v1/tareas -Headers $h `
  -Body (@{ titulo="preparar el deploy" } | ConvertTo-Json) -ContentType "application/json"
```

`Invoke-RestMethod` parsea el JSON solo y devuelve objetos. Para ver status y headers, usá
`curl.exe -i` como en los ejemplos de arriba.

## Los logs

Salida real, una línea JSON por request (recorté el path de la base y algunas líneas):

```json
{"time":"2026-11-14T23:55:35.9870297-06:00","level":"INFO","msg":"servidor iniciado","puerto":8090,"entorno":"dev","db":"...\\proy.db"}
{"time":"2026-11-14T23:55:36.8643877-06:00","level":"INFO","msg":"request","request_id":"6a604c7aaaee3a62","metodo":"GET","ruta":"/salud","status":200,"duracion_ms":0}
{"time":"2026-11-14T23:55:36.9451544-06:00","level":"INFO","msg":"request","request_id":"ed858610543f64c7","metodo":"POST","ruta":"/api/v1/auth/registro","status":201,"duracion_ms":40}
{"time":"2026-11-14T23:55:37.0011179-06:00","level":"INFO","msg":"request","request_id":"530a8e244b1e2d8e","metodo":"POST","ruta":"/api/v1/auth/registro","status":409,"duracion_ms":40}
{"time":"2026-11-14T23:55:37.0652165-06:00","level":"INFO","msg":"request","request_id":"9e75c8a1d6268143","metodo":"POST","ruta":"/api/v1/auth/login","status":200,"duracion_ms":39}
{"time":"2026-11-14T23:55:37.1429339-06:00","level":"INFO","msg":"request","request_id":"d1ffb5f50aaea1c9","metodo":"POST","ruta":"/api/v1/tareas","status":201,"duracion_ms":2}
{"time":"2026-11-14T23:55:37.1838984-06:00","level":"INFO","msg":"request","request_id":"6dfa0db329b1cc91","metodo":"GET","ruta":"/api/v1/tareas/1","status":200,"duracion_ms":0}
{"time":"2026-11-14T23:55:37.2259897-06:00","level":"INFO","msg":"request","request_id":"826d42a96772937f","metodo":"DELETE","ruta":"/api/v1/tareas/1","status":204,"duracion_ms":2}
{"time":"2026-11-14T23:55:37.2378511-06:00","level":"INFO","msg":"request","request_id":"91271796b9589274","metodo":"GET","ruta":"/api/v1/tareas/1","status":404,"duracion_ms":0}
{"time":"2026-11-14T23:55:37.2559598-06:00","level":"INFO","msg":"request","request_id":"9b0b8773b1ef33e9","metodo":"GET","ruta":"/api/v1/tareas","status":401,"duracion_ms":0}
{"time":"2026-11-14T23:55:37.3098586-06:00","level":"INFO","msg":"request","request_id":"5d2aeb59c0a1bfd9","metodo":"GET","ruta":"/api/v1/tareas/abc","status":400,"duracion_ms":0}
```

- **`duracion_ms: 40`** en registro y login es bcrypt trabajando. Los `0` son requests que solo
  tocan SQLite o nada.
- **`status` es un número**, así que `status >= 500` es un filtro y no una expresión regular.
- **`request_id`** conecta una línea de log con la respuesta HTTP que recibió el cliente y con
  cualquier otro log de esa misma operación.
- **La ruta es la original**, con `/api/v1`, no la que ve el `api` mux después del
  `StripPrefix`. El logger está afuera del strip: eso es lo que querés para buscar.

## Los tests

```bash
go test -v ./proyecto/...
```

```text
?   	tutorialgo/proyecto	[no test files]
?   	tutorialgo/proyecto/internal/auth	[no test files]
?   	tutorialgo/proyecto/internal/config	[no test files]
?   	tutorialgo/proyecto/internal/httpx	[no test files]
=== RUN   TestFlujoCRUD
--- PASS: TestFlujoCRUD (0.01s)
=== RUN   TestErrores
=== RUN   TestErrores/titulo_vacío
=== RUN   TestErrores/json_roto
=== RUN   TestErrores/campo_desconocido
=== RUN   TestErrores/id_inválido
=== RUN   TestErrores/no_existe
=== RUN   TestErrores/actualizar_no_existe
=== RUN   TestErrores/borrar_no_existe
=== RUN   TestErrores/método_no_permitido
--- PASS: TestErrores (0.00s)
    --- PASS: TestErrores/titulo_vacío (0.00s)
    --- PASS: TestErrores/json_roto (0.00s)
    --- PASS: TestErrores/campo_desconocido (0.00s)
    --- PASS: TestErrores/id_inválido (0.00s)
    --- PASS: TestErrores/no_existe (0.00s)
    --- PASS: TestErrores/actualizar_no_existe (0.00s)
    --- PASS: TestErrores/borrar_no_existe (0.00s)
    --- PASS: TestErrores/método_no_permitido (0.00s)
=== RUN   TestTituloLargo
--- PASS: TestTituloLargo (0.00s)
PASS
ok  	tutorialgo/proyecto/internal/tareas	0.038s
```

**Tres paquetes sin tests.** `auth`, `config` y `httpx` no tienen ninguno. La cobertura se ve
bien porque el camino principal pasa por `tareas`, pero bcrypt, la expiración de tokens y la
validación de config están sin red. Los capítulos 16 y 17 tienen esos tests; acá faltó
copiarlos. Es el ejercicio 7.

### Cómo está montado el test

```go
func nuevoMux(t *testing.T) http.Handler {
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
```

- **`t.TempDir()`** crea una carpeta única por test y la borra al terminar. Nunca tocás tu
  `tareas.db` real. Sin esto, los tests se pisan entre sí y con tu base de desarrollo.
- **`t.Cleanup`** registra el cierre; corre aunque el test falle. Es el `defer` de los tests,
  pero funciona entre helpers.
- **`sinAuth` es la identidad**: un middleware que devuelve el handler sin tocarlo. Así se
  testean los handlers sin bcrypt ni tokens. Es exactamente la razón por la que `Registrar`
  recibe `proteger` en vez de importar `auth`.
- **Migrar en el test** crea la tabla en la base temporal. El test no depende de un esquema
  externo.

```go
func pedir(t *testing.T, h http.Handler, metodo, ruta, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
```

- **`httptest.NewRequest`** arma un request sin abrir un socket. **`NewRecorder`** captura
  status, headers y body.
- **`h.ServeHTTP(rec, req)`** ejecuta la cadena completa: mux, middlewares y handler.
- **`t.Helper()`** hace que un fallo dentro de `pedir` se reporte en la línea del test que lo
  llamó, no adentro del helper.

### Qué prueba cada test

- **`TestFlujoCRUD`**: crear (y que el título venga normalizado), listar (`total` y `len`),
  actualizar (`hecha` en `true`), borrar (204) y volver a pedir (404). Es el ciclo de vida
  completo contra SQLite real.
- **`TestErrores`**: tabla de ocho casos. Cada uno es un status distinto y una rama distinta:
  422 por título vacío, 400 por JSON roto, 400 por campo desconocido, 400 por id no numérico,
  404 en los tres verbos, 405 por método no permitido.
- **`TestTituloLargo`**: 201 caracteres → 422. Verifica el límite por caracteres, no por bytes.

```bash
go test -race ./...
```

El `-race` compila con el detector de carreras. Acá no hay goroutines propias, pero el servidor
HTTP y los tests de otros capítulos sí las usan; correrlo sobre todo el módulo una vez antes de
un commit es barato.

## Lo que este proyecto hace bien

- **Una sola dirección de dependencias**, verificable mirando los `import`. No hay ciclos.
- **Errores de dominio traducidos en un solo lugar por capa.** El repo no sabe de HTTP; el
  handler no sabe de SQL; ninguno compara strings.
- **El middleware de auth se inyecta**, no se importa. Los tests corren sin bcrypt.
- **La base se configura por entorno**, y los tests usan `t.TempDir()`.
- **Los timeouts del servidor están puestos**, incluido el que evita Slowloris.
- **El apagado espera requests en curso**, con techo.

## Lo que está mal o simplificado (verificado)

1. **El error de config no es JSON.** `slog.SetDefault` va después de `config.Cargar()`, así que
   el arranque fallido loguea en texto. Mover el logger arriba.
2. **Puerto ocupado sale con exit 0.** La goroutine llama `stop()` en vez de `os.Exit(1)`. Un
   deploy roto parece exitoso.
3. **404 y 405 del mux son texto plano**, no `{"error":"..."}`. Rompe el contrato para clientes.
4. **Usuarios y sesiones viven en memoria; las tareas en disco.** Al reiniciar, las tareas
   siguen y los usuarios desaparecen: el login da 401 y no hay forma de recuperar la cuenta.
   Dos instancias no comparten ni usuarios ni sesiones.
5. **`auth.Logout` no está expuesto.** No hay forma de invalidar un token antes de que venza.
6. **El token se guarda en claro** en el map. Quien lea la memoria tiene sesiones válidas. Se
   guarda el SHA-256 del token y se compara el hash.
7. **`MaxOpenConns(1)` y queries anidadas.** Si dentro del loop de `Listar` llamás a otro método
   del repo, ese segundo query espera la única conexión que el `rows` sigue reteniendo: deadlock
   hasta que venza el context. Hoy no pasa, pero es una trampa a un query de distancia.
8. **`MaxBytesReader` devuelve 400, no 413.** El handler traduce cualquier error de decodificación
   a `"json inválido"`. Un body de 2 MB recibe un mensaje engañoso.
9. **`scanTarea` ignora el error de `time.Parse`.** Una fecha corrupta sale como año 1 sin
   avisar.
10. **`Recuperacion` no puede cambiar el status** si el handler ya escribió headers. Loguea el
    panic y sigue; el cliente recibe la respuesta cortada. No tiene arreglo desde un middleware.
11. **`APP_ENV` no se normaliza.** `PROD` o `" prod"` fallan, a diferencia del capítulo 17.
12. **Sin paginación, sin filtros, sin dueño.** `GET /tareas` devuelve todo, y todos los
    usuarios ven las tareas de todos.
13. **Sin rate limit en `/login`.** bcrypt encarece cada intento, pero un atacante con mil IPs
    igual prueba.
14. **Sin tests de `auth`, `config` ni `httpx`.**
15. **`PUT` reemplaza todo.** Un cliente que manda solo `titulo` apaga `hecha`.
16. **Migración no versionada.** `CREATE TABLE IF NOT EXISTS` no agrega columnas ni revierte.

Ninguna de estas es un bug de sintaxis: son decisiones y deudas. Saber cuáles son es parte de
"saber Go en producción".

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| `unknown driver "sqlite"` | falta el import en blanco | `_ "modernc.org/sqlite"` |
| `database is locked` | dos procesos o más de una conexión escribiendo | un proceso, `MaxOpenConns(1)` |
| `sql: database is closed` | usaste el repo después de `Cerrar` | revisá el orden en `main` |
| `unsupported Scan, storing driver.Value type int64 into type *bool` | escaneaste `INTEGER` a `bool` | escaneá a `int` y comparás |
| `http: superfluous response.WriteHeader` | dos `WriteHeader` | uno solo; revisá `Recuperacion` |
| el JSON sale `null` en vez de `[]` | slice nil | `make([]Tarea, 0)` |
| `duplicate column name` al migrar | migración sin `IF NOT EXISTS` | idempotencia o migraciones versionadas |
| el token no funciona después de reiniciar | sesiones en memoria | tabla de sesiones |
| el usuario existe pero el login da 401 tras reiniciar | usuarios en memoria | tabla de usuarios |
| `bind: Only one usage of each socket address` y exit 0 | puerto ocupado y `stop()` | matá el proceso viejo; corregí el exit code |
| `404 page not found` en texto plano | ruta inexistente | revisá el prefijo `/api/v1` |
| `405` en texto plano | método no registrado | usá el método de la tabla |
| `{"error":"id inválido"}` | `/tareas/abc` | el id va en número |
| `curl` manda el JSON vacío en Windows | comillas sin escapar | `--data-binary @archivo.json` o `Invoke-RestMethod` |
| acentos raros en la consola | codepage de Windows | `chcp 65001` o PowerShell 7 |

## Por qué importa en el trabajo

- **Esta es la forma de un servicio Go real**: `main` cablea, `internal/` separa, los paquetes
  chicos se testean solos. Los frameworks aparecen cuando el equipo crece, no antes.
- **La traducción de errores a status** es el contrato con el frontend y con otros servicios.
  Un 500 donde va un 422 hace que el cliente muestre "error del servidor" cuando el usuario
  solo se olvidó un campo.
- **El middleware como función** es lo que después te deja agregar roles, rate limiting,
  métricas y tracing sin tocar un handler.
- **El context que atraviesa todo** (`r.Context()` → `QueryContext`) es lo que hace que un
  request abandonado no siga consumiendo base de datos.
- **Los tests con `httptest` y `t.TempDir()`** corren en CI en un segundo, sin puertos ni
  servicios externos. Es la diferencia entre tests que se corren y tests que se saltean.
- **Los logs estructurados con `request_id`** son lo que permite responder "¿qué pasó con el
  request del cliente?" sin adivinar.

## Ejercicios

1. Normalizá `APP_ENV` en `config.Cargar` con `strings.ToLower(strings.TrimSpace(...))` y agregá
   un test de tabla con `" PROD "`, `"prod"` y `"production"`. Verificá que `"production"` sigue
   siendo error.
2. Exponé `POST /api/v1/auth/logout`, protegido con `ConAuth`. Necesitás el token adentro del
   handler: guardalo en el context en `ConAuth` o hacé que `Validar` devuelva la sesión. Test:
   login → logout → `/auth/yo` da 401.
3. Arreglá los dos bugs de `main`: poné `slog.SetDefault` **antes** de `config.Cargar()`, y en la
   goroutine del servidor usá `os.Exit(1)` (o un channel de error) cuando `ListenAndServe` falla
   por algo que no sea `ErrServerClosed`. Verificá con el puerto ocupado que el exit code es 1
   y que el log es JSON en los dos casos.
4. Agregá `PATCH /api/v1/tareas/{id}` con `Titulo *string` y `Hecha *bool`. Un campo `nil`
   significa "no lo toques". Escribí tres tests: solo título, solo `hecha`, y body vacío.
   ¿Qué status devolvés si no viene ningún campo?
5. Versioná el esquema: agregá `usuario_id INTEGER` a `tareas` con una migración que funcione en
   una base que ya tiene datos. Para filtrar por dueño, el handler de `main` lee el usuario del
   context y le pasa el `usuarioID` a `Listar`; no hagas que `tareas` importe `auth`. ¿Qué pasa
   con las tareas viejas sin dueño?
6. Envolvé el mux para que los 404 y 405 también sean JSON: un `ServeMux` propio que capture la
   respuesta, o un handler por defecto. Test: `GET /api/v1/nada` devuelve
   `Content-Type: application/json` y `{"error":"no encontrado"}`.
7. Copiá los tests del capítulo 16 (`auth`) y del 17 (`config`) a sus paquetes acá. Para que
   `auth` sea rápido, inyectá el costo de bcrypt o el hasher como parámetro: un test que tarda
   40 ms por hash se vuelve lento con veinte casos.
8. Guardá el SHA-256 del token en `sesiones` en vez del token. `Validar` hashea lo que llega y
   busca. Ajustá `TestTokenExpirado` si lo copiaste; el resto de los tests debería pasar sin
   cambios.
9. Agregá paginación a `GET /tareas`: `?limite=20&desde=0`, con default y máximo. Devolvé
   `total` como el count real de la tabla, no el largo de la página. Test: dos páginas sin
   solapamiento.
10. Agregá un `BenchmarkListar` con 1000 filas en una base temporal y medí antes y después de
    agregar un índice en `hecha`. ¿Cuándo el índice no ayuda?

## Docker

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/servidor ./proyecto

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/servidor /servidor
WORKDIR /home/nonroot
ENV DB_RUTA=/home/nonroot/tareas.db
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/servidor"]
```

Es el mismo patrón del capítulo 18, con dos líneas propias del proyecto:

- **`WORKDIR /home/nonroot`** porque el proceso no es root y necesita un directorio escribible
  donde caiga el `tareas.db` relativo del default.
- **`ENV DB_RUTA=/home/nonroot/tareas.db`** es redundante con el `WORKDIR` (el default relativo
  ya resolvería ahí), pero deja el path explícito en `docker inspect` y en el log de arranque.
  Si cambiás el `WORKDIR`, la base sigue en un lugar conocido.
- **`CGO_ENABLED=0`** funciona porque el driver es `modernc.org/sqlite`, puro Go. Con
  `mattn/go-sqlite3` haría falta CGO y una imagen base con libc.
- **`ENTRYPOINT` en forma exec** para que el binario sea PID 1 y reciba SIGTERM: es lo que
  activa el apagado ordenado.

```bash
docker build -f proyecto/Dockerfile -t tutorialgo-proyecto .
docker run --rm -p 8080:8080 \
  -e DB_RUTA=/home/nonroot/datos/tareas.db \
  -v "$PWD/datos:/home/nonroot/datos" \
  tutorialgo-proyecto
```

El volumen va a un subdirectorio y el `-e` apunta la base ahí: montar sobre `/home/nonroot`
entero reemplaza el home del usuario. Sin volumen, cada `docker run` arranca con una base vacía:
las tareas se pierden, y los usuarios también porque además viven en memoria.

> El build no se pudo ejecutar en este entorno: `docker --version` responde pero el daemon de
> Docker Desktop no estaba corriendo. Los comandos son los del capítulo 18, verificados ahí.

Volver al [índice](../README.md).
