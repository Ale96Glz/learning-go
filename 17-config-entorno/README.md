# 17 — Configuración y entorno

**Qué vas a construir:** cargar toda la configuración de un servicio desde variables de
entorno, con defaults, conversión de tipos, validación completa y secretos que no se filtran
en los logs.

**Qué necesitas antes:** capítulos 01 a 16.

---

## Paso 0 — Por qué variables de entorno

El mismo binario tiene que correr en tu máquina, en staging y en producción. Lo único que
cambia es la configuración: puerto, base de datos, timeouts, secretos. Si compilás valores
distintos por entorno, dejás de poder decir "esto es lo que probé".

En Java/C# tenés un sistema de configuración con archivos y precedencia: `application.yml` +
`@Value`/`Environment` en Spring, `appsettings.json` + `IConfiguration` + el provider de
variables de entorno en .NET. En Go no hay framework: leés `os.Getenv`, convertís y validás.
Son 60 líneas, y a cambio sabés exactamente de dónde sale cada valor.

Reglas del capítulo:

1. **Defaults para todo lo que no es secreto.** El servicio arranca sin configurar nada en
   desarrollo.
2. **El secreto no tiene default.** Un default público es un secreto quemado.
3. **Leer una vez, al arrancar.** Nada de `os.Getenv` repartido por los handlers: la config se
   pasa como dependencia.
4. **Validar todo junto y fallar rápido.** Mejor no arrancar que arrancar a medias.

## Paso 1 — El struct con tipos reales

```go
type Config struct {
	Puerto       int
	Entorno      string
	DBRuta       string
	Timeout      time.Duration
	NivelLog     string
	TokenSecreto string
}
```

Todos los valores vienen de `os.Getenv` como string. El struct los guarda **ya convertidos**:
`int`, `time.Duration`, string normalizado. Eso concentra la conversión en un solo lugar; el
resto del programa usa `cfg.Timeout` sin volver a parsear.

La alternativa —guardar todo como string y convertir en cada uso— reparte `strconv.Atoi` por
diez archivos y multiplica los errores. Convertí en el borde.

## Paso 2 — Defaults primero

```go
cfg := Config{
	Puerto:   8080,
	Entorno:  "dev",
	DBRuta:   "tareas.db",
	Timeout:  10 * time.Second,
	NivelLog: "info",
}
```

- **`8080`** es el default. El entorno solo lo pisa si existe.
- **`"dev"`** es el entorno seguro: logs verbosos, sin optimizaciones. Producción se declara,
  no se asume.
- **`"tareas.db"`** es una ruta relativa: en desarrollo queda al lado del binario.
- **`10 * time.Second`**: `time.Duration` es un `int64` de nanosegundos; la multiplicación por
  `time.Second` da un valor legible en el código.
- **`"info"`** es el nivel de log razonable.

- **`TokenSecreto` no aparece:** su zero value es `""` y la validación lo va a rechazar. Es
  deliberado: no hay secreto por defecto.

## Paso 3 — Puerto: string a número, con rango

```go
if v := os.Getenv("PORT"); v != "" {
	p, err := strconv.Atoi(v)
	if err != nil || p < 1 || p > 65535 {
		errs = append(errs, fmt.Errorf("PORT inválido: %q", v))
	} else {
		cfg.Puerto = p
	}
}
```

- **`os.Getenv` devuelve `""` si la variable no existe.** En Java, `System.getenv("PORT")`
  devuelve `null`; acá "no existe" y "está vacía" son el mismo caso, y para config es lo que
  querés: vacío = usá el default.
- **`if v := ...; v != ""`** limita `v` al bloque (capítulo 03).
- **`strconv.Atoi`** convierte string a int y devuelve error. No hay cast implícito.
- **`err != nil || p < 1 || p > 65535`** junta las dos validaciones: que sea número y que sea
  un puerto TCP válido. `"0"`, `"70000"` y `"abc"` fallan por el mismo camino.
- **`%q`** muestra el valor entre comillas: `PORT inválido: "abc"` deja ver si había un espacio
  o un carácter raro. Un `%s` sobre `"abc "` te confunde.
