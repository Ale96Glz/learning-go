# 06 — Interfaces

**Qué vas a construir:** un servicio que guarda usuarios, con dos implementaciones
intercambiables (memoria y archivo) y sin cambiar una línea del servicio. Es el capítulo
que explica la inyección de dependencias en Go, sin frameworks.

**Qué necesitas antes:** capítulos 01 a 05.

---

## Paso 1 — Una interfaz es un contrato implícito

```go
type Repositorio interface {
	Guardar(u Usuario) error
	Buscar(id int64) (Usuario, error)
}
```

Una interfaz dice: "cualquier tipo que tenga **estos métodos** puede usarse acá". No dice
quién lo implementa, ni hay que declararlo.

En Java/C# escribirías `class Memoria implements Repositorio` o `: IRepositorio`. En Go no
existe esa declaración: si el tipo tiene los métodos, **ya cumple**. Eso se llama
satisfacción implícita y tiene dos consecuencias enormes:

1. Podés hacer que un tipo tuyo cumpla una interfaz de una librería sin tocar la librería.
2. Las interfaces se pueden definir **después**, donde las necesitás, sin obligar a las
   implementaciones a conocerte.

El compilador lo verifica en el momento de asignar. Si `Memoria` no tuviera `Buscar`, esto
no compila:

```go
var r Repositorio = NuevaMemoria() // *Memoria does not implement Repositorio
```

### La interfaz nace del consumidor

```go
type Servicio struct {
	repo Repositorio
	log  io.Writer
}
```

`Repositorio` está declarada cerca de `Servicio`, que es quien la **usa**, no cerca de
`Memoria` o `Archivo`, que la cumplen. Esa es la convención de Go: **la interfaz pertenece
al consumidor**. Si mañana agregás `RepoSQL`, no tocás el servicio; solo agregás un tipo con
los dos métodos.

Y es **chica**: dos métodos. Cuanto menos pide, más tipos la cumplen y más fácil es
testearla. Una interfaz de diez métodos es casi siempre un diseño que conviene partir.

## Paso 2 — Implementación 1: memoria

```go
type Memoria struct {
	datos map[int64]Usuario
}

func NuevaMemoria() *Memoria {
	return &Memoria{datos: make(map[int64]Usuario)}
}
```

- `datos` es un map de ID a usuario. El zero value de un map es `nil`, y escribir en un map
  nil hace panic. Por eso el constructor hace `make`.
- `NuevaMemoria` devuelve `*Memoria`. El puntero es necesario: si devolviera un valor,
  cada copia tendría su propio map y los datos se perderían.

```go
func (m *Memoria) Guardar(u Usuario) error {
	if u.ID == 0 {
		return errors.New("id requerido")
	}
	m.datos[u.ID] = u
	return nil
}
```

Primero valida, después escribe. Un `ID == 0` no es un usuario válido: es el zero value,
señal de que nadie lo asignó. El método devuelve `error` aunque nunca falle por otra cosa,
porque así cumple la interfaz.

```go
func (m *Memoria) Buscar(id int64) (Usuario, error) {
	u, ok := m.datos[id]
	if !ok {
		return Usuario{}, ErrNoEncontrado
	}
	return u, nil
}
```

El **patrón `v, ok`**: una lectura de map devuelve dos valores, el valor y un bool que dice
si la clave existía. Es la forma de distinguir "el usuario está vacío" de "no existe".

Fijate en el valor de retorno del error: un `Usuario{}` vacío. Go no tiene `null` para
structs, así que devolvés el zero value y el caller **debe** mirar el error antes de usarlo.

`Memoria` es ideal para tests y prototipos. En producción no sirve: se pierde al reiniciar.

## Paso 3 — Implementación 2: archivo JSON-lines

### Guardar

```go
func (a *Archivo) Guardar(u Usuario) error {
	f, err := os.OpenFile(a.ruta, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("abrir %s: %w", a.ruta, err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(u); err != nil {
		return fmt.Errorf("escribir usuario: %w", err)
	}
	return nil
}
```

- **`os.OpenFile`** abre con flags. `O_CREATE` crea si no existe, `O_APPEND` escribe al
  final sin pisar lo anterior, `O_WRONLY` es solo escritura. Los flags se combinan con `|`
  porque son bits.
- **`0o644`** son los permisos para cuando el archivo se crea: lectura/escritura para el
  dueño, lectura para el resto. Es notación octal (`0o`). En Windows se traduce a algo
  aproximado; en Linux es lo que importa.
- **El error se envuelve con `%w`**: el mensaje dice qué archivo, y la causa original queda
  disponible para `errors.Is`.
- **`defer f.Close()`** va inmediatamente después de comprobar el error. Si `OpenFile` falla,
  `f` es nil y no hay que cerrar nada: por eso el `defer` va después.
- **`json.NewEncoder(f).Encode(u)`** escribe el struct como una línea JSON y agrega `\n`. Es
  el formato JSON-lines: una línea, un objeto. Ideal para logs y archivos de eventos, porque
  se puede leer secuencialmente sin cargar todo en memoria.
