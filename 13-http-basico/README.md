# 13 — HTTP básico

**Qué vas a construir:** una API REST de tareas con `net/http` puro: listar, crear, leer,
reemplazar y borrar. Con tests que no abren ningún puerto.

**Qué necesitas antes:** capítulos 01 a 12.

---

## Paso 0 — Lo mínimo que hay que saber de HTTP

Un servidor HTTP recibe un **request** (método, ruta, headers, body) y devuelve un
**response** (status, headers, body). En Go, un handler es una función:

```go
func (s *Server) salud(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}
```

- **`w http.ResponseWriter`** es la salida: escribís headers con `w.Header().Set(...)` y el
  body con `w.Write(...)`.
- **`r *http.Request`** es la entrada: `r.Method`, `r.URL.Path`, `r.Header`, `r.Body`.
- `w.Write` devuelve `(int, error)`; acá se ignora porque no hay nada útil que hacer si el
  cliente cortó.

Si venís de Java (Servlets, Spring) o C# (ASP.NET), esto es el equivalente a un
`HttpServlet.doGet` o a un `Controller`: recibe request y escribe response. La diferencia es
que en Go no hay framework obligatorio: el servidor HTTP está en la librería estándar.

## Paso 1 — El estado del servidor

```go
type Server struct {
	mu     sync.Mutex
	tareas map[int]Tarea
	nextID int
}

func NuevoServer() *Server {
	return &Server{tareas: make(map[int]Tarea), nextID: 1}
}
```

- **`tareas`** guarda el estado en memoria. En el capítulo 15 se reemplaza por SQLite.
- **`mu`** protege el map: los handlers corren en goroutines distintas y un map no es seguro
  para escritura concurrente. Sin el mutex, `concurrent map writes` mata el proceso.
- **`nextID`** también va bajo el lock: dos POST simultáneos podrían leer el mismo número y
  duplicar IDs.
- **`NuevoServer`** crea el map con `make`. Un map nil no se puede escribir.

## Paso 2 — El mux y por qué `Handler()` no arranca nada

```go
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/salud", s.salud)
	mux.HandleFunc("/tareas", s.coleccion)
	mux.HandleFunc("/tareas/", s.individual)
	return conLogging(mux)
}
```

- **`http.NewServeMux()`** es el router de la librería estándar. Mapea rutas a handlers.
- **`/salud`** matchea exacto.
- **`/tareas`** matchea exacto: `GET /tareas` y `POST /tareas`.
- **`/tareas/`** con barra final matchea el **subárbol**: cualquier cosa que empiece con
  `/tareas/`. Por eso `individual` tiene que sacar el ID del path a mano.
- **`conLogging(mux)`** envuelve el mux en un middleware (paso 8). Devuelve un
  `http.Handler`.

`Handler()` **arma** el handler pero no abre el puerto. Ese corte es fundamental: en los
tests se llama directo, sin red. `main` es el único que sabe de `:8080`.

## Paso 3 — `GET /tareas`: copiar bajo lock, ordenar afuera

```go
case http.MethodGet:
	s.mu.Lock()
	lista := make([]Tarea, 0, len(s.tareas))
	for _, t := range s.tareas {
		lista = append(lista, t)
	}
	s.mu.Unlock()

	sort.Slice(lista, func(i, j int) bool { return lista[i].ID < lista[j].ID })
	responderJSON(w, http.StatusOK, lista)
```

- Se copia el map a un slice **dentro** del lock. Leer el map mientras otro handler escribe
  es un data race.
- Se **suelta el lock antes** de ordenar y serializar. Si mantuvieras el lock mientras
  `json.Encode` escribe al cliente, un cliente lento bloquearía a todos los demás. Esa es una
  regla de oro: **el lock protege datos, no llamadas de red**.
- **`sort.Slice`** ordena con un comparador `menor(a, b) bool`. El map no tiene orden; la API
  sí tiene que tenerlo para que el cliente vea resultados estables. (En Go 1.21+ también
  podrías usar `slices.SortFunc`.)
- **`make([]Tarea, 0, len(s.tareas))`** reserva la capacidad exacta: un solo `alloc`.
- **`http.StatusOK`** es 200.