- **No se corta acá**: se acumula el error y se sigue leyendo. Lo ves en el paso 8.

## Paso 4 — Entorno: normalizar y permitir solo valores conocidos

```go
switch v := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))); v {
case "", "dev":
	cfg.Entorno = "dev"
case "staging", "prod":
	cfg.Entorno = v
default:
	errs = append(errs, fmt.Errorf("APP_ENV inválido: %q (usa dev, staging o prod)", v))
}
```

- **`TrimSpace` + `ToLower`** aceptan `"PROD"`, `" prod "` y `"Prod"`. Sin normalizar, el
  deploy con mayúsculas falla por una diferencia cosmética.
- **`case "", "dev"`**: si no está seteada, es dev. El default y el valor explícito se tratan
  igual.
- **`case "staging", "prod"`** es una **allowlist**. Cualquier otro valor es error, no un
  fallback silencioso. Si alguien escribe `"production"` y el código solo entiende `"prod"`,
  querés enterarte al arrancar, no cuando un `if cfg.Entorno == "prod"` nunca se cumple.
- **`%q`** en el mensaje de error muestra lo que llegó después de normalizar.

Un enum de verdad sería un tipo `Entorno` con constantes (capítulo 02). Acá alcanza con string
validado porque el valor se usa para comparar y loguear, no para lógica crítica.

## Paso 5 — Ruta de la base: cualquier valor no vacío sirve

```go
if v := os.Getenv("DB_RUTA"); v != "" {
	cfg.DBRuta = v
}
```

No hay validación de contenido: una ruta puede ser `tareas.db`, `/data/app.db` o
`file:memdb1?mode=memory`. Validarla requeriría conocer el motor. Lo que sí importa es que si
la variable está vacía no pises el default con `""`.

## Paso 6 — Timeout: `time.ParseDuration`

```go
if v := os.Getenv("TIMEOUT"); v != "" {
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		errs = append(errs, fmt.Errorf("TIMEOUT inválido: %q (ej: 10s, 1m)", v))
	} else {
		cfg.Timeout = d
	}
}
```

- **`time.ParseDuration("45s")`** acepta `300ms`, `10s`, `1m30s`, `2h`. Es el formato que
  espera cualquiera que haya usado Go.
- **`"10"` sin unidad falla**: `time: missing unit in duration "10"`. Es una fuente de
  confusión real; el mensaje de error dice el ejemplo.
- **`"-1s"` parsea bien pero se rechaza con `d <= 0`.** Un timeout negativo no tiene sentido y
  haría que todo venza antes de empezar.
- **`"0s"` también se rechaza**: timeout cero significa "sin timeout" en algunos contextos,
  pero acá preferimos que sea explícito y positivo.

Para imprimirlo, **`%s` o `%v`, nunca `%d`**: `%d` muestra `45000000000` (nanosegundos), que
parece un bug.

## Paso 7 — Nivel de log: otra allowlist

```go
if v := strings.ToLower(os.Getenv("LOG_NIVEL")); v != "" {
	switch v {
	case "debug", "info", "warn", "error":
		cfg.NivelLog = v
	default:
		errs = append(errs, fmt.Errorf("LOG_NIVEL inválido: %q", v))
	}
}
```

Mismo patrón que `APP_ENV`: normalizar, permitir una lista corta, error si no está.

Detalle: acá hay `ToLower` pero **no** `TrimSpace`, a diferencia de `APP_ENV`. `LOG_NIVEL="
INFO "` falla. Es una inconsistencia del ejemplo; si querés robustez, agregá el trim (o
centralizá la lectura en un helper `leerEnv(nombre)` que siempre normalice).

## Paso 8 — El secreto: obligatorio y sin default

```go
cfg.TokenSecreto = os.Getenv("TOKEN_SECRETO")
if len(cfg.TokenSecreto) < 32 {
	errs = append(errs, errors.New("TOKEN_SECRETO es obligatorio (mínimo 32 caracteres)"))
}
```

- **No hay default.** Si falta, el servicio no arranca. Un secreto por defecto en el código es
  un secreto público: está en el repo, en el historial y en la imagen Docker.
