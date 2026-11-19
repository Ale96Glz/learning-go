# 14 — Router y middleware

**Qué vas a construir:** la misma API de tareas, pero con el router de Go 1.22 (`GET
/tareas/{id}`), versionado por prefijo y tres middlewares: request ID, logging y recuperación
de panics.

**Qué necesitas antes:** capítulos 01 a 13.

---

## Paso 1 — El router de la librería estándar ya alcanza

```go
api := http.NewServeMux()
api.HandleFunc("GET /tareas", a.listar)
api.HandleFunc("POST /tareas", a.crear)
api.HandleFunc("GET /tareas/{id}", a.obtener)
api.HandleFunc("DELETE /tareas/{id}", a.borrar)
```

Desde Go 1.22, `ServeMux` entiende **método + patrón**:

- **`GET /tareas`**: solo GET. Un PATCH a esa ruta devuelve **405** y el header `Allow`
  automáticamente, sin que escribas nada.
- **`{id}`** captura **un** segmento del path. `/tareas/7` sí; `/tareas/7/historial` no.
- **`r.PathValue("id")`** devuelve el texto capturado. Convertirlo y validarlo sigue siendo tu
  trabajo.
- Si registrás `/tareas` y `/tareas/{id}`, cada patrón atiende lo suyo; el más específico
  gana.

Antes de 1.22 esto requería `httprouter`, `chi` o `gorilla/mux`. Para CRUD normal ya no. Un
router de terceros sigue teniendo sentido si querés grupos, middlewares por grupo o rutas con
regex; para este curso, la stdlib alcanza y no agrega dependencias.

## Paso 2 — Versionado con `StripPrefix`

```go
raiz := http.NewServeMux()
raiz.Handle("/api/v1/", http.StripPrefix("/api/v1", api))
raiz.HandleFunc("GET /salud", func(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
})
```

- **`raiz.Handle("/api/v1/", ...)`** registra un subárbol: todo lo que empiece con `/api/v1/`.
- **`http.StripPrefix("/api/v1", api)`** le quita ese prefijo al path y le pasa el resto al
  mux interno. Un request a `/api/v1/tareas/7` llega a `api` como `/tareas/7`.
- **`PathValue("id")` sigue funcionando** porque lo resuelve el mux interno, que es el que
  conoce el patrón `{id}`. El prefijo se quitó antes, pero el patrón se aplica después.
- **`/salud`** queda fuera del prefijo: es el health check del proceso, no de la API
  versionada.

Así podés publicar `/api/v2/` mañana apuntando a otro mux, sin duplicar handlers ni romper a
los clientes viejos.

La ruta de debug:

```go
raiz.HandleFunc("GET /debug/panic", func(http.ResponseWriter, *http.Request) {
	panic("falla simulada")
})
```

Existe solo para demostrar que el middleware de recuperación convierte un panic en 500. En un
servicio real, esa ruta no se publica.

## Paso 3 — La cadena de middlewares

```go
type Middleware func(http.Handler) http.Handler

func Cadena(h http.Handler, ms ...Middleware) http.Handler {
	for i := len(ms) - 1; i >= 0; i-- {
		h = ms[i](h)
	}
	return h
}
```

```go
return Cadena(raiz, ConRequestID, ConLogging, ConRecuperacion)
```

`Cadena` recorre el slice **al revés** y va envolviendo. El resultado es:

```text
ConRequestID( ConLogging( ConRecuperacion( raiz ) ) )
```

Y en tiempo de ejecución:

```text
RequestID -> Logging -> Recuperación -> router -> handler
```

El orden es diseño, no detalle:

- **RequestID primero**: genera o hereda el ID y lo deja en el context. Todo lo que venga
  después puede usarlo.
- **Logging segundo**: ya puede leer el ID y mide desde antes de entrar al handler.
- **Recuperación último**: está lo más cerca posible del handler, que es donde ocurre el
  panic. Si estuviera primero, un panic en Logging no se recuperaría.

Si invertís Logging y Recuperación, el log podría no registrar el 500. Si ponés RequestID al
final, el log no tiene ID. El orden se elige por dependencias entre capas.

## Paso 4 — `ConRequestID`

```go
func ConRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = nuevoID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), contextoKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
```

- **`r.Header.Get("X-Request-ID")`** lee el header. Si el cliente o el gateway ya mandan uno,
  se **respeta**: así se correlacionan logs entre servicios.
- Si no viene, se genera. Se usa `crypto/rand`, no `math/rand`: un ID predecible permite
  adivinar requests ajenos.
- **`w.Header().Set("X-Request-ID", id)`** lo devuelve en la respuesta, para que el usuario
  pueda reportar "falló el request abc123".
