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

	"tutorialgo/proyecto/internal/auth"
	"tutorialgo/proyecto/internal/config"
	"tutorialgo/proyecto/internal/httpx"
	"tutorialgo/proyecto/internal/tareas"
)

func main() {
	cfg, err := config.Cargar()
	if err != nil {
		slog.Error("config inválida", "error", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	repo, err := tareas.Abrir(cfg.DBRuta)
	if err != nil {
		slog.Error("abrir base de datos", "error", err)
		os.Exit(1)
	}
	defer repo.Cerrar()

	migrarCtx, cancelar := context.WithTimeout(context.Background(), cfg.Timeout)
	err = repo.Migrar(migrarCtx)
	cancelar()
	if err != nil {
		slog.Error("migrar", "error", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Puerto),
		Handler:           construirRutas(repo, auth.Nuevo(24*time.Hour)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("servidor iniciado",
			"puerto", cfg.Puerto, "entorno", cfg.Entorno, "db", cfg.DBRuta)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("servidor", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	stop()
	slog.Info("apagando: esperando requests en curso")

	apagadoCtx, cancelar := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancelar()
	if err := srv.Shutdown(apagadoCtx); err != nil {
		slog.Error("shutdown forzado", "error", err)
		os.Exit(1)
	}
	slog.Info("apagado completo")
}

func construirRutas(repo *tareas.Repo, authSvc *auth.Servicio) http.Handler {
	api := http.NewServeMux()

	api.HandleFunc("POST /auth/registro", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Nombre   string `json:"nombre"`
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := httpx.Decodificar(w, r, &in); err != nil {
			httpx.Error(w, http.StatusBadRequest, "json inválido")
			return
		}
		u, err := authSvc.Registrar(in.Nombre, in.Email, in.Password)
		switch {
		case errors.Is(err, auth.ErrDuplicado):
			httpx.Error(w, http.StatusConflict, "email ya registrado")
		case errors.Is(err, auth.ErrDatos):
			httpx.Error(w, http.StatusUnprocessableEntity, "nombre, email o contraseña (8-72) inválidos")
		case err != nil:
			httpx.Error(w, http.StatusInternalServerError, "error interno")
		default:
			httpx.JSON(w, http.StatusCreated, u)
		}
	})

	api.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := httpx.Decodificar(w, r, &in); err != nil {
			httpx.Error(w, http.StatusBadRequest, "json inválido")
			return
		}
		token, expira, err := authSvc.Login(in.Email, in.Password)
		if errors.Is(err, auth.ErrCredenciales) {
			httpx.Error(w, http.StatusUnauthorized, "credenciales inválidas")
			return
		}
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{
			"token":  token,
			"expira": expira.UTC().Format(time.RFC3339),
			"tipo":   "Bearer",
		})
	})

	api.Handle("GET /auth/yo", authSvc.ConAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, auth.UsuarioDesde(r.Context()))
	})))

	// El handler de tareas recibe el middleware de auth, así no conoce nada de tokens.
	tareas.NuevoHandler(repo).Registrar(api, authSvc.ConAuth)

	raiz := http.NewServeMux()
	raiz.HandleFunc("GET /salud", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	raiz.HandleFunc("GET /listo", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := repo.Listo(ctx); err != nil {
			httpx.Error(w, http.StatusServiceUnavailable, "no listo")
			return
		}
		w.Write([]byte("listo"))
	})
	raiz.Handle("/api/v1/", http.StripPrefix("/api/v1", api))

	return httpx.Cadena(raiz, httpx.RequestID, httpx.Logging, httpx.Recuperacion)
}
