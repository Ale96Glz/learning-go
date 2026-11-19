package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrCredenciales = errors.New("credenciales inválidas")
	ErrDuplicado    = errors.New("email ya registrado")
	ErrDatos        = errors.New("datos inválidos")
)

type Usuario struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Hash  []byte `json:"-"` // json:"-" garantiza que nunca salga en una respuesta
}

type Sesion struct {
	UsuarioID int64
	Expira    time.Time
}

type Auth struct {
	mu       sync.Mutex
	porEmail map[string]*Usuario
	porID    map[int64]*Usuario
	sesiones map[string]Sesion
	nextID   int64
}

func NuevaAuth() *Auth {
	return &Auth{
		porEmail: make(map[string]*Usuario),
		porID:    make(map[int64]*Usuario),
		sesiones: make(map[string]Sesion),
	}
}

// Hash de relleno para que un email inexistente tarde lo mismo que uno existente.
// Sin esto, el tiempo de respuesta revela qué emails están registrados.
var hashFalso, _ = bcrypt.GenerateFromPassword([]byte("contraseña-de-relleno"), bcrypt.DefaultCost)

func (a *Auth) Registrar(email, password string) (*Usuario, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") || len(password) < 8 || len(password) > 72 {
		// bcrypt ignora todo después de 72 bytes: mejor rechazar que truncar.
		return nil, ErrDatos
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, existe := a.porEmail[email]; existe {
		return nil, ErrDuplicado
	}
	a.nextID++
	u := &Usuario{ID: a.nextID, Email: email, Hash: hash}
	a.porEmail[email] = u
	a.porID[u.ID] = u
	return u, nil
}

func (a *Auth) Login(email, password string) (string, time.Time, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	a.mu.Lock()
	u, existe := a.porEmail[email]
	a.mu.Unlock()

	if !existe {
		// Comparación falsa para igualar tiempos. Luego el mismo error.
		_ = bcrypt.CompareHashAndPassword(hashFalso, []byte(password))
		return "", time.Time{}, ErrCredenciales
	}
	if err := bcrypt.CompareHashAndPassword(u.Hash, []byte(password)); err != nil {
		return "", time.Time{}, ErrCredenciales
	}

	token, err := nuevoToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expira := time.Now().Add(time.Hour)

	a.mu.Lock()
	a.sesiones[token] = Sesion{UsuarioID: u.ID, Expira: expira}
	a.mu.Unlock()
	return token, expira, nil
}

func (a *Auth) Validar(token string) (*Usuario, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	s, ok := a.sesiones[token]
	if !ok {
		return nil, ErrCredenciales
	}
	if time.Now().After(s.Expira) {
		delete(a.sesiones, token) // limpieza perezosa
		return nil, ErrCredenciales
	}
	return a.porID[s.UsuarioID], nil
}

func nuevoToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type claveUsuario struct{}

func (a *Auth) ConAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			responderError(w, http.StatusUnauthorized, "falta el token Bearer")
			return
		}
		u, err := a.Validar(token)
		if err != nil {
			responderError(w, http.StatusUnauthorized, "token inválido o expirado")
			return
		}
		ctx := context.WithValue(r.Context(), claveUsuario{}, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func Rutas(a *Auth) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /registro", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			responderError(w, http.StatusBadRequest, "json inválido")
			return
		}
		u, err := a.Registrar(in.Email, in.Password)
		switch {
		case errors.Is(err, ErrDuplicado):
			responderError(w, http.StatusConflict, "email ya registrado")
		case errors.Is(err, ErrDatos):
			responderError(w, http.StatusUnprocessableEntity, "email inválido o contraseña de 8 a 72 caracteres")
		case err != nil:
			responderError(w, http.StatusInternalServerError, "error interno")
		default:
			responderJSON(w, http.StatusCreated, u)
		}
	})

	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			responderError(w, http.StatusBadRequest, "json inválido")
			return
		}
		token, expira, err := a.Login(in.Email, in.Password)
		if errors.Is(err, ErrCredenciales) {
			// Mismo mensaje para email inexistente y contraseña incorrecta.
			responderError(w, http.StatusUnauthorized, "credenciales inválidas")
			return
		}
		if err != nil {
			responderError(w, http.StatusInternalServerError, "error interno")
			return
		}
		responderJSON(w, http.StatusOK, map[string]any{
			"token":  token,
			"expira": expira.UTC().Format(time.RFC3339),
			"tipo":   "Bearer",
		})
	})

	mux.Handle("GET /yo", a.ConAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.Context().Value(claveUsuario{}).(*Usuario)
		responderJSON(w, http.StatusOK, u)
	})))

	mux.HandleFunc("GET /salud", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	return mux
}

func responderJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func responderError(w http.ResponseWriter, status int, msg string) {
	responderJSON(w, status, map[string]string{"error": msg})
}

func main() {
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           Rutas(NuevaAuth()),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Println("auth en http://localhost:8080")
	log.Fatal(srv.ListenAndServe())
}
