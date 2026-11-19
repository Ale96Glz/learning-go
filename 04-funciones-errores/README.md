# 04 — Funciones y errores

**Qué vas a construir:** un constructor que valida, dos formas de clasificar errores
(centinela y tipado) y limpieza garantizada con `defer`. Es el capítulo que más cambia tu
forma de escribir código si venís de Java o C#.

**Qué necesitas antes:** capítulos 01 a 03.

---

## Paso 1 — Funciones: varios retornos, sin ceremonia

```go
func NewUsuario(nombre string, edad int) (*Usuario, error)
```

Una función Go puede devolver varios valores. No es una tupla ni un objeto especial: es
sintaxis del lenguaje. El primero es el resultado; el segundo, el error.

```go
u, err := NewUsuario("Ana", 30)
```

Esto reemplaza al `try/catch`. En Java/C# una función que falla **lanza** una excepción y
el caller puede ignorarla hasta que explota lejos. Acá el error es un **valor de retorno**
común: si no lo mirás, lo estás ignorando a propósito (`_`) y se ve en el código.

```go
u, _ := NewUsuario("", 30) // ignorás el error: u es nil
```

El compilador no te obliga a chequearlo, pero el código lo grita. Y como el error es un
valor, podés envolverlo, compararlo, agregarle datos y testearlo sin frameworks.

## Paso 2 — `error` es una interfaz

```go
type error interface {
	Error() string
}
```

Cualquier tipo con un método `Error() string` **es** un error. No hay que heredar de
`Exception` ni extender `Exception`. Eso da tres herramientas:

### Centinela: un valor conocido

```go
var ErrEdadInvalida = errors.New("edad inválida")
```

Es una variable global con un valor fijo. El caller no compara textos (frágiles), compara
identidad:

```go
if errors.Is(err, ErrEdadInvalida) { ... }
```

Podés cambiar el mensaje cuando quieras y nadie se rompe. Es el equivalente a una excepción
concreta (`throw new EdadInvalidaException()`), pero sin clase y sin `try`.

### Tipado: un error que lleva datos

```go
type ValidationError struct {
	Campo  string
	Motivo string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("campo %q: %s", e.Campo, e.Motivo)
}
```

Cumple la interfaz (tiene `Error() string`) y además lleva `Campo` y `Motivo`. El caller
puede sacar esos datos y, por ejemplo, devolver un 422 con el campo que falló.

- El receptor es **puntero** (`e *ValidationError`). En el `errors.As` de más abajo eso
  importa: el target tiene que ser `*ValidationError`.
- `%q` imprime el nombre del campo entre comillas: `campo "nombre": ...`. Útil para ver
  espacios o valores vacíos.

## Paso 3 — El constructor

```go
func NewUsuario(nombre string, edad int) (*Usuario, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return nil, &ValidationError{Campo: "nombre", Motivo: "no puede estar vacío"}
	}
	...
	return &Usuario{Nombre: nombre, Edad: edad}, nil
}
```

Tres decisiones, todas deliberadas:

1. **Normaliza antes de validar.** `"   "` pasa a `""` y recién ahí se rechaza. Si validaras
   primero, un nombre de solo espacios pasaría el filtro.
2. **Devuelve `nil` con el error.** Nunca un usuario a medio construir. El caller tiene una
   regla simple: si `err != nil`, no uses `u`. En Java/C# esto no está garantizado: un
   constructor puede dejar el objeto en un estado inválido y lanzar después.
3. **El error de edad envuelve un centinela:**

```go
return nil, fmt.Errorf("crear usuario %q: %w", nombre, ErrEdadInvalida)
```

`%w` guarda `ErrEdadInvalida` **dentro** del nuevo error. El mensaje visible es
`crear usuario "Beto": edad inválida`, pero la cadena interna conserva el centinela, así
que `errors.Is(err, ErrEdadInvalida)` da `true`. Si usaras `%v`, el texto sería idéntico y
`errors.Is` daría `false`: ese es el bug que `%w` evita.

`%w` solo existe en `fmt.Errorf`. No es un verbo de impresión: no lo uses en `Printf`.

### El struct que se valida

```go
type Usuario struct {
	Nombre string
	Edad   int
}
```

Sin tags ni métodos. Un modelo de dominio mínimo. Los tags JSON aparecen en el capítulo 05.

## Paso 4 — Probar los casos

```go
casos := []struct {
	nombre string
	edad   int
}{
	{"Ana", 30},
	{"   ", 30},
	{"Beto", -5},
	{strings.Repeat("x", 81), 40},
}
```

Un slice de **structs anónimos**: cada fila tiene un nombre y una edad. Es la tabla de casos
más simple que existe y el patrón que después se convierte en un test de tabla (capítulo 10).
El cuarto caso usa `strings.Repeat("x", 81)` para generar un nombre demasiado largo sin
escribir 81 letras.

```go
for _, c := range casos {
	u, err := NewUsuario(c.nombre, c.edad)
	if err != nil {
		...
		continue
	}
	fmt.Printf("OK: %+v\n", *u)
}
```

- `continue` salta al siguiente caso: si hay error, no tiene sentido imprimir el "OK".
- `*u` desreferencia el puntero. `u` es `*Usuario`; `*u` es el struct. Con `%+v` ves
  `{Nombre:Ana Edad:30}`. Sin el `*`, `fmt` imprimiría la dirección o llamaría a un
  `String()` si existiera.

