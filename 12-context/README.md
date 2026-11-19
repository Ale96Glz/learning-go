# 12 — Context

**Qué vas a construir:** una función que simula una llamada externa y se corta cuando el
request se cancela o vence el plazo. Es el mecanismo que evita que un servidor siga
trabajando para un cliente que ya se fue.

**Qué necesitas antes:** capítulos 01 a 11, sobre todo goroutines y `select`.

---

## Qué es `context` y por qué existe

En un servicio HTTP, cada request tiene un ciclo de vida: el cliente puede cortar la
conexión, el gateway puede imponer un timeout, o el deploy puede apagar el proceso. Cuando eso
pasa, todo el trabajo que se estaba haciendo para ese request debe detenerse: la consulta a
la base, la llamada a otro servicio, el cálculo pesado.

`context` es el objeto que transporta esa señal de "cortá" por toda la cadena de llamadas. Y
además lleva un plazo (`deadline`) y algunos valores de alcance request (como un request ID).

Comparaciones:

- **C#**: `CancellationToken` y `CancellationTokenSource`. Muy parecido en espíritu.
- **Java**: no hay equivalente estándar; se usan timeouts por API o librerías como
  `CompletableFuture` con cancelación manual.
- **Go**: el `context` se pasa **explícitamente** como primer parámetro. No hay
  thread-local ni variable global.

Las reglas, que se repiten en todo el ecosistema:

1. **Primer parámetro**, siempre: `func Hacer(ctx context.Context, ...)`.
2. **No lo guardes en un struct** (salvo que el struct sea de un request y esté claro).
3. **No pases `nil`**: usá `context.Background()` o `context.TODO()`.
4. **No lo uses para dependencias** (base de datos, logger, config). Para eso, inyección.
5. **No lo crees adentro** de una función que lo recibe: se propaga el que te dieron.

## Paso 1 — Timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
defer cancel()

_, err := lenta(ctx, 300*time.Millisecond)
if errors.Is(err, context.DeadlineExceeded) {
	fmt.Println("1. timeout esperado:", err)
}
```

```text
1. timeout esperado: consultar servicio: context deadline exceeded
```

- **`context.Background()`** es el context raíz: nunca se cancela. Es el punto de partida de
  un `main`, un test o una goroutine que no viene de un request.
- **`WithTimeout`** devuelve un context hijo que se cancela solo a los 100 ms, y una función
  `cancel`.
- **`defer cancel()`** es obligatorio **aunque el timeout se cumpla**. Libera el timer
  interno y permite que el context se recolecte. Si no lo llamás, hay una fuga chica por cada
  request; en un servidor con miles por segundo, se nota.
- **`errors.Is(err, context.DeadlineExceeded)`** funciona porque `lenta` envuelve el error
  con `%w`. Nunca compares por texto.

### La función que respeta el context

```go
func lenta(ctx context.Context, d time.Duration) (string, error) {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-t.C:
		return "datos", nil
	case <-ctx.Done():
		return "", fmt.Errorf("consultar servicio: %w", ctx.Err())
	}
}
```

- **`time.NewTimer(d)`** crea un timer que dispara una vez. Es mejor que `time.After` en
  código que se llama mucho, porque podés **pararlo**.
- **`defer t.Stop()`** libera el timer si el context gana primero. Sin esto, el timer sigue
  vivo hasta que vence, aunque no lo uses. `time.After` no se puede parar.
- **`select`** espera lo primero que ocurra: el trabajo termina (`t.C`) o lo cancelan
  (`ctx.Done()`).
- **`ctx.Done()`** es un canal que se cierra al cancelar. No manda un valor; el `<-` se
  desbloquea con el cierre.
- **`ctx.Err()`** es `context.DeadlineExceeded` o `context.Canceled`, y `nil` si no se
  canceló. Se envuelve con `%w` para que el caller pueda preguntar con `errors.Is`.
- **`ctx` va primero** porque es la convención y porque herramientas como `go vet` lo
  esperan así.

Este `select` es el esqueleto de toda llamada externa en Go. En código real, `t.C` se
reemplaza por el resultado de un `http.Client.Do`, un `QueryContext` de SQL o un `Recv` de
una cola; el `ctx.Done()` queda igual.

## Paso 2 — Cancelación manual

```go
ctx2, cancel2 := context.WithCancel(context.Background())
go func() {
	time.Sleep(50 * time.Millisecond)
	cancel2()
}()
_, err = lenta(ctx2, time.Second)
if errors.Is(err, context.Canceled) {
	fmt.Println("2. cancelado esperado:", err)
}
```

```text
2. cancelado esperado: consultar servicio: context canceled
```

`WithCancel` no tiene plazo: se cancela cuando llamás a `cancel2()`. Acá una goroutine lo
hace a los 50 ms, simulando que el usuario cerró la pestaña o que el deploy mandó SIGTERM.

- **`cancel2` es seguro de llamar varias veces y desde varias goroutines.** No hace falta
  sincronizar.
- **`errors.Is(err, context.Canceled)`** distingue "lo cancelaron" de "venció el plazo".
  Esa diferencia importa: un timeout puede ser un 504, una cancelación del cliente no hay que
  responderla.
- En un servidor, este `cancel2` lo llama `http.Server` cuando detecta que el cliente cortó.
  Vos solo tenés que usar `r.Context()`.

## Paso 3 — Valores: request ID

```go
type claveID struct{}

