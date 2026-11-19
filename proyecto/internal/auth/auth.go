// Package auth maneja usuarios en memoria, hashing de passwords y tokens Bearer.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"tutorialgo/proyecto/internal/httpx"
)

var (
	ErrCredenciales = errors.New("credenciales inválidas")
	ErrDuplicado    = errors.New("email ya registrado")
	ErrDatos        = errors.New("datos inválidos")
)

type Usuario struct {
	ID     int64  `json:"id"`
	Nombre string `json:"nombre"`
	Email  string `json:"email"`
	Hash   []byte `json:"-"`
}

type sesion struct {
	usuarioID int64
	expira    time.Time
}

type Servicio struct {
	mu       sync.Mutex
	porEmail map[string]*Usuario
	porID    map[int64]*Usuario
	sesiones map[string]sesion
	nextID   int64
	ttl      time.Duration
}

func Nuevo(ttl time.Duration) *Servicio {
	return &Servicio{
		porEmail: make(map[string]*Usuario),
		porID:    make(map[int64]*Usuario),
		sesiones: make(map[string]sesion),
		ttl:      ttl,
	}
}

var hashFalso, _ = bcrypt.GenerateFromPassword([]byte("relleno-para-igualar-tiempos"), bcrypt.DefaultCost)

func (s *Servicio) Registrar(nombre, email, password string) (*Usuario, error) {
	nombre = strings.TrimSpace(nombre)
	email = strings.ToLower(strings.TrimSpace(email))
	if nombre == "" || !strings.Contains(email, "@") || len(password) < 8 || len(password) > 72 {
		return nil, ErrDatos
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, existe := s.porEmail[email]; existe {
		return nil, ErrDuplicado
	}
	s.nextID++
	u := &Usuario{ID: s.nextID, Nombre: nombre, Email: email, Hash: hash}
	s.porEmail[email] = u
	s.porID[u.ID] = u
	return u, nil
}

func (s *Servicio) Login(email, password string) (string, time.Time, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	s.mu.Lock()
	u, existe := s.porEmail[email]
	s.mu.Unlock()

	if !existe {
		_ = bcrypt.CompareHashAndPassword(hashFalso, []byte(password))
		return "", time.Time{}, ErrCredenciales
	}
	if err := bcrypt.CompareHashAndPassword(u.Hash, []byte(password)); err != nil {
		return "", time.Time{}, ErrCredenciales
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(b)
	expira := time.Now().Add(s.ttl)

	s.mu.Lock()
	s.sesiones[token] = sesion{usuarioID: u.ID, expira: expira}
	s.mu.Unlock()
	return token, expira, nil
}

func (s *Servicio) Validar(token string) (*Usuario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ses, ok := s.sesiones[token]
	if !ok {
		return nil, ErrCredenciales
	}
	if time.Now().After(ses.expira) {
		delete(s.sesiones, token)
		return nil, ErrCredenciales
	}
	return s.porID[ses.usuarioID], nil
}

func (s *Servicio) Logout(token string) {
	s.mu.Lock()
	delete(s.sesiones, token)
	s.mu.Unlock()
}

type claveUsuario struct{}

func (s *Servicio) ConAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			httpx.Error(w, http.StatusUnauthorized, "falta el token Bearer")
			return
		}
		u, err := s.Validar(token)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "token inválido o expirado")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claveUsuario{}, u)))
	})
}

func UsuarioDesde(ctx context.Context) *Usuario {
	u, _ := ctx.Value(claveUsuario{}).(*Usuario)
	return u
}