## Paso 5 — `errors.As` y `errors.Is`

```go
fmt.Printf("ERROR: %v\n", err)

var ve *ValidationError
if errors.As(err, &ve) {
	fmt.Printf("  -> campo inválido: %s (%s)\n", ve.Campo, ve.Motivo)
}

if errors.Is(err, ErrEdadInvalida) {
	fmt.Println("  -> el problema es la edad")
}
```

`%v` con un error llama a su `Error()`, así que imprime el mensaje. No hace falta `%s`.

**`errors.As`** pregunta: "¿hay un error de este **tipo** en la cadena?". Le pasás la
dirección de una variable del tipo destino (`&ve`, o sea `**ValidationError`) y, si lo
encuentra, rellena `ve`. Después podés leer sus campos.

```go
var ve *ValidationError   // ve es nil
errors.As(err, &ve)       // si encuentra uno, ve apunta a él
```

**`errors.Is`** pregunta: "¿hay este **valor** en la cadena?". Es para centinelas.

Los dos recorren la cadena de errores envueltos con `%w`. Por eso conviene envolver con
`%w` siempre: si cortás la cadena, estas dos funciones dejan de encontrar la causa.

Salida completa:

```text
OK: {Nombre:Ana Edad:30}
ERROR: campo "nombre": no puede estar vacío
  -> campo inválido: nombre (no puede estar vacío)
ERROR: crear usuario "Beto": edad inválida
  -> el problema es la edad
ERROR: campo "nombre": máximo 80 caracteres
  -> campo inválido: nombre (máximo 80 caracteres)
```

Fijate en el segundo error: `errors.Is` lo encuentra **aunque** el mensaje tenga el prefijo
`crear usuario "Beto":`. Esa es toda la diferencia entre `%w` y `%v`.

## Paso 6 — `defer`: limpieza que no se olvida

```go
func() {
	defer fmt.Println("3. cierro conexión")
	defer fmt.Println("2. hago rollback")
	fmt.Println("1. abro transacción")
}()
```

```text
1. abro transacción
2. hago rollback
3. cierro conexión
fin
```

- `func() { ... }()` es una función anónima **ejecutada en el momento**. Se usa acá para
  tener una función propia donde demostrar los `defer` sin ensuciar `main`.
- Los `defer` se apilan y se ejecutan al salir de la función, en orden **inverso** (LIFO).
  Por eso el cierre de conexión, que está primero en el código, corre último.
- Esto garantiza la limpieza aunque haya un `return` en el medio o un panic. Es el
  reemplazo del `try/finally` de Java/C# y del `using` de C#.
- Ojo: `os.Exit` **no** ejecuta los `defer` (lo viste en el capítulo 01). Es una salida
  brusca.

En código real:

```go
f, err := os.Open(ruta)
if err != nil { return err }
defer f.Close()
```

Escribís el `defer` inmediatamente después de abrir. Así no te olvidás ni en el camino de
error.

## Todo junto

`main.go` tiene, en orden: el centinela, el error tipado, el modelo, el constructor y la
tabla de casos. Si entendiste por qué el error de Beto imprime el prefijo pero
`errors.Is` igual lo encuentra, entendiste el capítulo.

## Ejecutar

```bash
go run ./04-funciones-errores
```

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| `errors.Is` da `false` con el mensaje correcto | usaste `%v` en vez de `%w` | envolvés con `%w` |
| `errors.As` no llena `ve` | pasaste `ve` en vez de `&ve` | `errors.As(err, &ve)` |
| panic al leer `u.Nombre` | `err != nil` y usaste `u` (nil) | chequeá el error primero |
| `defer` no corre | saliste con `os.Exit` | evitá `os.Exit` fuera de `main` |
| `cannot use ... as error` | tu tipo no tiene `Error() string` | agregá el método |
| error sin contexto | `return err` a secas | `fmt.Errorf("abrir %s: %w", ruta, err)` |

## Por qué importa en el trabajo

- **Errores con contexto** (`%w`) son la diferencia entre "unexpected EOF" y "leer config
  en /etc/app.yaml: unexpected EOF". En producción, el segundo te salva.
- **Errores tipados** permiten que la capa HTTP traduzca a status codes sin parsear strings.
- **`defer`** es cómo se cierran archivos, conexiones y transacciones sin fugas.
- **Validar en el constructor** evita que un dato inválido viaje tres capas y explote lejos
  del origen.

## Ejercicio

1. Agregá `ErrNombreLargo` como centinela y usalo en el segundo `if` en lugar del
   `ValidationError`. ¿Qué pierde el caller? ¿Qué gana?
2. Escribí `func (e *ValidationError) Unwrap() error` que devuelva `ErrEdadInvalida` cuando
   el campo sea `"edad"`. Verificá que `errors.Is` siga funcionando.
3. Reemplazá el `defer` de ejemplo por una función `abrirTransaccion()` que devuelve
   `(commit func(), rollback func())` y usala con `defer`. Es el patrón de un repositorio
   real.

Siguiente: [05 — Structs y métodos](../05-structs-metodos/).
