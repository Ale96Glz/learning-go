# Go desde cero, hasta backend real

Curso práctico en español. Cada carpeta es un capítulo con su explicación (`README.md`)
y código que **compila y se ejecuta**. No hay fragmentos sueltos: todo lo que lees lo
puedes correr.

Pensado para alguien que ya programa en otro lenguaje (Java, JS, Python...) y quiere
Go aplicado a trabajo real: APIs, routers, base de datos, tests, configuración y deploy.

---

## Requisitos

- Go 1.26 o superior (`go.mod` declara `go 1.26.0`). Verifica con `go version`.
- Un editor con soporte de Go (VS Code + extensión oficial, o GoLand).
- Terminal en esta carpeta (`tutorial go`).

## Cómo ejecutar

```bash
# primera vez: descarga dependencias
go mod tidy

# correr un capítulo
go run ./01-hola-mundo
go run ./13-http-basico

# correr todo lo que tiene tests
go test ./...

# formatear y revisar todo
gofmt -l .
go vet ./...
```

> En Windows, si la ruta tiene espacios funciona igual: el módulo se llama
> `tutorialgo`, no la carpeta. En PowerShell, `curl` es un alias de `Invoke-WebRequest`:
> usá `curl.exe` para los ejemplos.

## Arranca el proyecto final

```bash
go run ./proyecto
curl.exe -i http://localhost:8080/salud
go test ./...
```

La API queda en `http://localhost:8080`, crea `tareas.db` en la raíz y arranca sin
configurar nada. El recorrido completo (registro → login → tareas) y las variables de
entorno están en [proyecto/README.md](proyecto/README.md).

| Síntoma | Solución |
|---|---|
| `go: cannot find main module` | Ejecuta desde la raíz de `tutorial go` |
| `bind: Only one usage of each socket address` | El puerto 8080 está ocupado; cambia `PORT` |
| `no required module provides package` | Corre `go mod tidy` |

---

## Índice

La ruta es lineal: 00 → 18 y después el proyecto. Si ya conoces los fundamentos, entra
directo en [13 — HTTP básico](13-http-basico/).

| # | Capítulo | Qué aprendes | Aplicación real |
|---|----------|--------------|-----------------|
| 00 | [Empezar](00-empezar/) | Módulos, `run`, `build`, `test`, `fmt` | Armar un repo Go |
| 01 | [Hola mundo](01-hola-mundo/) | `package`, `import`, `func main`, `os.Args` | CLI mínima |
| 02 | [Variables y tipos](02-tipos-variables/) | Tipos, zero values, constantes, conversión | Leer config del entorno |
| 03 | [Control de flujo](03-control-flujo/) | `if`, `switch`, `for`, `range` | Validar entrada, reintentos |
| 04 | [Funciones y errores](04-funciones-errores/) | Múltiples retornos, `error`, wrapping | Validación con errores tipados |
| 05 | [Structs y métodos](05-structs-metodos/) | Structs, punteros, constructores, tags | Modelos de dominio |
| 06 | [Interfaces](06-interfaces/) | Interfaces chicas, `io.Writer`, inyección | Repositorio intercambiable |
| 07 | [Slices y maps](07-slices-maps/) | `append`, filtros, `map`, `sort` | Listar y agrupar datos |
| 08 | [Paquetes](08-paquetes/) | `internal/`, exportado vs privado | Separar capas |
| 09 | [Archivos y JSON](09-archivos-json/) | `os`, `encoding/json`, escritura atómica | Persistencia simple |
| 10 | [Testing](10-testing/) | Tests de tabla, `t.Helper`, `go test -race` | Tests que sirven |
| 11 | [Concurrencia](11-concurrencia/) | Goroutines, channels, `WaitGroup`, worker pool | Procesar en paralelo |
| 12 | [Context](12-context/) | Timeout, cancelación, propagación | Llamadas externas |
| 13 | [HTTP básico](13-http-basico/) | Server, handlers, JSON, `httptest` | API en memoria |
| 14 | [Routers y middleware](14-router-middleware/) | Params de ruta, subrouters, middlewares | API versionada |
| 15 | [Base de datos](15-base-datos/) | `database/sql`, SQLite, transacciones, migraciones | Repositorio SQL |
| 16 | [Auth](16-auth/) | bcrypt, tokens, middleware Bearer | Login real |
| 17 | [Config por entorno](17-config-entorno/) | Env vars, defaults, validación | 12-factor |
| 18 | [Producción](18-produccion/) | `slog`, graceful shutdown, Docker | Deploy |
| — | [**Proyecto**](proyecto/) | Todo junto | API de tareas con auth y SQLite |

## Lo que queda afuera (y cuándo sumarlo)

Acá todo se hace con la librería estándar a propósito: primero ves cómo funciona cada
pieza y después podés usar una herramienta que lo haga por vos sin que te quede oculto
qué hace.