## Paso 4 — `POST /tareas`: validar antes de tocar el estado

```go
r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
var entrada struct {
	Titulo string `json:"titulo"`
}
dec := json.NewDecoder(r.Body)
dec.DisallowUnknownFields()
if err := dec.Decode(&entrada); err != nil {
	responderError(w, http.StatusBadRequest, "json inválido: "+err.Error())
	return
}
```

- **`http.MaxBytesReader(w, r.Body, 1<<20)`** limita el body a 1 MB. Sin esto, un cliente
  puede mandar un body de 10 GB y agotar la RAM. `1<<20` es 1.048.576, escrito como bit shift.
- **Un struct anónimo** para la entrada: la API no acepta `ID` ni `Hecha` en el POST, solo
  `titulo`. Si usaras `Tarea`, el cliente podría mandar un `ID` y pisar el del servidor.
- **`DisallowUnknownFields`**: un `{"tItulo": "x"}` devuelve 400 en vez de crear una tarea con
  título vacío. Sin esto, un typo del cliente pasa en silencio. Es una decisión de producto;
  para una API interna, mejor 400.
- **`Decode`** decodifica el primer valor JSON del body. Devuelve error si el JSON está mal
  formado o si el body está vacío.
- El mensaje `"json inválido: "+err.Error()` **filtra el error interno**. Para un ejemplo está
  bien; en producción devolvés un mensaje genérico y logueás el detalle.

```go
entrada.Titulo = strings.TrimSpace(entrada.Titulo)
if entrada.Titulo == "" {
	responderError(w, http.StatusUnprocessableEntity, "titulo es obligatorio")
	return
}
```

Validación de negocio después de parsear: 400 es "no pude leer lo que mandaste", 422 es "lo
leí, pero no es válido". La distinción le dice al cliente si el problema es sintáctico o de
reglas.

```go
s.mu.Lock()
t := Tarea{ID: s.nextID, Titulo: entrada.Titulo}
s.tareas[t.ID] = t
s.nextID++
s.mu.Unlock()

w.Header().Set("Location", fmt.Sprintf("/tareas/%d", t.ID))
responderJSON(w, http.StatusCreated, t)
```

- El ID se asigna **dentro** del lock, junto con la escritura. Si lo asignaras afuera, dos
  requests podrían llevarse el mismo.
- **`Location`** es el header estándar que dice dónde quedó el recurso creado:
  `/tareas/3`. Un cliente REST lo espera.
- **201 Created** es el status correcto, no 200. Y el body devuelve la tarea completa, con su
  ID.
- **`%d`** en `Sprintf` es el ID.

Cuando el método no es GET ni POST:

```go
default:
	w.Header().Set("Allow", "GET, POST")
	responderError(w, http.StatusMethodNotAllowed, "método no permitido")
```

405 con el header `Allow` es lo que exige HTTP: el cliente sabe qué métodos existen en esa
ruta.

## Paso 5 — `/tareas/{id}` con el router viejo

```go
id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/tareas/"))
if err != nil || id <= 0 {
	responderError(w, http.StatusBadRequest, "id inválido")
	return
}
```

- **`strings.TrimPrefix`** quita `/tareas/` y deja `"7"`.
- **`strconv.Atoi`** convierte a int. Si el path es `/tareas/abc`, falla.
- **400, no 404**: un ID no numérico es una URL mal armada por el cliente. Un ID válido que no
  existe es 404. Distinguirlos le dice al cliente si el problema es suyo o del recurso.
- Este parseo manual es la razón por la que existe el capítulo 14: `GET /tareas/{id}` y
  `r.PathValue("id")` lo reemplazan.

```go
s.mu.Lock()
t, existe := s.tareas[id]
s.mu.Unlock()
```

Se lee bajo lock y se suelta enseguida. `existe` es el `ok` del map.

```go
case http.MethodGet:
	if !existe {
		responderError(w, http.StatusNotFound, "tarea no encontrada")
		return
	}
	responderJSON(w, http.StatusOK, t)
```

GET devuelve la tarea o 404. `return` corta el handler: sin él, seguiría al `case` siguiente
(y como los `case` de Go no tienen fallthrough, en realidad seguiría después del `switch`; el
`return` evita cualquier duda).

