# 01 — Hola mundo

**Qué vas a construir:** un programa que saluda, acepta el nombre por la terminal y
devuelve un código de salida que un script puede interpretar. Ese es el esqueleto de
cualquier herramienta de línea de comandos.

**Qué necesitas antes:** nada de Go. Si ya programaste en otro lenguaje, mejor: vas a ver
qué hace Go distinto y por qué.

---

## Paso 0 — El programa más chico que existe

Creá un archivo `main.go` con esto y ya tenés un programa:

```go
package main

func main() {}
```

```bash
go run ./01-hola-mundo
```

No imprime nada, pero compila, se ejecuta y termina bien. Dos líneas, dos ideas:

- **`package main`** significa "este archivo produce un ejecutable". Si el paquete se
  llamara `saludo`, sería una librería que otros importan, no algo que se corre.
- **`func main()`** es el punto de entrada. El sistema operativo arranca el binario y Go
  llama a esta función. No recibe parámetros ni devuelve nada.

Fijate en lo que **no** hay: ninguna clase que envuelva la función, ningún `static`,
ninguna firma especial. `main` dentro de `package main` es toda la regla. Por eso un
programa Go se lee de arriba abajo, sin saltar entre archivos para entender dónde arranca.

## Paso 1 — Imprimir

```go
package main

import "fmt"

func main() {
	fmt.Println("Hola, mundo")
}
```

```bash
go run ./01-hola-mundo
# Hola, mundo
```

`fmt` es el paquete de la librería estándar que formatea y escribe texto. El `import` es
lo que hace que el nombre `fmt` exista en tu archivo. Y acá aparece la primera diferencia
fuerte con otros lenguajes: **Go no compila si importás algo que no usás**, y tampoco si
declarás una variable que no usás.

```text
./main.go:4:2: "fmt" imported and not used
```

No es un capricho: si el import está, es porque se usa; si la variable está, es porque
alguien la lee. Eso elimina una clase entera de ruido al revisar código ajeno. Acostumbrate
a que el compilador sea estricto: te está ahorrando una discusión en el *code review*.

## Paso 2 — Leer lo que te pasan por la terminal

Cuando ejecutás `go run ./01-hola-mundo Ana`, la palabra `Ana` llega al programa. ¿Dónde?
En `os.Args`, un **slice de strings** con todo lo que se escribió en la línea de comandos.

Para verlo, imprimilo entero:

```go
import (
	"fmt"
	"os"
)

func main() {
	for i, a := range os.Args {
		fmt.Printf("%d = %q\n", i, a)
	}
}
```

```bash
go run ./01-hola-mundo Ana María
# 0 = "C:\\Users\\...\\01-hola-mundo.exe"
# 1 = "Ana"
# 2 = "María"
```

Leé ese resultado con cuidado, porque explica el `[1:]` que vas a ver en todo el código Go:

- El índice **0** no es un argumento: es la ruta del ejecutable. `go run` compila a un
  binario temporal y lo ejecuta, por eso ves esa ruta y no `go`.
- Tus datos empiezan en el índice **1**.
- `Ana María` sin comillas se parte en dos argumentos porque el shell separa por espacios.
  Si querés una sola palabra con espacio adentro, va entre comillas: `"Ana María"`.

Por eso el corte es siempre `args := os.Args[1:]`: "todo menos el nombre del programa".

`%d` y `%q` en ese `Printf` son **verbos de formato**: huecos que se rellenan con los
argumentos, en orden. `%d` es un entero, `%q` es un string entre comillas (útil justamente
para ver si hay espacios). La tabla completa está en
[verbos de formato](../00-empezar/verbos-formato.md); por ahora alcanza con `%s`, `%d`,
`%q` y `\n`, que es un salto de línea.

## Paso 3 — Default + override

Un programa útil no puede exigir argumentos para funcionar. Si no te pasan nada, usa un
valor por defecto; si te pasan algo, eso gana:

```go
args := os.Args[1:]

nombre := "mundo"
if len(args) > 0 {
	nombre = strings.Join(args, " ")
}
```

- `nombre := "mundo"` es el **default**.
- `if len(args) > 0` pregunta "¿me pasaron algo?" antes de leer `args[0]`. Si lo leyeras
  sin preguntar, con la lista vacía el programa explota con `index out of range`. Ese es
  el error número uno al empezar.
- `strings.Join(args, " ")` une todas las palabras con un espacio. Sin eso, saludarías
  solo a `Ana` y perderías `María`.

Este patrón —**default, override si viene algo**— se repite en toda configuración de
servidor: `PORT=8080` por defecto, `$PORT` si está definida.

## Paso 4 — Salida con formato

```go
fmt.Printf("Hola, %s!\n", nombre)
```

