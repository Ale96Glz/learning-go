# 18 — Producción

**Qué vas a construir:** el mismo servidor HTTP, pero operable: logs JSON que una plataforma
puede filtrar, endpoints de salud separados, timeouts contra clientes lentos, apagado ordenado
sin cortar requests en curso y una imagen Docker mínima.

**Qué necesitas antes:** capítulos 01 a 17.

---

## Paso 0 — Qué significa "producción"

No es una feature nueva: son propiedades operativas. En desarrollo alcanza con que funcione;
en producción además necesitás:

- **Ver qué pasó** cuando algo falla, sin entrar al servidor. Eso son logs estructurados.
- **Saber si el proceso está vivo y si puede atender.** Son dos preguntas distintas y dos
  endpoints distintos.
- **No quedar colgado** por un cliente que abre una conexión y no manda nada, o por un handler
  lento.
- **Apagarse sin cortar requests** cuando hay un deploy. El orquestador manda una señal y el
  proceso tiene que terminar lo que está haciendo.
- **Una imagen chica y sin shell**, para reducir superficie de ataque.

En Java/C# esto lo cubren Actuator/health, Serilog/ILogger, los límites de Tomcat/Kestrel y el
host lifetime de .NET. En Go son APIs de la librería estándar, y por eso conviene entenderlas.

## Paso 1 — `slog`: logs estructurados

```go
slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
	Level: slog.LevelInfo,
})))
```

- **`slog`** es el logger estructurado de la librería estándar (Go 1.21+). Reemplaza a `log`
  para código nuevo.
- **`NewJSONHandler`** emite **una línea JSON por evento**, ideal para que lo ingieran
  Datadog, Loki, CloudWatch o Elastic. Cada campo es consultable; un texto plano obliga a
  regex.
- **`os.Stdout`**: en un contenedor, la salida estándar es lo que recolecta la plataforma. No
  escribas a un archivo dentro del contenedor: muere con él.
- **`Level: slog.LevelInfo`** descarta `Debug`. En desarrollo podrías usar `LevelDebug`; en el
  ejercicio lo conectás con `LOG_NIVEL` del capítulo 17.
- **`SetDefault`** hace que `slog.Info(...)` funcione sin pasar el logger. También redirige el
  paquete `log` viejo al handler nuevo, así que las librerías que todavía usan `log.Printf`
  no rompen el formato.

Una línea real de este servidor:

```json
{"time":"2026-11-14T23:52:30.5267597-06:00","level":"INFO","msg":"servidor iniciado","addr":":8080"}
```

- **`time`**, **`level`**, **`msg`** son las claves fijas. El resto son atributos.
- **`"addr":":8080"`** es un par clave-valor. En JSON el valor es string, no texto pegado al
  mensaje.
- **La hora sale con la zona local** (`-06:00`). En producción suele preferirse UTC para
  correlacionar entre regiones:

  ```go
  ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
      if a.Key == slog.TimeKey {
          a.Value = slog.TimeValue(a.Value.Time().UTC())
      }
      return a
  }
  ```

### La forma de llamarlo

```go
slog.Info("request",
	"metodo", r.Method,
	"ruta", r.URL.Path,
	"status", rec.status,
	"duracion_ms", time.Since(inicio).Milliseconds(),
)
```

- **Después del mensaje van pares clave-valor.** `"metodo"` es la clave, `r.Method` el valor.
  No es `Printf`: no hay verbos.
- **Si pasás una cantidad impar de argumentos**, slog agrega `!BADKEY` y lo ves en el log. No
  compila mal, así que revisá los pares.
- **`status` y `duracion_ms` son números** en el JSON. Eso permite `status >= 500` y promedios
  en el sistema de logs; si fueran texto, no.
- **`time.Since(inicio).Milliseconds()`** devuelve `int64`. Para requests rápidos da `0`; si
  necesitás más precisión, `Microseconds`.
- **`slog.Error("fallo el servidor", "error", err)`** sigue el mismo formato. Un `error` se
  serializa con su mensaje.

## Paso 2 — Liveness y readiness: dos preguntas distintas

```go
// Liveness: ¿el proceso está vivo? Si falla, el orquestador reinicia.
mux.HandleFunc("GET /salud", func(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
})
```

`/salud` responde **200 si el proceso puede atender HTTP**. No consulta la base, ni la cola, ni
nada externo. Si el proceso está trabado en un deadlock, el request no responde y el
orquestador lo reinicia.