- **`context.WithValue`** guarda el ID con una clave de tipo propio (`contextoKey{}`), no un
  string. Dos paquetes con la clave `"id"` se pisarían.
- **`r.WithContext(ctx)`** devuelve una **copia** del request con el context nuevo. El
  original no se muta. `next` recibe la copia.

Un detalle: el header se escribe **antes** de llamar al handler. Si el handler hace panic
después de escribir headers, el `X-Request-ID` ya salió.

## Paso 5 — `ConLogging`

```go
func ConLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		id, _ := r.Context().Value(contextoKey{}).(string)
		log.Printf("req=%s %s %s -> %d (%s)",
			id, r.Method, r.URL.Path, rec.status, time.Since(inicio).Round(time.Millisecond))
	})
}
```

- **`inicio := time.Now()`** antes de llamar al handler; el log mide el tiempo total.
- **`statusRecorder`** envuelve el `ResponseWriter` para capturar el status (capítulo 13).
  Se inicializa en 200 porque si el handler nunca llama `WriteHeader`, ese es el status real.
- **`r.Context().Value(contextoKey{})`** recupera el ID que puso `ConRequestID`. El `_`
  descarta el `ok`; si faltara, el log mostraría un string vacío.
- **`log.Printf`** usa verbos: `%s` para el ID, método y path; `%d` para el status; `%s` para
  la duración. `log` agrega fecha y hora. En el capítulo 18 se reemplaza por `slog` con pares
  clave-valor, que es lo correcto para producción.
- El log se emite **después** de que el handler termina, por eso conoce el status final.

Límite conocido: este recorder no implementa `http.Flusher` ni `http.Hijacker`. Si tu handler
hace streaming (SSE) o WebSockets, el wrapper los esconde y el handler no puede usarlos. En
esos casos hay que implementar los métodos y delegar. Para CRUD normal no importa.

## Paso 6 — `ConRecuperacion`

```go
func ConRecuperacion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recuperado: %v", rec)
				responderError(w, http.StatusInternalServerError, "error interno")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
```

- **`recover()` solo funciona dentro de un `defer`**. Si lo llamás en otro lado, devuelve
  `nil` y el panic sigue.
- **`rec`** es el valor del panic: puede ser un string, un error o cualquier cosa. `%v` lo
  imprime.
- **`net/http` ya recupera el panic**, pero por conexión: loguea el stack y **corta esa
  conexión** sin responder. El cliente ve un reset, no un 500; en HTTP/2 se cae la conexión
  entera, con los requests en vuelo. Y si el panic ocurrió con un mutex tomado, el lock queda
  tomado y los siguientes requests se cuelgan. El middleware evita las tres cosas: responde
  500, mantiene viva la conexión y no deja el estado a medias.
- **El 500 no siempre se puede enviar**: si el handler ya escribió headers y parte del body,
  `responderError` no puede cambiar el status y la respuesta queda incompleta. El servidor
  sigue vivo, que es lo importante.
- Loguear el panic es obligatorio. Un 500 sin traza es un bug imposible de encontrar. En
  producción se agrega el stack con `debug.Stack()`.

## Paso 7 — Generar el ID

```go
func nuevoID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}
```

- **8 bytes = 64 bits** aleatorios, impredecibles. Se imprimen como 16 caracteres hex.
- **`rand.Read`** de `crypto/rand` puede fallar (raro, pero puede). El fallback usa el
  timestamp en hexadecimal: no es seguro, pero es mejor que no tener ID. En un servicio serio,
  si `crypto/rand` falla, conviene fallar el request.

## Paso 8 — Handlers con `PathValue`

```go
func parseID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		responderError(w, http.StatusBadRequest, "id inválido")
		return 0, false
	}
	return id, true
}
```

- **`r.PathValue("id")`** devuelve `""` si el patrón no capturó nada, y `Atoi("")` falla.
- Devuelve `(int, bool)` para que el handler corte sin repetir el `if`:
  ```go
  id, ok := parseID(w, r)
  if !ok {
      return
  }
  ```
- 400 para ID inválido, 404 para ID válido inexistente. La misma distinción del capítulo 13.

