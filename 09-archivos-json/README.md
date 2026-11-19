# 09 — Archivos y JSON

**Qué vas a construir:** persistir una lista de tareas en un archivo JSON sin corromperla si
el proceso muere a mitad de escritura. Es el patrón de escritura atómica que vas a reusar
para configuración, cachés y exportaciones.

**Qué necesitas antes:** capítulos 01 a 08.

---

## Paso 1 — El modelo y los tags

```go
type Tarea struct {
	ID     int    `json:"id"`
	Titulo string `json:"titulo"`
	Hecha  bool   `json:"hecha"`
}
```

Los tags `json:"..."` definen cómo se llama cada campo en el archivo: `id`, `titulo`,
`hecha`. Sin tag, `encoding/json` usaría el nombre exacto del campo (`ID`, `Titulo`,
`Hecha`), con mayúscula. En un archivo de datos o una API se usa minúscula.

`encoding/json` es a Go lo que Jackson a Java o `System.Text.Json` a C#: serializa structs
a JSON y viceversa, guiándose por tags y nombres de campos. No hace falta registrar el tipo
ni generar código.

## Paso 2 — Leer: `cargar`

```go
func cargar(ruta string) ([]Tarea, error) {
	b, err := os.ReadFile(ruta)
	if errors.Is(err, os.ErrNotExist) {
		return []Tarea{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("leer %s: %w", ruta, err)
	}
```

- **`os.ReadFile`** abre, lee todo, cierra y devuelve `[]byte`. Es un atajo cómodo para
  archivos chicos. Para archivos enormes conviene `os.Open` + `bufio.Scanner`, como en el
  capítulo 06.
- **`errors.Is(err, os.ErrNotExist)`** distingue "el archivo no existe" de cualquier otro
  error. Para un archivo de datos, el primer arranque no es un fallo: devolvés una lista
  vacía. Si tratás ese caso como error, el servicio no arranca nunca la primera vez.
- **Cualquier otro error se envuelve con `%w`**: el mensaje dice qué archivo, y la causa
  (permisos, disco lleno) queda disponible para `errors.Is`.
- **`[]Tarea{}` vs `nil`**: se devuelve un slice vacío, no nil. Los dos tienen `len 0`, pero
  el vacío se serializa como `[]` y nil como `null`. Para una API, `[]` es mejor.

```go
	var tareas []Tarea
	if err := json.Unmarshal(b, &tareas); err != nil {
		return nil, fmt.Errorf("parsear %s: %w", ruta, err)
	}
	return tareas, nil
```

- **`json.Unmarshal(b, &tareas)`** llena el slice. El `&` es obligatorio: la función
  necesita modificar tu variable, y en Go los argumentos son copias (capítulo 05).
- Si el JSON está mal formado, el error se envuelve con el nombre del archivo. Sin ese
  contexto, `invalid character 'n'` no te dice **cuál** de tus diez archivos falló.
- Un detalle: `Unmarshal` **ignora campos desconocidos** del JSON. Si el archivo tiene
  `"prioridad": 1` y el struct no, no falla. Para rechazarlos se usa
  `dec.DisallowUnknownFields()`, que requiere un `Decoder` en vez de `Unmarshal`.

## Paso 3 — Escribir sin corromper: `guardar`

La forma ingenua es `os.WriteFile(ruta, b, 0o644)`. El problema: si el proceso muere a mitad
de escritura, el archivo queda truncado y perdiste todo. La solución es escribir en un
temporal y renombrar:

```go
f, err := os.CreateTemp(filepath.Dir(ruta), ".tareas-*.tmp")
if err != nil {
	return fmt.Errorf("crear temporal: %w", err)
}
tmp := f.Name()
defer os.Remove(tmp) // no-op si el rename tuvo éxito
```

