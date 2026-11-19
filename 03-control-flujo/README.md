# 03 — Control de flujo

**Qué vas a construir:** validar una entrada, clasificar valores, recorrer colecciones y
reintentar una operación que falla. Los cuatro patrones que vas a escribir todos los días.

**Qué necesitas antes:** capítulos 01 y 02.

---

## Paso 1 — `if` con declaración inicial

```go
if edad, err := parseEdad("30"); err != nil {
	fmt.Println("entrada inválida:", err)
} else if edad < 18 {
	fmt.Println("menor de edad")
} else {
	fmt.Printf("adulto de %d\n", edad)
}
```

La parte rara para quien viene de Java/C# es el `;`. Antes del punto y coma va una
declaración (`edad, err := parseEdad("30")`), y el `if` evalúa solo lo que está después
(`err != nil`).

¿Para qué? Para que `edad` y `err` existan **solo** dentro del `if/else`. Fuera, no existen:

```go
fmt.Println(edad) // undefined: edad
```

Eso evita el clásico `err` que se declara arriba, se reusa en cinco llamadas y en algún
momento te olvidás de chequearlo. Acá cada error vive en el bloque donde se evalúa. Es el
patrón que vas a ver en el 90% del código Go:

```go
if err := hacerAlgo(); err != nil {
	return err
}
```

El orden importa: si `err != nil`, **no uses `edad`**. Puede ser `0` (el valor que
devolvimos en el error) y tratarlo como un adulto de 0 años sería un bug silencioso.

### La función que valida

```go
func parseEdad(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("edad %q no es un número: %w", s, err)
	}
	if n < 0 || n > 120 {
		return 0, errors.New("edad fuera de rango")
	}
	return n, nil
}
```

- **`strings.TrimSpace(s)`** saca espacios de los extremos. `" 30 "` se convierte en `"30"`.
  Sin esto, `Atoi` falla por un espacio invisible.
- **`%q`** imprime el valor entre comillas (`"abc"`), así ves exactamente qué entró.
- **`%w`** envuelve el error original dentro del nuevo. El mensaje es más claro y, además,
  `errors.Is(err, strconv.ErrSyntax)` sigue funcionando capas arriba. Se explica a fondo en
  el capítulo 04; por ahora: `%w` para errores, `%v` para cualquier otra cosa.
- **`errors.New`** crea un error simple, sin causa anterior. Se usa cuando no hay nada que
  envolver: "la edad está fuera de rango" es la causa.
- **`return 0, nil`** al final: el camino feliz. En Go se devuelve el valor y `nil` como
  error; no hay excepciones.

Salida: `adulto de 30`.

## Paso 2 — `switch`: dos formas

### Con expresión

```go
for _, nota := range []int{95, 80, 61, 40} {
	fmt.Printf("%d -> %s\n", nota, clasificar(nota))
}
```

```text
95 -> excelente
80 -> aprobado
61 -> reprobado
40 -> reprobado
```

Ese `for _, nota := range` recorre un slice literal de cuatro notas. El `_` descarta el
índice porque no lo necesitamos; sin él, el compilador daría error por variable no usada.

```go
func clasificar(nota int) string {
	switch {
	case nota >= 90:
		return "excelente"
	case nota >= 70:
		return "aprobado"
	default:
		return "reprobado"
	}
}
```

Acá el `switch` **no tiene expresión**: equivale a `switch true`, y cada `case` es un
booleano que se evalúa en orden. Entra al primero que sea verdadero. Es la forma limpia de
escribir una cadena de `if/else if`.

Dos diferencias con Java/C#:

1. **No hay `fallthrough` implícito.** Al terminar un `case`, sale del `switch`. No existe
   el bug de olvidarse un `break` y ejecutar el caso siguiente.
2. **`return` dentro del `case` corta la función entera**, obviamente. Si no hubiera
   `return`, el `case` termina y sigue después del `switch`.

### Sin expresión, con strings

```go
estado := "activo"
switch {
case estado == "activo":
	fmt.Println("puede operar")
case estado == "bloqueado":
	fmt.Println("no puede operar")
default:
	fmt.Println("estado desconocido")
}
```

Salida: `puede operar`. El `default` cubre cualquier valor no contemplado; en un `switch`
de estados reales conviene tenerlo siempre, aunque sea para loguear "estado raro".

## Paso 3 — `for`: la única estructura de repetición

Go no tiene `while` ni `do/while`. `for` cubre los tres casos:

```go
suma := 0
for i := 1; i <= 5; i++ { // clásico
	suma += i
}
```

```go
intentos := 0
for intentos < 3 { // "while"
	intentos++
}
```

```go
for { // infinito
	break
}
```

Salidas: `suma: 15`, `intentos: 3`.

- La primera forma es idéntica a Java/C#: inicio, condición, paso.
- La segunda omite inicio y paso: solo condición. Es el `while`.
- La tercera no tiene nada: es un loop infinito. Se sale con `break` o `return`.

`i++` es una **sentencia**, no una expresión: no podés escribir `x := i++`. Y `++i` no
existe; solo hay post-incremento. Son decisiones para mantener el lenguaje chico.

## Paso 4 — `range`

### Sobre un slice

```go
frutas := []string{"manzana", "pera", "uva"}
for i, f := range frutas {
	fmt.Printf("%d: %s\n", i, f)
}
```

```text
0: manzana
1: pera
2: uva
```

En un slice, `range` devuelve **índice y valor**. Si solo querés el valor, `for _, f :=`.
Si solo querés el índice, `for i := range frutas`. Si escribís `for i, f :=` y no usás `f`,
no compila.

### Sobre un map (orden aleatorio)

