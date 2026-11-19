# 10 — Testing

**Qué vas a construir:** tests que documentan el comportamiento de tres funciones, con el
patrón de tabla que usa casi todo el ecosistema Go. Sin frameworks, sin anotaciones.

**Qué necesitas antes:** capítulos 01 a 09.

---

## Paso 1 — El archivo de test

En Go no hay JUnit ni xUnit. Hay tres reglas:

1. El archivo termina en `_test.go`.
2. La función empieza con `Test` y recibe `*testing.T`:
   ```go
   func TestAlgo(t *testing.T)
   ```
3. El paquete es el mismo que el código (`package main`), así podés testear funciones no
   exportadas. Si quisieras probar solo la API pública, usarías `package main_test`.

Nada de `@Test`, nada de `[Fact]`. El nombre y la firma son todo. El comando es
`go test`; el resto es convención.

## Paso 2 — El test mínimo

```go
func TestSumar(t *testing.T) {
	if got := Sumar(2, 3); got != 5 {
		t.Fatalf("Sumar(2,3) = %d, esperaba 5", got)
	}
}
```

- **`if got := Sumar(2, 3); got != 5`** es el patrón de `if` con inicialización (capítulo
  03): `got` solo vive en ese bloque.
- **`t.Fatalf`** marca el test como fallido y **corta** ese test. Si la suma está mal, no
  tiene sentido seguir.
- El mensaje ideal dice **qué** se llamó, **qué** devolvió y **qué** se esperaba:
  `Sumar(2,3) = 4, esperaba 5`. Un `assert failed` no sirve.

Para una función de una línea, esto es todo. No hace falta tabla.

## Paso 3 — Test de tabla

Cuando la misma lógica se prueba con varios casos, la tabla es el patrón estándar:

```go
func TestDividir(t *testing.T) {
	casos := []struct {
		nombre    string
		a, b      float64
		esperado  float64
		esperaErr error
	}{
		{"positivos", 10, 4, 2.5, nil},
		{"negativos", -9, 3, -3, nil},
		{"cero arriba", 0, 5, 0, nil},
		{"división por cero", 1, 0, 0, ErrDivisionPorCero},
	}
```

- **`[]struct{...}`** es el mismo slice de structs anónimos del capítulo 04, pero con un
  campo `nombre` para identificar cada caso. Leer la tabla es leer la especificación.
- **`a, b float64`**: dos campos en una línea.
- **`esperaErr error`**: `nil` significa "no debe fallar"; un centinela significa "debe
  fallar con este error".
- El caso de división por cero espera `ErrDivisionPorCero`. El resultado esperado es `0`
  porque es lo que devuelve la función en ese camino.

### `t.Run`: un subtest por fila

```go
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := Dividir(c.a, c.b)
			...
		})
	}
```

`t.Run(nombre, fn)` crea un subtest. Ventajas concretas:

- La salida de `go test -v` muestra `TestDividir/división_por_cero`, no un fallo genérico.
- Podés correr uno solo: `go test -run 'TestDividir/negativos'`.
- Si un caso falla, los demás siguen ejecutándose.

**Captura de variables**: desde Go 1.22, cada iteración del `range` tiene su propia variable
`c`, así que la closure ve el valor correcto. Antes de 1.22 había que hacer
`c := c` adentro del loop. Si leés código viejo, vas a ver ese `c := c` y ahora sabés por
qué está.

### La lógica del test

```go
			if !errors.Is(err, c.esperaErr) {
				t.Fatalf("error = %v, esperaba %v", err, c.esperaErr)
			}
			if err != nil {
				return
			}
			if got != c.esperado {
				t.Errorf("Dividir(%v, %v) = %v, esperaba %v", c.a, c.b, got, c.esperado)
			}
```

- **`errors.Is(err, c.esperaErr)`** funciona también cuando `c.esperaErr` es `nil`:
  `errors.Is(nil, nil)` es `true`, y `errors.Is(unError, nil)` es `false`. Así una sola
  comparación cubre "esperaba error" y "no esperaba error".
- **`t.Fatalf`** si el error no coincide: sin el error correcto, comparar el resultado no
  tiene sentido. Corta el subtest.
- **`if err != nil { return }`**: si esperábamos error y llegó, ya validamos lo que
  importaba; el resultado puede ser cualquier cosa.
- **`t.Errorf`** para el valor: marca el fallo y **sigue**, así reporta más de un problema en
  la misma corrida.
- **`%v`** en los mensajes imprime floats, enteros y errores sin ceremonia. `%q` se usa para
  strings, para ver espacios.

Nunca compares `err.Error()` con un string: cambiar el mensaje rompe el test sin que cambie
el comportamiento. Se compara identidad con `errors.Is` (capítulo 04).

## Paso 4 — Tests de validación

```go
func TestValidarEmail(t *testing.T) {
	validos := []string{"ana@example.com", "a@b.co", "  x@y.com  "}
	for _, e := range validos {
		if err := ValidarEmail(e); err != nil {
			t.Errorf("ValidarEmail(%q) devolvió error: %v", e, err)
		}
	}

	invalidos := []string{"", "sin-arroba", "@ejemplo.com", "ana@"}
	for _, e := range invalidos {
		err := ValidarEmail(e)
		if !errors.Is(err, ErrEmailInvalido) {
			t.Errorf("ValidarEmail(%q) = %v, esperaba ErrEmailInvalido", e, err)
		}
	}
}
```

