package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// claveID es un tipo propio y no exportado para evitar colisiones de claves
// en el context. Nunca uses un string suelto como clave.
type claveID struct{}

func main() {
	// 1. Timeout: la operación no puede tardar más de 100ms.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel() // SIEMPRE: libera el timer aunque no se agote

	_, err := lenta(ctx, 300*time.Millisecond)
	if errors.Is(err, context.DeadlineExceeded) {
		fmt.Println("1. timeout esperado:", err)
	}

	// 2. Cancelación manual: el usuario cerró la pestaña, el deploy reinicia, etc.
	ctx2, cancel2 := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel2()
	}()
	_, err = lenta(ctx2, time.Second)
	if errors.Is(err, context.Canceled) {
		fmt.Println("2. cancelado esperado:", err)
	}

	// 3. Propagación + valores: request ID para logs correlacionados.
	ctx3 := context.WithValue(context.Background(), claveID{}, "req-123")
	procesar(ctx3, 10*time.Millisecond)

	// 4. Un context ya cancelado no arranca nada.
	ctx4, cancel4 := context.WithCancel(context.Background())
	cancel4()
	if _, err := lenta(ctx4, time.Second); err != nil {
		fmt.Println("4. ni siquiera intentó:", err)
	}

	// 5. Deadline absoluto (fecha límite, no duración).
	deadline := time.Now().Add(20 * time.Millisecond)
	ctx5, cancel5 := context.WithDeadline(context.Background(), deadline)
	defer cancel5()
	_, err = lenta(ctx5, time.Second)
	fmt.Println("5. deadline:", err)
}

// lenta simula una llamada externa (HTTP, DB, cola) que respeta el context.
// Toda función que hace I/O debe recibir ctx como PRIMER parámetro.
func lenta(ctx context.Context, d time.Duration) (string, error) {
	t := time.NewTimer(d)
	defer t.Stop() // evita fugas de timer

	select {
	case <-t.C:
		return "datos", nil
	case <-ctx.Done():
		return "", fmt.Errorf("consultar servicio: %w", ctx.Err())
	}
}

func procesar(ctx context.Context, d time.Duration) {
	id, _ := ctx.Value(claveID{}).(string)
	fmt.Println("3. procesando", id)

	if _, err := lenta(ctx, d); err != nil {
		fmt.Println("   error:", err)
		return
	}
	fmt.Println("   ok")
}
