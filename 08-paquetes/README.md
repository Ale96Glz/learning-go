# 08 — Paquetes

**Qué vas a construir:** un paquete reutilizable con API pública mínima y estado privado,
consumido desde `main`. Es la unidad de organización de Go: no hay clases ni namespaces,
hay paquetes.

**Qué necesitas antes:** capítulos 01 a 07.

---

## Paso 1 — Qué es un paquete y cómo se importa

Un **paquete** es una carpeta con archivos `.go` que declaran el mismo `package`. Todos los
archivos de una carpeta comparten el paquete y pueden usar los nombres privados entre sí.

```text
08-paquetes/
├── main.go                  package main
└── internal/
    └── saludo/
        └── saludo.go        package saludo
```

La ruta de import se arma con el **módulo** (el `module` de `go.mod`) más las carpetas:

```go
import "tutorialgo/08-paquetes/internal/saludo"
```

```text
tutorialgo         <- module en go.mod
  08-paquetes      <- carpeta
    internal       <- carpeta
      saludo       <- carpeta = paquete
```

El nombre con el que lo usás (`saludo.Hola`) es el `package saludo` declarado adentro, no la
última carpeta. Casi siempre coinciden; cuando no, manda el `package`. Por eso es una mala
idea que no coincidan.

Comparado con Java/C#: un paquete se parece a un *namespace*, pero arrastra código, no solo
nombres. Y a diferencia de una clase, no hay `public`/`private` como palabra clave: la
visibilidad se decide por la **mayúscula inicial** de cada identificador.

## Paso 2 — Exportado vs privado: la mayúscula

```go
type Saludo struct {     // exportado: se ve desde otros paquetes
	nombre string        // privado: solo saludo.go y compañía
	idioma string        // privado
	veces  int           // privado
}

func NuevoSaludo(...) *Saludo   // exportado
func formatear(...) string      // privado
```

La regla es una sola: **si empieza con mayúscula, es público; si empieza con minúscula, es
privado**. Aplica a tipos, funciones, campos, constantes y variables.

Esto tiene consecuencias que en Java/C# no ves:

- No podés tener un campo público y un getter privado del mismo nombre. O el campo es
  público (y cualquiera lo toca) o es privado (y necesitás un método).
- Los campos privados de un struct **no se pueden inicializar desde otro paquete**:
  ```go
  s := saludo.Saludo{nombre: "Ana"} // error: nombre is not exported
  ```
  Por eso existe el constructor: es la única puerta de entrada.
- Un tipo exportado con todos los campos privados es una **cápsula**: se usa a través de
  métodos, y el paquete controla las invariantes.

En Java/C# declarás `private` explícitamente en cada miembro; acá el default visual es
minúscula, y la ausencia de palabra clave hace que la API pública sea fácil de escanear: es
lo que empieza con mayúscula.

## Paso 3 — El constructor y los defaults

```go
func NuevoSaludo(nombre, idioma string) *Saludo {
	if idioma == "" {
		idioma = "es"
	}
	return &Saludo{nombre: nombre, idioma: idioma}
}
```

- Convención `NewX`: devuelve un valor listo para usar.
- Si no pasan idioma, aplica `"es"`. El default vive **en el paquete**, no en cada caller.
- Devuelve `*Saludo` para que `Texto()` pueda mutar `veces`.
- `veces` queda en su zero value: `0`. No hace falta asignarlo.

No hay constructores mágicos, ni `init` de struct, ni sobrecarga de `new`. Una función
`NewX` es todo.

## Paso 4 — El método que muta y oculta

```go
func (s *Saludo) Texto() string {
	s.veces++
	return formatear(s.idioma, s.nombre) + fmt.Sprintf(" (llamada %d)", s.veces)
}
```

- **`s.veces++`** modifica el struct. Por eso el receptor es puntero: con receptor valor se
  modificaría una copia y el contador siempre daría 1.
- **`formatear` es privada**: es un detalle de implementación. El consumidor no puede
  llamarla ni sabe que existe.
- **`fmt.Sprintf`** devuelve un string; no imprime. Se concatena con `+`.
- **`%d`** imprime el contador.

`main` solo puede llamar a `Texto()`. No puede leer `s.veces`, ni resetearlo, ni saltarse el
incremento. Eso es encapsulamiento real: el estado y las reglas viven juntos.

## Paso 5 — Una función de paquete

```go
func Hola(nombre string) string {
	return formatear("es", nombre)
}
```

No todo es un método. `Hola` es una función suelta que no necesita estado. La regla:

- Si la operación depende de datos del tipo, es **método** (`s.Texto()`).
- Si es una operación pura sobre parámetros, es **función** (`saludo.Hola("Ana")`).

En Java/C# todo tiende a meterse en una clase aunque no tenga estado (una clase `Utils` con
métodos estáticos). En Go una función de paquete es lo normal y no necesita contenedor.

## Paso 6 — El formateo privado

