# 02 — Variables y tipos

**Qué vas a construir:** leer configuración del entorno, convertirla al tipo correcto y
armar un struct de config con valores por defecto. Es lo primero que hace cualquier
servicio al arrancar.

**Qué necesitas antes:** capítulo 01. Nada más.

---

## Paso 1 — Las dos formas de declarar una variable

Dentro de una función vas a usar casi siempre la forma corta:

```go
nombre := "Ana"
edad := 30
```

`:=` declara **y** asigna, y el tipo lo deduce el compilador del valor de la derecha. Es
el equivalente a `var nombre string = "Ana"`, pero sin repetir el tipo.

La otra forma es `var`, y sirve cuando querés el **zero value** (el valor inicial
automático) o cuando declarás algo que vas a llenar después:

```go
var activo bool    // false
var saldo float64  // 0
var alias string   // ""
var puntero *int   // nil
```

Esto es una diferencia grande con Java y C#. Ahí una variable local sin inicializar no se
puede usar, y un objeto sin asignar es `null` (una bomba). En Go **todo tiene un valor
inicial utilizable**, y ese valor depende del tipo:

| Tipo | Zero value |
|---|---|
| `int`, `int64`, `float64` | `0` |
| `string` | `""` (string vacío, no `null`) |
| `bool` | `false` |
| punteros, slices, maps, interfaces | `nil` |
| struct | un struct con todos sus campos en zero value |

Por eso en Go no existe `NullPointerException` al arrancar: un `string` nunca es null, un
`int` nunca es null. El precio es que `""` y `0` pueden significar "sin dato"; de eso te
ocupás con validación, no con punteros nulos por todos lados.

### El print, verbo por verbo

```go
fmt.Printf("%s (%d) activo=%t saldo=%.2f alias=%q puntero=%v\n",
	nombre, edad, activo, saldo, alias, puntero)
```

```text
Ana (30) activo=false saldo=0.00 alias="" puntero=<nil>
```

| Verbo | Argumento | Qué hace |
|---|---|---|
| `%s` | `nombre` | string tal cual |
| `%d` | `edad` | entero en base 10 |
| `%t` | `activo` | booleano |
| `%.2f` | `saldo` | float con 2 decimales (redondea) |
| `%q` | `alias` | string **entre comillas**: así ves que está vacío |
| `%v` | `puntero` | representación por defecto; un puntero nil se ve `<nil>` |

`%q` parece cosmético, pero es la forma de distinguir `""` de `" "`: con `%s` los dos se
ven igual (nada), con `%q` ves `""` y `" "`. Cuando depures strings, usalo.

## Paso 2 — No hay conversión implícita

```go
var precio int = 100
impuesto := 0.21
total := float64(precio) * (1 + impuesto)
```

En Java o C#, `precio * (1 + impuesto)` compila solo: el `int` se promueve a `double`
automáticamente. **En Go no.** Si escribís:

```go
total := precio * (1 + impuesto)
```

el error es:

```text
invalid operation: precio * (1 + impuesto) (mismatched types int and float64)
```

Tenés que convertir a mano con `float64(precio)`. Es más verboso, y es a propósito: una
conversión puede perder información y el compilador quiere que lo decidas vos, no que
ocurra en silencio. La conversión es explícita pero **no valida nada**: `int(3.9)` da `3`
(trunca), y `int64(unIntMuyGrande)` puede desbordar. Convertir no es lo mismo que validar.

Para pasar de texto a número no se usa un cast sino `strconv` (`strconv.Atoi`,
`strconv.ParseFloat`), porque puede fallar y por eso devuelve un `error`. Lo ves en el
paso 5.

## Paso 3 — Constantes e `iota`

```go
const (
	appName     = "tutorial-go"
	maxIntentos = 3
)
```

Una constante se evalúa en compilación y no se puede reasignar. No necesita tipo: `3` es
una constante **sin tipo**, y se adapta al contexto donde la uses (podés multiplicarla por
un `time.Duration` o pasarla a una función que pide `int64`). Eso es más flexible que en
Java/C#, donde una constante tiene un tipo fijo.