```bash
go run ./01-hola-mundo Ana
# Hola, Ana!
```

Tres detalles chicos que importan:

- `%s` es el hueco para un string; el segundo argumento (`nombre`) lo llena.
- `\n` es el salto de línea. `Printf` **no** lo agrega solo; `Println` sí.
- Si te equivocás de verbo (`%d` con un string), el programa compila igual y en pantalla
  aparece `%!d(string=Ana)`. El error no es de compilación, es de salida: por eso conviene
  saber qué verbo va con qué tipo.

La diferencia entre las tres funciones, en una tabla mental:

| Función | Separa con | Agrega salto | Usa verbos |
|---|---|---|---|
| `fmt.Print(a, b)` | Nada | No | No |
| `fmt.Println(a, b)` | Espacio | Sí | No |
| `fmt.Printf(fmt, a, b)` | Lo que digas vos | Solo si ponés `\n` | Sí |

Regla práctica: `Println` para mensajes sueltos y depuración rápida, `Printf` cuando
necesitás control del formato.

## Paso 5 — Errores a `stderr` y código de salida

Un programa no solo imprime: también le dice al sistema si terminó bien o mal. Eso es el
**exit code**: `0` es éxito, cualquier otro número es fallo.

```go
if nombre == "error" {
	fmt.Fprintln(os.Stderr, "pediste un error a propósito")
	os.Exit(1)
}
```

```bash
go run ./01-hola-mundo error
# Hola, error!
# pediste un error a propósito
echo $LASTEXITCODE   # PowerShell: 1
```

La primera línea salió antes del `if`: el saludo se imprime siempre. Lo que cambia al
final es el exit code.

Dos piezas:

- **`fmt.Fprintln(os.Stderr, ...)`** escribe en la salida de errores, no en la normal. La
  diferencia se ve cuando redirigís: `mi-cli > resultado.txt` guarda solo la salida normal,
  y los errores siguen apareciendo en pantalla. Es la convención de todo Unix (y de tus
  scripts de CI).
- **`os.Exit(1)`** termina el proceso en ese instante. Ojo: **no ejecuta los `defer`**.
  Es una salida brusca, a propósito. En un servicio real casi nunca lo usás: devolvés el
  error hacia arriba y dejás que `main` decida (lo ves en el capítulo 04).

Que el exit code sea 1 es lo que permite a un script hacer `if ! mi-cli; then ...`, y a
Docker o Kubernetes marcar el contenedor como fallido y reiniciarlo.

## Todo junto

Este es el programa completo, el mismo que está en `main.go`:

```go
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]

	nombre := "mundo"
	if len(args) > 0 {
		nombre = strings.Join(args, " ")
	}

	fmt.Printf("Hola, %s!\n", nombre)

	if nombre == "error" {
		fmt.Fprintln(os.Stderr, "pediste un error a propósito")
		os.Exit(1)
	}
}
```

Ya no hay nada nuevo. Si algo no te cierra, volvé al paso que lo introduce: los cinco
pasos son literalmente estas líneas en orden.

## Ejecutar

```bash
go run ./01-hola-mundo
go run ./01-hola-mundo Ana
go run ./01-hola-mundo Ana María
go run ./01-hola-mundo error; echo $LASTEXITCODE   # PowerShell
```

## Errores que vas a ver

| Mensaje | Qué pasó | Solución |
|---|---|---|
| `"fmt" imported and not used` | Importaste y no lo usás | Borrá el import |
| `declared and not used: args` | Variable creada y nunca leída | Usala o borrala (`_ = args` no, arreglá el código) |
| `index out of range [0]` | Leíste `os.Args[1]` sin argumentos | Preguntá `len(args) > 0` antes |
| `%!d(string=Ana)` | Verbo de número con string | Usá `%s` |
| `Hola, Ana!PS C:\>` | `Printf` sin `\n` | Agregá el salto |

## Por qué esto importa en el trabajo

Casi todas las herramientas internas que vas a escribir son esto: reciben argumentos,
tienen defaults, imprimen resultado a `stdout`, errores a `stderr` y devuelven un exit
code. Un CLI de migraciones, un *seeder*, un comando de deploy. El resto de Go es más
sofisticado, pero el esqueleto no cambia.

## Ejercicio

1. Agregá un argumento `--mayus` que imprima el saludo en mayúsculas sin agregar
   librerías (pista: `strings.ToUpper`, y `strings.HasPrefix` para detectar la bandera).
2. Hacé que sin argumentos imprima `Hola, mundo!` y, si le pasás `mundo`, lo mismo.
   Después cambiá el default a tu nombre y verificá que solo cambia una línea.

Siguiente: [02 — Variables y tipos](../02-tipos-variables/).
