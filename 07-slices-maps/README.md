# 07 — Slices y maps

**Qué vas a construir:** una lista de tareas que se filtra, se ordena, se agrupa por
etiqueta y se deduplica. Son las cuatro operaciones que vas a hacer sobre datos en cualquier
servicio.

**Qué necesitas antes:** capítulos 01 a 06.

---

## Paso 1 — El modelo y el slice literal

```go
type Tarea struct {
	ID        int
	Titulo    string
	Prioridad int // 1 = alta
	Hecha     bool
	Etiquetas []string
}
```

`Etiquetas` es un slice dentro de un struct. Eso significa que el struct tiene una cabecera
de slice (puntero, largo y capacidad), no los strings en sí. Copiar el struct copia la
cabecera, pero **comparte el array** de etiquetas. Es la trampa del capítulo 05, y acá la
vas a ver en acción.

```go
tareas := []Tarea{
	{ID: 1, Titulo: "Desplegar", Prioridad: 1, Etiquetas: []string{"devops", "urgente"}},
	...
}
```

- `[]Tarea{...}` es un slice literal. Los elementos internos omiten el tipo (`{ID: 1, ...}`
  en vez de `Tarea{ID: 1, ...}`) porque ya se sabe cuál es.
- Se inicializan **con nombres de campo**: si mañana agregás un campo, no se rompe.
- Un slice es la versión Go de `ArrayList`/`List<T>`: tamaño dinámico, acceso por índice.

## Paso 2 — `append`: siempre reasigná

```go
tareas = append(tareas, Tarea{ID: 5, Titulo: "Tests", ...})
```

`append` **devuelve un slice nuevo**. Si el array interno tiene capacidad libre, escribe ahí
y devuelve el mismo array con `len` más grande; si no, reserva uno nuevo, copia todo y
devuelve ese. En los dos casos, el slice que te importa es el retorno.

```go
append(tareas, t)   // error lógico: el elemento se descarta
```

Eso compila (el valor de retorno se puede ignorar) y no hace nada visible: es un bug
silencioso. **Siempre `x = append(x, ...)`.**

Detalle de rendimiento: `append` a un slice de 10.000 elementos que crece de a uno reasigna
unas cuantas veces (Go duplica la capacidad), no 10.000. Aun así, si sabés el tamaño final,
reservalo:

```go
pendientes := make([]Tarea, 0, len(tareas))
```

`make([]T, 0, n)` crea un slice vacío (`len 0`) con capacidad `n`. Es la diferencia entre
una asignación y quince.

## Paso 3 — Filtrar: creá un slice nuevo

```go
pendientes := make([]Tarea, 0, len(tareas))
for _, t := range tareas {
	if !t.Hecha {
		pendientes = append(pendientes, t)
	}
}
fmt.Printf("pendientes: %d/%d\n", len(pendientes), len(tareas))
```

```text
pendientes: 4/5
```

La tentación es borrar del slice original mientras lo recorrés. No lo hagas: al borrar, los
índices se corren y el `range` se salta elementos. Crear uno nuevo es más simple y no tiene
esos bordes.

`len(pendientes)` y `len(tareas)` son largos actuales, no capacidades. `%d/%d` imprime los
dos enteros.

Fijate que los elementos son **copias** del struct. Modificar `pendientes[0].Titulo` no
cambia `tareas[0].Titulo`. Pero modificar `pendientes[0].Etiquetas[0]` **sí** cambia el
original, porque el slice interno se comparte.

## Paso 4 — Buscar y borrar con el paquete `slices`

```go
if slices.ContainsFunc(tareas, func(t Tarea) bool { return t.Titulo == "Tests" }) {
	fmt.Println("hay una tarea de Tests")
}
```

`slices.ContainsFunc` recorre y devuelve `true` si algún elemento cumple la función. La
función es un predicado: recibe un elemento y devuelve bool. No hay que escribir el loop.

```go
original := len(tareas)
tareas = slices.DeleteFunc(tareas, func(t Tarea) bool { return t.ID == 4 })
fmt.Printf("borradas: %d\n", original-len(tareas))
```

```text
hay una tarea de Tests
borradas: 1
```

`slices.DeleteFunc` mueve los elementos para tapar el hueco y devuelve el slice con el nuevo
`len`. **Modifica el array subyacente**: si tenías otra variable apuntando al mismo array,
también ve el cambio. Y hay que reasignar (`tareas = ...`), porque el `len` cambió.

