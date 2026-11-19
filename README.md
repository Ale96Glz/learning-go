# Go desde cero, hasta backend real

Curso práctico en español. Cada carpeta es un capítulo con su explicación (`README.md`)
y código que **compila y se ejecuta**. No hay fragmentos sueltos: todo lo que lees lo
puedes correr.

Pensado para alguien que ya programa en otro lenguaje (Java, JS, Python...) y quiere
Go aplicado a trabajo real: APIs, routers, base de datos, tests, configuración y deploy.

---

## Requisitos

- Go 1.22 o superior (el tutorial usa `go 1.26`). Verifica con `go version`.
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
> `tutorialgo`, no la carpeta.

---

## Índice

| # | Capítulo | Qué aprendes | Aplicación real |
|---|----------|--------------|-----------------|
| 00 | [Empezar](00-empezar/) | módulos, `run`, `build`, `test`, `fmt` | armar un repo Go |
| 01 | [Hola mundo](01-hola-mundo/) | `package`, `import`, `func main`, `os.Args` | CLI mínima |
| 02 | [Variables y tipos](02-tipos-variables/) | tipos, zero values, constantes, conversión | leer config del entorno |
| 03 | [Control de flujo](03-control-flujo/) | `if`, `switch`, `for`, `range` | validar entrada, reintentos |
| 04 | [Funciones y errores](04-funciones-errores/) | múltiples retornos, `error`, wrapping | validación con errores tipados |
| 05 | [Structs y métodos](05-structs-metodos/) | structs, punteros, constructores, tags | modelos de dominio |
| 06 | [Interfaces](06-interfaces/) | interfaces chicas, `io.Writer`, inyección | repositorio intercambiable |
| 07 | [Slices y maps](07-slices-maps/) | `append`, filtros, `map`, `sort` | listar y agrupar datos |
| 08 | [Paquetes](08-paquetes/) | `internal/`, exportado vs privado | separar capas |
| 09 | [Archivos y JSON](09-archivos-json/) | `os`, `encoding/json`, escritura atómica | persistencia simple |
| 10 | [Testing](10-testing/) | tests de tabla, `t.Helper`, `go test -race` | tests que sirven |
| 11 | [Concurrencia](11-concurrencia/) | goroutines, channels, `WaitGroup`, worker pool | procesar en paralelo |
| 12 | [Context](12-context/) | timeout, cancelación, propagación | llamadas externas |
| 13 | [HTTP básico](13-http-basico/) | server, handlers, JSON, `httptest` | API en memoria |
| 14 | [Routers y middleware](14-router-middleware/) | params de ruta, subrouters, middlewares | API versionada |
| 15 | [Base de datos](15-base-datos/) | `database/sql`, SQLite, transacciones, migraciones | repositorio SQL |
| 16 | [Auth](16-auth/) | bcrypt, tokens, middleware Bearer | login real |
| 17 | [Config por entorno](17-config-entorno/) | env vars, defaults, validación | 12-factor |
| 18 | [Producción](18-produccion/) | `slog`, graceful shutdown, Docker | deploy |
| — | [**Proyecto**](proyecto/) | todo junto | API de tareas con auth y SQLite |

## Qué NO incluye (a propósito)

gRPC, ORMs, frameworks web (Gin/Echo/chi en el código), Kubernetes, generics avanzados,
frontend. Todo eso se aprende después; meterlo acá hace el tutorial más largo y más
frágil. En los capítulos donde aplica se explica **qué** usarías en el trabajo y **por qué**.

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

Empieza por [00-empezar](00-empezar/).