```go
// Readiness: ¿puede atender tráfico? Si la DB no responde, no me manden requests.
mux.HandleFunc("GET /listo", func(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if !baseDeDatosLista(ctx) {
		http.Error(w, "no listo", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("listo"))
})
```

- **`/listo` sí consulta dependencias.** Si la base no responde, devuelve **503** y el balanceador
  deja de mandarle tráfico, pero **no reinicia** el proceso.
- **`context.WithTimeout(r.Context(), 2*time.Second)`** hereda la cancelación del request y le
  pone un techo. Si el cliente se va o el ping tarda más de 2 s, el context se cancela.
- **`defer cancel()`** libera el timer apenas termina el handler.
- **`http.Error(w, "no listo", 503)`** escribe el mensaje, el status y los headers de texto.
  Es la forma corta de un error HTTP en texto plano.
- **El status importa**: 200 = listo; cualquier otro = no me mandes tráfico. Un 500 también
  sirve, pero 503 dice "temporalmente no disponible", que es exactamente el caso.

```go
// En un servicio real sería db.PingContext(ctx).
func baseDeDatosLista(ctx context.Context) bool {
	return ctx.Err() == nil
}
```

Es un **stub**: no hay base en este capítulo. En el proyecto final sería
`return db.PingContext(ctx) == nil`. Y no cualquier error: solo "no puedo hablar con la base"
debería dar no-listo; un error de permisos en una tabla no se arregla reiniciando.

### Por qué no mezclarlos

Si `/salud` consultara la base y la base se cae, el orquestador reinicia **todos** los pods una
y otra vez. La base sigue caída, los pods reinician en loop y ahora además perdiste el servicio
que podía responder errores útiles. Liveness es "¿este proceso está sano?", no "¿el mundo
entero está sano?".

En Kubernetes se conectan así:

```yaml
livenessProbe:
  httpGet: { path: /salud, port: 8080 }
readinessProbe:
  httpGet: { path: /listo, port: 8080 }
```

El detalle de orquestación cambia (Kubernetes, ECS, Nomad), pero el contrato es el mismo:
liveness reinicia, readiness saca de rotación.

## Paso 3 — Los cuatro timeouts del servidor

```go
srv := &http.Server{
	Addr:              ":8080",
	Handler:           conLogging(mux),
	ReadHeaderTimeout: 5 * time.Second,
	ReadTimeout:       10 * time.Second,
	WriteTimeout:      20 * time.Second,
	IdleTimeout:       60 * time.Second,
}
```

| Campo | Qué limita | Por qué ese valor |
|---|---|---|
| `ReadHeaderTimeout` | desde que se acepta la conexión hasta que llegan los headers | 5 s corta Slowloris: un cliente que abre y no manda nada |
| `ReadTimeout` | la lectura completa del request, body incluido | 10 s; un body más lento que eso es un cliente roto o malicioso |
| `WriteTimeout` | desde el fin de los headers hasta el fin de la respuesta | 20 s; cubre handlers lentos sin dejar conexiones eternas |
| `IdleTimeout` | una conexión keep-alive sin requests | 60 s; libera conexiones ociosas |

Sin `ReadHeaderTimeout`, el default es **cero: sin límite**. Miles de conexiones abiertas sin
mandar nada agotan los file descriptors y el servidor deja de aceptar. Es el ataque más barato
que existe.

`WriteTimeout` es el que muerde cuando un handler se pasa: la respuesta se corta a la mitad y
el cliente ve un error de red, no un 500. Por eso los handlers largos necesitan su propio
`context` con deadline, no confiar en el timeout del servidor. El default de `MaxHeaderBytes`
es 1 MB; si tus headers pueden ser más grandes, subilo a sabiendas.

## Paso 4 — Un handler lento para ver el apagado

```go
mux.HandleFunc("GET /trabajo", func(w http.ResponseWriter, r *http.Request) {
	time.Sleep(2 * time.Second)
	fmt.Fprintln(w, "trabajo terminado")
})
```

Duerme 2 s y responde. Existe para que puedas lanzar un request, mandar Ctrl+C mientras corre y
ver que el apagado **espera** a que termine. En código real sería una consulta pesada o un
reporte; y en vez de `Sleep` habría un `select` sobre `r.Context().Done()` para abandonar si el
cliente se fue.

## Paso 5 — Apagado ordenado

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

go func() {
	slog.Info("servidor iniciado", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("fallo el servidor", "error", err)
		os.Exit(1)
	}
}()

