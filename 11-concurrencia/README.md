# 11 — Concurrencia

**Qué vas a construir:** seis patrones de concurrencia que cubren casi todo lo que vas a
necesitar en un servicio: lanzar tareas en paralelo, productor/consumidor, pool de workers,
timeouts, contadores compartidos y cancelación.

**Qué necesitas antes:** capítulos 01 a 10.

---

## La idea que cambia todo

Un **goroutine** es una función que corre de forma concurrente. No es un hilo del sistema:
pesa unos pocos KB y el runtime de Go multiplexa miles sobre unos pocos hilos. Lanzar uno es
tan barato que se escribe en una línea:

```go
go hacerAlgo()
```

Comparado con Java/C#, no hay `Thread`, ni `ExecutorService`, ni `Task.Run` con `await`. Hay
`go` y canales. Y no hay una palabra para esperar: se usan `sync.WaitGroup` o canales.

La regla de oro: **"no compartas memoria para comunicarte; comunicate para compartir
memoria"**. Los canales son el mecanismo principal; los candados (`Mutex`) existen, pero son
el plan B.

## Paso 1 — Goroutines y `WaitGroup`

```go
var wg sync.WaitGroup
for i := 1; i <= 3; i++ {
	wg.Add(1)
	go func(id int) {
		defer wg.Done()
		time.Sleep(time.Duration(id) * 10 * time.Millisecond)
		fmt.Println("goroutine", id, "terminó")
	}(i)
}
wg.Wait()
fmt.Println("todas terminaron")
```

```text
goroutine 1 terminó
goroutine 2 terminó
goroutine 3 terminó
todas terminaron
```

- **`wg.Add(1)` va antes de lanzar la goroutine.** Si lo pusieras adentro, `Wait` podría
  ejecutarse antes de que la goroutine se registre, y el programa terminaría sin esperar.
  Es el bug clásico.
- **`defer wg.Done()`** garantiza el decremento aunque la goroutine entre en panic (antes de
  que el panic mate el proceso). Sin el `defer`, un `return` temprano dejaría el contador
  colgado y `Wait` esperaría para siempre.
- **`(i)` al final** pasa el valor como parámetro. Sin eso, la closure capturaría la
  variable del loop; desde Go 1.22 cada iteración tiene su propia variable, pero pasar el
  parámetro es explícito y no depende de la versión.
- **`time.Duration(id) * 10 * time.Millisecond`**: `id` es `int`, se convierte a
  `time.Duration` (int64 de nanosegundos) y se multiplica por 10 ms. La goroutine 1 duerme
  10 ms, la 2 20 ms, la 3 30 ms.
- **`wg.Wait()`** bloquea hasta que el contador llegue a 0.

El orden de los `Println` **no está garantizado**: acá los sleeps lo hacen predecible, pero
el scheduler podría ejecutarlos en otro orden. Si el orden importa, hay que sincronizar.

## Paso 2 — Canal con buffer y `close`

```go
numeros := make(chan int, 3)
numeros <- 1
numeros <- 2
close(numeros)
for n := range numeros {
	fmt.Println("recibido", n)
}
```

```text
recibido 1
recibido 2
```

- **`make(chan int, 3)`** crea un canal con capacidad 3. Los envíos no bloquean hasta
  llenarlo. Con capacidad 0 (sin buffer), cada envío espera a que alguien reciba.
- **`numeros <- 1`** envía; **`<-numeros`** recibe.
- **`close(numeros)`** marca el canal como cerrado: no se pueden enviar más valores, pero se
  pueden recibir los que quedan en el buffer.
- **`for n := range numeros`** recibe hasta que el canal se cierra y se vacía. Es el
  consumidor idiomático.

Reglas de `close`:

- **El productor cierra, nunca el consumidor.** El que envía sabe cuándo terminó.
- Cerrar un canal dos veces hace panic.
- Enviar a un canal cerrado hace panic.
- Recibir de un canal cerrado devuelve el zero value al instante (por eso `range` termina).

## Paso 3 — Worker pool: el patrón que evita tumbar el servidor