`original-len(tareas)` es la cuenta de borrados. Guardar el largo antes es el patrón para
saber cuántos se eliminaron.

## Paso 5 — Ordenar sin escribir un algoritmo

```go
slices.SortFunc(pendientes, func(a, b Tarea) int {
	if a.Prioridad != b.Prioridad {
		return a.Prioridad - b.Prioridad // 1 (alta) primero
	}
	return strings.Compare(a.Titulo, b.Titulo)
})
```

```text
por prioridad:
  P1 Desplegar
  P1 Tests
  P2 Code review
  P3 Documentar
```

El comparador recibe dos elementos y devuelve un entero con tres significados:

- **negativo**: `a` va antes que `b`,
- **cero**: son equivalentes,
- **positivo**: `a` va después.

Acá: si las prioridades difieren, la resta da negativo cuando `a` es menor (prioridad 1
primero). Si empatan, `strings.Compare` ordena alfabéticamente. `strings.Compare` ya devuelve
-1, 0 o 1, así que se puede retornar directo.

Dos advertencias:

1. La resta funciona con `int` chicos, pero si los valores pueden ser enormes (o `int64`),
   puede desbordar. En ese caso usá comparaciones explícitas (`if a < b { return -1 }`).
2. `SortFunc` **no es estable**: dos tareas con la misma prioridad y mismo título pueden
   cambiar de orden entre corridas. Si el orden de los empates importa, usá
   `slices.SortStableFunc`.

`%d` en `P%d` es la prioridad; la `P` es texto literal.

## Paso 6 — Agrupar con `map[string][]string`

```go
porEtiqueta := make(map[string][]string)
for _, t := range tareas {
	for _, e := range t.Etiquetas {
		porEtiqueta[e] = append(porEtiqueta[e], t.Titulo)
	}
}
```

- Un map de string a slice de strings: "etiqueta → títulos".
- **`porEtiqueta[e]` de una clave que no existe devuelve el zero value**: `nil` para un
  slice. Y `append(nil, x)` funciona. Por eso no hace falta inicializar cada clave:
  ```go
  porEtiqueta[e] = append(porEtiqueta[e], t.Titulo)
  ```
  hace "leer, agregar, guardar" en una línea.

### Imprimir un map en orden

```go
claves := make([]string, 0, len(porEtiqueta))
for k := range porEtiqueta {
	claves = append(claves, k)
}
slices.Sort(claves)
for _, k := range claves {
	fmt.Printf("  %-8s %v\n", k, porEtiqueta[k])
}
```

```text
por etiqueta:
  calidad  [Tests]
  devops   [Desplegar Code review]
  docs     [Documentar]
  equipo   [Code review]
  urgente  [Desplegar]
```

`range` sobre un map **no garantiza orden** (Go lo randomiza). Para imprimir estable se
extraen las claves, se ordenan y se recorre el slice. Es el patrón de todo reporte o JSON
estable.

- `%-8s` alinea la clave a la izquierda en 8 caracteres como mínimo, para que la segunda
  columna quede derecha.
- `%v` sobre un `[]string` imprime `[a b c]`.

## Paso 7 — Contar con un map

```go
prioridad := map[int]int{}
for _, t := range tareas {
	prioridad[t.Prioridad]++
}
fmt.Println("\nconteo por prioridad:", prioridad)
delete(prioridad, 3)
```

```text
conteo por prioridad: map[1:2 2:1 3:1]
sin prioridad 3: map[1:2 2:1]
```

`prioridad[t.Prioridad]++` funciona sin inicializar porque la lectura de una clave ausente
devuelve `0`, y `0 + 1 = 1`. Es el contador idiomático de Go.

`delete(m, k)` borra la clave. Borrar una clave inexistente **no falla**; es un no-op.

Dato curioso pero útil: `fmt` imprime los maps **con las claves ordenadas**, para que la
salida sea estable. Pero `range` no. No confundas "se ve ordenado al imprimir" con "está
ordenado al recorrer".

## Paso 8 — La trampa del map nil

```go
var nilMap map[string]int
fmt.Println("leer map nil:", nilMap["x"], "len:", len(nilMap))
// nilMap["x"] = 1 // panic: assignment to entry in nil map
```

