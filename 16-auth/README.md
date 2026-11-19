# 16 — Auth

**Qué vas a construir:** registro y login con contraseñas hasheadas, tokens de sesión opacos y
una ruta protegida por middleware. Es la versión mínima y correcta de lo que Spring Security o
ASP.NET Identity hacen por vos.

**Qué necesitas antes:** capítulos 01 a 15.

---

## Paso 0 — El modelo de amenaza, antes de escribir código

Cuatro reglas que no se negocian:

1. **Nunca guardes la contraseña.** Ni cifrada de forma reversible. Solo un **hash** lento.
2. **El hash tiene que ser lento a propósito.** Si tu base se filtra, el atacante prueba
   millones de contraseñas por segundo contra un SHA-256. bcrypt está diseñado para que cada
   intento cueste decenas de milisegundos.
3. **No reveles qué emails existen.** Si el login responde "ese email no existe" para uno y
   "contraseña incorrecta" para otro, cualquiera puede enumerar tus usuarios. Un solo mensaje
   para los dos casos, y tiempo de respuesta parecido.
4. **El token de sesión es una credencial.** Quien lo tenga, entra. Tratarlo como una
   contraseña: aleatorio, largo, por HTTPS, y con vencimiento.

**Sesión opaca vs JWT:** acá el token es una clave aleatoria que apunta a una sesión guardada
en el servidor. Es simple, se revoca borrando la entrada y no hay criptografía que firmar. JWT
tiene sentido cuando muchos servicios necesitan validar el token sin consultar una base común;
el costo es que revocar uno antes de que venza es complicado. Para un monolito, sesión opaca.

## Paso 1 — Errores de dominio y modelo

```go
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
```

- **Tres centinelas**, uno por tipo de fallo. El handler los traduce a 401, 409 y 422 con
  `errors.Is`. Ninguna capa compara strings.
- **`Hash []byte`** porque `bcrypt.GenerateFromPassword` devuelve bytes. Guardar el hash
  completo (60 caracteres) incluye el algoritmo, el costo y el **salt** adentro: no hay una
  columna `salt` separada. Es el mismo formato que `BCryptPasswordEncoder` de Spring o
  `BCrypt.Net` en C#.
- **`json:"-"`** es la línea más importante del struct. Sin ese tag, un campo exportado se
  serializa, y `encoding/json` convierte `[]byte` a **base64**. La respuesta tendría
  `"Hash":"JDJhJDEwJC4uLg=="` y el hash viajaría al cliente. En Java/C# un campo privado no se
  serializa por defecto; en Go **sí**, porque "exportado" es la única condición. Es un footgun
  real.
- **`Sesion` no tiene tag JSON** porque nunca se serializa: es estado interno.

```go
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
```

- **Dos índices** al mismo usuario: por email (para login) y por ID (para resolver la sesión).
  Las dos búsquedas son O(1).
- **`sync.Mutex`** porque los handlers corren en goroutines concurrentes y los maps no son
  seguros para escritura concurrente.
- **`NuevaAuth` inicializa los tres maps.** Un map nil se lee bien pero explota al escribir
  (capítulo 07).

## Paso 2 — `Registrar`: validar, hashear, guardar

```go
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
	...
}
```

- **Normalizar primero:** `ANA@Example.com ` y `ana@example.com` son el mismo usuario. Sin
  esto, el chequeo de duplicado se saltea con mayúsculas y tenés dos cuentas para la misma
  persona.
- **`len(password)` cuenta bytes, no caracteres.** Un límite de 72 bytes con emojis o acentos
  se alcanza antes de lo que parece. bcrypt trabaja con bytes y su límite es 72; pasado eso,
  las versiones modernas de `x/crypto` devuelven
  `bcrypt: password length exceeds 72 bytes`. Validarlo acá convierte un 500 en un 422 claro.
- **Mínimo 8** es una política arbitraria y mínima. Lo importante: no exigir composición
  rara ("una mayúscula, un símbolo") que empuja a la gente a `Password1!`. Mejor largo.
- **`bcrypt.DefaultCost`** vale 10: el hash tarda decenas de milisegundos. Subirlo a 12 lo
  hace ~4 veces más lento; el número correcto es el que tarda ~100 ms en tu hardware.