- **`filepath.Dir(ruta)`** devuelve la carpeta del destino. El temporal se crea **en la misma
  carpeta**, y eso no es un detalle: `os.Rename` es atómico solo dentro del mismo
  filesystem. Si el temporal estuviera en `C:` y el destino en `D:`, el rename puede fallar
  o no ser atómico.
- **`.tareas-*.tmp`**: el `*` lo reemplaza `CreateTemp` por un número aleatorio, así dos
  procesos no chocan.
- **`defer os.Remove(tmp)`**: si algo falla antes del rename, el temporal se borra. Si el
  rename tuvo éxito, el archivo ya no existe en esa ruta y el `Remove` es un no-op. Es una
  red de seguridad, no el camino normal.

```go
enc := json.NewEncoder(f)
enc.SetIndent("", "  ")
if err := enc.Encode(tareas); err != nil {
	f.Close()
	return fmt.Errorf("codificar json: %w", err)
}
```

- **`json.NewEncoder(f)`** escribe directo al archivo, sin armar todo el JSON en memoria. Para
  listas grandes es mejor que `Marshal` + `WriteFile`.
- **`SetIndent("", "  ")`** hace el archivo legible (dos espacios). Sin esto, todo va en una
  línea.
- **`Encode` agrega un `\n` al final**, por eso el archivo termina bien.
- **`f.Close()` manual en el error**: acá no hay `defer f.Close()` porque el cierre tiene que
  ocurrir **antes** del rename. Un `defer` correría después de renombrar, cuando ya no
  importa (o peor, después de devolver el error). Por eso se cierra a mano en cada camino.

```go
if err := f.Sync(); err != nil {
	f.Close()
	return fmt.Errorf("sync: %w", err)
}
if err := f.Close(); err != nil {
	return fmt.Errorf("cerrar temporal: %w", err)
}
if err := os.Rename(tmp, ruta); err != nil { // atómico en el mismo filesystem
	return fmt.Errorf("renombrar: %w", err)
}
```

- **`f.Sync()`** fuerza a que los datos lleguen al disco, no solo al buffer del sistema
  operativo. Sin esto, un corte de luz puede dejar el archivo vacío aunque `Close` haya
  devuelto bien. Es más lento, y por eso se usa en escrituras críticas, no en cada log.
- **`f.Close()`** antes del rename: en Windows no podés renombrar un archivo abierto. En
  Linux se podría, pero cerrarlo primero es lo correcto en todos lados.
- **`os.Rename`** reemplaza el destino de forma atómica. El lector ve el archivo viejo o el
  nuevo, nunca uno a medias. Esa es toda la magia.

El orden es: escribir → `Sync` → `Close` → `Rename`. Saltearse cualquiera de los cuatro
rompe la garantía.

## Paso 4 — El flujo de `main`

```go
ruta := "tareas.json"
_ = os.Remove(ruta) // ejemplo repetible

tareas, err := cargar(ruta)
if err != nil {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
fmt.Printf("cargadas %d tareas\n", len(tareas))
```

- **`os.Remove`** con `_ =` ignora el error a propósito: si el archivo no existe, no importa.
  Es solo para que el ejemplo dé el mismo resultado en cada corrida.
- **`cargar`** en el primer arranque devuelve lista vacía, no error. Por eso imprime `0`.
- **`Fprintln(os.Stderr, ...)`** separa el error de la salida normal, y `os.Exit(1)` corta.
  Fijate que no hay `defer` pendiente en `main`; si lo hubiera, `Exit` no lo ejecutaría.

```go
tareas = append(tareas,
	Tarea{ID: 1, Titulo: "Aprender Go"},
	Tarea{ID: 2, Titulo: "Escribir tests", Hecha: true},
)
if err := guardar(ruta, tareas); err != nil { ... }
```

`append` con varios elementos de una vez; `Hecha: true` en el segundo, `false` (zero value)
en el primero. Después se guarda.

```go
tareas, err = cargar(ruta)
...
for _, t := range tareas {
	fmt.Printf("  #%d %-20s hecha=%t\n", t.ID, t.Titulo, t.Hecha)
}
```

