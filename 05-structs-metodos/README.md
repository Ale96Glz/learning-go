# 05 — Structs y métodos

**Qué vas a construir:** un modelo de dominio (`Usuario`) que se valida al crearse, se
serializa a JSON para una API, se suspende, y un `Admin` que lo reutiliza. Es el capítulo
donde se entiende por qué Go no necesita clases.

**Qué necesitas antes:** capítulos 01 a 04.

---

## Paso 1 — Un struct es un tipo de datos, no una clase

```go
type Estado string

const (
	Activo     Estado = "activo"
	Suspendido Estado = "suspendido"
)

type Usuario struct {
	ID     int64     `json:"id"`
	Nombre string    `json:"nombre"`
	Email  string    `json:"email"`
	Estado Estado    `json:"estado"`
	Creado time.Time `json:"creado_en"`
}
```

Leelo de arriba abajo:

- **`type Estado string`** crea un tipo nuevo **basado en** string. No es un alias: un
  `Estado` no se puede pasar donde se pide un `string` sin convertir, y viceversa. Eso evita
  que mandes un estado cualquiera a una función que espera un nombre. Es lo más parecido a
  un `enum` de Java/C# que hay en Go.
- **`Activo` y `Suspendido`** son constantes de ese tipo. Adentro valen `"activo"` y
  `"suspendido"`, así que `%s` y `%v` los imprimen legibles y JSON los guarda como string.
  Un `int` con `iota` también serviría, pero entonces la base de datos guardaría `0` y `1`,
  ilegibles para quien mire la tabla.
- **`struct`** agrupa campos con nombre y tipo. `ID` es `int64`, no `int`: los IDs de una
  base de datos pueden superar el rango de un `int` de 32 bits, y `int64` es el estándar.
- **`time.Time`** es un struct de la librería estándar con fecha, hora y zona. Se guarda en
  UTC (lo hace el constructor) y se serializa como RFC 3339.
- **`json:"..."`** son **tags**: texto que leen otras librerías, no Go. `encoding/json` los
  usa para decidir el nombre de la clave. Sin tag, la clave sería `"Nombre"` con mayúscula,
  porque el campo se llama así. En una API se usa `snake_case`, y por eso el tag.

### Crear un struct: tres formas

```go
u := Usuario{Nombre: "Ana"}          // campos con nombre (recomendado)
u := Usuario{"Ana"}                  // posicional: frágil
var u Usuario                        // zero value
```

- **Con nombres**: si mañana agregás un campo en el medio, este código sigue compilando y
  haciendo lo mismo. Es la forma que vas a usar el 99% de las veces.
- **Posicional**: depende del orden exacto de los campos. Agregar o reordenar rompe
  callers en silencio (o no compila, si cambian los tipos). Evitalo fuera de structs
  internos y chicos.
- **`var u Usuario`**: todos los campos en su zero value (`0`, `""`, `nil`, `time.Time{}`).
  Es válido y seguro de leer, pero no está validado. No lo uses para datos de entrada.

En Java/C# un objeto se crea con `new` y vive en el heap. Acá un `Usuario` es un **valor**:
se copia al asignarlo, al pasarlo a una función y al meterlo en un slice. Es el
comportamiento de un `struct` de C#, no el de una clase de Java. Eso explica el "bug #1" del
paso 7.

## Paso 2 — El constructor

```go
func NewUsuario(nombre, email string) (*Usuario, error) {
	nombre = strings.TrimSpace(nombre)
	email = strings.ToLower(strings.TrimSpace(email))
	if nombre == "" {
		return nil, fmt.Errorf("nombre vacío")
	}
	if !strings.Contains(email, "@") {
		return nil, fmt.Errorf("email inválido: %q", email)
	}
	return &Usuario{
		Nombre: nombre,
		Email:  email,
		Estado: Activo,
		Creado: time.Now().UTC(),
	}, nil
}
```

Go no tiene constructores mágicos ni `new` con argumentos. La convención es una función
`NewX` que devuelve `(*X, error)`. Y no es solo convención: es la única forma de garantizar
invariantes.

- **`TrimSpace`** saca espacios de los extremos. **`ToLower`** normaliza el email: como en
  la práctica los emails no distinguen mayúsculas, `ANA@Example.com` y `ana@example.com`
  deben ser el mismo usuario. Si no normalizás, tu tabla tiene duplicados.
- **`if nombre == ""`**: después del trim, un nombre de solo espacios queda vacío y se
  rechaza. Validar antes del trim sería un bug.
- **`%q`** en el error muestra el email entre comillas, así un `" ana@ "` se ve con sus
  espacios.
- **`return &Usuario{...}`** devuelve un **puntero**. Si devolviera un valor, cada copia
  sería independiente y `ana.Suspender()` no afectaría al original.
- **`time.Now().UTC()`**: guardar en UTC evita el clásico bug de fechas que cambian según
  la zona del servidor. Se convierte a la zona del usuario al mostrarla.