<-ctx.Done()
stop() // un segundo Ctrl+C ahora mata el proceso de inmediato
```

- **`signal.NotifyContext`** devuelve un context que se cancela cuando llega `os.Interrupt`
  (Ctrl+C en Windows/Linux) o `syscall.SIGTERM` (lo que manda Docker/Kubernetes al parar).
- **`defer stop()`** desregistra el handler al salir. Llamarlo dos veces es seguro.
- **`ListenAndServe` va en una goroutine** porque bloquea. `main` tiene que quedar libre para
  esperar la señal.
- **`http.ErrServerClosed` no es un error**: es lo que devuelve `ListenAndServe` cuando alguien
  llamó a `Shutdown`. Si lo trataras como fallo, cada deploy terminaría con exit 1.
- **`os.Exit(1)`** si el puerto está ocupado o falla el listen. `main` está bloqueado en
  `<-ctx.Done()`, así que la goroutine tiene que matar el proceso; no hay a quién devolverle el
  error. Los `defer` no corren, pero acá no hay recursos abiertos.
- **`stop()` después de `<-ctx.Done()`** devuelve el manejo de señales al default: un segundo
  Ctrl+C mata el proceso de inmediato. Es la salida de emergencia si el apagado se cuelga.

```go
slog.Info("apagando: esperando requests en curso")
shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

if err := srv.Shutdown(shutdownCtx); err != nil {
	slog.Error("no se pudo cerrar ordenadamente", "error", err)
	os.Exit(1)
}
slog.Info("apagado completo")
```

- **`srv.Shutdown`** deja de aceptar conexiones nuevas, cierra las ociosas y **espera** a que
  terminen las que están en curso. Es distinto de `srv.Close`, que las corta de inmediato.
- **`context.WithTimeout(..., 10*time.Second)`** pone un techo a la espera. Si un handler no
  termina en 10 s, `Shutdown` devuelve `context deadline exceeded`, se loguea y el proceso sale
  con 1. En Kubernetes, `terminationGracePeriodSeconds` tiene que ser **mayor** que estos 10 s,
  o el orquestador mata el proceso antes.
- **`shutdownCtx` no deriva del context de la señal**, porque ese ya está cancelado. Se crea
  uno nuevo con `Background()`.
- **El orden de los logs** cuenta la historia: "apagando" sale apenas llega la señal, el
  "request" del handler en curso sale cuando termina, y "apagado completo" al final.

### La secuencia, paso a paso

Con un `GET /trabajo` en vuelo y un Ctrl+C:

```text
t=0.0  entra GET /trabajo (duerme 2 s)
t=0.1  Ctrl+C -> ctx.Done()
       stop() -> el próximo Ctrl+C mata
       log: "apagando: esperando requests en curso"
       Shutdown: no acepta más, espera
t=2.1  el handler escribe "trabajo terminado"
       log: request GET /trabajo 200 duracion_ms=2000
       Shutdown devuelve nil
       log: "apagado completo"
t=2.1  el proceso termina con exit 0
```

Sin `Shutdown`, el proceso muere en `t=0.1` y el cliente recibe la conexión cortada. Ese es
todo el valor del patrón.

Dos límites: `Shutdown` **no espera conexiones secuestradas** (WebSockets, hijack), hay que
trackearlas aparte; y si el handler ignora `r.Context()`, el apagado espera igual hasta el
timeout.

## Paso 6 — `conLogging`

```go
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func conLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"metodo", r.Method,
			"ruta", r.URL.Path,
			"status", rec.status,
			"duracion_ms", time.Since(inicio).Milliseconds(),
		)
	})
}
```

Es el mismo middleware del capítulo 13, con `slog` en vez de `log`.

- **`statusRecorder`** envuelve el `ResponseWriter` para capturar el status, porque la interfaz
  no tiene un getter. Embebe el original, así todos los demás métodos siguen disponibles.
- **`status: http.StatusOK`** es el default correcto: si el handler nunca llama `WriteHeader`,
  el servidor responde 200.
- **`WriteHeader`** guarda el código **antes** de delegar. Si lo guardara después, un panic en
  el original perdería el dato.
- **`time.Since(inicio).Milliseconds()`** da la duración total, incluido el handler.
- **El log se emite después**, por eso conoce el status final y la duración.
- **No hay recuperación de panics acá.** Si un handler hace panic, `net/http` corta esa
  conexión y loguea el stack (capítulo 14). En un servicio real se agrega
  `ConRecuperacion` a la cadena, antes de este logger, para devolver un 500.

Límite conocido: este recorder no implementa `http.Flusher` ni `http.Hijacker`. Para SSE o
WebSockets hay que implementarlos y delegar. Para JSON normal, no importa.

## Paso 7 — El Dockerfile

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/servidor ./18-produccion

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/servidor /servidor
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/servidor"]
```

