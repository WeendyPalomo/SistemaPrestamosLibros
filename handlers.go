package main

import (
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"net/url" // Importar el paquete url para url.QueryEscape
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// Definición de la estructura Libro
type Libro struct {
	ID            string `json:"id" firestore:"id,omitempty"`
	Nombre        string `json:"nombre" firestore:"nombre"`
	Autor         string `json:"autor" firestore:"autor"`
	Ano           int    `json:"ano" firestore:"ano"`
	Descripcion   string `json:"descripcion" firestore:"descripcion"`
	ImagenURL     string `json:"imagenURL" firestore:"imagen"`
	Copias        int    `json:"copias" firestore:"copias"`
	Disponible    bool   `json:"disponible" firestore:"disponible"`                 // Nuevo campo: true si está disponible para préstamo
	PrestadoPorID string `json:"prestadoPorID" firestore:"prestadoPorID,omitempty"` // ID de la persona que lo tiene prestado
}

// Definición de la estructura Persona
type Persona struct {
	ID         string `json:"id" firestore:"id,omitempty"`
	Nombre     string `json:"nombre" firestore:"nombre"`
	Cedula     string `json:"cedula" firestore:"cedula"`
	Ano        int    `json:"ano" firestore:"ano"`      // CAMBIADO: Ahora es int para consistencia numérica
	Contrasena string `json:"-" firestore:"contrasena"` // Ignorar en JSON, no almacenar en el cliente
	Rol        string `json:"rol" firestore:"rol"`
}

// Definición de la estructura Prestamo
type Prestamo struct {
	ID              string    `json:"id" firestore:"id,omitempty"`
	LibroID         string    `json:"libroID" firestore:"libroID"`                                     // ID del libro prestado
	PersonaID       string    `json:"personaID" firestore:"personaID"`                                 // ¡Asegúrate de que esta etiqueta firestore sea correcta!
	FechaPrestamo   time.Time `json:"fechaPrestamo" firestore:"fechaPrestamo"`                         // Fecha en que se realizó el préstamo
	FechaDevolucion time.Time `json:"fechaDevolucion,omitempty" firestore:"fechaDevolucion,omitempty"` // Fecha de devolución (opcional, se llena al devolver)
	Activo          bool      `json:"activo" firestore:"activo"`                                       // true si el préstamo está activo, false si ya se devolvió
}

// DevolucionDisplayData combina Prestamo, Libro, y Persona para mostrar en la tabla de devoluciones
type DevolucionDisplayData struct {
	PrestamoID    string
	LibroID       string // Necesario para la devolución
	LibroNombre   string
	AutorNombre   string
	UsuarioID     string // Necesario para la devolución
	UsuarioNombre string
	FechaPrestamo time.Time
	Activo        bool
}

// Definición de la estructura DatosPagina
type DatosPagina struct {
	Libros            []Libro
	LibrosDisponibles []Libro                 // Para el formulario de préstamo
	Personas          []Persona               // Para el formulario de préstamo (ahora solo para referencia, no para selección)
	Prestamos         []Prestamo              // Para listar préstamos (sin usar directamente en devoluciones.html)
	DevolucionesData  []DevolucionDisplayData // Nuevo campo para la tabla de devoluciones
	Detalle           *Libro
	Año               int
	Usuario           string
	Rol               string
	SearchQuery       string
	Mensaje           string // Nuevo campo para mensajes de éxito/error
	TipoMensaje       string // "success" o "danger"
}

// Nueva estructura para la respuesta JSON de LibrosHandler (para AJAX)
type LibrosResponse struct {
	Libros  []Libro `json:"libros"`
	Usuario string  `json:"usuario"`
	Rol     string  `json:"rol"`
}

// NUNCA OLVIDES AGREGAR ESTAS NUEVAS FUNCIONES AL renderTemplate GLOBAL
// Aquí está la función auxiliar `inc`
var funcs = template.FuncMap{
	"inc": func(i int) int {
		return i + 1
	},
	"formatDate": func(t time.Time) string { // Función para formatear fechas en la plantilla
		return t.Format("02/01/2006") // Formato DD/MM/YYYY
	},
}

func renderTemplate(w http.ResponseWriter, r *http.Request, archivo string, data interface{}) {
	tmpl, err := template.New("base.html").Funcs(funcs).ParseFiles("templates/base.html", "templates/"+archivo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println("Error cargando plantilla:", archivo, err)
		return
	}
	tmpl.ExecuteTemplate(w, "base", data)
}

// LibrosHandler fetches and displays the list of books, with optional search.
func LibrosHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	searchQuery := r.URL.Query().Get("q")
	// Obtener mensajes de la URL (si existen)
	mensaje := r.URL.Query().Get("msg")
	tipoMensaje := r.URL.Query().Get("msg_type")

	query := FirestoreClient.Collection("libro").Query

	iter := query.Documents(ctx)

	var allLibros []Libro
	// Declarar err en el ámbito de la función, si se va a usar fuera del bucle.
	// En este caso, no se usa fuera, así que se puede declarar con := dentro del bucle.

	for {
		doc, errLoop := iter.Next() // Renombrado err a errLoop
		if errLoop == iterator.Done {
			break
		}
		if errLoop != nil {
			log.Printf("Error al iterar libros: %v", errLoop)
			http.Error(w, "Error al cargar libros", http.StatusInternalServerError)
			return
		}
		var libro Libro
		if errLoop := doc.DataTo(&libro); errLoop != nil { // Renombrado err a errLoop
			log.Printf("Error al mapear datos de libro: %v", errLoop)
			continue // Saltar este documento y continuar con el siguiente
		}
		libro.ID = doc.Ref.ID // Asignar el ID del documento
		allLibros = append(allLibros, libro)
	}

	var filteredLibros []Libro
	if searchQuery != "" {
		lowerSearchQuery := strings.ToLower(searchQuery)
		for _, libro := range allLibros {
			lowerTitle := strings.ToLower(libro.Nombre)
			lowerAuthor := strings.ToLower(libro.Autor)

			if strings.Contains(lowerTitle, lowerSearchQuery) || strings.Contains(lowerAuthor, lowerSearchQuery) {
				filteredLibros = append(filteredLibros, libro)
			}
		}
	} else {
		filteredLibros = allLibros
	}

	// Obtener usuario y rol de las cookies para ambas respuestas (HTML y AJAX)
	usuario := ""
	rol := ""
	if c, errCookie := r.Cookie("usuario"); errCookie == nil { // Renombrado err a errCookie
		usuario = c.Value
	}
	if c, errCookie := r.Cookie("rol"); errCookie == nil { // Renombrado err a errCookie
		rol = c.Value
	}

	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		// Para solicitudes AJAX, devolver un JSON con libros, usuario y rol
		response := LibrosResponse{
			Libros:  filteredLibros,
			Usuario: usuario,
			Rol:     rol,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil { // Codificar la nueva estructura
			log.Printf("Error al codificar JSON para AJAX: %v", err)
			http.Error(w, "Error interno del servidor", http.StatusInternalServerError)
		}
		return
	}

	// Si no es una solicitud AJAX, renderiza la plantilla HTML completa
	data := DatosPagina{
		Libros:      filteredLibros,
		Año:         time.Now().Year(),
		Usuario:     usuario, // Asegurarse de que el usuario se pase a la plantilla HTML
		Rol:         rol,     // Asegurarse de que el rol se pase a la plantilla HTML
		SearchQuery: searchQuery,
		Mensaje:     mensaje,     // Pasar el mensaje a la plantilla
		TipoMensaje: tipoMensaje, // Pasar el tipo de mensaje a la plantilla
	}

	renderTemplate(w, r, "libros.html", data)
}