- **`[]byte(password)`** porque la API de bcrypt recibe bytes.
- **El hash se calcula antes de tomar el lock**: es la parte caraa y no necesita el lock.
  Tomarlo durante 60 ms bloquearía todos los registros y logins.

```go
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
```

- **El chequeo de duplicado va dentro del lock.** Si lo hicieras afuera, dos registros
  simultáneos con el mismo email podrían pasar los dos y uno pisaría al otro.
- **`_, existe :=`** es el `v, ok` del map: acá solo importa si la clave estaba.
- **`a.nextID++` también bajo lock**, o dos registros se llevan el mismo número.
- **Se devuelve el puntero completo, con hash.** Quien serializa es el handler, y el tag
  `json:"-"` lo protege. A nivel interno, el hash existe y se usa en el login.

### Qué contiene un hash bcrypt

```text
$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy
 │   │  └────────────────────┬─────────────────────────┘
 │   │                       └── salt (22) + hash (31)
 │   └── costo (2^10 rondas)
 └── versión del algoritmo
```

El **salt** es aleatorio por usuario y está embebido. Por eso dos personas con la misma
contraseña tienen hashes distintos, y por eso no se compara con `==`: se usa
`bcrypt.CompareHashAndPassword`, que lee costo y salt del hash guardado, rehashea el candidato
y compara.

## Paso 3 — `Login`: mismo error y mismo tiempo para todos

```go
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
	...
}
```

- **Se copia el puntero bajo lock y se suelta antes del bcrypt.** El hash no cambia después de
  crearse, así que leerlo afuera es seguro. Si mantuvieras el lock durante la comparación,
  cada login serializaría a los demás.
- **Email inexistente → comparar contra `hashFalso`.** Sin eso, un email desconocido responde
  en microsegundos y uno existente en 60 ms: la diferencia de tiempo delata qué cuentas
  existen. El `_` descarta el resultado; solo importa que se ejecute.
- **Mismo `ErrCredenciales`** para email inexistente y contraseña incorrecta. El handler
  devuelve el mismo 401 y el mismo mensaje.
- **`CompareHashAndPassword` devuelve error si no coincide**, no un bool. Cualquier error se
  traduce al mismo centinela.

```go
	token, err := nuevoToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expira := time.Now().Add(time.Hour)

	a.mu.Lock()
	a.sesiones[token] = Sesion{UsuarioID: u.ID, Expira: expira}
	a.mu.Unlock()
	return token, expira, nil
```

- **Token nuevo por login.** Dos logins del mismo usuario generan dos sesiones válidas: eso
  permite "una sesión por dispositivo".
- **`time.Now().Add(time.Hour)`**: vencimiento absoluto de una hora. En la respuesta se
  formatea en UTC, pero internamente la comparación usa el mismo reloj.
- **La sesión se guarda bajo lock.** La clave del map es el token.

### El hash de relleno

```go
var hashFalso, _ = bcrypt.GenerateFromPassword([]byte("contraseña-de-relleno"), bcrypt.DefaultCost)
```

Es una variable de paquete: se calcula **una vez al arrancar**, no por request. Se ignora el
error con `_`; si fallara (prácticamente imposible), `hashFalso` sería nil y la comparación
falsa volvería a ser instantánea. En código real, ese error se maneja y el servicio no arranca.

> La mitigación de tiempo reduce un canal lateral, pero **no reemplaza** un límite de intentos.
> bcrypt hace caro cada intento; no impide que un atacante haga mil desde mil IPs.

## Paso 4 — `Validar`: buscar la sesión y chequear el vencimiento

```go
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
```

- **Buscar en el map es O(1).** No hay que recorrer todas las sesiones.
- **`time.Now().After(s.Expira)`** compara contra el vencimiento. Un token vencido se borra en
  ese momento: **limpieza perezosa**. No hace falta un goroutine que recorra sesiones cada
  minuto; se limpian cuando alguien las usa. El costo es que una sesión que nadie vuelve a
  usar queda en memoria hasta el reinicio. Para este tamaño, aceptable.
- **Se devuelve el usuario**, resuelto por `porID`, para que el handler no tenga que volver a
  buscar.
- **`defer a.mu.Unlock()`** cubre los tres `return`, incluido el camino de error.

