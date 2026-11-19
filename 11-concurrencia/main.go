package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	// 1. Goroutines + WaitGroup: "lanza N cosas y espera que terminen".
	var wg sync.WaitGroup
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		// Pasamos i como parámetro para no depender del valor del loop.
		go func(id int) {
			defer wg.Done()
			time.Sleep(time.Duration(id) * 10 * time.Millisecond)
			fmt.Println("goroutine", id, "terminó")
		}(i)
	}
	wg.Wait()
	fmt.Println("todas terminaron")

	// 2. Canal con buffer y close: el productor cierra, el consumidor usa range.
	numeros := make(chan int, 3)
	numeros <- 1
	numeros <- 2
	close(numeros)
	for n := range numeros {
		fmt.Println("recibido", n)
	}

	// 3. Worker pool: el patrón que evita abrir 10.000 conexiones a la vez.
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
	fmt.Println("suma de cuadrados 1..10:", suma)

	// 4. select: esperar el primero que llegue, con timeout.
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

	// 5. Mutex vs atomic para datos compartidos.
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

	// 6. Cancelación básica (el capítulo 12 la usa en serio).
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	select {
	case <-time.After(time.Second):
		fmt.Println("no debería pasar")
	case <-ctx.Done():
		fmt.Println("cancelado:", ctx.Err())
	}
}