```text
leer map nil: 0 len: 0
```

- **Leer** de un map nil devuelve el zero value: es seguro.
- **Escribir** en un map nil hace panic. Por eso todo map se crea con `make(...)` o con un
  literal `map[string]int{}`. El literal vacío es válido y ya está inicializado.
- `len(nilMap)` es 0.

Un map nil es útil para "no hay datos" y para comparar con `nil`, pero no para acumular.

## Paso 9 — Copiar un slice: `Clone` vs asignación

```go
base := []int{1, 2, 3}
copia := slices.Clone(base)
copia[0] = 99
fmt.Println("base:", base, "copia:", copia)
```

```text
base: [1 2 3] copia: [99 2 3]
```

```go
copia := base        // comparte el array: copia[0] = 99 cambia base
copia := slices.Clone(base) // copia real
```

- La asignación copia la **cabecera** (puntero, largo, capacidad). Los dos slices apuntan al
  mismo array.
- `slices.Clone` reserva un array nuevo y copia los elementos. `Clone` de un slice nil
  devuelve nil.
- Lo mismo aplica a maps: `maps.Clone` para una copia superficial. Si los valores son
  punteros o slices, la copia sigue siendo superficial.

## Paso 10 — Un set y deduplicar

```go
entrada := []string{"a", "b", "a", "c", "b"}
vistos := map[string]struct{}{}
unicos := make([]string, 0, len(entrada))
for _, s := range entrada {
	if _, ok := vistos[s]; ok {
		continue
	}
	vistos[s] = struct{}{}
	unicos = append(unicos, s)
}
fmt.Println("únicos:", unicos)
```

```text
únicos: [a b c]
```

- **`struct{}`** es un tipo vacío que ocupa **0 bytes**. Un `map[string]struct{}` es el set
  idiomático de Go: solo importa si la clave está, no hay valor que guardar.
- `vistos[s] = struct{}{}` agrega; `_, ok := vistos[s]` consulta.
- El orden de `unicos` es el de la primera aparición, porque se construye desde `entrada`,
  no desde el map. Si deduplicás recorriendo el map, perdés el orden.

## Todo junto

`main.go` es una secuencia de operaciones sobre la misma lista: agregar, filtrar, buscar,
borrar, ordenar, agrupar, contar y deduplicar. Cada bloque está separado por un comentario.
Leelo de corrido después de este capítulo y no debería quedar ninguna línea sin explicar.

## Ejecutar

```bash
go run ./07-slices-maps
```

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| `append` no agrega nada | no reasignaste | `x = append(x, ...)` |
| `assignment to entry in nil map` | map sin `make` | `make(map[K]V)` o literal |
| se saltean elementos al borrar en un `range` | índices que se corren | filtrar a un slice nuevo |
| el map cambia de orden | `range` randomiza | ordená las claves |
| `copia[0]` cambia el original | copiaste la cabecera | `slices.Clone` |
| `index out of range` | accedés `s[i]` con `i == len(s)` | el último índice es `len-1` |
| comparador de `SortFunc` inconsistente | devuelve bool o no respeta el orden | -1 / 0 / 1 |

## Por qué importa en el trabajo

- **Filtrar y ordenar** es el 80% de un endpoint de listado (`GET /tareas?estado=...`).
- **Agrupar con maps** es cómo se arman reportes y respuestas anidadas.
- **`slices`** (Go 1.21+) reemplazó a `sort.Slice` y a los loops manuales: menos código y
  menos bugs.
- **La trampa de la copia** es la causa de bugs difíciles: dos partes del sistema modifican
  el mismo array sin saberlo.

## Ejercicio

1. Ordená `tareas` por `Hecha` (pendientes primero) y después por prioridad, usando
   `slices.SortStableFunc`. ¿Por qué estable?
2. Agregá una función `filtrar(tareas []Tarea, pred func(Tarea) bool) []Tarea` y reemplazá
   el loop del paso 3. Generalizala para cualquier tipo con generics
   (`func filtrar[T any](xs []T, pred func(T) bool) []T`). Es tu primera función genérica.
3. Contá cuántas tareas hay por etiqueta con `map[string]int` y devolvé las etiquetas
   ordenadas por cantidad descendente.

Siguiente: [08 — Paquetes](../08-paquetes/).