| En vez de... | Acá usamos | Lo sumás cuando... |
|---|---|---|
| Gin, Echo, chi | `net/http` y `ServeMux` | Necesites binding, validación o middleware de terceros |
| GORM, sqlc | `database/sql` y SQL a mano | El esquema crezca y convenga generar código o migraciones |
| gRPC | HTTP + JSON | Haya comunicación entre servicios con contratos estrictos y baja latencia |
| Kubernetes | Docker y `docker run` | Tengas varios servicios que orquestar |
| Generics avanzados | Interfaces chicas | El mismo código se repita para tipos distintos |
| Frontend | Respuestas JSON | Haya un cliente que consuma la API |

No es una lista de herramientas prohibidas: es el orden en que conviene aprenderlas.
Cuando las uses en el trabajo, vas a saber qué problema resuelven y qué cambias por esa
comodidad.

## Convenciones

- Código idiomático: errores como valores, interfaces chicas, sin `panic` para flujo normal.
- Comentarios solo para el *porqué*, no para repetir el código.
- Cada capítulo termina con un ejercicio corto. Hazlo: leer no enseña Go, escribir sí.

## Los `%v`, `%t` y demás verbos

En los ejemplos vas a ver `fmt.Printf("%s tiene %d años\n", nombre, edad)`. Ese `%s` y
ese `%d` son **verbos de formato**: huecos que `fmt` rellena con los argumentos en orden.
`%v` es el comodín para cualquier valor y `%t` es para booleanos. La referencia completa,
con tabla y errores típicos, está en
[00-empezar/verbos-formato.md](00-empezar/verbos-formato.md).

## Para seguir aprendiendo

Los recursos oficiales primero: cuando algo no cuadre, ellos mandan. El resto son atajos
para practicar o para entender el *por qué* detrás de una decisión.

### Documentación oficial

| Recurso | Cuándo abrirlo |
|---|---|
| [Documentación](https://go.dev/doc/) | Índice de todo lo oficial: instalación, módulos, testing, diagnóstico |
| [A Tour of Go](https://go.dev/tour/) | Recorrido interactivo de sintaxis, interfaces y concurrencia |
| [Tutoriales](https://go.dev/doc/tutorial/) | Guías paso a paso: módulos, errores, JSON, base de datos, generics, fuzzing, `govulncheck` |
| [How to Write Go Code](https://go.dev/doc/code) | Cómo organizar paquetes dentro de un módulo y usar el comando `go` |
| [Effective Go](https://go.dev/doc/effective_go) | Convenciones de estilo; aviso: es de 2009 y no cubre generics ni módulos |
| [Code Review Comments](https://go.dev/wiki/CodeReviewComments) | El detalle de estilo que falta en *Effective Go*: errores, interfaces, nombres |
| [pkg.go.dev](https://pkg.go.dev/) | Documentación de la librería estándar y de cualquier módulo |
| [Especificación del lenguaje](https://go.dev/ref/spec) | La definición exacta, para cuando hay duda |
| [Go Playground](https://go.dev/play/) | Probar código en el navegador sin instalar nada |
| [Blog](https://go.dev/blog/) y [charlas grabadas](https://go.dev/talks/) | Decisiones de diseño y conferencias del equipo de Go |
| [Wiki: Learn](https://go.dev/wiki/Learn) | Listado comunitario de cursos y libros |

### Práctica

| Recurso | Formato |
|---|---|
| [Go by Example](https://gobyexample.com/) | Ejemplos cortos y anotados; sirve de hoja de referencia |
| [Learn Go with Tests](https://quii.gitbook.io/learn-go-with-tests/) | Libro gratis: se aprende Go escribiendo tests (TDD) |
| [Exercism — track de Go](https://exercism.org/tracks/go) | 168 ejercicios con mentoría humana gratuita |
| [Gophercises](https://gophercises.com/) | ~20 mini-proyectos en video, con foco en stdlib y concurrencia |
| [inancgumus/learngo](https://github.com/inancgumus/learngo) | Más de 1000 ejemplos, ejercicios y quizzes para corregir |

### Video

| Recurso | Formato |
|---|---|
| [Curso de Go desde cero (freeCodeCamp en español)](https://www.freecodecamp.org/espanol/news/aprende-go-curso-de-golang-desde-cero) | Curso completo en español, gratis |
| [Go Programming – Full Course (Tech With Tim)](https://www.youtube.com/watch?v=V-lI7AmusGs) | Curso en inglés, de sintaxis a concurrencia |

Un orden que funciona: el [Tour](https://go.dev/tour/) antes del capítulo 01, *Learn Go with
Tests* al llegar al capítulo 10, y *Code Review Comments* + las charlas oficiales al terminar
el [proyecto](proyecto/).

Empieza por [00-empezar](00-empezar/).