- **Mínimo 32 caracteres** es un piso, no una garantía. 32 bytes aleatorios son 256 bits; 32
  letras `a` no. La longitud solo detecta el caso más obvio.
- **No se valida el contenido** ni se compara con nada. Solo se exige que exista.
- **`errors.New`** porque no hay nada que formatear.

Para generarlo:

```bash
openssl rand -hex 32        # 64 caracteres hex
```

En Windows sin OpenSSL, el ejercicio 5 te hace el generador en Go.

En producción el valor lo inyecta la plataforma: Kubernetes Secret, Docker secret, Vault, AWS
Secrets Manager. **Nunca** un `.env` commiteado.

## Paso 9 — `errors.Join`: todos los errores de una vez

```go
return cfg, errors.Join(errs...)
```

`errs` es un `[]error`. `errors.Join` combina todos en uno solo:

```go
err := errors.Join(
	errors.New("PORT inválido"),
	errors.New("TIMEOUT inválido"),
)
fmt.Println(err)
```

```text
PORT inválido
TIMEOUT inválido
```

- **Imprime uno por línea**, no un mensaje concatenado. Se lee.
- **Si el slice está vacío, devuelve `nil`.** Ese es el camino feliz, sin `if len(errs) > 0`.
- **`errors.Is` y `errors.As` recorren todas las ramas** (Go 1.20+). Si uno de los errores
  envuelve un centinela, lo encontrás aunque esté mezclado con otros.
- En Java/C# el equivalente es acumular en una lista de validación (`BindingResult`,
  `ModelState`) y devolverlas juntas. La diferencia es que acá el error es un valor común.

¿Por qué juntar y no devolver en el primer fallo? Porque configurar un servicio nuevo suele
tener varios errores a la vez: falta el secreto, el puerto está mal y el entorno no existe.
Descubrirlos de a uno significa tres ciclos de "arrancar, leer, corregir". El mensaje con
cinco líneas se arregla en una pasada.

Contrato importante: **si `err != nil`, `cfg` no sirve**, aunque venga parcialmente lleno. El
caller no debe usarlo. `main` lo respeta.

## Paso 10 — `String()` que redacta el secreto

```go
func (c Config) String() string {
	secreto := "***"
	if c.TokenSecreto == "" {
		secreto = "(vacío)"
	}
	return fmt.Sprintf(
		"Config{puerto:%d entorno:%s db:%s timeout:%s log:%s secreto:%s}",
		c.Puerto, c.Entorno, c.DBRuta, c.Timeout, c.NivelLog, secreto)
}
```

- **El secreto nunca se imprime**, pero saber si está vacío es útil para depurar. Por eso hay
  dos casos.
- **Receptor valor** (`c Config`): así **tanto `Config` como `*Config`** implementan
  `fmt.Stringer`. Si fuera `func (c *Config) String()`, imprimir un `Config` por valor no
  usaría el método y mostraría los campos crudos.
- **`%s` para `time.Duration`** porque implementa `String()` y muestra `45s`.
- **`%d`, `%s`** arman una línea compacta. `Println` la usa al imprimir el struct.

```go
fmt.Println("config cargada:", cfg)
// config cargada: Config{puerto:9090 entorno:prod db:tareas.db timeout:45s log:debug secreto:***}
```

### Dónde sí se filtra

`String()` protege `%v`, `%s` y `%+v` (todos llaman a `String()`). **No protege `%#v`:**

```go
fmt.Printf("%#v\n", cfg)
// main.Config{Puerto:9090, Entorno:"prod", ..., TokenSecreto:"0123..."}
```

`%#v` es la representación Go del valor e ignora `String()`. Es el verbo de depuración, no el
de logs. Si tu equipo lo usa, implementá también `GoString() string` (devolviendo
`c.String()`), que es lo que `%#v` consulta.

**Tampoco protege JSON.** `encoding/json` no mira `String()`: serializa los campos exportados.
`json.Marshal(cfg)` incluiría `"TokenSecreto":"..."`. Si alguna vez exponés la config en un
endpoint de debug, poné `json:"-"` en el campo o usá un DTO con solo los campos públicos.
Mismo problema en `fmt`, JSON y cualquier librería que reflexione sobre el struct: son
sistemas distintos y cada uno respeta cosas distintas (capítulo 05).