- Si `Encode` falla, se envuelve el error. Nota que el `defer` corre igual: el archivo se
  cierra en los dos caminos.

### Buscar

```go
func (a *Archivo) Buscar(id int64) (Usuario, error) {
	f, err := os.Open(a.ruta)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Usuario{}, ErrNoEncontrado
		}
		return Usuario{}, err
	}
	defer f.Close()
```

- **`os.ErrNotExist`** es el error centinela de "el archivo no existe". Se compara con
  `errors.Is`, no con `==`, porque el error puede venir envuelto por el sistema operativo.
- **"No existe el archivo" no es un fallo**: para un almacén de datos, el primer arranque
  tiene el archivo vacío. Se traduce a `ErrNoEncontrado`, que es un error de dominio.
- Cualquier **otro** error (permisos, disco) se devuelve tal cual: ese sí es un problema
  real y no debe confundirse con "no encontrado".

```go
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var u Usuario
		if err := json.Unmarshal(sc.Bytes(), &u); err != nil {
			return Usuario{}, fmt.Errorf("leer línea: %w", err)
		}
		if u.ID == id {
			return u, nil
		}
	}
	if err := sc.Err(); err != nil {
		return Usuario{}, err
	}
	return Usuario{}, ErrNoEncontrado
```

- **`bufio.Scanner`** lee línea por línea sin cargar el archivo entero. Por defecto acepta
  líneas de hasta 64 KB; si tus JSON son más grandes, hay que subir el buffer con
  `sc.Buffer(...)`. Es un límite real que muerde en producción.
- **`json.Unmarshal(sc.Bytes(), &u)`** llena `u` (por eso el `&`). Si una línea está
  corrupta, devuelve error y se envuelve con contexto.
- **`sc.Err()`** es obligatorio después del loop. `Scan()` devuelve `false` al terminar
  **o** al fallar; sin este chequeo, un error de lectura a mitad de archivo pasa como "no
  encontrado". Es el mismo patrón que vas a ver con `rows.Err()` en SQL.
- Si terminó el archivo sin match, `ErrNoEncontrado`. Un recorrido lineal es O(n); para
  búsquedas frecuentes esto no escala, y ahí entra una base de datos (capítulo 15).

## Paso 4 — El servicio depende de la interfaz

```go
type Servicio struct {
	repo Repositorio
	log  io.Writer
	seq  int64
}

func NuevoServicio(repo Repositorio, log io.Writer) *Servicio {
	return &Servicio{repo: repo, log: log}
}
```

- **`repo Repositorio`**: puede ser memoria, archivo o SQL. El servicio no sabe cuál.
- **`log io.Writer`**: `io.Writer` es la interfaz más famosa de Go:
  ```go
  type Writer interface {
      Write(p []byte) (n int, err error)
  }
  ```
  Cualquier cosa que sepa escribir bytes la cumple: `os.Stdout`, un `*os.File`, un
  `bytes.Buffer` (para tests), o hasta el `http.ResponseWriter` de la API. Al pedir
  `io.Writer` en vez de `*os.File`, el servicio se puede testear sin tocar el disco.
- **`seq`** genera IDs. En un servicio real vendrían de la base de datos.

```go
func (s *Servicio) Crear(nombre string) (Usuario, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return Usuario{}, errors.New("nombre vacío")
	}
	s.seq++
	u := Usuario{ID: s.seq, Nombre: nombre}
	if err := s.repo.Guardar(u); err != nil {
		return Usuario{}, fmt.Errorf("guardar usuario: %w", err)
	}
	fmt.Fprintf(s.log, "usuario creado: %d %s\n", u.ID, u.Nombre)
	return u, nil
}
```

- Valida, asigna ID y delega el guardado en `repo`. **No hay `if esMemoria` ni `switch`**:
  eso es el punto del capítulo.
- **`fmt.Fprintf(s.log, ...)`** es `Printf` pero escribe en un `io.Writer`. `%d` es el ID,
  `%s` el nombre. Funciona igual con la consola, un archivo o un buffer de test.
- Si `Guardar` falla, el error se envuelve y se devuelve. El `seq` ya se incrementó: en un
  sistema real, el ID lo asigna la base y este problema no existe.

## Paso 5 — `main` elige la implementación

```go
repos := []Repositorio{NuevaMemoria(), NuevoArchivo(ruta)}

for i, repo := range repos {
	fmt.Printf("--- repositorio #%d (%T) ---\n", i+1, repo)
	svc := NuevoServicio(repo, os.Stdout)
	...
}
```

- Un slice de **interfaces**. Adentro hay un `*Memoria` y un `*Archivo`, tipos distintos, y
  el loop los trata igual. Eso es polimorfismo, sin herencia.
- **`%T`** imprime el **tipo dinámico**, el que está guardado adentro de la interfaz:
  `*main.Memoria`, `*main.Archivo`. Es la forma rápida de confirmar qué implementación te
  tocó.