## Paso 5 — Generar el token

```go
func nuevoToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```

- **32 bytes = 256 bits** de `crypto/rand`. Adivinarlo es inviable. `math/rand` **no** sirve:
  es predecible y está pensado para simulaciones, no para secretos.
- **`rand.Read` llena el slice**; el `_` descarta la cantidad de bytes (siempre 32 si no hay
  error).
- **`hex.EncodeToString`** produce 64 caracteres `[0-9a-f]`, seguros para un header HTTP y
  legibles en logs. Base64 ocupa menos, pero incluye `/` y `+`; hex evita escapes.
- **Si `rand.Read` falla, se devuelve el error.** El handler lo convierte en 500: no se inventa
  un token débil como fallback.

## Paso 6 — `ConAuth`: el middleware

```go
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
```

- **`Authorization: Bearer <token>`** es el esquema estándar para tokens. `CutPrefix` devuelve
  el resto y un bool; no hace falta `Split` ni índices.
- **`token == ""`** cubre `Authorization: Bearer ` (con espacio y nada más). Sin ese chequeo,
  buscarías una sesión con clave vacía.
- **401 en los dos casos**, con mensajes distintos: "falta el token" es un error del cliente
  que no lo mandó; "inválido o expirado" no distingue cuál de los dos es, a propósito. No le
  des al atacante información sobre si el token existió.
- **`claveUsuario{}` como clave de context**: un tipo propio, no un string. Si dos paquetes
  usan `"user"`, se pisan (capítulo 12).
- **`r.WithContext(ctx)`** devuelve una copia del request. El original no se muta y `next` ve
  el context nuevo.
- **`next.ServeHTTP`** solo se llama si la autenticación pasó. El handler protegido no necesita
  saber nada de tokens.
- **El prefijo `Bearer ` distingue mayúsculas.** El RFC dice que el esquema no las distingue;
  en la práctica todos mandan `Bearer`. Si querés ser estricto con el estándar, normalizá el
  header antes de comparar.

## Paso 7 — `POST /registro`: traducir errores a status

```go
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
```

- **Struct anónimo** solo con los campos que el cliente puede mandar. Nunca aceptes `ID` ni
  `Hash` desde el body: el servidor los decide.
- **`MaxBytesReader`** limita el body a 1 MB; **`Decode`** lee el primer valor JSON.
- **`switch` sin expresión**: evalúa los `case` en orden y entra al primero verdadero. El
  `default` es el camino feliz.
- **`errors.Is`** funciona porque los centinelas se devuelven (o envuelven) desde `Registrar`.
  Si mañana agregás contexto con `%w`, esto sigue funcionando.
- **409 Conflict** para email repetido. Es el status correcto: el recurso ya existe y la
  operación no se puede completar por eso.
- **422 Unprocessable Entity** para datos que se leyeron bien pero no cumplen las reglas. 400
  se reserva para el JSON que no se pudo parsear.
- **500 con mensaje genérico**: `"error interno"`, sin `err.Error()`. No filtres detalles
  internos al cliente; loguealos del lado del servidor (capítulo 18).
- **201 Created** y el usuario en el body. El hash no aparece por el tag `json:"-"`.

Dos límites de este handler, para que los tengas presentes:

- **No usa `DisallowUnknownFields`**, así que `{"email":...,"password":...,"admin":true}` se
  acepta y el campo extra se ignora. No es peligroso acá porque el struct solo tiene dos
  campos, pero es una inconsistencia con el capítulo 13.
- **`Decode` acepta un JSON y descarta lo que venga después.** Un body con dos objetos pegados
  no da error. Si querés rechazarlo, después del `Decode` hacés otro y exigís `io.EOF`.

## Paso 8 — `POST /login`: el token en la respuesta

```go
mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
	...
	token, expira, err := a.Login(in.Email, in.Password)
	if errors.Is(err, ErrCredenciales) {
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
```

- **El parseo del body es idéntico al de registro.** En un proyecto real se extrae a una
  función `decodificarJSON`.
- **401 con el mismo mensaje para todos los fallos de credenciales.** No importa si el email
  no existe o la contraseña está mal.
- **200 con el token**, no 201: no se creó un recurso direccionable.
- **`map[string]any`** para armar una respuesta ad hoc. `any` acepta string y cualquier otro
  tipo.
