package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Estado string

const (
	Activo     Estado = "activo"
	Suspendido Estado = "suspendido"
)

// Usuario es un modelo de dominio. Los tags `json:"..."` definen cómo se
// serializa en la API; sin tag, la clave sería "Nombre".
type Usuario struct {
	ID     int64     `json:"id"`
	Nombre string    `json:"nombre"`
	Email  string    `json:"email"`
	Estado Estado    `json:"estado"`
	Creado time.Time `json:"creado_en"`
}

// NewUsuario es el constructor: centraliza las invariantes.
// El struct es exportado, pero se espera que se cree por acá.
func NewUsuario(nombre, email string) (*Usuario, error) {
	nombre = strings.TrimSpace(nombre)
	email = strings.ToLower(strings.TrimSpace(email))
	if nombre == "" {
		return nil, fmt.Errorf("nombre vacío")
	}
	if !strings.Contains(email, "@") {
		return nil, fmt.Errorf("email inválido: %q", email)
	}
	return &Usuario{
		Nombre: nombre,
		Email:  email,
		Estado: Activo,
		Creado: time.Now().UTC(),
	}, nil
}

// Método con receptor puntero: modifica el usuario original.
func (u *Usuario) Suspender() {
	u.Estado = Suspendido
}

// Método con receptor valor: solo lee. Se usa para implementar fmt.Stringer,
// así fmt imprime algo legible en vez de la dirección de memoria.
func (u Usuario) String() string {
	return fmt.Sprintf("%s <%s> [%s]", u.Nombre, u.Email, u.Estado)
}

// ¿Está activo? Nombre natural, no GetActivo().
func (u Usuario) Activo() bool {
	return u.Estado == Activo
}

// Embedding (composición), no herencia: Admin "tiene un" Usuario y hereda
// sus métodos y campos promovidos.
type Admin struct {
	Usuario
	Permisos []string
}

func (a Admin) Puede(p string) bool {
	for _, x := range a.Permisos {
		if x == p {
			return true
		}
	}
	return false
}

func main() {
	ana, err := NewUsuario("  Ana  ", "ANA@Example.com")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	ana.ID = 1
	fmt.Println(ana) // usa String()
	fmt.Println("activo:", ana.Activo())

	ana.Suspender()
	fmt.Println(ana)

	b, _ := json.MarshalIndent(ana, "", "  ")
	fmt.Println(string(b))

	// Embedding
	admin := Admin{
		Usuario:  Usuario{Nombre: "Root", Email: "root@example.com", Estado: Activo},
		Permisos: []string{"usuarios:leer", "usuarios:borrar"},
	}
	fmt.Println("admin:", admin.Nombre, "puede borrar:", admin.Puede("usuarios:borrar"))

	// Copia vs puntero: esto sorprende a todos la primera vez.
	copia := admin.Usuario // copia el valor
	copia.Nombre = "Otro"
	fmt.Println("original:", admin.Nombre, "| copia:", copia.Nombre)

	// Struct vacío: zero values, seguro de leer.
	var vacio Usuario
	fmt.Printf("vacío: %+v activo=%t\n", vacio, vacio.Activo())
}