## Paso 11 — `main`: fallar rápido

```go
func main() {
	cfg, err := Cargar()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config inválida:\n%v\n", err)
		os.Exit(1)
	}

	fmt.Println("config cargada:", cfg)
	fmt.Printf("escucharía en :%d durante %s (entorno %s)\n", cfg.Puerto, cfg.Timeout, cfg.Entorno)
}
```

- **`Cargar` va primero**, antes de abrir la base o el puerto. Si la config está mal, no hay
  nada que arrancar.
- **`Fprintf(os.Stderr, ...)`** manda el detalle a la salida de errores. El `\n` antes de `%v`
  separa el título.
- **`os.Exit(1)`** corta con código de fallo. Acá no hay `defer` pendiente; si hubiera, no
  correría (capítulo 01).
- **`%d` para el puerto, `%s` para `time.Duration`.** El `:8080` es texto literal.
- En un servicio real, después de esto viene `slog`, la base y el servidor (capítulos 15, 16 y
  18). Todos reciben `cfg` como parámetro; ninguno lee el entorno por su cuenta.

## Salidas reales

Sin ninguna variable:

```text
config inválida:
TOKEN_SECRETO es obligatorio (mínimo 32 caracteres)
exit status 1
```

Con todo configurado:

```text
config cargada: Config{puerto:9090 entorno:prod db:tareas.db timeout:45s log:debug secreto:***}
escucharía en :9090 durante 45s (entorno prod)
```

Con varios errores a la vez:

```text
config inválida:
PORT inválido: "abc"
APP_ENV inválido: "qa" (usa dev, staging o prod)
TIMEOUT inválido: "0s" (ej: 10s, 1m)
LOG_NIVEL inválido: "verbose"
TOKEN_SECRETO es obligatorio (mínimo 32 caracteres)
exit status 1
```

En PowerShell:

```powershell
$env:TOKEN_SECRETO = "0123456789abcdef0123456789abcdef"
$env:PORT = "9090"
$env:TIMEOUT = "45s"
$env:APP_ENV = "prod"
$env:LOG_NIVEL = "debug"
go run ./17-config-entorno
Remove-Item Env:PORT, Env:TIMEOUT, Env:APP_ENV, Env:LOG_NIVEL, Env:TOKEN_SECRETO
```

`$env:X` define la variable para esa sesión de PowerShell. Si abrís otra terminal, no la ve.
En bash es `PORT=9090 go run ./17-config-entorno`, que la define solo para ese proceso.

## Los tests

```bash
go test -v ./17-config-entorno
```

```text
=== RUN   TestDefaults
--- PASS: TestDefaults (0.00s)
=== RUN   TestValoresValidos
--- PASS: TestValoresValidos (0.00s)
=== RUN   TestInvalidos
=== RUN   TestInvalidos/puerto_no_numérico
=== RUN   TestInvalidos/puerto_fuera_de_rango
=== RUN   TestInvalidos/entorno_desconocido
=== RUN   TestInvalidos/timeout_negativo
=== RUN   TestInvalidos/timeout_sin_unidad
=== RUN   TestInvalidos/secreto_corto
--- PASS: TestInvalidos (0.00s)
=== RUN   TestSecretoNoSeFiltra
--- PASS: TestSecretoNoSeFiltra (0.00s)
PASS
ok  	tutorialgo/17-config-entorno	0.020s
```

### Aislar el entorno

```go
func limpiar(t *testing.T) {
	t.Helper()
	for _, k := range []string{"PORT", "APP_ENV", "DB_RUTA", "TIMEOUT", "LOG_NIVEL"} {
		t.Setenv(k, "")
	}
	t.Setenv("TOKEN_SECRETO", strings.Repeat("s", 32))
}
```

- **`t.Setenv`** setea una variable y la **restaura al terminar el test**, aunque falle. No
  contamina los demás tests ni tu shell.
- **`t.Setenv(k, "")`** deja la variable vacía, que `Cargar` interpreta como "no seteada" para
  los campos con default. Sin este paso, un `PORT` de tu máquina haría fallar el test.