Dos listas, dos loops: uno para lo que debe pasar, otro para lo que debe fallar. Menos
ceremonia que una tabla cuando no hay múltiples campos que comparar.

- **`%q`** en el mensaje muestra el email entre comillas: `ValidarEmail("")` deja ver que está
  vacío. Con `%s` no verías nada.
- **`"  x@y.com  "`** prueba el `TrimSpace` de la función. Si el trim se rompe, este caso
  falla.
- **`"@ejemplo.com"`** prueba `arroba <= 0` (el `@` está en la posición 0).
- **`"ana@"`** prueba `arroba == len(s)-1` (termina en `@`).

La función valida poco a propósito: un `@` en el medio. No valida dominios ni puntos. Un
test no debe exigir más de lo que el código promete; si querés validación real, cambiás el
código y agregás casos.

```go
func ValidarEmail(s string) error {
	s = strings.TrimSpace(s)
	arroba := strings.Index(s, "@")
	if arroba <= 0 || arroba == len(s)-1 {
		return fmt.Errorf("%w: %q", ErrEmailInvalido, s)
	}
	return nil
}
```

`strings.Index` devuelve `-1` si no encuentra. `arroba <= 0` cubre "no hay" y "está al
principio". `%w` envuelve el centinela para que `errors.Is` lo encuentre, y `%q` agrega el
valor al mensaje.

## Paso 5 — Correr los tests

```bash
go test ./10-testing          # corre y resume
go test -v ./10-testing       # muestra cada test y subtest
go test -run TestDividir ./10-testing
go test -race ./10-testing    # detecta data races (capítulo 11)
go test -cover ./10-testing   # porcentaje de líneas cubiertas
go test ./...                 # todo el módulo
```

Salida típica:

```text
=== RUN   TestDividir
=== RUN   TestDividir/positivos
=== RUN   TestDividir/negativos
=== RUN   TestDividir/cero_arriba
=== RUN   TestDividir/división_por_cero
--- PASS: TestDividir (0.00s)
...
PASS
ok  	tutorialgo/10-testing	0.012s
```

Los tests **no imprimen** nada por defecto. La comunicación es `t.Errorf`/`t.Fatalf`, no
`fmt.Println`. Si necesitás depurar, `t.Log` (solo se muestra con `-v` o si el test falla) y
`t.Logf`.

`go test` cachea resultados. Si querés forzar: `go test -count=1 ./...`.

## Paso 6 — `main` también se puede ejecutar

```go
func main() {
	fmt.Println("2+3 =", Sumar(2, 3))
	if r, err := Dividir(10, 4); err == nil {
		fmt.Println("10/4 =", r)
	}
	if _, err := Dividir(1, 0); errors.Is(err, ErrDivisionPorCero) {
		fmt.Println("error esperado:", err)
	}
	fmt.Println("email:", ValidarEmail("ana@example.com"))
}
```

El `main` existe para que puedas correr el ejemplo con `go run`. No participa de los tests.
Su lógica es la misma que la de los tests, pero impresa:

```text
2+3 = 5
10/4 = 2.5
error esperado: división por cero
email: <nil>
```

`ValidarEmail` devuelve `nil` para un email válido, y `fmt.Println` imprime `<nil>` porque
`%v` de un error nil es eso. No es un bug: es la representación.

## Todo junto

Tres funciones chicas, tres tests. `Sumar`: test mínimo. `Dividir`: tabla con error y
resultado. `ValidarEmail`: listas de válidos e inválidos. Ese repertorio cubre el 90% de los
tests que vas a escribir.

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| `no tests to run` | El archivo no termina en `_test.go` o la función no empieza con `Test` | Renombrá |
| `undefined: Dividir` | `package main_test` sin importar el paquete | Usá `package main` o importá |
| El test pasa pero el bug sigue | Solo probaste el camino feliz | Agregá casos de error |
| `%v` muestra `<nil>` en un mensaje | Imprimiste un error nil | Es normal, o usá `%q`/condicional |
| Un caso falla y los demás no corren | `t.Fatalf` a nivel del test padre | Usá `t.Run` por caso |
| `-race` no compila en Windows sin CGO | Falta toolchain de C | Instalá gcc o corré `-race` en CI Linux |

## Por qué importa en el trabajo

- **Tabla + `t.Run`**: el patrón que vas a encontrar en cualquier repo Go. Agregar un caso
  nuevo es una línea.
- **Tests como documentación**: leer `TestDividir` te dice el contrato mejor que el README.
- **`errors.Is` en tests**: te permite cambiar mensajes sin romper tests, y detectar si
  alguien cortó la cadena de errores.
- **`go test ./...`** en CI es la barrera mínima antes de un merge.

## Ejercicio

1. Agregá a `TestDividir` los casos `{"infinito", 1, 0.0000001, 10000000, nil}` y
   `{"negativo entre cero", -1, 0, 0, ErrDivisionPorCero}`. ¿El primero pasa tal cual?
2. Escribí `TestValidarEmailTabla` con el formato de tabla y un campo `quiereErr bool`.
   Compará con la versión de dos listas: ¿cuál se lee mejor?
3. Agregá `t.Helper()` a una función `comprobarError(t *testing.T, got, esperado error)` y
   usala en los dos tests. Mirá cómo cambia el número de línea del fallo.

Siguiente: [11 — Concurrencia](../11-concurrencia/).