Línea por línea:

- **`# syntax=docker/dockerfile:1`** habilita la sintaxis moderna del builder. Es un comentario
  especial, no un comentario común.
- **`FROM ... AS build`** es la primera etapa: tiene compilador y código fuente, pero **no** va
  a la imagen final.
- **`COPY go.mod go.sum ./` + `RUN go mod download`** copian solo los módulos primero. Docker
  cachea esa capa: si cambiás código pero no dependencias, no vuelve a descargar todo. Copiar
  todo antes de `go mod download` desperdicia la caché.
- **`COPY . .`** copia el resto del módulo. El contexto de build es la **raíz del módulo**, no
  la carpeta del capítulo: por eso `go.sum` está ahí y el paquete se referencia como
  `./18-produccion`.
- **`CGO_ENABLED=0`** produce un binario estático, sin dependencia de la libc. Es lo que permite
  correr en `distroless/static` o `scratch`. Acá funciona porque SQLite es `modernc.org/sqlite`
  (puro Go); un driver con C, como `mattn/go-sqlite3`, requeriría CGO y otra imagen base.
- **`-trimpath`** quita las rutas absolutas de tu máquina del binario. Más reproducible y no
  filtra tu estructura de carpetas.
- **`-ldflags="-s -w"`** quita la tabla de símbolos y la información de debug: el binario pesa
  menos. El costo es que los stack traces pierden números de línea; si necesitás depurar
  producción con precisión, guardá una build con símbolos aparte.
- **`FROM gcr.io/distroless/static-debian12:nonroot`** es la imagen final: solo el binario y lo
  mínimo (CA certificates, zona horaria). No tiene shell, ni `apt`, ni `curl`. Menos superficie
  de ataque y menos CVEs. A cambio, no podés hacer `docker exec sh`; se depura con logs y
  contenedores efímeros.
- **`USER nonroot:nonroot`** corre sin privilegios. Si el proceso se compromete, no es root.
- **`ENTRYPOINT ["/servidor"]`** en forma **exec** (array JSON). El proceso es PID 1 y recibe
  SIGTERM directamente. Con la forma shell (`ENTRYPOINT /servidor`) habría un shell en el medio
  que puede no reenviar la señal, y el apagado ordenado no funcionaría.

Construir y correr (con el contexto en la raíz del módulo):

```bash
docker build -f 18-produccion/Dockerfile -t tutorialgo-18 .
docker run --rm -p 8080:8080 tutorialgo-18
```

`docker stop` manda SIGTERM y espera; ahí ves el apagado ordenado en los logs. Si el daemon de
Docker no está corriendo, el comando falla antes de construir: iniciá Docker Desktop.

## Cómo probarlo

```bash
go run ./18-produccion
```

En otra terminal:

```bash
curl.exe -i http://localhost:8080/salud
curl.exe -i http://localhost:8080/listo
curl.exe -i http://localhost:8080/trabajo
```

```text
HTTP/1.1 200 OK
Content-Type: text/plain; charset=utf-8
Content-Length: 2

ok
```

```text
HTTP/1.1 200 OK
Content-Type: text/plain; charset=utf-8
Content-Length: 5

listo
```

```text
HTTP/1.1 200 OK
Content-Type: text/plain; charset=utf-8
Content-Length: 18

trabajo terminado
```

`/trabajo` tarda 2 segundos: `curl` se queda esperando. Ese es el request que vas a usar para
probar el apagado: lanzalo, y mientras corre mandá Ctrl+C en la terminal del servidor.

Los logs reales de esas tres llamadas:

```json
{"time":"2026-11-14T23:52:30.5267597-06:00","level":"INFO","msg":"servidor iniciado","addr":":8080"}
{"time":"2026-11-14T23:52:31.1540864-06:00","level":"INFO","msg":"request","metodo":"GET","ruta":"/salud","status":200,"duracion_ms":0}
{"time":"2026-11-14T23:52:31.1886784-06:00","level":"INFO","msg":"request","metodo":"GET","ruta":"/listo","status":200,"duracion_ms":0}
{"time":"2026-11-14T23:52:33.2122192-06:00","level":"INFO","msg":"request","metodo":"GET","ruta":"/trabajo","status":200,"duracion_ms":2000}
```