```go
const workers = 3
trabajos := make(chan int)
resultados := make(chan int)

var wgWorkers sync.WaitGroup
for w := 1; w <= workers; w++ {
	wgWorkers.Add(1)
	go func() {
		defer wgWorkers.Done()
		for j := range trabajos {
			resultados <- j * j
		}
	}()
}
```

- **`trabajos := make(chan int)`** no tiene buffer: cada envío espera a que un worker reciba.
  Eso es *backpressure*: si los workers están ocupados, el productor se frena solo.
- **Tres workers** hacen `for j := range trabajos`. Cuando el canal se cierra, el `range`
  termina y el worker sale.
- Cada worker calcula `j * j` y lo manda a `resultados`. El canal de resultados también es
  sin buffer.

```go
go func() { // productor
	for i := 1; i <= 10; i++ {
		trabajos <- i
	}
	close(trabajos)
}()
go func() { // cierra resultados cuando no quedan workers
	wgWorkers.Wait()
	close(resultados)
}()

suma := 0
for r := range resultados {
	suma += r
}
```

```text
suma de cuadrados 1..10: 385
```

La cadena de cierres es el punto delicado, y hay que entenderla para no colgar el programa:

1. **El productor cierra `trabajos`** cuando terminó de enviar. Los workers salen de su
   `range`.
2. **Un goroutine aparte espera a los workers** (`wgWorkers.Wait()`) y, recién ahí, cierra
   `resultados`. No lo puede hacer `main` antes, porque todavía podrían llegar valores.
3. **`main` recorre `resultados`** y termina cuando el canal se cierra.

Si falta el paso 2, `main` espera para siempre. Si falta `close(trabajos)`, los workers nunca
salen. Si `main` cierra `resultados` en vez de la goroutine, panic por enviar a un canal
cerrado. **Un cierre de menos y el programa se cuelga; uno de más y explota.**

¿Por qué 3 workers y no 10.000 goroutines? Podrías lanzar 10.000 y también funcionaría, pero
si cada trabajo hace una consulta a la base, abrís 10.000 conexiones y la tumbás. El pool
limita la concurrencia a lo que el recurso aguanta.

## Paso 4 — `select` y timeout

```go
lento := make(chan string)
go func() {
	time.Sleep(200 * time.Millisecond)
	lento <- "respuesta"
}()

select {
case r := <-lento:
	fmt.Println("llegó:", r)
case <-time.After(50 * time.Millisecond):
	fmt.Println("timeout: seguimos sin bloquear")
}
```

```text
timeout: seguimos sin bloquear
```

- **`select`** espera a que **uno** de los casos esté listo y ejecuta ese. Si varios están
  listos, elige uno al azar.
- **`time.After(50ms)`** devuelve un canal que recibe un valor al vencer. Es un caso más:
  "si en 50 ms no llegó nada, hacé esto".
- Acá el productor tarda 200 ms y el timeout es de 50, así que gana el timeout.

Un detalle que no se ve: **la goroutine que envía queda bloqueada para siempre**, porque
nadie recibe `"respuesta"`. En este programa termina igual, pero en un servidor de larga vida
eso es una fuga. La solución real es `context` (paso 6) o un canal con buffer de 1.

`time.After` crea un timer que no se puede cancelar. En un loop que se ejecuta miles de
veces, usá `time.NewTimer` + `defer t.Stop()` (capítulo 12).

## Paso 5 — `Mutex` vs `atomic`

```go
var mu sync.Mutex
contador := 0
var atomico int64
var wgCont sync.WaitGroup
for i := 0; i < 100; i++ {
	wgCont.Add(1)
	go func() {
		defer wgCont.Done()
		mu.Lock()
		contador++
		mu.Unlock()
		atomic.AddInt64(&atomico, 1)
	}()
}
wgCont.Wait()
fmt.Println("contador:", contador, "atómico:", atomic.LoadInt64(&atomico))
```

```text
contador: 100 atómico: 100
```