ctx3 := context.WithValue(context.Background(), claveID{}, "req-123")
procesar(ctx3, 10*time.Millisecond)
```

```go
func procesar(ctx context.Context, d time.Duration) {
	id, _ := ctx.Value(claveID{}).(string)
	fmt.Println("3. procesando", id)

	if _, err := lenta(ctx, d); err != nil {
		fmt.Println("   error:", err)
		return
	}
	fmt.Println("   ok")
}
```

```text
3. procesando req-123
   ok
```

- **La clave es un tipo propio** (`type claveID struct{}`), no un string. Si dos paquetes
  usan `"id"` como clave, uno pisa al otro. Con un tipo no exportado, solo tu paquete puede
  crear esa clave: no hay colisión posible.
- **`ctx.Value(claveID{})`** devuelve `any` (interface{}), así que hay que hacer type
  assertion. El `_` descarta el `ok`; en código real conviene:
  ```go
  id, ok := ctx.Value(claveID{}).(string)
  if !ok { id = "sin-id" }
  ```
- **El valor es de solo lectura** y viaja hacia abajo. No sirve para que una capa de abajo
  devuelva algo hacia arriba.
- **Solo para datos del request**: request ID, usuario autenticado, trace ID. **Nunca** para
  la base de datos, el logger o la configuración: eso va por inyección, y si lo metés en el
  context, perdés el tipado y el grafo de dependencias.
- **`procesar` propaga el mismo `ctx`** a `lenta`. Ese es el punto: si el request se cancela,
  la llamada de abajo también se entera.

En este ejemplo `procesar` no devuelve error: imprime y sigue. En código real devolvería el
error hacia arriba. Se simplificó para que el ejemplo se lea.

## Paso 4 — Un context ya cancelado no arranca nada

```go
ctx4, cancel4 := context.WithCancel(context.Background())
cancel4()
if _, err := lenta(ctx4, time.Second); err != nil {
	fmt.Println("4. ni siquiera intentó:", err)
}
```

```text
4. ni siquiera intentó: consultar servicio: context canceled
```

`lenta` entra al `select` con `ctx.Done()` ya cerrado, así que gana ese caso de inmediato: no
crea el timer ni espera. Toda función que respeta el context tiene este comportamiento.

Esto es lo que permite "cortar temprano" en un pipeline: si el primer paso falla y cancela,
los siguientes ni se ejecutan.

## Paso 5 — Deadline absoluto

```go
deadline := time.Now().Add(20 * time.Millisecond)
ctx5, cancel5 := context.WithDeadline(context.Background(), deadline)
defer cancel5()
_, err = lenta(ctx5, time.Second)
fmt.Println("5. deadline:", err)
```

```text
5. deadline: consultar servicio: context deadline exceeded
```

- **`WithDeadline`** recibe una **fecha** (`time.Time`), no una duración.
- **`WithTimeout(d)`** es azúcar de `WithDeadline(time.Now().Add(d))`.
- ¿Cuándo usar cada uno? Cuando varias operaciones deben compartir un presupuesto total:
  ```go
  deadline := time.Now().Add(2 * time.Second)
  ctxA, _ := context.WithDeadline(ctx, deadline)
  ctxB, _ := context.WithDeadline(ctx, deadline)
  ```
  Las dos vencen en el mismo instante absoluto, aunque una arranque antes que la otra. Con
  `WithTimeout(2s)` cada una tendría su propio reloj y la segunda podría tardar 4 s en total.

El `defer cancel5()` sigue siendo necesario: libera el timer aunque el deadline ya haya
pasado.

## Todo junto

`main` recorre los cinco usos: timeout, cancelación, valores, context ya cancelado y deadline
absoluto. `lenta` es la única función que hace "I/O" y es la que muestra el patrón real.
`procesar` muestra la propagación y la lectura de un valor.

## Ejecutar

```bash
go run ./12-context
```

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| `context deadline exceeded` inesperado | el timeout es más chico que el trabajo | subí el plazo o revisá la operación |
| `context canceled` apenas arranca | el context padre ya estaba cancelado | revisá quién lo canceló |
| el servidor sigue trabajando tras el corte | no propagaste el `ctx` | pasalo a la DB y al HTTP client |
| fuga de timers/memoria | falta `defer cancel()` | agregalo siempre |
| `ctx.Value` devuelve `nil` | clave distinta (tipo o instancia) | usá el mismo tipo no exportado |
| `context.WithValue` para config | mal uso | inyección de dependencias |
| dos timeouts que suman | usaste `WithTimeout` en cada paso | `WithDeadline` compartido |

## Por qué importa en el trabajo

- **Un request que se cancela no debe seguir gastando CPU ni base de datos.** Ese es el
  ahorro más grande en un servicio con carga.
- **Los timeouts en cascada** son la diferencia entre un servicio que degrada y uno que se
  cuelga esperando a un tercero que no responde.
- **El request ID en el context** es lo que permite correlacionar logs entre servicios.
- **`WithDeadline` compartido** es cómo se respeta un presupuesto total de request (por
  ejemplo, 2 s para todo, sin importar cuántas llamadas internas haya).

## Ejercicio

1. Hacé que `procesar` devuelva `error` y que `main` lo imprima. Verificá que
   `errors.Is(err, context.DeadlineExceeded)` sigue funcionando a través de dos capas.
2. Cambiá `lenta` para que use `time.After` en vez de `NewTimer`+`Stop`. ¿Qué se pierde?
3. Creá un context con deadline compartido y llamá a `lenta` dos veces con 30 ms y 80 ms.
   Verificá que la segunda hereda el tiempo restante, no 80 ms nuevos.
4. Agregá un `context.WithValue` con el ID de usuario y usalo en `lenta` solo para el
   mensaje de error. ¿Por qué no conviene usarlo para la base de datos?

Siguiente: [13 — HTTP básico](../13-http-basico/).