```go
func (a *API) borrar(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok { return }
	a.mu.Lock()
	_, existe := a.tareas[id]
	delete(a.tareas, id)
	a.mu.Unlock()
	if !existe {
		responderError(w, http.StatusNotFound, "tarea no encontrada")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

`delete` de una clave inexistente no falla; por eso se consulta `existe` **antes** de borrar.
Se hace todo dentro del lock para que no haya carrera entre "existe" y "borrar".

`listar` copia el map bajo lock y responde. A diferencia del capítulo 13, **no ordena**: el
map no garantiza orden y la respuesta puede variar. Es una inconsistencia del ejemplo;
agregar `slices.SortFunc` es el ejercicio.

## Paso 9 — Respuestas y `statusRecorder`

```go
func responderJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
```

Headers → status → body, en ese orden. El error de `Encode` se ignora acá; con el status ya
enviado, no hay mucho que hacer. En el capítulo 13 se loguea.

`statusRecorder` es el mismo wrapper: embebe el `ResponseWriter`, intercepta `WriteHeader` y
guarda el código.

## Paso 10 — `main`

```go
srv := &http.Server{
	Addr:              ":8080",
	Handler:           NuevaAPI().Rutas(),
	ReadHeaderTimeout: 5 * time.Second,
}
log.Println("escuchando en http://localhost:8080/api/v1/tareas")
if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
	log.Fatal(err)
}
```

Igual que en el capítulo 13: `Rutas()` arma todo, `main` solo escucha. Por eso los tests
pueden probar la API entera sin abrir el puerto. El apagado ordenado llega en el capítulo 18.

## Paso 11 — Los tests

```go
h := NuevaAPI().Rutas()
rec := httptest.NewRecorder()
h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tareas", strings.NewReader(`{"titulo":"revisar PR"}`)))
```

El test recorre el camino completo: middlewares, `StripPrefix`, mux interno y handler. Cuatro
tests cubren lo importante:

- **`TestRutasConPatrones`**: crear, obtener por ID, 404 y borrar.
- **`TestMetodoNoPermitido`**: PATCH devuelve 405 sin que el handler lo maneje.
- **`TestRequestID`**: sin header se genera, con header se respeta.
- **`TestRecuperacionDePanic`**: `/debug/panic` da 500 y el servidor sigue respondiendo
  `/salud`.

El último es el más valioso: prueba que un panic no tumba el proceso.

## Correr y probar

```bash
go run ./14-router-middleware
curl.exe http://localhost:8080/salud
curl.exe http://localhost:8080/api/v1/tareas
curl.exe -X POST http://localhost:8080/api/v1/tareas -d '{\"titulo\":\"revisar PR\"}'
curl.exe http://localhost:8080/api/v1/tareas/1 -i
curl.exe -X DELETE http://localhost:8080/api/v1/tareas/1 -i
curl.exe -X PATCH http://localhost:8080/api/v1/tareas -i
curl.exe http://localhost:8080/debug/panic -i
go test -v ./14-router-middleware
```

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| 404 en `/api/v1/tareas` | El mux interno espera `/tareas` | Revisá el `StripPrefix` |
| `PathValue("id")` vacío | El patrón no captura o estás en otro mux | Usá `{id}` en el patrón correcto |
| 405 sin header `Allow` | Lo armaste a mano y lo olvidaste | Con patrones por método es automático |
| El panic corta la conexión y el cliente no recibe 500 | Falta `ConRecuperacion` | Agregalo en la cadena |
| El log no tiene request ID | `ConRequestID` va después de `ConLogging` | Invertí el orden |
| El handler no puede hacer streaming | El recorder no implementa `Flusher` | Implementalo o no envuelvas esa ruta |
| `superfluous WriteHeader` | Escribiste status dos veces | Un solo `WriteHeader` |
| ID predecible | Usaste `math/rand` | `crypto/rand` |

## Por qué importa en el trabajo

- **El router estándar** elimina una dependencia y una capa de magia. Para CRUD, alcanza.
- **El request ID** es lo primero que se agrega en un servicio real: sin él, no podés cruzar
  logs.
- **La recuperación de panics** es la diferencia entre un 500 y una conexión cortada sin
  respuesta.
- **`StripPrefix` + versionado** permite evolucionar la API sin romper clientes.
- **La cadena de middlewares** es el patrón con el que se agregan auth, CORS, rate limiting y
  métricas, siempre en el mismo lugar.

## Ejercicio

1. Ordená `listar` por ID con `slices.SortFunc`, igual que el capítulo 13. Escribí un test que
   verifique el orden.
2. Agregá `ConCORS` como middleware que responda `OPTIONS` y agregue
   `Access-Control-Allow-Origin`. ¿Va antes o después de `ConLogging`? Justificá.
3. Agregá un middleware `ConTimeout` que aplique `context.WithTimeout` de 2 s al request y
   devuelva 504 si el handler se pasa. Usá `r.Context()`.
4. Hacé que `ConRecuperacion` loguee `debug.Stack()` y verificá que el test sigue pasando.

Siguiente: [15 — Base de datos](../15-base-datos/).