// EditarLibroHandler handles displaying the edit form and processing updates.
func EditarLibroHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("DEBUG: Entrando a EditarLibroHandler")
	// Verificar rol de administrador
	rol := ""
	if c, errCookie := r.Cookie("rol"); errCookie == nil { // Renombrado err a errCookie
		rol = c.Value
	}
	log.Printf("DEBUG: Rol del usuario: %s", rol)
	if rol != "admin" {
		log.Println("DEBUG: Acceso denegado a EditarLibroHandler (no admin)")
		http.Error(w, "Acceso denegado. Solo administradores pueden editar libros.", http.StatusForbidden)
		return
	}

	if r.Method == http.MethodGet {
		log.Println("DEBUG: Método GET en EditarLibroHandler")
		bookID := r.URL.Query().Get("id")
		log.Printf("DEBUG: ID del libro a editar (GET): %s", bookID) // Log más específico
		if bookID == "" {
			log.Println("DEBUG: ID de libro no proporcionado en GET")
			http.Error(w, "ID de libro no proporcionado", http.StatusBadRequest)
			return
		}

		ctx := context.Background()
		doc, err := FirestoreClient.Collection("libro").Doc(bookID).Get(ctx)
		if err != nil {
			log.Printf("DEBUG: Error al obtener libro %s de Firestore: %v", bookID, err)
			http.Error(w, "Libro no encontrado: "+err.Error(), http.StatusNotFound)
			return
		}
		log.Println("DEBUG: Libro obtenido de Firestore")

		var libro Libro
		if err := doc.DataTo(&libro); err != nil {
			log.Printf("DEBUG: Error al parsear datos del libro %s: %v", bookID, err)
			http.Error(w, "Error al parsear datos del libro: "+err.Error(), http.StatusInternalServerError)
			return
		}
		libro.ID = doc.Ref.ID                                                   // Asignar el ID del documento al campo ID de la estructura
		log.Printf("DEBUG: Datos del libro parseados para edición: %+v", libro) // Log con datos parseados

		usuario := ""
		if c, errCookie := r.Cookie("usuario"); errCookie == nil { // Renombrado err a errCookie
			usuario = c.Value
		}
		log.Printf("DEBUG: Usuario logueado: %s", usuario)

		data := DatosPagina{
			Detalle: &libro,
			Año:     time.Now().Year(),
			Usuario: usuario,
			Rol:     rol,
		}
		log.Println("DEBUG: Renderizando editar_libros.html")
		renderTemplate(w, r, "editar_libros.html", data)
		return
	}

	if r.Method == http.MethodPost {
		log.Println("DEBUG: Método POST en EditarLibroHandler")
		bookID := r.FormValue("id")
		nombre := r.FormValue("nombre")
		autor := r.FormValue("autor")
		descripcion := r.FormValue("descripcion")
		imagen := r.FormValue("imagen")
		anoStr := r.FormValue("ano")
		copiasStr := r.FormValue("copias")
		disponibleStr := r.FormValue("disponible") // Obtener el valor de disponible

		ano, err := strconv.Atoi(anoStr)
		if err != nil {
			log.Printf("DEBUG POST: Año inválido: %s, Error: %v", anoStr, err)
			http.Redirect(w, r, "/libros?msg=Año inválido&msg_type=danger", http.StatusSeeOther)
			return
		}
		copias, err := strconv.Atoi(copiasStr)
		if err != nil {
			log.Printf("DEBUG POST: Número de copias inválido: %s, Error: %v", copiasStr, err)
			http.Redirect(w, r, "/libros?msg=Número de copias inválido&msg_type=danger", http.StatusSeeOther)
			return
		}
		// Un checkbox no enviado (desmarcado) resulta en un valor vacío, no "off".
		// Si se envía "on", significa que está marcado. Si es vacío, está desmarcado.
		disponible := (disponibleStr == "on")

		updates := []firestore.Update{
			{Path: "nombre", Value: nombre},
			{Path: "autor", Value: autor},
			{Path: "descripcion", Value: descripcion},
			{Path: "imagen", Value: imagen},
			{Path: "ano", Value: ano},
			{Path: "copias", Value: copias},
			{Path: "disponible", Value: disponible}, // Actualizar el campo disponible
		}
		log.Printf("DEBUG POST: Actualizaciones a enviar a Firestore: %+v", updates)

		ctx := context.Background()
		_, err = FirestoreClient.Collection("libro").Doc(bookID).Update(ctx, updates)
		if err != nil {
			log.Printf("DEBUG POST: Error al actualizar libro %s en Firestore: %v", bookID, err)
			http.Redirect(w, r, "/libros?msg=Error al actualizar el libro&msg_type=danger", http.StatusSeeOther)
			return
		}

		log.Printf("✅ Libro actualizado exitosamente: %s (ID: %s)", nombre, bookID)
		http.Redirect(w, r, "/libros?msg=Libro actualizado exitosamente&msg_type=success", http.StatusSeeOther)
	}
}