```go
func formatear(idioma, nombre string) string {
	switch idioma {
	case "en":
		return "Hello, " + nombre
	case "pt":
		return "Olá, " + nombre
	default:
		return "Hola, " + nombre
	}
}
```

`switch` sobre string, con `default` como red de seguridad. Devuelve en cada rama, así que no
hace falta `break` (capítulo 03). La concatenación con `+` alcanza para dos strings; cuando
hay muchos huecos o tipos, `fmt.Sprintf` es más claro.

Este es el punto del capítulo: `formatear` puede cambiar mañana (agregar `"fr"`, usar
`Sprintf`, cargar plantillas) y `main` no se entera. La API pública es `Hola` y `Texto`; el
resto es libertad del paquete.

## Paso 7 — `main` como consumidor

```go
func main() {
	fmt.Println(saludo.Hola("Ana"))

	s := saludo.NuevoSaludo("Beto", "es")
	fmt.Println(s.Texto())
	fmt.Println(s.Texto())
}
```

```text
Hola, Ana
Hola, Beto (llamada 1)
Hola, Beto (llamada 2)
```

- `saludo.Hola` es el nombre completamente calificado. No hay `using` ni `import static`:
  siempre `paquete.Función`.
- La segunda y tercera línea muestran el estado privado funcionando: `main` no toca `veces`,
  pero cada llamada lo incrementa.
- `fmt.Println` no usa verbos: imprime el string y agrega salto.

## Paso 8 — `internal/`: el candado del módulo

La carpeta `internal` tiene un significado especial: **solo el código dentro del árbol que
la contiene puede importarla**.

```text
tutorialgo/08-paquetes/internal/saludo   -> lo puede importar 08-paquetes y sus hijos
tutorialgo/otro-capitulo                 -> NO puede
otro módulo                              -> NO puede
```

¿Para qué? Para separar lo que es API pública del módulo de lo que es implementación
interna. Si mañana cambiás `saludo`, sabés que nadie de afuera depende de esos detalles. Es
el equivalente a `internal` en Java (paquete privado de módulo) o a `internal` en C#, pero
verificado por el compilador.

En el proyecto final, todos los paquetes del servicio están en
`proyecto/internal/...`: nadie fuera del módulo puede importarlos.

## Paso 9 — Leer un paquete sin abrir el código

```bash
go doc ./08-paquetes/internal/saludo
```

```text
package saludo // import "tutorialgo/08-paquetes/internal/saludo"

func Hola(nombre string) string
type Saludo struct{ ... }
    func NuevoSaludo(nombre, idioma string) *Saludo
```

Muestra **solo lo exportado**, con los comentarios de documentación. `formatear` no
aparece, aunque exista. Ese es el contrato que ve un consumidor.

Si escribís `go doc` en el código, te acostumbrás a documentar lo público y a mantener la
superficie chica.

## Todo junto

El módulo tiene dos paquetes: `main` (el ejecutable) y `saludo` (la librería). `main`
importa, `saludo` exporta lo mínimo. El estado (`veces`) y la lógica (`formatear`) quedan
adentro. Ese corte es el que después se repite en capas: handler, servicio, repositorio.

## Ejecutar

```bash
go run ./08-paquetes
go doc ./08-paquetes/internal/saludo
go list ./...        # todos los paquetes del módulo
```

## Errores que vas a ver

| Mensaje | Qué pasó | Solución |
|---|---|---|
| `use of internal package not allowed` | importaste `internal` desde afuera | mové el import o el paquete |
| `cannot refer to unexported field` | campo en minúscula desde otro paquete | usá el constructor o un método |
| `undefined: saludo.Hola` | no importaste o el nombre es privado | revisá import y mayúscula |
| `import cycle not allowed` | A importa B y B importa A | extraé lo común a un tercer paquete |
| `found packages main and saludo` | dos `package` en la misma carpeta | un paquete por carpeta |
| `no required module provides package` | ruta mal escrita o falta `go get` | revisá el `module` y la ruta |

## Por qué importa en el trabajo

- **La API pública es un contrato**: cada nombre exportado es algo que otros pueden usar y
  de lo que no podés arrepentirte. Exportá poco.
- **`internal/`** evita que un paquete se convierta en dependencia accidental de otro
  equipo.
- **Un paquete por responsabilidad**: `auth`, `tareas`, `config`. Si un paquete necesita
  conocer diez cosas del resto, está mal cortado.
- **`go doc`** es cómo se explora una dependencia sin leer miles de líneas.

## Ejercicio

1. Agregá el idioma `"fr"` a `formatear` y verificá que `main` no cambia. Ese es el test de
   un buen encapsulamiento.
2. Agregá `func (s *Saludo) Reset()` que ponga `veces` en 0. ¿Por qué el receptor tiene que
   ser puntero?
3. Mové `Saludo` a `08-paquetes/saludo` (sin `internal`) e intentá importarlo desde otro
   capítulo. Después volvé a `internal` y comprobá que ya no se puede. Ese es el candado.

Siguiente: [09 — Archivos y JSON](../09-archivos-json/).