- **`expira.UTC().Format(time.RFC3339)`**: UTC siempre, con formato ISO 8601
  (`2026-11-15T06:47:26Z`). `time.RFC3339` no incluye fracciones de segundo; por eso el valor
  se ve redondeado. Si necesitaras precisión, `time.RFC3339Nano`.
- **Las claves del map salen ordenadas alfabéticamente** (`expira`, `tipo`, `token`), porque
  `encoding/json` ordena las claves de un map. No es el orden en que las escribiste. Si el
  orden importara, usarías un struct.

## Paso 9 — `GET /yo`: leer el usuario del context

```go
mux.Handle("GET /yo", a.ConAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(claveUsuario{}).(*Usuario)
	responderJSON(w, http.StatusOK, u)
})))
```

- **`mux.Handle`** (no `HandleFunc`) porque `ConAuth` devuelve un `http.Handler`. La función
  interna se convierte con `http.HandlerFunc`.
- **La type assertion no chequea `ok`**: el único lugar que pone esa clave es `ConAuth`, y
  esta ruta solo es alcanzable a través de él. Si agregás otra ruta y te olvidás del
  middleware, el panic es inmediato y ruidoso: prefiero eso a un `u == nil` silencioso.
  Aun así, en un handler grande conviene `u, ok := ...` y un 500 explícito.
- **No se busca de nuevo en la base**: el usuario ya vino en el context. El handler no conoce
  tokens ni maps.
- **El hash sigue sin salir**, porque es el mismo struct con `json:"-"`.

## Paso 10 — `main`

```go
func main() {
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           Rutas(NuevaAuth()),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Println("auth en http://localhost:8080")
	log.Fatal(srv.ListenAndServe())
}
```

`Rutas` arma el mux completo; `main` solo escucha. `ReadHeaderTimeout` evita el ataque
Slowloris: un cliente que abre la conexión y no manda headers no la retiene para siempre.

`log.Fatal(srv.ListenAndServe())` es simple pero **incorrecto para producción**: también corta
el proceso cuando el servidor se cierra de forma ordenada (`http.ErrServerClosed`). El capítulo
18 lo arregla con `signal.NotifyContext` y `Shutdown`.

## Flujo real

Estas son las respuestas reales del servidor (recorté `Date` y `Content-Length`):

```text
POST /registro  {"email":"Ana@Example.com","password":"secreto123"}
HTTP/1.1 201 Created
{"id":1,"email":"ana@example.com"}

POST /registro  {"email":"ana@example.com","password":"secreto123"}   (otra vez)
HTTP/1.1 409 Conflict
{"error":"email ya registrado"}

POST /registro  {"email":"b@example.com","password":"corta"}
HTTP/1.1 422 Unprocessable Entity
{"error":"email inválido o contraseña de 8 a 72 caracteres"}

POST /registro  {no es json}
HTTP/1.1 400 Bad Request
{"error":"json inválido"}

POST /login     {"email":"ana@example.com","password":"incorrecta"}
HTTP/1.1 401 Unauthorized
{"error":"credenciales inválidas"}

POST /login     {"email":"ana@example.com","password":"secreto123"}
HTTP/1.1 200 OK
{"expira":"2026-11-15T06:47:26Z","tipo":"Bearer","token":"bbcfb3e2...79bc"}

GET /yo  Authorization: Bearer <token>
HTTP/1.1 200 OK
{"id":1,"email":"ana@example.com"}

GET /yo  (sin header)
HTTP/1.1 401 Unauthorized
{"error":"falta el token Bearer"}

GET /yo  Authorization: Bearer no-existe
HTTP/1.1 401 Unauthorized
{"error":"token inválido o expirado"}
```

Tres cosas para mirar:

- El email se guardó en minúsculas aunque entró con mayúsculas.
- La respuesta de registro y de `/yo` **no tiene** `hash` ni `Hash`.
- El token es hex de 64 caracteres, y el `expira` está una hora adelante.

En PowerShell, con el servidor corriendo:

```powershell
curl.exe -s -X POST http://localhost:8080/registro -H "Content-Type: application/json" `
  -d '{\"email\":\"Ana@Example.com\",\"password\":\"secreto123\"}'

