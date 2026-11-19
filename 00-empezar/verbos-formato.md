# Verbos de formato: `%v`, `%t`, `%d`, `%w`...

Si vienes de otros lenguajes, esto es lo primero que desconcierta de los `print` en Go.
No es sintaxis del lenguaje: es una **minilengua de `fmt`** para decir "acá va un valor,
y quiero verlo así".

## La idea en una frase

```go
fmt.Printf("Hola, %s! Tienes %d años.\n", "Ana", 30)
// Hola, Ana! Tienes 30 años.
```

- `%` marca un **hueco**.
- La letra que sigue (`s`, `d`, `v`...) dice **cómo representar** el valor.
- Los argumentos después de la cadena se llenan **en orden**, uno por hueco.
- `\n` es un salto de línea (no es un verbo).

Si hay más huecos que argumentos verás `%!s(MISSING)`; si hay más argumentos que huecos,
`fmt` agrega `%!(EXTRA ...)`. Y si el verbo no corresponde al tipo, por ejemplo `%d` con
un string, la salida es `%!d(string=Ana)` en vez de fallar: el programa compila y el bug
aparece en pantalla. Por eso `go vet` revisa los `Printf`.

## La tabla que vas a consultar

| Verbo | Qué imprime | Ejemplo | Salida |
|---|---|---|---|
| `%v` | El valor "por defecto", sirve para casi todo | `%v`, `[]int{1,2}` | `[1 2]` |
| `%+v` | Struct **con nombres de campo** | `%+v`, `Usuario{"Ana",30}` | `{Nombre:Ana Edad:30}` |
| `%#v` | Representación Go (para depurar) | `%#v`, `Usuario{"Ana",30}` | `main.Usuario{Nombre:"Ana", Edad:30}` |
| `%T` | El **tipo** del valor | `%T`, `30` / `"Ana"` | `int` / `string` |
| `%t` | Booleano | `%t`, `true` | `true` |
| `%d` | Entero en base 10 | `%d`, `42` | `42` |
| `%5d` | Entero, ancho mínimo 5, alineado a la derecha | `%5d`, `42` | `   42` |
| `%-5d` | Igual, alineado a la izquierda | `%-5d`, `42` | `42   ` |
| `%05d` | Rellena con ceros | `%05d`, `42` | `00042` |
| `%s` | String | `%s`, `"Ana"` | `Ana` |
| `%q` | String **entre comillas** y con escapes | `%q`, `"Ana"` | `"Ana"` |
| `%c` | El número como carácter Unicode | `%c`, `65` | `A` |
| `%x` | Entero en hexadecimal | `%x`, `255` | `ff` |
| `%f` | Float | `%f`, `3.14159` | `3.141590` |
| `%.2f` | Float con 2 decimales | `%.2f`, `3.14159` | `3.14` |
| `%p` | Dirección de memoria del puntero | `%p`, `&x` | `0xc0000140a0` |
| `%%` | Un `%` literal | `100%%` | `100%` |
| `%w` | **Solo en `fmt.Errorf`**: envuelve un error | Ver abajo | — |

## `%v` en detalle (el más usado)

`%v` significa "value" y elige una representación razonable según el tipo:

```go
fmt.Printf("%v\n", 42)                     // 42
fmt.Printf("%v\n", []string{"a", "b"})     // [a b]
fmt.Printf("%v\n", map[string]int{"a": 1}) // map[a:1]
fmt.Printf("%v\n", nil)                    // <nil>
inicio := time.Now()
fmt.Printf("%v\n", inicio)                 // 2026-11-14 10:00:00 +0000 UTC
```

Dos matices que importan:

1. **Structs:** `%v` imprime los valores `{Ana 30}`; `%+v` agrega los nombres
   `{Nombre:Ana Edad:30}`. Cuando estás depurando un struct, usa `%+v`.
2. **Tipos con `String()`:** si tu tipo implementa `func (x T) String() string`, tanto
   `%v` como `%s` usan ese método. Así `fmt.Println(u)` imprime algo legible en vez de
   `{...}` (lo ves en el capítulo 05).

## `%t` en detalle

`%t` es solo para `bool`. Sin él, `%v` también imprime `true`/`false`. La diferencia es
expresar la intención: `%t` dice "esto es un booleano"; si algún día cambias el tipo a
`int`, `go vet` te avisa de que `%t` ya no corresponde.

```go
activo := true
fmt.Printf("activo=%t\n", activo) // activo=true
```

## `%w`: no imprime, envuelve

```go
return fmt.Errorf("abrir config: %w", err)
```

`%w` es especial: guarda el error original dentro del nuevo, para que `errors.Is` y
`errors.As` lo encuentren capas arriba. **Solo existe en `fmt.Errorf`**, no en `Printf`.
Si usas `%v` en su lugar, el mensaje se ve igual pero la cadena de errores se rompe.
En el capítulo 04 está explicado con ejemplos.

## La familia de funciones de `fmt`

| Función | Qué hace | Agrega salto de línea |
|---|---|---|
| `fmt.Print(a, b)` | Imprime valores tal cual | No |
| `fmt.Println(a, b)` | Imprime separando con espacios | **Sí** |
| `fmt.Printf(formato, args...)` | Imprime usando verbos | No (pon `\n`) |
| `fmt.Sprintf(...)` | Devuelve un `string`, no imprime | No |
| `fmt.Fprintf(w, ...)` | Escribe en un `io.Writer` (archivo, HTTP, consola) | No |
| `fmt.Errorf(...)` | Devuelve un `error` | No |
| `fmt.Fprintln(w, ...)` | Escribe en `w` con espacios y salto | Sí |

Regla práctica: **`Println` para personas, `Printf` cuando necesitas control del formato,
`Sprintf` cuando el resultado se guarda o se envía, `Fprintf` cuando el destino no es la
consola.**

## Alineación: `%-20s` y compañía

El número entre `%` y la letra es el **ancho mínimo**. Es lo que hace que una tabla de
texto quede derecha:

```go
fmt.Printf("%-20s %5d\n", "desplegar", 12)
fmt.Printf("%-20s %5d\n", "revisar", 3)
```

```text
desplegar               12
revisar                  3
```

- `-` alinea a la izquierda (por defecto es derecha).
- El ancho es un **mínimo**: si el texto es más largo, no se corta.
- `%05d` rellena con ceros a la izquierda; útil para IDs y fechas.

## Verbos y `slog` no se mezclan

`log/slog` no usa `%v`; usa pares clave-valor:

```go
slog.Info("request", "metodo", "GET", "status", 200)
```

Si escribes `slog.Info("request %s", r.Method)`, el `%s` queda literal en el mensaje. Los
capítulos 14 y 18 usan el estilo correcto.

## Errores frecuentes

| Síntoma | Causa | Solución |
|---|---|---|
| `%!d(string=Ana)` | Verbo de número con string | Usa `%s` o `%v` |
| `%!s(MISSING)` | Faltó un argumento | Agrega el valor |
| `%!(EXTRA string=x)` | Sobró un argumento | Revisa el orden |
| `%!t(int=1)` | `%t` con algo que no es bool | Convierte o usa `%d` |
| Todo en una línea | `Printf` sin `\n` | Agrega `\n` o usa `Println` |

Volver al [índice](../README.md) o a [01 — Hola mundo](../01-hola-mundo/).