`iota` genera una secuencia:

```go
type Nivel int

const (
	Debug Nivel = iota // 0
	Info               // 1
	Error              // 2
)
```

Cada línea repite la expresión de la anterior, así que `Info` y `Error` también valen
`iota` (que aumenta en cada línea). Es el reemplazo de los `enum` de Java/C#, con una
diferencia: acá `Nivel` es un `int` de verdad, así que podés hacer aritmética con él (y
podés crear `Nivel(99)`, que no existe). El `enum` de Java es más cerrado; el de Go es más
liviano y menos seguro. Si querés el equivalente a `switch` exhaustivo, lo escribís a mano.

### `String()`: cómo hacer que un número se imprima con nombre

```go
func (n Nivel) String() string {
	niveles := [...]string{"DEBUG", "INFO", "ERROR"}
	if n < 0 || int(n) >= len(niveles) {
		return fmt.Sprintf("Nivel(%d)", int(n))
	}
	return niveles[n]
}
```

- `niveles := [...]string{...}` es un **array** de tamaño fijo (3). Los corchetes con
  `...` dejan que el compilador cuente los elementos. Acá funciona como tabla de búsqueda:
  `niveles[0]` es `"DEBUG"`.
- El `if` es la validación de rango: si `n` es `-1` o `99`, `niveles[n]` haría panic
  (`index out of range`). Como `Nivel` es un `int`, cualquiera puede crear un valor
  inválido; el método se defiende.
- `fmt.Sprintf` es como `Printf` pero **devuelve** el string en vez de imprimirlo. Por eso
  se puede retornar.
- `int(n)` es necesario porque `len()` devuelve `int` y no se puede comparar con `Nivel`
  sin convertir (volvemos al paso 2).

Con ese método, `fmt` imprime el nombre:

```go
nivel := Info
fmt.Printf("nivel %d = %s\n", nivel, nivel)
// nivel 1 = INFO
```

Leé bien esa línea, porque es sutil: **el mismo valor impreso dos veces da dos cosas
distintas**. `%d` imprime el número (`1`), porque pediste un entero. `%s` y `%v` llaman a
`String()` e imprimen `INFO`. Es decir: el verbo decide si se usa `String()` o no. Si
querés el nombre, `%s` o `%v`; si querés el número, `%d`.

## Paso 4 — `time.Duration` y las constantes sin tipo

```go
const segundos = 30
timeout := segundos * time.Second
fmt.Printf("timeout: %v\n", timeout)
// timeout: 30s
```

`time.Duration` es un `int64` que cuenta **nanosegundos**, pero tiene `String()` y por eso
`%v` lo muestra legible (`30s`, `1h30m0s`). Si usaras `%d`, verías `30000000000`:

```go
fmt.Printf("%d\n", timeout) // 30000000000
```

Ese es un error clásico al loguear timeouts: el número no dice nada y parece un bug. Usá
`%v` o `%s` para `time.Duration`.

## Paso 5 — Leer el entorno (string siempre, default tuyo)

Las variables de entorno son **siempre strings**. No hay enteros ni booleanos: si querés
un `int`, lo convertís y manejás el error.

```go
puerto := 8080
if v := os.Getenv("PORT"); v != "" {
	p, err := strconv.Atoi(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "PORT inválido (%q): %v\n", v, err)
		os.Exit(1)
	}
	puerto = p
}
```

- **`puerto := 8080` primero.** El default vale; el entorno solo lo pisa si existe.
- **`os.Getenv("PORT")` devuelve `""` si la variable no existe.** En Java,
  `System.getenv("PORT")` devuelve `null`. Acá no hay null: "no existe" y "está vacía"
  son el mismo caso, y para configuración eso es lo que querés (vacío = usá el default).