Por qué esto importa: sin constructor, cualquiera escribe
`Usuario{Email: "basura"}` y el error aparece tres capas después, cuando la API ya respondió
un 500. Con constructor, el dato inválido no existe.

## Paso 3 — Métodos: funciones con receptor

```go
func (u *Usuario) Suspender() {
	u.Estado = Suspendido
}

func (u Usuario) Activo() bool {
	return u.Estado == Activo
}
```

Un método es una función con un **receptor** entre `func` y el nombre. Ese receptor es el
equivalente al `this`/`self`, pero explícito y con una decisión de diseño: ¿puntero o valor?

- **`(u *Usuario)`** recibe la dirección del struct. Puede modificarlo. `Suspender` cambia
  `Estado` en el usuario original.
- **`(u Usuario)`** recibe una **copia**. Puede leer, pero si asignara `u.Estado`, el
  original no cambiaría. `Activo` solo lee, así que alcanza.
- **`ana.Suspender()`** se traduce automáticamente a `(&ana).Suspender()` cuando `ana` es una
  variable. Por eso no escribís `&`. Si `ana` no fuera direccionable (por ejemplo, el
  resultado de una función), no compilaría.

Regla práctica: **si un tipo tiene algún método con puntero, usá puntero en todos**. Mezclar
es legal pero confuso, porque unos métodos mutan y otros no, y el lector no lo ve en la
llamada.

En Java/C# todos los métodos de instancia reciben la referencia al objeto (o el valor, si es
`struct`). Acá elegís por método, y esa elección es parte de la documentación.

## Paso 4 — `String()`: por qué `fmt.Println(ana)` imprime lindo

```go
func (u Usuario) String() string {
	return fmt.Sprintf("%s <%s> [%s]", u.Nombre, u.Email, u.Estado)
}
```

`fmt` (y `log`, y `slog` en muchos casos) revisa si el valor implementa esta interfaz:

```go
type Stringer interface {
	String() string
}
```

Si la implementa, usa `String()` en lugar de imprimir la estructura cruda. Por eso:

```go
fmt.Println(ana) // Ana <ana@example.com> [activo]
```

Sin `String()`, verías `&{1 Ana ana@example.com activo 2026-11-15 ...}`. El método es
opcional, pero para cualquier tipo de dominio que aparezca en logs, escribilo.

Detalles que importan:

- **`%s` dentro de `String()`** es un hueco por campo. `%s` funciona con `Estado` porque su
  tipo base es string.
- **El receptor es valor.** Eso hace que **tanto `Usuario` como `*Usuario`** implementen
  `Stringer`, y por eso `fmt.Println(ana)` funciona aunque `ana` sea un puntero.
- **`%v` y `%s` llaman a `String()`; `%+v` también.** Es un mito que `%+v` muestre los
  campos "crudos": si el tipo tiene `String()`, `%+v` la usa. Lo vas a ver al final del
  capítulo, con el struct vacío.
- **`%#v` sí ignora `String()`** y muestra la representación Go:
  `main.Usuario{Nombre:"Ana", ...}`. Es el verbo para depurar tipos, no para logs.
- **`%d` no llama a `String()`** (lo viste en el capítulo 02 con `Nivel`).

Si necesitás ver los campos de un struct que tiene `String()`, no pelees con los verbos:
usá `%#v`, o desarmá el struct en el log con los campos que te interesan.

## Paso 5 — JSON usa los tags, no `String()`

```go
b, _ := json.MarshalIndent(ana, "", "  ")
fmt.Println(string(b))
```

```json
{
  "id": 1,
  "nombre": "Ana",
  "email": "ana@example.com",
  "estado": "suspendido",
  "creado_en": "2026-11-15T05:37:38.0682218Z"
}
```

- **`MarshalIndent`** serializa con sangría; `Marshal` devuelve todo en una línea. En una
  API real usás `json.NewEncoder(w).Encode(v)`, que es más eficiente, pero para ver el
  resultado acá viene bien.
- **Devuelve `([]byte, error)`**. El `_` ignora el error porque un `Usuario` con estos
  campos siempre se puede serializar. En código real no ignores ese error sin pensar: un
  `map` con una clave no serializable o un `chan` sí fallan.
- **`string(b)`** convierte los bytes a texto para imprimir.
- **Las claves son las de los tags**, no las de Go: `nombre`, no `Nombre`. Y `String()` no
  participa: si el JSON se generara con `String()`, sería un string, no un objeto. `fmt` y
  `encoding/json` son dos mundos separados que miran cosas distintas.

## Paso 6 — Embedding: composición, no herencia

```go
type Admin struct {
	Usuario
	Permisos []string
}
```

`Admin` **contiene** un `Usuario` sin nombre de campo. Eso se llama *embedding* y hace que Go
**promueva** los campos y métodos:

```go
admin.Nombre          // equivale a admin.Usuario.Nombre
admin.Suspender()     // equivale a admin.Usuario.Suspender()
admin.Usuario.Email   // el campo embebido sigue existiendo
```

Pero no es herencia:

