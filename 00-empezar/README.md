# 00 — Empezar

Antes de escribir código, entiende cómo Go organiza un proyecto. Es distinto a Java o
Node y es la causa del 80% de los errores del primer día.

## 1. Verifica la instalación

```bash
go version
# go version go1.26.1 windows/amd64
```

Si no aparece, instala desde https://go.dev/dl/ y abre una terminal nueva.

## 2. Qué es un módulo

Un **módulo** es la unidad de dependencias: una carpeta con un `go.mod` que declara su
nombre y su versión de Go.

```bash
cat go.mod
# module tutorialgo
# go 1.26.0
```

- `module tutorialgo` es el **prefijo de import** de todo el proyecto.
- Los imports internos se escriben con ese prefijo: `tutorialgo/08-paquetes/internal/saludo`.
- No hay `node_modules` ni `vendor` por defecto: las dependencias viven en la caché
  global de Go (`go env GOMODCACHE`).

En el trabajo normalmente verás `module github.com/empresa/servicio`. El nombre no
necesita existir, pero si publicas el repo conviene que coincida con su URL.

## 3. Comandos que vas a usar todos los días

| Comando | Qué hace |
|---|---|
| `go run ./01-hola-mundo` | compila y ejecuta (no deja binario) |
| `go build ./...` | compila todo; falla si algo no compila |
| `go test ./...` | corre todos los tests |
| `go vet ./...` | detecta errores sospechosos (formato de `Printf`, locks copiados...) |
| `gofmt -w .` | formatea; **no se discute el estilo** |
| `go mod tidy` | sincroniza `go.mod`/`go.sum` con lo que importas |
| `go doc fmt.Println` | documentación en la terminal |

`go build` con un `package main` deja un `.exe` (Windows) en la carpeta actual. Para
producción se usa `go build -o bin/servidor ./proyecto`.

## 4. Estructura típica de un servicio

```text
servicio/
├── go.mod
├── main.go                 # package main: solo arranca cosas
├── internal/               # código privado del módulo
│   ├── config/
│   ├── tareas/
│   └── auth/
└── Dockerfile
```

Regla mental: **`main` arma el programa, `internal` hace el trabajo**. Si tu `main.go`
tiene 600 líneas de lógica, está mal. Lo vas a ver aplicado en el [proyecto](../proyecto/).

## 5. Un archivo Go, de arriba a abajo

```go
package main // 1. a qué paquete pertenece este archivo

import "fmt" // 2. qué necesita

func main() { // 3. punto de entrada del ejecutable
	fmt.Println("hola")
}
```

- `package main` + `func main()` = programa ejecutable.
- Un paquete es una carpeta. Todos los `.go` de una carpeta deben declarar el mismo paquete.
- El import va **sin usar** es error de compilación, no warning. Go no perdona imports muertos.

## 6. Imprimir: `Print`, `Println`, `Printf` y los verbos

Ya vas a ver `fmt.Printf("...%s...%d...", x, y)`. Entender eso ahora te ahorra horas:

```go
fmt.Print("a", "b")                 // ab        (sin salto)
fmt.Println("a", 1, true)           // a 1 true  (con espacios y salto)
fmt.Printf("Hola, %s (%d)\n", "Ana", 30) // Hola, Ana (30)  (control total)
```

- `%s` = string, `%d` = entero, `%t` = bool, `%v` = "lo que sea".
- `%` no es de Go: es de `fmt`. Los argumentos llenan los huecos **en orden**.
- `Printf` **no** agrega salto de línea: ponlo tú con `\n`.
- `%v` es el comodín: `fmt.Printf("%v\n", []int{1,2})` imprime `[1 2]`.
- `%+v` es igual pero muestra los nombres de campo de un struct.
- `%w` no se usa acá: solo en `fmt.Errorf` para envolver errores (capítulo 04).

La tabla completa (con `%q`, `%T`, `%.2f`, `%-20s`, errores como `%!d(string=...)`) está
en [verbos-formato.md](verbos-formato.md). Vale la pena leerla antes del capítulo 01.

## 7. Errores comunes al empezar

| Error | Causa |
|---|---|
| `no required module provides package ...` | falta `go mod tidy` o el import está mal |
| `declared and not used` | variable sin usar; usa `_` o bórrala |
| `imported and not used` | import sin usar; bórralo |
| `undefined: X` | X está en otro paquete y no lo exportaste (minúscula) |
| `cannot use ... as ...` | tipos distintos; Go no convierte solo |

## Ejercicio

1. `go run ./01-hola-mundo`
2. Rompe el código a propósito: borra un `fmt.Println` y deja el import. Corre
   `go build ./01-hola-mundo` y lee el error.
3. Arregla con `gofmt -w .` y compila de nuevo.

Siguiente: [01 — Hola mundo](../01-hola-mundo/).