- **`v := os.Getenv(...)` dentro del `if`** declara `v` solo para ese bloque. Se llama
  "if con inicialización" y evita dejar variables sueltas en toda la función.
- **`strconv.Atoi`** convierte string a int y devuelve `(int, error)`. Si el valor es
  `"8080 "` con espacio, falla: no hay trim automático.
- **`fmt.Fprintf(os.Stderr, ...)`** escribe el error en la salida de errores. `%q` muestra
  el valor entre comillas para que veas exactamente qué llegó; `%v` imprime el error.
- **`os.Exit(1)`** termina el proceso. Si la config está mal, es mejor no arrancar que
  arrancar a medias y fallar requests.

```bash
PORT=abc go run ./02-tipos-variables
# PORT inválido ("abc"): strconv.Atoi: parsing "abc": invalid syntax
# exit status 1
```

## Paso 6 — El struct de configuración

```go
cfg := Config{
	Puerto:     puerto,
	Debug:      os.Getenv("DEBUG") == "true",
	Timeout:    timeout,
	Directorio: "./datos",
}
fmt.Printf("config: %+v\n", cfg)
```

- Se inicializa con **nombres de campo**. Es más largo que `Config{puerto, false, ...}` y
  es lo correcto: si mañana agregás un campo en el medio, la versión posicional se rompe
  en silencio.
- `os.Getenv("DEBUG") == "true"` es un parser de bool de una línea. Acepta solo `true`
  exacto: `1`, `TRUE` o `yes` dan `false`. Sin librería, sí, pero sé consciente del límite.
- `%+v` imprime el struct **con los nombres de los campos**:

```text
config: {Puerto:8080 Debug:false Timeout:30s Directorio:./datos}
```

Sin el `+`, `%v` imprime `{8080 false 30s ./datos}` y tenés que contar posiciones para
saber qué es cada cosa. Para depurar structs, `%+v` siempre.

## Todo junto

El archivo `main.go` es exactamente estos seis pasos en orden. Abrilo y leelo de corrido:
cada bloque debería resultarte conocido. Si algo no cierra, es el paso que falta repasar.

## Ejecutar

```bash
go run ./02-tipos-variables
$env:PORT="9090"; go run ./02-tipos-variables          # PowerShell
$env:DEBUG="true"; go run ./02-tipos-variables
$env:PORT="abc"; go run ./02-tipos-variables           # falla a propósito
```

## Errores que vas a ver

| Mensaje | Qué pasó | Solución |
|---|---|---|
| `mismatched types int and float64` | mezclaste tipos sin convertir | `float64(x)` explícito |
| `declared and not used: x` | variable que nunca leés | usala o borrala |
| `cannot use "8080" (untyped string) as int` | `os.Getenv` siempre es string | `strconv.Atoi` |
| `timeout: 30000000000` | `%d` con `time.Duration` | `%v` |
| `PORT inválido ("")` | trataste vacío como valor | preguntá `v != ""` |
| `undefined: Info` | constante de otro paquete | `paquete.Info` |

## Por qué importa en el trabajo

Cada servicio arranca leyendo `PORT`, `DATABASE_URL`, `LOG_LEVEL`. El patrón de este
capítulo —default, override, convertir, fallar rápido si está mal— es literalmente la
primera función de un `main` real (lo vas a ver completo en el capítulo 17). Y el zero
value explica por qué en Go casi no vas a escribir `if x == nil` para strings y números.

## Ejercicio

1. Agregá `NIVEL` al entorno (`debug`, `info`, `error`) y convertilo a `Nivel` con un
   `switch`. Si viene un valor desconocido, usá `Info` y avisá por `stderr`.
2. Cambiá `Debug` para aceptar `1`, `true`, `TRUE` y `yes`. Escribilo como una función
   `parseBool(string) (bool, error)` y probala con los cuatro casos.
3. Imprimí `cfg` con `%v` y con `%+v` y compará. ¿Cuál usarías en un log de arranque?

Siguiente: [03 — Control de flujo](../03-control-flujo/).