```text
cargadas 0 tareas
  #1 Aprender Go          hecha=false
  #2 Escribir tests       hecha=true
```

- **`tareas, err = cargar(...)`** usa `=` y no `:=` porque las variables ya existen. Es el
  patrón de "releer para verificar".
- **`%-20s`** alinea el título a la izquierda en 20 caracteres como mínimo: si es más corto,
  rellena con espacios; si es más largo, no corta. Sin eso, las columnas se tuercen.
- **`#`** es texto literal, **`%d`** el ID, **`%t`** el booleano.

## Paso 5 — El error que explica

```go
_ = os.WriteFile("roto.json", []byte("{no es json"), 0o600)
if _, err := cargar("roto.json"); err != nil {
	fmt.Println("error esperado:", err)
}
```

```text
error esperado: parsear roto.json: invalid character 'n' looking for beginning of object key string
```

`0o600` son permisos: solo el dueño lee y escribe. El contenido es JSON inválido a propósito.
El mensaje tiene dos capas:

- `invalid character 'n' ...` viene de `encoding/json`: dice qué está mal.
- `parsear roto.json: ...` lo agregamos nosotros con `%w`: dice **dónde**.

Ninguna de las dos sola alcanza. Por eso se envuelve siempre con el contexto que la capa de
abajo no tiene.

Al final, `os.Remove` limpia `roto.json` y `tareas.json` para que puedas correr el ejemplo
de nuevo.

## Todo junto

`main.go` tiene tres funciones: el modelo, `cargar` y `guardar`. `main` las usa en el orden
real de un servicio: leer al arrancar, modificar, escribir, releer. La parte de escritura
atómica es la que más importa en producción y la que más se copia mal.

## Ejecutar

```bash
go run ./09-archivos-json
```

El ejemplo borra sus archivos al final. Si querés ver el JSON, comentá los últimos `Remove`
y volvé a correrlo.

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| `invalid character ...` sin archivo | No envolviste con `%w` | `fmt.Errorf("parsear %s: %w", ruta, err)` |
| Archivo vacío después de un corte | No usaste `Sync` | `f.Sync()` antes de `Close` |
| `rename ...: Access is denied` en Windows | Archivo abierto | `Close` antes de `Rename` |
| `cannot unmarshal number into Go struct field` | Tipo del campo no coincide | Revisá el struct o el JSON |
| El JSON sale en una línea | Falta `SetIndent` | `enc.SetIndent("", "  ")` |
| `null` en vez de `[]` | Devolviste un slice nil | `return []Tarea{}, nil` |

## Por qué importa en el trabajo

- **Escritura atómica** es cómo se guardan configuraciones y cachés sin quedar corruptas. El
  patrón temporal + `Sync` + `Rename` se repite en todo sistema serio.
- **Errores con contexto** convierten un mensaje inútil en uno accionable.
- **JSON-lines vs JSON completo**: un archivo entero se reescribe; JSON-lines (capítulo 06)
  se agrega al final. Elegir bien depende de si leés todo o agregás eventos.
- **`DisallowUnknownFields`** es la diferencia entre aceptar un typo en silencio y devolver
  400. En una API interna, mejor 400.

## Ejercicio

1. Agregá `Prioridad int` al struct con tag `"prioridad"` y `omitempty`. Guardá una tarea sin
   prioridad y mirá el archivo: el campo no aparece. ¿Qué pasa al releerlo?
2. Hacé `guardar` que reciba un `io.Writer` además de la ruta, y escribí el JSON también a
   `os.Stdout` para depurar. Es el mismo `io.Writer` del capítulo 06.
3. Provocá el error de rename abriendo el archivo antes de guardar en Windows. Después
   arreglalo cerrando antes. Ese bug es real y aparece en producción.

Siguiente: [10 — Testing](../10-testing/).