- **`t.Helper()`** hace que, si el helper falla, el error apunte a la línea del test que lo
  llamó, no adentro de `limpiar`.
- **`strings.Repeat("s", 32)`** genera un secreto válido por longitud sin hardcodear uno real.

`t.Setenv` **no se puede usar en un test paralelo** (`t.Parallel`): el entorno es global y
cambiarlo mientras otro test corre sería una carrera. Si necesitás paralelismo, `Cargar`
debería recibir un `func(string) string` en vez de leer `os.Getenv`.

### Qué prueba cada test

- **`TestDefaults`**: con todo vacío, puerto 8080, entorno dev y timeout 10s. Es el contrato
  de "funciona sin configurar nada".
- **`TestValoresValidos`**: los cinco campos toman el valor del entorno. Verifica que no haya
  un default pisando el valor.
- **`TestInvalidos`**: tabla con seis errores. No compara el mensaje exacto, solo que
  **mencione la variable**. Así podés mejorar el texto sin romper tests.
- **`TestSecretoNoSeFiltra`**: `cfg.String()` no contiene el secreto. Es el test que evita que
  alguien "mejore" el log y lo filtre.

Ninguno de estos tests necesita red, base ni puerto. La config es una función pura de las
variables de entorno (una vez que las inyectás).

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| `PORT inválido: "abc"` | No es número | Pasá un entero |
| `PORT inválido: "70000"` | Fuera de 1-65535 | Usá un puerto válido |
| `TIMEOUT inválido: "10"` | Falta la unidad | `10s`, `500ms`, `1m` |
| `TIMEOUT inválido: "-1s"` | Duración negativa | Valor positivo |
| `APP_ENV inválido: "qa"` | No está en la allowlist | `dev`, `staging` o `prod` |
| `TOKEN_SECRETO es obligatorio` | Falta o es corto | 32+ caracteres aleatorios |
| Funciona local y no en Docker | La variable no está en el contenedor | `-e`/`env_file`/Secret |
| El secreto aparece en un log | Usaste `%#v` o serializaste el struct | `%v` + `String()`, o `json:"-"` |
| El timeout se ve como `45000000000` | `%d` con `time.Duration` | `%v` o `%s` |
| El test falla solo en tu máquina | `PORT` real en el entorno | `t.Setenv` en el test |
| `$env:PORT` "no funciona" | La definiste en otra terminal | Definila en la misma sesión |

## Por qué importa en el trabajo

- **Un solo binario, muchos entornos.** Es la base de Docker y de cualquier deploy repetible.
- **Fallar al arrancar es barato.** Un servicio que arranca con el secreto vacío y falla en el
  primer request es mucho más caro de diagnosticar.
- **Todos los errores juntos** convierten una configuración de 20 variables en una tarea de
  una pasada.
- **La redacción de secretos** evita el incidente clásico: un log de arranque con el token de
  producción, indexado para siempre en el sistema de logs.
- **La config inyectada** hace los tests deterministas y el código testeable sin variables
  globales.

## Ejercicio

1. Agregá `MAX_CONNS int` con default `5`, rango `1..100`. Sumalo al struct, a `Cargar`, a
   `String()` y a los tests (default, válido, fuera de rango).
2. Extraé la lectura de cada variable a funciones `leerPuerto(getenv func(string) string)`, etc.
   Así los tests no dependen de `os.Setenv` y podés correrlos en paralelo. Empezá por `PORT`.
3. Implementá `func (c Config) GoString() string { return c.String() }` y comprobá con
   `fmt.Printf("%#v\n", cfg)` que el secreto ya no aparece. ¿Qué otro verbo sigue mostrando los
   campos crudos?
4. Agregá `json:"-"` a `TokenSecreto` y un test que haga `json.Marshal(cfg)` y falle si el
   secreto aparece en el resultado. ¿Por qué `String()` no alcanzaba?
5. Escribí un `main` que genere un secreto con `crypto/rand`, lo imprima en hex y explique por
   qué no se debe usar `math/rand` para esto.

Siguiente: [18 — Producción](../18-produccion/).
