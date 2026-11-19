package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// slog es el logger estructurado de la librería estándar (Go 1.21+).
	// JSON en producción: lo ingieren Datadog, Loki, CloudWatch, etc.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	mux := http.NewServeMux()

	// Liveness: ¿el proceso está vivo? Si falla, el orquestador reinicia.
	mux.HandleFunc("GET /salud", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// Readiness: ¿puede atender tráfico? Si la DB no responde, no me manden requests.
	mux.HandleFunc("GET /listo", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if !baseDeDatosLista(ctx) {
			http.Error(w, "no listo", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("listo"))
	})

	// Handler lento para ver el graceful shutdown en acción.
	mux.HandleFunc("GET /trabajo", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		fmt.Fprintln(w, "trabajo terminado")
	})

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           conLogging(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Ctrl+C (Windows/Linux) o SIGTERM (Docker/K8s) -> cancelación del context raíz.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("servidor iniciado", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("fallo el servidor", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop() // un segundo Ctrl+C ahora mata el proceso de inmediato

	slog.Info("apagando: esperando requests en curso")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("no se pudo cerrar ordenadamente", "error", err)
		os.Exit(1)
	}
	slog.Info("apagado completo")
}

// En un servicio real sería db.PingContext(ctx).
func baseDeDatosLista(ctx context.Context) bool {
	return ctx.Err() == nil
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func conLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		// Campos estructurados: se pueden filtrar y agregar, cosa que un texto plano no.
		slog.Info("request",
			"metodo", r.Method,
			"ruta", r.URL.Path,
			"status", rec.status,
			"duracion_ms", time.Since(inicio).Milliseconds(),
		)
	})
}