$r = curl.exe -s -X POST http://localhost:8080/login -H "Content-Type: application/json" `
  -d '{\"email\":\"ana@example.com\",\"password\":\"secreto123\"}' | ConvertFrom-Json

curl.exe -i http://localhost:8080/yo -H "Authorization: Bearer $($r.token)"
```

`ConvertFrom-Json` convierte el body en un objeto y `$r.token` saca el campo.

## Los tests

```bash
go test -v ./16-auth
```

```text
=== RUN   TestFlujoCompleto
--- PASS: TestFlujoCompleto (0.08s)
=== RUN   TestCredencialesInvalidas
--- PASS: TestCredencialesInvalidas (0.12s)
=== RUN   TestValidacionesDeRegistro
=== RUN   TestValidacionesDeRegistro/email_sin_arroba
=== RUN   TestValidacionesDeRegistro/password_corta
=== RUN   TestValidacionesDeRegistro/password_>72
--- PASS: TestValidacionesDeRegistro (0.00s)
    --- PASS: TestValidacionesDeRegistro/email_sin_arroba (0.00s)
    --- PASS: TestValidacionesDeRegistro/password_corta (0.00s)
    --- PASS: TestValidacionesDeRegistro/password_>72 (0.00s)
=== RUN   TestEmailDuplicado
--- PASS: TestEmailDuplicado (0.08s)
=== RUN   TestTokenExpirado
--- PASS: TestTokenExpirado (0.04s)
PASS
ok  	tutorialgo/16-auth	0.393s
```

Qué prueba cada uno:

- **`TestFlujoCompleto`**: registro → login → `/yo` con token; y 401 sin token y con token
  basura. Es el test de integración de toda la cadena, sin abrir puerto.
- **`TestCredencialesInvalidas`**: contraseña incorrecta y email inexistente devuelven
  **exactamente** `ErrCredenciales`. Compara con `!=` porque son centinelas sin envolver.
- **`TestValidacionesDeRegistro`**: tabla con los tres límites (sin `@`, corta, >72). Los
  subtest permiten ver cuál falló.
- **`TestEmailDuplicado`**: registra `ana@` y después `ANA@`; el segundo debe dar
  `ErrDuplicado`. Prueba la normalización, no solo el map.
- **`TestTokenExpirado`**: genera un token, inserta una sesión ya vencida **directo en el map**
  y verifica que `Validar` la rechaza. Es un test de caja blanca: vive en `package main`, así
  que puede tocar campos privados. Por eso toma el lock antes de escribir: hasta en un test,
  el map se protege.

Los tests de bcrypt tardan ~50-80 ms por hash, y eso es **esperado**. Si un test de auth corre
en 0 ms, algo está mal: probablemente estés comparando strings en vez de hashes.

### Una advertencia sobre el test del hash

```go
if strings.Contains(rec.Body.String(), "hash") || strings.Contains(rec.Body.String(), "$2") {
	t.Fatalf("la respuesta filtró el hash: %s", rec.Body)
}
```

La intención es correcta, pero **no atrapa el caso real**. Si borraras `json:"-"`:

- la clave sería `"Hash"` con mayúscula, y `Contains(..., "hash")` distingue mayúsculas;
- el valor sería base64 (`"JDJhJDEwJC4uLg=="`), que no contiene `$2`.

Comprobado: el JSON sale `{"id":1,"email":"a@b.com","Hash":"JDJhJDEwJC4uLg=="}` y las dos
búsquedas dan `false`. La forma correcta es decodificar el body y afirmar que la clave no
existe, o buscar `"Hash"` y el prefijo base64 `"JDJh"`. Es el tipo de test que da falsa
seguridad: pasa siempre. El ejercicio 4 te pide arreglarlo.

## Lo que este ejemplo no hace (y en producción importa)

- **Todo vive en memoria.** Al reiniciar se pierden usuarios y sesiones; con dos instancias,
  el token emitido por una no lo conoce la otra. La solución es una tabla de usuarios
  (capítulo 15/proyecto) y sesiones compartidas (otra tabla o Redis).
- **El token se guarda en claro.** Quien lea el map (o la tabla) tiene sesiones válidas. Se
  guarda el **SHA-256** del token y se compara el hash de lo que llega. Así, un dump no sirve
  para autenticarse.
- **No hay límite de intentos.** bcrypt encarece cada intento, pero un atacante distribuido
  igual prueba. Falta rate limiting por IP y por email, y bloqueo progresivo. Es lo primero
  que se agrega después de esto.
- **No hay logout ni refresh.** Borrar la entrada de `sesiones` es un endpoint de tres líneas.
  Rotar el token antes de que venza es el siguiente paso.
- **No hay HTTPS.** Un Bearer token viaja en cada request; sin TLS, cualquiera en la red lo
  lee. TLS va en el reverse proxy o en el servidor.
- **La validación de email es mínima** (`strings.Contains("@")`). No verifica dominio ni
  existencia. En producción se manda un mail de confirmación.
- **`len(password)` son bytes**: una contraseña de 30 emojis supera los 72 y se rechaza. Es
  correcto para bcrypt, pero el mensaje al usuario debería explicarlo.
- **`log.Fatal` en `main`** también dispara en un cierre ordenado.
- **No hay `DisallowUnknownFields`** en los decoders.

## Errores que vas a ver

| Síntoma | Causa | Solución |
|---|---|---|
| `crypto/bcrypt: hashedSecret too short` | guardaste un hash truncado o vacío | guardá los 60 caracteres completos |
| `bcrypt: password length exceeds 72 bytes` | no validaste antes | rechazá con 422 en `Registrar` |
| `401` con un token recién emitido | header sin `Bearer ` o con minúsculas | `Authorization: Bearer <token>` |
| `panic: interface conversion` en `/yo` | registraste la ruta sin `ConAuth` | envolvé el handler |
| el hash aparece en el JSON | falta `json:"-"` | agregalo y arreglá el test |
| dos emails iguales con mayúsculas | no normalizaste | `ToLower(TrimSpace(...))` |
| login tarda distinto según el email | falta `hashFalso` | compará siempre un hash |
| sesión inválida tras reiniciar | sesiones en memoria | tabla de sesiones |
| `undefined: bcrypt` | falta la dependencia | `go get golang.org/x/crypto/bcrypt` |
| todos los `%` del test salen raros | consola Windows sin UTF-8 | `chcp 65001` o usá PowerShell 7 |

## Por qué importa en el trabajo

- **Guardar contraseñas mal es el bug más caro que existe**: sobrevive al deploy, a la rotación
  y al firewall. bcrypt con costo adecuado es el mínimo.
- **El timing attack** es real y fácil de medir desde afuera. La comparación falsa es una línea
  que lo tapa.
- **El middleware de auth** es el patrón con el que después agregás roles, permisos y rate
  limiting, siempre en el mismo lugar.
- **El usuario en el context** es cómo las capas de abajo saben quién está operando sin pasar
  un parámetro por diez funciones.
- **Los status codes** son el contrato: 401 (no sé quién sos), 403 (sé quién sos, no podés),
  409 (conflicto), 422 (datos inválidos). Confundirlos rompe clientes.

## Ejercicio

1. Agregá `POST /logout`, protegido con `ConAuth`, que borre la sesión del token recibido. Para
   eso necesitás pasar el token al context o cambiar `ConAuth` para guardar la sesión. Escribí
   un test que verifique que después del logout `/yo` da 401.
2. Implementá un límite de 5 intentos fallidos por email cada minuto. Devolvé 429 con
   `Retry-After`. ¿Dónde guardás los intentos y cómo los limpiás?
3. Guardá en `sesiones` el SHA-256 del token en vez del token. `Validar` hashea lo que llega y
   busca. Los tests existentes siguen pasando si no tocan el map; el de expiración necesita
   ajustarse.
4. Arreglá el test del hash: decodificá el body a `map[string]any` y fallá si existe la clave
   `Hash` o `hash`. Verificá que el test **falla** si borrás `json:"-"`, y después volvé a
   ponerlo. Ese es el único test que vale para esto.
5. Medí el costo: `go test -bench` con un `BenchmarkHash` que use `bcrypt.DefaultCost` y otro
   con costo 12. Elegí el número que dé ~100 ms en tu máquina y explicá el trade-off.

Siguiente: [17 — Configuración y entorno](../17-config-entorno/).
