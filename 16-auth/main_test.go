package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func ahoraMenosUnSegundo() time.Time { return time.Now().Add(-time.Second) }

func TestFlujoCompleto(t *testing.T) {
	h := Rutas(NuevaAuth())

	// Registro
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/registro",
		strings.NewReader(`{"email":"Ana@Example.com","password":"secreto123"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("registro = %d; body=%s", rec.Code, rec.Body)
	}
	// La respuesta no debe traer el hash.
	if strings.Contains(rec.Body.String(), "hash") || strings.Contains(rec.Body.String(), "$2") {
		t.Fatalf("la respuesta filtró el hash: %s", rec.Body)
	}

	// Login correcto
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader(`{"email":"ana@example.com","password":"secreto123"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d; body=%s", rec.Code, rec.Body)
	}
	var login struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if login.Token == "" {
		t.Fatal("token vacío")
	}

	// /yo con token
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/yo", nil)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/yo = %d; body=%s", rec.Code, rec.Body)
	}

	// /yo sin token
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/yo", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/yo sin token = %d, esperaba 401", rec.Code)
	}

	// /yo con token basura
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/yo", nil)
	req.Header.Set("Authorization", "Bearer no-existe")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/yo con token inválido = %d, esperaba 401", rec.Code)
	}
}

func TestCredencialesInvalidas(t *testing.T) {
	a := NuevaAuth()
	if _, err := a.Registrar("ana@example.com", "secreto123"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := a.Login("ana@example.com", "incorrecta"); err != ErrCredenciales {
		t.Errorf("password incorrecta: %v", err)
	}
	if _, _, err := a.Login("otra@example.com", "secreto123"); err != ErrCredenciales {
		t.Errorf("email inexistente: %v", err)
	}
}

func TestValidacionesDeRegistro(t *testing.T) {
	a := NuevaAuth()
	casos := []struct {
		nombre   string
		email    string
		password string
		esperado error
	}{
		{"email sin arroba", "ana", "secreto123", ErrDatos},
		{"password corta", "ana@example.com", "corta", ErrDatos},
		{"password >72", "ana@example.com", strings.Repeat("x", 73), ErrDatos},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if _, err := a.Registrar(c.email, c.password); err != c.esperado {
				t.Errorf("err = %v, esperaba %v", err, c.esperado)
			}
		})
	}
}

func TestEmailDuplicado(t *testing.T) {
	a := NuevaAuth()
	if _, err := a.Registrar("ana@example.com", "secreto123"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Registrar("ANA@example.com", "secreto123"); err != ErrDuplicado {
		t.Errorf("duplicado = %v, esperaba ErrDuplicado", err)
	}
}

func TestTokenExpirado(t *testing.T) {
	a := NuevaAuth()
	u, err := a.Registrar("ana@example.com", "secreto123")
	if err != nil {
		t.Fatal(err)
	}
	token, err := nuevoToken()
	if err != nil {
		t.Fatal(err)
	}

	a.mu.Lock()
	a.sesiones[token] = Sesion{UsuarioID: u.ID, Expira: ahoraMenosUnSegundo()}
	a.mu.Unlock()

	if _, err := a.Validar(token); err != ErrCredenciales {
		t.Errorf("token expirado = %v, esperaba ErrCredenciales", err)
	}
}