- `NuevoServicio(repo, os.Stdout)`: `os.Stdout` cumple `io.Writer`, así que los logs van a
  consola. En un test pasarías un `bytes.Buffer`.
- **`_, _ = svc.Crear("Beto")`** descarta usuario y error a propósito: acá solo interesa que
  quede guardado. En código real, ignorar un error es una decisión que se justifica con un
  comentario.

Salida:

```text
--- repositorio #1 (*main.Memoria) ---
usuario creado: 1 Ana
usuario creado: 2 Beto
encontrado: {ID:1 Nombre:Ana}
id 999: no encontrado (errors.Is funciona a través de capas)
--- repositorio #2 (*main.Archivo) ---
...
```

### La capa de arriba no sabe qué hay abajo

```go
if _, err := repo.Buscar(999); errors.Is(err, ErrNoEncontrado) {
	fmt.Println("id 999: no encontrado (errors.Is funciona a través de capas)")
}
```

`ErrNoEncontrado` se definió arriba, lo devuelven las dos implementaciones y lo consulta
`main`. El mensaje de `errors.Is` es la moraleja: **el error de dominio atraviesa la
interfaz sin perder identidad**. Eso es lo que permite que la capa HTTP traduzca cualquier
`ErrNoEncontrado` a 404 sin conocer el repositorio.

## Paso 6 — La trampa del nil en una interfaz

```go
var r Repositorio
if r == nil {
	fmt.Println("interfaz nil: no hay implementación")
}
```

Una interfaz vacía es `nil`, y eso funciona. Pero una interfaz es un **par (tipo, valor)**,
y una interfaz es `nil` solo si **los dos** son nil:

```go
var m *Memoria              // puntero nil
var r Repositorio = m       // r NO es nil: tiene tipo *Memoria y valor nil
fmt.Println(r == nil)       // false
r.Buscar(1)                 // panic: nil pointer dereference
```

El tipo está seteado, así que la comparación da `false`. Y llamar un método sobre ese nil
puede funcionar (si el método tolera el receptor nil) o explotar, como acá.

**Regla**: no metas punteros nil en interfaces. Si una función puede no tener valor,
devolvé `nil` explícito:

```go
func nuevoRepo(usarArchivo bool) Repositorio {
	if !usarArchivo {
		return nil   // interfaz nil de verdad
	}
	return NuevoArchivo("usuarios.jsonl")
}
```

Para detectar el caso difícil hay un truco con reflexión, pero si llegaste ahí, el diseño
está mal: arreglá el origen.

## Todo junto

El archivo se lee en cuatro bloques: contrato (`Repositorio`), implementación de memoria,
implementación de archivo, servicio y `main`. Las dos implementaciones no se conocen entre
sí; el servicio no conoce ninguna; `main` las conecta. Ese es el patrón que después se
repite con base de datos, auth y HTTP.

## Ejecutar

```bash
go run ./06-interfaces
```

Deja `usuarios.jsonl` en la carpeta actual. Es normal: el ejemplo lo borra al arrancar, pero
podés abrirlo para ver el formato de una línea por usuario.

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| `*Memoria does not implement Repositorio` | falta un método o la firma no coincide | revisá nombre y parámetros exactos |
| `assignment to entry in nil map` | map sin `make` | inicializalo en el constructor |
| `r != nil` con un puntero nil | interfaz con tipo seteado | no metas punteros nil |
| `scanner: token too long` | línea de más de 64 KB | `sc.Buffer` o `bufio.Reader` |
| "no encontrado" cuando el archivo tenía un error | falta `sc.Err()` | chequealo después del loop |
| `%T` imprime `*main.Memoria` y esperabas `Memoria` | guardaste un puntero | usá `%T` para confirmar |

## Por qué importa en el trabajo

- **Testear sin infraestructura**: el servicio se prueba con `Memoria` y un `bytes.Buffer`,
  sin base de datos ni red.
- **Cambiar SQLite por Postgres** sin tocar handlers: el repositorio es una interfaz.
- **`io.Writer`** aparece en logs, archivos, HTTP y compresión. Entenderla es entender la
  mitad de la librería estándar.
- **Interfaces chicas** evitan el `interface{}` gigante que todo lo sabe y nada documenta.

## Ejercicio

1. Agregá `Borrar(id int64) error` a `Repositorio` y a las dos implementaciones. En
   `Archivo`, la forma simple es reescribir el archivo sin el usuario; implementala y
   notá por qué duele (pista: es el argumento a favor de una base de datos).
2. Escribí un test de `Servicio` usando `Memoria` y `bytes.Buffer`, y verificá que el log
   contiene `"usuario creado: 1 Ana"`. Sin tocar disco.
3. Implementá `RepoNulo` que no hace nada (todas las operaciones devuelven error) y usalo
   como default en `NuevoServicio`. ¿Para qué sirve un repo que siempre falla?

Siguiente: [07 — Slices y maps](../07-slices-maps/).