// EliminarLibroHandler handles deleting a book.
func EliminarLibroHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("DEBUG: Entrando a EliminarLibroHandler")
	// Verificar rol de administrador
	rol := ""
	if c, errCookie := r.Cookie("rol"); errCookie == nil { // Renombrado err a errCookie
		rol = c.Value
	}
	log.Printf("DEBUG: Rol del usuario en Eliminar: %s", rol)
	if rol != "admin" {
		log.Println("DEBUG: Acceso denegado a EliminarLibroHandler (no admin)")
		http.Error(w, "Acceso denegado. Solo administradores pueden eliminar libros.", http.StatusForbidden)
		return
	}

	if r.Method != http.MethodPost {
		log.Println("DEBUG: Método no permitido en EliminarLibroHandler")
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	bookID := r.FormValue("id")
	log.Printf("DEBUG: ID del libro a eliminar recibido: %s", bookID)
	if bookID == "" {
		log.Println("DEBUG: ID de libro no proporcionado en POST para eliminar")
		http.Error(w, "ID de libro no proporcionado", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	_, err := FirestoreClient.Collection("libro").Doc(bookID).Delete(ctx) // Usar := para la primera declaración y asignación
	if err != nil {
		log.Printf("DEBUG: Error al eliminar libro %s de Firestore: %v", bookID, err)
		http.Error(w, "Error al eliminar libro: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Libro eliminado exitosamente: (ID: %s)", bookID)
	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/libros", http.StatusSeeOther)
}

// DevolucionesHandler handles displaying the returns form and processing submissions.
func DevolucionesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	usuarioNombre := ""
	rol := ""
	if c, errCookie := r.Cookie("usuario"); errCookie == nil { // Renombrado err a errCookie
		usuarioNombre = c.Value
	}
	if c, errCookie := r.Cookie("rol"); errCookie == nil { // Renombrado err a errCookie
		rol = c.Value
	}

	// Obtener mensajes de la URL (si existen)
	mensaje := r.URL.Query().Get("msg")
	tipoMensaje := r.URL.Query().Get("msg_type")

	if r.Method == http.MethodGet {
		var devolucionesData []DevolucionDisplayData

		// Obtener el ID de la persona logueada
		var loggedInPersonaID string
		if usuarioNombre == "" {
			// Si no hay usuario logueado, no hay préstamos que mostrar
			data := DatosPagina{
				DevolucionesData: []DevolucionDisplayData{}, // Vacío
				Año:              time.Now().Year(),
				Usuario:          usuarioNombre,
				Rol:              rol,
				Mensaje:          "Debes iniciar sesión para ver tus préstamos.",
				TipoMensaje:      "info",
			}
			renderTemplate(w, r, "devoluciones.html", data)
			return
		}

		iterPersona := FirestoreClient.Collection("persona").Where("nombre", "==", usuarioNombre).Documents(ctx)
		personaDoc, errInner := iterPersona.Next() // Usar := para declarar una nueva 'errInner' en el ámbito del bloque
		if errInner != nil {
			log.Printf("DEBUG Devoluciones: Error al buscar persona '%s' para devoluciones: %v", usuarioNombre, errInner)
			data := DatosPagina{
				DevolucionesData: []DevolucionDisplayData{}, // Corregido: Usar slice vacío del tipo correcto
				Año:              time.Now().Year(),
				Usuario:          usuarioNombre,
				Rol:              rol,
				Mensaje:          "No se encontró tu usuario en la base de datos.",
				TipoMensaje:      "danger",
			}
			renderTemplate(w, r, "devoluciones.html", data)
			return
		}
		loggedInPersonaID = personaDoc.Ref.ID
		log.Printf("DEBUG Devoluciones: Usuario logueado: %s (ID: %s)", usuarioNombre, loggedInPersonaID)

		// 1. Obtener todos los préstamos activos del usuario logueado
		iterPrestamos := FirestoreClient.Collection("prestamos").
			Where("activo", "==", true).
			Where("personaID", "==", loggedInPersonaID). // Filtrar por el ID del usuario logueado
			Documents(ctx)
		log.Printf("DEBUG Devoluciones: Consultando préstamos activos para personaID: %s", loggedInPersonaID)

		for {
			docPrestamo, errLoop := iterPrestamos.Next() // Usar := para declarar una nueva 'errLoop' en el ámbito del bucle
			if errLoop == iterator.Done {
				log.Println("DEBUG Devoluciones: No más préstamos activos para este usuario.")
				break
			}
			if errLoop != nil {
				log.Printf("Error al iterar préstamos activos: %v", errLoop)
				http.Error(w, "Error al cargar préstamos", http.StatusInternalServerError)
				return
			}

			var prestamo Prestamo
			if errLoop = docPrestamo.DataTo(&prestamo); errLoop != nil { // Usar = para asignar a la 'errLoop' del bucle
				log.Printf("DEBUG Devoluciones: Error al mapear datos de préstamo %s: %v", docPrestamo.Ref.ID, errLoop)
				continue
			}
			prestamo.ID = docPrestamo.Ref.ID
			log.Printf("DEBUG Devoluciones: Préstamo encontrado: ID %s, LibroID %s, PersonaID %s", prestamo.ID, prestamo.LibroID, prestamo.PersonaID)

			// 2. Obtener los detalles del libro asociado al préstamo
			libroDoc, errLoop := FirestoreClient.Collection("libro").Doc(prestamo.LibroID).Get(ctx) // Usar := para declarar una nueva 'errLoop' en el ámbito del bucle
			if errLoop != nil {
				log.Printf("DEBUG Devoluciones: Error al obtener libro (ID: %s) para préstamo %s: %v", prestamo.LibroID, prestamo.ID, errLoop)
				// Si el libro no se encuentra, podemos saltar este préstamo o mostrar un error
				continue
			}
			var libro Libro
			if errLoop = libroDoc.DataTo(&libro); errLoop != nil { // Usar = para asignar a la 'errLoop' del bucle
				log.Printf("DEBUG Devoluciones: Error al mapear datos de libro para préstamo %s: %v", prestamo.ID, errLoop)
				continue
			}
			libro.ID = libroDoc.Ref.ID // <--- ¡AQUÍ ESTÁ LA CORRECCIÓN CLAVE! Asignar el ID del documento del libro.
			log.Printf("DEBUG Devoluciones: Detalles del libro obtenidos: Nombre '%s', Autor '%s', ID '%s'", libro.Nombre, libro.Autor, libro.ID)

			// 3. Obtener los detalles de la persona asociada al préstamo
			personaDoc, errLoop := FirestoreClient.Collection("persona").Doc(prestamo.PersonaID).Get(ctx) // Usar := para declarar una nueva 'errLoop' en el ámbito del bucle
			if errLoop != nil {
				log.Printf("DEBUG Devoluciones: Error al obtener persona (ID: %s) para préstamo %s: %v", prestamo.PersonaID, prestamo.ID, errLoop)
				continue
			}
			var persona Persona
			if errLoop = personaDoc.DataTo(&persona); errLoop != nil { // Usar = para asignar a la 'errLoop' del bucle
				log.Printf("DEBUG Devoluciones: Error al mapear datos de persona para préstamo %s: %v", prestamo.ID, errLoop)
				continue
			}
			persona.ID = personaDoc.Ref.ID // Asignar el ID de la persona también, por si acaso
			log.Printf("DEBUG Devoluciones: Detalles de la persona obtenidos: Nombre '%s', ID '%s'", persona.Nombre, persona.ID)

			// 4. Construir el objeto para mostrar en la plantilla
			devolucionesData = append(devolucionesData, DevolucionDisplayData{
				PrestamoID:    prestamo.ID,
				LibroID:       libro.ID, // Ahora libro.ID debería tener un valor
				LibroNombre:   libro.Nombre,
				AutorNombre:   libro.Autor,
				UsuarioID:     persona.ID,
				UsuarioNombre: persona.Nombre,
				FechaPrestamo: prestamo.FechaPrestamo,
				Activo:        prestamo.Activo,
			})
			log.Printf("DEBUG Devoluciones: Añadido préstamo a la lista de visualización: %s (LibroID: %s)", libro.Nombre, libro.ID)
		}

		data := DatosPagina{
			DevolucionesData: devolucionesData,
			Año:              time.Now().Year(),
			Usuario:          usuarioNombre, // Usar usuarioNombre para el display
			Rol:              rol,
			Mensaje:          mensaje,
			TipoMensaje:      tipoMensaje,
		}
		renderTemplate(w, r, "devoluciones.html", data)
		return
	}

	if r.Method == http.MethodPost {
		log.Println("DEBUG: Entrando al método POST /devoluciones")
		prestamoID := r.FormValue("prestamoID")
		libroID := r.FormValue("libroID")

		if prestamoID == "" || libroID == "" {
			// ... manejo de error ...
		}

		err := FirestoreClient.RunTransaction(ctx, func(ctx_tx context.Context, tx *firestore.Transaction) error {
			// --- 1. Leer ambos documentos ANTES de escribir ---
			prestamoRef := FirestoreClient.Collection("prestamos").Doc(prestamoID)
			prestamoDoc, errTx := tx.Get(prestamoRef)
			if errTx != nil {
				return errTx
			}
			var prestamo Prestamo
			if errTx = prestamoDoc.DataTo(&prestamo); errTx != nil {
				return errTx
			}

			libroRef := FirestoreClient.Collection("libro").Doc(libroID)
			libroDoc, errTx := tx.Get(libroRef)
			if errTx != nil {
				return errTx
			}
			var libro Libro
			if errTx = libroDoc.DataTo(&libro); errTx != nil {
				return errTx
			}

			// --- 2. Ahora hacer las escrituras que dependan de esos datos ---
			// Eliminar el préstamo
			tx.Delete(prestamoRef)
			// Calcular nuevas copias
			nuevasCopias := libro.Copias + 1
			updates := []firestore.Update{{Path: "copias", Value: nuevasCopias}}
			if !libro.Disponible && nuevasCopias > 0 {
				updates = append(updates,
					firestore.Update{Path: "disponible", Value: true},
					firestore.Update{Path: "prestadoPorID", Value: ""},
				)
			}
			tx.Update(libroRef, updates)

			return nil
		})

		if err != nil {
			log.Printf("🔥 ERROR transacción devolución: %v", err)
			if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Error al procesar devolución"))
				return
			}
			http.Redirect(w, r, "/devoluciones?msg=Error al procesar devolución&msg_type=danger", http.StatusSeeOther)
			return
		}

		// Transacción OK
		if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/devoluciones?msg=Devolución exitosa&msg_type=success", http.StatusSeeOther)
	}
}

func RegistrarHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		renderTemplate(w, r, "registrar.html", nil)
		return
	}

	nombre := r.FormValue("nombre")
	cedula := r.FormValue("cedula")
	anoStr := r.FormValue("ano")
	contrasena := r.FormValue("contrasena")
	rol := "usuario" // Rol por defecto para nuevos registros

	if nombre == "" || cedula == "" || anoStr == "" || contrasena == "" {
		http.Error(w, "Todos los campos son obligatorios", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	// Verificar si el usuario ya existe por cédula o nombre (opcional)
	iter := FirestoreClient.Collection("persona").Where("cedula", "==", cedula).Documents(ctx)
	doc, errInner := iter.Next()       // Renombrado 'err' a 'errInner'
	if errInner == nil && doc != nil { // Usar errInner
		http.Error(w, "Ya existe un usuario con esa cédula.", http.StatusConflict)
		return
	}

	// Convertir anoStr a int antes de guardar
	ano, errAno := strconv.Atoi(anoStr)
	if errAno != nil {
		log.Printf("Error al convertir año '%s' a int: %v", anoStr, errAno)
		http.Error(w, "Año de nacimiento inválido", http.StatusBadRequest)
		return
	}

	// Crear nuevo documento de persona
	personaDoc := map[string]interface{}{
		"nombre":     nombre,
		"cedula":     cedula,
		"ano":        ano,        // CAMBIADO: Guardar como int
		"contrasena": contrasena, // En un entorno real, la contraseña debería ser hasheada
		"rol":        rol,
	}

	_, _, errInner = FirestoreClient.Collection("persona").Add(ctx, personaDoc) // Renombrado 'err' a 'errInner'
	if errInner != nil {                                                        // Usar errInner
		log.Printf("Error al registrar persona en Firestore: %v", errInner)
		http.Error(w, "Error al registrar usuario", http.StatusInternalServerError)
		return
	}

	log.Println("✅ Usuario registrado:", nombre)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		tmpl, err := template.ParseFiles("templates/base.html", "templates/login.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.ExecuteTemplate(w, "base", nil)
		return
	}

	if r.Method == http.MethodPost {
		nombre := r.FormValue("nombre")
		contrasena := r.FormValue("contrasena")

		if nombre == "" || contrasena == "" {
			http.Error(w, "Campos requeridos", http.StatusBadRequest)
			return
		}

		// *** ESTA ES LA PARTE QUE CONSULTA FIRESTORE PARA EL LOGIN ***
		iter := FirestoreClient.Collection("persona").
			Where("nombre", "==", nombre).
			Where("contrasena", "==", contrasena). // En un entorno real, comparar hash de contraseñas
			Documents(r.Context())

		doc, errInner := iter.Next() // Renombrado 'err' a 'errInner'
		if errInner != nil {         // Usar errInner
			// Si no se encuentra un documento o hay otro error, las credenciales son incorrectas
			http.Error(w, "Credenciales incorrectas", http.StatusUnauthorized)
			return
		}

		rol := "usuario" // Rol por defecto
		if rdoc := doc.Data()["rol"]; rdoc != nil {
			if val, ok := rdoc.(string); ok {
				rol = val
			}
		}

		http.SetCookie(w, &http.Cookie{Name: "usuario", Value: nombre, Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "rol", Value: rol, Path: "/"})

		log.Println("✅ Sesión iniciada:", nombre, "| Rol:", rol)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "usuario", Value: "", Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "rol", Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// PrestamoHandler maneja la visualización del formulario de préstamo y el procesamiento de envíos.
func PrestamoHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	usuario := ""
	rol := ""
	if c, errCookie := r.Cookie("usuario"); errCookie == nil { // Renombrado err a errCookie
		usuario = c.Value
	}
	if c, errCookie := r.Cookie("rol"); errCookie == nil { // Renombrado err a errCookie
		rol = c.Value
	}

	// Obtener mensajes de la URL (si existen)
	mensaje := r.URL.Query().Get("msg")
	tipoMensaje := r.URL.Query().Get("msg_type")

	if r.Method == http.MethodGet {
		// --- Lógica para el método GET: Cargar todos los libros y el usuario logueado ---
		var allLibros []Libro
		iterLibros := FirestoreClient.Collection("libro").Documents(ctx) // Obtener todos los libros
		for {
			doc, errLoop := iterLibros.Next() // Usar := para declarar una nueva 'errLoop' en el ámbito del bucle
			if errLoop == iterator.Done {
				break
			}
			if errLoop != nil {
				log.Printf("Error al iterar libros: %v", errLoop)
				http.Error(w, "Error al cargar libros", http.StatusInternalServerError)
				return
			}
			var libro Libro
			if errLoop := doc.DataTo(&libro); errLoop != nil { // Renombrado err a errLoop
				log.Printf("Error al mapear datos de libro: %v", errLoop)
				continue
			}
			libro.ID = doc.Ref.ID
			allLibros = append(allLibros, libro)
		}

		// No necesitamos cargar todas las personas para el GET, ya que el usuario es autodetectado.
		// Sin embargo, mantenemos DatosPagina.Personas como slice vacío o nil si no se usa.
		data := DatosPagina{
			LibrosDisponibles: allLibros,   // Ahora pasamos todos los libros aquí para la selección
			Personas:          []Persona{}, // Ya no necesitamos la lista completa de personas para el select
			Año:               time.Now().Year(),
			Usuario:           usuario, // Se pasa el nombre del usuario logueado
			Rol:               rol,
			Mensaje:           mensaje,
			TipoMensaje:       tipoMensaje,
		}
		renderTemplate(w, r, "prestamos.html", data)
		return
	}

	if r.Method == http.MethodPost {
		// --- Lógica para el método POST: Registrar el préstamo ---
		libroID := r.FormValue("libroID")
		// La fecha y el usuario se obtienen internamente, no del formulario
		fechaPrestamo := time.Now() // Obtener la fecha actual directamente en el backend

		if libroID == "" { // Solo validar que el libroID no esté vacío
			http.Redirect(w, r, "/prestamos?msg=Seleccione un libro para el préstamo&msg_type=danger", http.StatusBadRequest)
			return
		}

		// Obtener el ID de la persona logueada
		var personaID string
		if usuario == "" {
			http.Redirect(w, r, "/login?msg="+url.QueryEscape("Debes iniciar sesión para registrar un préstamo")+"&msg_type=danger", http.StatusSeeOther)
			return
		}

		// Buscar el ID de la persona en Firestore basado en el nombre de usuario de la cookie
		iterPersona := FirestoreClient.Collection("persona").Where("nombre", "==", usuario).Documents(ctx)
		personaDoc, errInner := iterPersona.Next() // Renombrado err a errInner
		if errInner != nil {                       // Usar errInner
			log.Printf("DEBUG Prestamo POST: Error al buscar persona '%s': %v", usuario, errInner)
			http.Redirect(w, r, "/prestamos?msg="+url.QueryEscape("No se encontró tu usuario en la base de datos.")+"&msg_type=danger", http.StatusSeeOther)
			return
		}
		personaID = personaDoc.Ref.ID
		log.Printf("DEBUG Prestamo POST: Usuario logueado: %s (ID: %s)", usuario, personaID)

		// Iniciar una transacción de Firestore
		err := FirestoreClient.RunTransaction(ctx, func(ctx_tx context.Context, tx *firestore.Transaction) error {
			// 1. Obtener el libro para verificar disponibilidad
			libroRef := FirestoreClient.Collection("libro").Doc(libroID)
			libroDoc, errTx := tx.Get(libroRef) // Renombrado err a errTx
			if errTx != nil {
				log.Printf("DEBUG Prestamo POST: Error al obtener libro %s: %v", libroID, errTx)
				return errTx // Libro no encontrado o error de Firestore
			}
			var libro Libro
			if errTx := libroDoc.DataTo(&libro); errTx != nil { // Renombrado err a errTx
				log.Printf("DEBUG Prestamo POST: Error al mapear datos de libro %s: %v", libroID, errTx)
				return errTx // Error al mapear datos del libro
			}
			// La lógica de disponibilidad se basa en `Copias`
			if libro.Copias <= 0 { // Verificar si hay copias disponibles
				log.Printf("DEBUG Prestamo POST: Libro %s no disponible (copias: %d).", libroID, libro.Copias)
				return &http.ProtocolError{ErrorString: "El libro no está disponible para préstamo o no quedan copias."}
			}
			log.Printf("DEBUG Prestamo POST: Libro %s disponible (copias: %d).", libroID, libro.Copias)

			// 2. Crear el nuevo documento de préstamo
			nuevoPrestamoRef := FirestoreClient.Collection("prestamos").NewDoc()
			nuevoPrestamo := Prestamo{
				LibroID:       libroID,
				PersonaID:     personaID, // ¡Este es el campo crucial que se guarda!
				FechaPrestamo: fechaPrestamo,
				Activo:        true,
			}
			errTx = tx.Create(nuevoPrestamoRef, nuevoPrestamo) // Renombrado err a errTx
			if errTx != nil {
				log.Printf("DEBUG Prestamo POST: Error al crear el préstamo en Firestore: %v", errTx)
				return errTx // Error al crear el préstamo
			}
			log.Printf("DEBUG Prestamo POST: Préstamo creado en Firestore para libro %s y persona %s", libroID, personaID)

			// 3. Actualizar el libro: reducir copias y marcar como no disponible (si las copias llegan a 0)
			nuevasCopias := libro.Copias - 1
			actualizacionesLibro := []firestore.Update{
				{Path: "copias", Value: nuevasCopias},
			}
			if nuevasCopias <= 0 {
				actualizacionesLibro = append(actualizacionesLibro, firestore.Update{Path: "disponible", Value: false})
				actualizacionesLibro = append(actualizacionesLibro, firestore.Update{Path: "prestadoPorID", Value: personaID}) // Limpiar quien lo tiene prestado
			}
			tx.Update(libroRef, actualizacionesLibro)
			log.Printf("DEBUG Prestamo POST: Libro %s copias actualizadas a %d.", libroID, nuevasCopias)

			return nil // Transacción exitosa
		})

		if err != nil {
			log.Printf("Error en transacción de préstamo: %v", err)
			if protoErr, ok := err.(*http.ProtocolError); ok {
				http.Redirect(w, r, "/prestamos?msg="+url.QueryEscape(protoErr.Error())+"&msg_type=danger", http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/prestamos?msg=Error al registrar el préstamo&msg_type=danger", http.StatusSeeOther)
			return
		}

		log.Printf("✅ Préstamo registrado exitosamente: LibroID '%s', PersonaID '%s'", libroID, personaID)
		http.Redirect(w, r, "/prestamos?msg=Préstamo registrado exitosamente&msg_type=success", http.StatusSeeOther)
		return
	}
}

func RegistrarLibroHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		usuario := ""
		rol := ""
		if c, errCookie := r.Cookie("usuario"); errCookie == nil { // Renombrado err a errCookie
			usuario = c.Value
		}
		if c, errCookie := r.Cookie("rol"); errCookie == nil { // Renombrado err a errCookie
			rol = c.Value
		}

		data := DatosPagina{
			Año:     time.Now().Year(),
			Usuario: usuario,
			Rol:     rol,
		}
		renderTemplate(w, r, "registrar_libro.html", data)
		return
	}

	if r.Method == http.MethodPost {
		nombre := r.FormValue("nombre")
		autor := r.FormValue("autor")
		descripcion := r.FormValue("descripcion")
		imagen := r.FormValue("imagen")
		anoStr := r.FormValue("ano")
		copiasStr := r.FormValue("copias")

		ano, errInner := strconv.Atoi(anoStr) // Renombrado err a errInner
		if errInner != nil {                  // Usar errInner
			http.Error(w, "Año inválido", http.StatusBadRequest)
			return
		}
		copias, errInner := strconv.Atoi(copiasStr) // Renombrado err a errInner
		if errInner != nil {                        // Usar errInner
			http.Error(w, "Número de copias inválido", http.StatusBadRequest)
			return
		}

		// Al registrar un libro, inicialmente está disponible
		doc := Libro{
			Nombre:      nombre,
			Autor:       autor,
			Descripcion: descripcion,
			Ano:         ano,
			ImagenURL:   imagen,
			Copias:      copias,
			Disponible:  true, // Nuevo libro, por defecto disponible
		}

		_, _, errInner = FirestoreClient.Collection("libro").Add(r.Context(), doc) // Renombrado 'err' a 'errInner'
		if errInner != nil {                                                       // Usar errInner
			http.Error(w, "Error al registrar libro", http.StatusInternalServerError)
			log.Println("Error Firestore libro:", errInner)
			return
		}

		log.Println("✅ Libro registrado:", nombre)
		// Redirige a la página de libros con un parámetro de éxito
		http.Redirect(w, r, "/libros?msg=Libro registrado exitosamente&msg_type=success", http.StatusSeeOther)
		return // Asegúrate de retornar después de la redirección
	}
}

func PersonasHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	usuario := ""
	rol := ""
	if c, errCookie := r.Cookie("usuario"); errCookie == nil { // Renombrado err a errCookie
		usuario = c.Value
	}
	if c, errCookie := r.Cookie("rol"); errCookie == nil { // Renombrado err a errCookie
		rol = c.Value
	}

	var personas []Persona
	iter := FirestoreClient.Collection("persona").Documents(ctx)
	for {
		doc, errLoop := iter.Next() // Renombrado err a errLoop
		if errLoop == iterator.Done {
			break
		}
		if errLoop != nil {
			log.Printf("Error al iterar personas: %v", errLoop)
			http.Error(w, "Error al cargar personas", http.StatusInternalServerError)
			return
		}

		// Mapeo manual para manejar inconsistencias de tipo
		var persona Persona
		persona.ID = doc.Ref.ID
		data := doc.Data()

		if nombre, ok := data["nombre"].(string); ok {
			persona.Nombre = nombre
		} else {
			log.Printf("Advertencia: Tipo inesperado para 'nombre' en persona %s: %T", persona.ID, data["nombre"])
		}

		if cedula, ok := data["cedula"].(string); ok {
			persona.Cedula = cedula
		} else if cedulaFloat, ok := data["cedula"].(float64); ok {
			// Si es un número, convertir a string (ej. 1234567890.0 -> "1234567890")
			persona.Cedula = strconv.FormatFloat(cedulaFloat, 'f', 0, 64)
			log.Printf("DEBUG: Convertida cédula float a string para persona %s: %s", persona.ID, persona.Cedula)
		} else {
			log.Printf("Advertencia: Tipo inesperado para 'cedula' en persona %s: %T", persona.ID, data["cedula"])
			persona.Cedula = "" // Valor por defecto
		}

		if anoFloat, ok := data["ano"].(float64); ok { // Firestore devuelve números como float64
			persona.Ano = int(anoFloat)
		} else if anoStr, ok := data["ano"].(string); ok { // Si es un string, intentar convertir a int
			parsedAno, errParseAno := strconv.Atoi(anoStr)
			if errParseAno == nil {
				persona.Ano = parsedAno
			} else {
				log.Printf("Advertencia: No se pudo convertir 'ano' '%s' a int para persona %s: %v", anoStr, persona.ID, errParseAno)
				persona.Ano = 0 // Valor por defecto o manejar como error
			}
		} else {
			log.Printf("Advertencia: Tipo inesperado para 'ano' en persona %s: %T", persona.ID, data["ano"])
			persona.Ano = 0 // Valor por defecto
		}

		if contrasena, ok := data["contrasena"].(string); ok {
			persona.Contrasena = contrasena
		} else {
			log.Printf("Advertencia: Tipo inesperado para 'contrasena' en persona %s: %T", persona.ID, data["contrasena"])
		}

		if rolData, ok := data["rol"].(string); ok {
			persona.Rol = rolData
		} else {
			log.Printf("Advertencia: Tipo inesperado para 'rol' en persona %s: %T", persona.ID, data["rol"])
		}

		personas = append(personas, persona)
	}

	data := DatosPagina{
		Personas: personas,
		Año:      time.Now().Year(),
		Usuario:  usuario,
		Rol:      rol,
	}
	renderTemplate(w, r, "personas.html", data)
}

func EliminarPersonaHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("DEBUG: Entrando a EliminarPersonaHandler")

	// Verificar rol
	rol := ""
	if c, errCookie := r.Cookie("rol"); errCookie == nil {
		rol = c.Value
	}
	log.Printf("Rol detectado: %s", rol)
	if rol != "admin" {
		http.Error(w, "Acceso denegado", http.StatusForbidden)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	personID := r.FormValue("id")
	log.Printf("ID recibido para eliminar: %s", personID)
	if personID == "" {
		http.Error(w, "ID de persona no proporcionado", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	_, err := FirestoreClient.Collection("persona").Doc(personID).Delete(ctx)
	if err != nil {
		log.Printf("🔥 Error al eliminar persona con ID %s: %v", personID, err)
		http.Error(w, "Error al eliminar persona: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Persona eliminada correctamente: %s", personID)

	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Eliminado"))
		return
	}

	http.Redirect(w, r, "/personas", http.StatusSeeOther)
}