- No hay `super` ni `override` polimórfico. Si `Admin` define un método con el mismo nombre,
  el embebido queda tapado, pero no hay despacho dinámico.
- `Admin` **no es un** `Usuario` para efectos de interfaces. Para polimorfismo se usan
  interfaces, y eso es el capítulo 06.
- El campo embebido se puede reemplazar o dejar en zero value, como cualquier campo.

```go
func (a Admin) Puede(p string) bool {
	for _, x := range a.Permisos {
		if x == p {
			return true
		}
	}
	return false
}
```

Un chequeo lineal de permisos. Con pocos permisos es lo más simple y no necesita map. Si
fueran cientos y se consultaran por request, convendría un `map[string]struct{}` (capítulo
07). El receptor es valor: leer un slice no muta nada.

## Paso 7 — Copia vs referencia: el bug #1

```go
copia := admin.Usuario // copia el valor
copia.Nombre = "Otro"
fmt.Println("original:", admin.Nombre, "| copia:", copia.Nombre)
// original: Root | copia: Otro
```

Asignar un struct **copia todos sus campos**. Si venís de Java, donde `copia = admin` hace
que las dos variables apunten al mismo objeto, esto sorprende. En C# es como un `struct`;
en Go **todos** los structs son así.

Reglas que se desprenden:

- Si querés compartir y mutar, pasá `*Usuario` y guardá punteros.
- Si querés que una función no pueda modificar tu struct, pasalo por valor (copia).
- **Slices y maps son la excepción**: copian la cabecera, pero comparten el array subyacente.
  `copia := admin.Permisos` y modificar `copia[0]` **sí** afecta a `admin.Permisos`. Para
  una copia real usás `slices.Clone` (capítulo 07).

Esa mezcla —structs por valor, slices/maps por referencia— es la fuente de la mayoría de los
bugs sutiles al empezar. Ante la duda, preguntate: "¿esto es un struct o un slice/map?".

## Paso 8 — Zero value y los prints

```go
var vacio Usuario
fmt.Printf("vacío: %+v activo=%t\n", vacio, vacio.Activo())
```

```text
vacío:  <> [] activo=false
```

- `var vacio Usuario` tiene `Nombre: ""`, `Email: ""`, `Estado: ""` y `Creado` en la fecha
  cero (`0001-01-01`). Es seguro de leer: `vacio.Activo()` devuelve `false` sin panic.
- El `%+v` **no** muestra los campos: muestra `String()`, que con campos vacíos da `" <> []"`.
  Es la prueba de lo que dijimos en el paso 4. Si querés ver los campos, `%#v`.
- `%t` imprime el booleano.

Este es el argumento más fuerte a favor de los zero values: podés tener un struct sin
inicializar y no explota. El costo es que tenés que validar antes de usarlo como dato real;
por eso existe el constructor.

## Todo junto

El orden del archivo es: tipos (`Estado`, `Usuario`), constructor, métodos de instancia,
`String`, `Admin`, y `main`. Cada bloque de `main` corresponde a un paso: crear, imprimir,
suspender, serializar, embeber, copiar, vacío. Leelo de corrido.

## Ejecutar

```bash
go run ./05-structs-metodos
```

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| `copia` modificado y el original también | Es un slice o un map | `slices.Clone`, `maps.Clone` |
| `Suspender` no cambia el estado | Receptor valor | Receptor puntero |
| `fmt.Println(u)` imprime `&{...}` | Falta `String()` o el receptor es puntero y el valor no es direccionable | Agregá `String()` con receptor valor |
| JSON con `"Nombre"` | Falta el tag | `` `json:"nombre"` `` |
| `cannot use Usuario as Estado` | Tipos distintos aunque el base sea string | `Estado(s)` explícito |
| Fecha corrida unas horas | Guardaste hora local | `.UTC()` al guardar |

## Por qué importa en el trabajo

- **Modelos de dominio** con constructor y `String()` son la base de cualquier servicio: se
  validan una vez y se loguean legibles.
- **Tags JSON** son el contrato de tu API. Cambiar un tag rompe clientes.
- **Embedding** es cómo se reutiliza comportamiento sin herencia: `Admin` tiene un
  `Usuario` y agrega permisos, sin una jerarquía frágil.
- **Value semantics** explica por qué en Go se pasan punteros a estructuras grandes y
  valores a las chicas; no es solo rendimiento, es qué puede mutar cada función.

## Ejercicio

1. Agregá `Direccion` (`Calle`, `Ciudad`, `CP`) como campo embebido opcional
   `*Direccion` con tag `json:"direccion,omitempty"`, y un método `DireccionCompleta() string`
   que devuelva `""` si el puntero es nil. Verificá el JSON con y sin dirección.
2. Hacé que `String()` muestre `"suspendido"` en mayúsculas sin romper `%s` en otros lados.
3. Implementá `func (u *Usuario) Renombrar(nombre string) error` que normalice y valide
   igual que el constructor, y usala desde `main`. ¿Qué pasa si `u` es nil?

Siguiente: [06 — Interfaces](../06-interfaces/).