```go
case http.MethodPut:
	if !existe { 404 }
	var entrada struct {
		Titulo string `json:"titulo"`
		Hecha  bool   `json:"hecha"`
	}
	if err := json.NewDecoder(r.Body).Decode(&entrada); err != nil { 400 }
	entrada.Titulo = strings.TrimSpace(entrada.Titulo)
	if entrada.Titulo == "" { 422 }

	s.mu.Lock()
	s.tareas[id] = Tarea{ID: id, Titulo: entrada.Titulo, Hecha: entrada.Hecha}
	actualizada := s.tareas[id]
	s.mu.Unlock()
	responderJSON(w, http.StatusOK, actualizada)
```

- **PUT reemplaza el recurso completo**: manda título y `hecha`. Si el cliente omite `hecha`,
  se pone `false` (zero value). Eso es correcto para PUT; para actualización parcial se usa
  PATCH.
- El `ID` se toma del path, no del body: el cliente no decide el ID.
- Acá falta `MaxBytesReader` y `DisallowUnknownFields`, que sí están en el POST. **Es una
  inconsistencia real del ejemplo**; el ejercicio te pide corregirla.
- Se relee `s.tareas[id]` para devolver el valor guardado (aunque acá es idéntico).

```go
case http.MethodDelete:
	if !existe { 404 }
	s.mu.Lock()
	delete(s.tareas, id)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
```

**204 No Content**: la operación funcionó y no hay body. No llames a `responderJSON` con
`nil`, porque escribirías `null`.

## Paso 6 — Responder JSON en un solo lugar

```go
func responderJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("codificar respuesta: %v", err)
	}
}
```

El orden es obligatorio:

1. **Headers primero.** Después de `WriteHeader`, ya no se pueden cambiar.
2. **`WriteHeader(status)`** manda el status.
3. **`Encode(v)`** escribe el body y agrega `\n`.

Si `Encode` falla (por ejemplo, un valor no serializable), el status **ya se envió**: no se
puede convertir en un 500. Solo queda loguear. Por eso las respuestas se arman con tipos
simples y verificados.

`any` es `interface{}`: acepta cualquier valor. `json.Encoder` decide cómo serializarlo.

```go
func responderError(w http.ResponseWriter, status int, msg string) {
	responderJSON(w, status, map[string]string{"error": msg})
}
```

Un error siempre tiene la misma forma: `{"error": "..."}`. Eso le permite al cliente parsearlo
sin adivinar.

## Paso 7 — `statusRecorder`: ver el status que ya se escribió

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

`http.ResponseWriter` no tiene un método para leer el status que se escribió. El wrapper
embebe el original (así todos los demás métodos siguen funcionando) y **sobrescribe
`WriteHeader`** para guardar el código antes de delegar.

Si un handler nunca llama `WriteHeader`, el servidor asume 200. Por eso el recorder se crea
con `status: http.StatusOK`.

## Paso 8 — El middleware de logging

```go
func conLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(inicio).Round(time.Millisecond))
	})
}
```

- Un middleware es una función que recibe un handler y devuelve otro. Envuelve: hace algo
  antes, llama al siguiente, hace algo después.
- **`http.HandlerFunc(...)`** convierte una función en un `http.Handler`.
- **`time.Since(inicio)`** devuelve un `time.Duration`; `.Round(time.Millisecond)` lo redondea
  para el log.
- **`log.Printf`** usa verbos de `fmt`: `%s` método y ruta, `%d` status, `%s` duración.
  `log` agrega fecha y hora solo. En el capítulo 18 se cambia por `slog`.
- El middleware se aplica a **todo** el mux, incluidas las rutas que no existen (404 del
  router también se loguean).

## Paso 9 — `main`: el servidor

```go
srv := &http.Server{
	Addr:              ":8080",
	Handler:           NuevoServer().Handler(),
	ReadHeaderTimeout: 5 * time.Second,
}
log.Println("escuchando en http://localhost:8080")
if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
	log.Fatal(err)
}
```

- **`http.Server`** permite configurar timeouts. El mínimo indispensable es
  `ReadHeaderTimeout`: sin él, un cliente puede abrir una conexión y no mandar nunca los
  headers (Slowloris), agotando las conexiones.