```go
stock := map[string]int{"pera": 3, "uva": 10}
for nombre, cantidad := range stock {
	fmt.Printf("%s=%d ", nombre, cantidad)
}
```

```text
pera=3 uva=10
```

Puede salir al revés en la próxima corrida. **Go randomiza el orden a propósito** para que
nunca dependas de él. Si necesitás orden (para un JSON estable, para un test), extraés las
claves a un slice, las ordenás con `slices.Sort` y recorrés eso. Lo ves en el capítulo 07.

### Sobre un string

```go
for i, r := range "año" {
	fmt.Printf("%d:%c ", i, r)
}
// 0:a 1:ñ 3:o
```

`range` sobre un string devuelve la posición del **byte** y la **runa** (carácter Unicode).
Fijate que `o` está en el índice 3, no 2: la `ñ` ocupa dos bytes en UTF-8. Si iteraras con
`for i := 0; i < len(s); i++` obtendrías bytes sueltos y romperías los acentos.

## Paso 5 — `break` con etiqueta

```go
buscar:
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			if i*j == 6 {
				encontrado = true
				break buscar
			}
		}
	}
```

Sin la etiqueta, `break` sale **solo del `for` interno**; el externo sigue iterando y el
programa hace trabajo de más. Con `break buscar`, sale de los dos.

La etiqueta va antes del `for` y con dos puntos. Si querés salir de una función entera, no
uses etiqueta: usá `return`, que es más claro. La etiqueta es para el caso de búsqueda en
matriz o en resultados paginados, donde `return` no aplica porque hay código después.

(En Java existe el mismo `break etiqueta`; en C# no, ahí se usa `goto` o un `bool` de
control. Go eligió la versión de Java.)

Salida: `encontrado: true`.

## Paso 6 — Reintentos: una función que recibe una función

```go
if err := conReintentos(3, func(intento int) error {
	if intento < 3 {
		return fmt.Errorf("servicio no disponible (intento %d)", intento)
	}
	fmt.Println("servicio respondió en el intento", intento)
	return nil
}); err != nil {
	fmt.Println("se agotaron los intentos:", err)
}
```

```text
servicio respondió en el intento 3
```

Esto parece complicado, así que vamos por partes.

**La función `conReintentos` recibe una función como parámetro:**

```go
func conReintentos(max int, fn func(intento int) error) error
```

`fn func(intento int) error` significa "una función que recibe un `int` y devuelve un
`error`". Es lo mismo que un `delegate` o un `Function<Integer, ...>` en Java/C#, pero sin
ceremonia: las funciones son valores.

**El `for` la llama hasta que devuelva `nil`:**

```go
var err error
for i := 1; i <= max; i++ {
	if err = fn(i); err == nil {
		return nil
	}
}
return fmt.Errorf("después de %d intentos: %w", max, err)
```

- `var err error` vive fuera del loop para poder devolver el **último** error al salir.
- `if err = fn(i); err == nil` es la misma forma del paso 1, pero con `=` (asignación) en
  vez de `:=`, porque `err` ya existe.
- Si ningún intento funciona, se envuelve el último error con `%w` y el número de intentos.

**La función anónima es el trabajo que se reintenta.** En este ejemplo simula un servicio
que falla dos veces y responde en la tercera. En producción sería una llamada HTTP:

```go
err := conReintentos(3, func(intento int) error {
	return llamarServicio(ctx)
})
```

El detalle importante: **el callback no sabe que lo están reintentando** salvo por el
parámetro `intento`. Eso permite decidir si vale la pena el siguiente intento (por ejemplo,
no reintentar un 400, sí un 503). En el capítulo 12 se le agrega `context` para poder
cancelar entre intentos.

## Todo junto

`main.go` sigue este orden: validación, clasificación, `for`, `range`, etiqueta y
reintentos. Cada bloque está separado por un comentario y corresponde a un paso de arriba.
Leelo de corrido y debería leerse como prosa.

## Ejecutar

```bash
go run ./03-control-flujo
```

## Errores que vas a ver

| Mensaje | Qué pasó | Solución |
|---|---|---|
| `undefined: edad` fuera del `if` | la variable del `if` es local al bloque | declarala antes, o usá el valor dentro |
| `declared and not used: f` | `for i, f := range` sin usar `f` | `for i := range` o `_` |
| `break` no sale del loop externo | `break` solo corta el `for` más cercano | etiqueta o `return` |
| el map cambia de orden entre corridas | `range` randomiza | ordená las claves |
| `forever` / el programa no termina | `for` sin condición y sin `break` | asegurate una salida |
| `i++` como valor (`x := i++`) | en Go es sentencia | `x := i; i++` |

## Por qué importa en el trabajo

- **Reintentos**: toda llamada a un servicio externo los necesita, con backoff. Este es el
  esqueleto.
- **`switch` sin expresión**: la forma de traducir códigos y estados sin `if` anidados.
- **`for` con condición**: el loop de "mientras haya trabajo" de un worker.
- **`break` con etiqueta**: búsqueda en resultados paginados sin traer todo.

## Ejercicio

1. Agregá `parseNota` con el mismo patrón de `parseEdad` (0 a 100) y usala en
   `clasificar`. Probá `"  95 "` y `"101"`.
2. Cambiá `conReintentos` para que reciba también una espera (`time.Duration`) y haga
   `time.Sleep` entre intentos. ¿Qué pasa con `max = 0`?
3. Recorré un `map[string]int` de stock e imprimí las claves ordenadas alfabéticamente
   (pista: `slices.Sort` sobre las claves).

Siguiente: [04 — Funciones y errores](../04-funciones-errores/).