Sin protección, `contador++` es **leer, sumar y escribir**. Dos goroutines pueden leer el
mismo valor y una de las dos sumas se pierde. Ese es el *data race*, y no siempre se
reproduce: aparece bajo carga y es un infierno de depurar.

- **`mu.Lock()` / `mu.Unlock()`** protege una sección crítica. Todo lo que esté entre los dos
  corre de a uno. Si hay que leer y escribir varios datos de forma coherente, es lo que
  necesitás.
- **`defer mu.Unlock()`** es más seguro si la sección tiene `return` o puede hacer panic. Acá
  se usa `Unlock` explícito porque la sección es de una línea.
- **`atomic.AddInt64`** es una operación atómica del hardware: más rápida, pero solo sirve
  para operaciones simples sobre un valor. No podés hacer "leer A, comparar con B, escribir
  C" de forma atómica sin un mutex.
- **`atomic.LoadInt64`** para leer el valor final sin race.

Regla: si podés resolverlo con un canal, hacelo; si es un contador simple, `atomic`; si es
estado compuesto, `Mutex`. Y siempre medí con `go test -race`.

## Paso 6 — Cancelación con `context`

```go
ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
defer cancel()
select {
case <-time.After(time.Second):
	fmt.Println("no debería pasar")
case <-ctx.Done():
	fmt.Println("cancelado:", ctx.Err())
}
```

```text
cancelado: context deadline exceeded
```

- **`context.Background()`** es el context raíz, el punto de partida.
- **`WithTimeout(..., 100ms)`** devuelve un context que se cancela solo al vencer, y una
  función `cancel`.
- **`defer cancel()`** es obligatorio aunque el timeout se cumpla: libera el timer interno.
  `go vet` marca los `cancel` olvidados.
- **`ctx.Done()`** es un canal que se cierra cuando el context se cancela. Se usa en
  `select` como cualquier otro caso.
- **`ctx.Err()`** dice por qué: `context.DeadlineExceeded` por timeout,
  `context.Canceled` si lo cancelaste a mano.

El capítulo 12 lo lleva a funciones reales: el context se pasa como primer parámetro y se
propaga a la base de datos y a las llamadas HTTP, para que todo se corte junto.

## Todo junto

El archivo es un recorrido de patrones, en orden de complejidad: esperar goroutines, canal
con buffer, pool, timeout, contadores y cancelación. Cada bloque es independiente; podés
comentarlos y correr el resto.

## Ejecutar

```bash
go run ./11-concurrencia
go test -race ./...        # detecta data races en tus tests
```

La salida de las goroutines puede variar de orden entre corridas. Eso no es un bug.

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| `fatal error: all goroutines are asleep - deadlock!` | Nadie recibe o nadie envía | Revisá quién cierra cada canal |
| `send on closed channel` | Cerraste del lado del consumidor | Cierra solo el productor |
| `close of closed channel` | Dos cierres | Un solo dueño del cierre |
| El programa termina sin imprimir nada | No esperaste las goroutines | `WaitGroup` o canal |
| Contador menor a 100 | Data race | Mutex o atomic |
| Se cuelga en `wg.Wait()` | Falta un `Done` | `defer wg.Done()` |
| `concurrent map writes` | Map compartido sin lock | `sync.Map` o mutex |

## Por qué importa en el trabajo

- **Worker pools** son cómo se procesan colas sin abrir mil conexiones.
- **Backpressure** con canales sin buffer evita que un productor rápido desborde la memoria.
- **`select` + timeout** es la base de toda llamada externa con deadline.
- **`-race`** en CI atrapa los bugs que no se reproducen a mano.

## Ejercicio

1. Cambiá el worker pool a 1 worker y comprobá que el resultado es el mismo. Después a 10 y
   medí con `time`. ¿Dónde deja de mejorar?
2. Hacé que el productor del paso 4 use un canal con buffer de 1 y explicá por qué ya no
   queda la goroutine bloqueada.
3. Reemplazá el `select` del paso 4 por `for` con `time.NewTimer` y `t.Stop()` y verificá que
   no hay fugas. Es el patrón del capítulo 12.

Siguiente: [12 — Context](../12-context/).