Fijate en `duracion_ms: 2000` para `/trabajo` y `0` para las otras. El campo es un número, no un
string: eso es lo que hace que valga la pena el log estructurado.

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| el puerto sigue ocupado al reiniciar | el proceso anterior no murió | matalo, o revisá que `Shutdown` termine |
| el deploy corta requests | no hay `Shutdown`, o el `ENTRYPOINT` es shell | usá `Shutdown` y forma exec |
| el proceso no se apaga nunca | un handler ignora el context y tarda más que el timeout | deadline propio y `Shutdown` con techo |
| Kubernetes reinicia el pod en loop | `/salud` consulta la base | liveness sin dependencias |
| el balanceador manda tráfico a un pod sin base | `/listo` no consulta nada | readiness con `PingContext` |
| `http: superfluous response.WriteHeader` | escribiste el status dos veces | un solo `WriteHeader` |
| conexiones agotadas sin tráfico | falta `ReadHeaderTimeout` | ponelo |
| el cliente ve la respuesta cortada | `WriteTimeout` menor que el handler | subilo o acortá el handler |
| `!BADKEY` en un log | pasaste un argumento impar a `slog.Info` | revisá los pares clave-valor |
| `exec format error` en el contenedor | binario para otra arquitectura | `--platform linux/amd64` o build multi-arch |
| `no such file or directory` al correr en distroless | binario dinámico | `CGO_ENABLED=0` |
| no podés entrar al contenedor | distroless no tiene shell | logs + contenedor de debug |

## Por qué importa en el trabajo

- **Los logs JSON son la diferencia entre `grep` y una consulta.** "Todos los requests con
  status 500 en la última hora" es un filtro, no una expresión regular.
- **Liveness y readiness mal separados** causan incidentes en cascada: un pod reiniciando no
  arregla una base caída.
- **Los timeouts** son la defensa más barata contra clientes lentos y ataques de agotamiento.
- **El apagado ordenado** es lo que permite deployar sin errores para los usuarios. Sin esto,
  cada release tira requests en curso.
- **Una imagen distroless nonroot** reduce el impacto de una vulnerabilidad en el proceso: no
  hay shell, no hay root, no hay herramientas para el atacante.

## Lo que falta para producción de verdad

Este capítulo cubre lo mínimo operativo, no todo. En un servicio real se agrega:

- **Métricas** (Prometheus/OpenTelemetry) y **tracing** distribuido.
- **Request ID** en logs y respuestas (capítulo 14) para correlacionar.
- **Recuperación de panics** como middleware (capítulo 14) y errores JSON consistentes.
- **Rate limiting**, autenticación (capítulo 16) y CORS.
- **Pool de base de datos** configurado según el motor (capítulo 15).
- **Migraciones** versionadas, backups y pruebas de restore.
- **TLS** en el proxy o en el servidor; el binario no lo trae.
- **CI/CD** con `go test -race`, `go vet`, análisis de vulnerabilidades y build reproducible.
- **`.dockerignore`** para no copiar bases, binarios y archivos temporales al contexto.

## Ejercicio

1. Conectá `LOG_NIVEL` del capítulo 17 con `slog.HandlerOptions.Level`: `debug` → `LevelDebug`,
   `info` → `LevelInfo`, `warn` → `LevelWarn`, `error` → `LevelError`. Verificá que un
   `slog.Debug` no aparece con `info`.
2. Agregá un `ConRecuperacion` (capítulo 14) a la cadena, **antes** de `conLogging`. Probá con
   una ruta que haga panic y comprobá que el cliente recibe 500 y el log tiene el panic.
3. Hacé que `/listo` consulte una base SQLite real con `PingContext` y devuelva 503 si está
   cerrada. Escribí un test que cierre la base y verifique el 503.
4. Hacé que `/trabajo` respete `r.Context()`: si el cliente corta, debe terminar antes de los
   2 s. Medilo con `curl --max-time 1` y mirá el log.
5. Escribí un `Dockerfile` multi-arquitectura y agregá un `.dockerignore` que excluya `*.db`,
   `*.jsonl`, `*.exe` y `.git`. Construí con `docker buildx build --platform linux/amd64,linux/arm64`.
6. Agregá `ReplaceAttr` para que `time` salga en UTC y verificá que cambia el offset del log.

Fin del recorrido. El [proyecto final](../proyecto/) junta todo esto en un solo servicio.