- **`ListenAndServe` bloquea** hasta que el servidor falla o lo apagan.
- **`http.ErrServerClosed`** es el error normal de un cierre ordenado. No es un fallo: se
  ignora. El apagado ordenado completo está en el capítulo 18.
- **`log.Fatal`** imprime y llama a `os.Exit(1)`.

## Paso 10 — Probar la API sin abrir un puerto

```go
h := NuevoServer().Handler()

rec := httptest.NewRecorder()
req := httptest.NewRequest(http.MethodPost, "/tareas", strings.NewReader(`{"titulo":"comprar café"}`))
h.ServeHTTP(rec, req)
if rec.Code != http.StatusCreated { ... }
```

- **`httptest.NewRecorder()`** es un `ResponseWriter` que guarda status, headers y body en
  memoria.
- **`httptest.NewRequest(metodo, ruta, body)`** arma un request sin red. El body es cualquier
  `io.Reader`: `strings.NewReader` para un JSON literal.
- **`h.ServeHTTP(rec, req)`** ejecuta el handler completo, con middlewares incluidos.
- **`rec.Code`** es el status, **`rec.Body.Bytes()`** el body.

El test `TestCRUDTareas` hace el ciclo completo: POST 201, GET lista, GET por ID, DELETE 204,
GET 404. El `TestErroresDeEntrada` usa una tabla con JSON roto, título vacío, campo
desconocido, ID no numérico, ID inexistente y método no permitido. Sin puerto, sin esperas,
sin limpieza.

## Correr y ejercitar

```bash
go run ./13-http-basico
```

```bash
curl.exe http://localhost:8080/salud
curl.exe http://localhost:8080/tareas
curl.exe -X POST http://localhost:8080/tareas -H "Content-Type: application/json" -d '{\"titulo\":\"probar\"}'
curl.exe http://localhost:8080/tareas/1
curl.exe -X PUT http://localhost:8080/tareas/1 -d '{\"titulo\":\"probar de nuevo\",\"hecha\":true}'
curl.exe -X DELETE http://localhost:8080/tareas/1 -i
curl.exe -X PATCH http://localhost:8080/tareas -i
```

```bash
go test -v ./13-http-basico
```

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| `concurrent map writes` | Map sin lock | `mu.Lock()` alrededor de cada acceso |
| Todos los requests ven el mismo ID | `nextID` fuera del lock | Asignalo dentro |
| `superfluous response.WriteHeader call` | Escribiste headers dos veces | Un solo `WriteHeader` |
| El body no llega al cliente | Faltó `WriteHeader` o headers después de escribirlo | Orden: headers → status → body |
| 405 sin `Allow` | Falta el header | `w.Header().Set("Allow", ...)` |
| `http: request body too large` | `MaxBytesReader` | Es el comportamiento buscado |
| El cliente lento bloquea todo | Lock durante el encode | Copiá y soltá el lock |
| 404 en una ruta que existe | Patrón con/sin barra final | `/tareas` exacto, `/tareas/` subárbol |

## Por qué importa en el trabajo

- **`net/http` alcanza para una API interna**: no necesitás framework para CRUD.
- **Separar `Handler()` de `main`** es lo que hace testeable toda la API.
- **Validar entrada** (tamaño, campos, tipos) es la primera línea de defensa.
- **Status codes correctos** son el contrato con el cliente: 201, 204, 400, 404, 405, 422.
- **Timeouts del servidor** evitan que un cliente malicioso tumbe el proceso.

## Ejercicio

1. Agregá `MaxBytesReader` y `DisallowUnknownFields` al PUT para que sea consistente con el
   POST. Escribí un test que lo verifique.
2. Agregá `PATCH /tareas/{id}` que actualice solo los campos presentes usando
   `map[string]json.RawMessage` o un struct con punteros.
3. Agregá paginación a `GET /tareas`: `?limite=10&desde=20`, con defaults y validación.
4. Hacé que `POST /tareas` rechace títulos de más de 100 caracteres con 422 y un mensaje que
   diga el máximo.

Siguiente: [14 — Router y middleware](../14-router-middleware/).
