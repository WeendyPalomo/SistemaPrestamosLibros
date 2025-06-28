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
	Ano        int    `json:"ano" firestore:"ano"`
	Contrasena string `json:"-" firestore:"contrasena"` // Ignorar en JSON, no almacenar en el cliente
	Rol        string `json:"rol" firestore:"rol"`
}

// Definición de la estructura Prestamo
type Prestamo struct {
	ID              string    `json:"id" firestore:"id,omitempty"`
	LibroID         string    `json:"libroID" firestore:"libroID"`                                     // ID del libro prestado
	PersonaID       string    `json:"personaID" firestore:"personaID"`                                 // ID de la persona que lo prestó
	FechaPrestamo   time.Time `json:"fechaPrestamo" firestore:"fechaPrestamo"`                         // Fecha en que se realizó el préstamo
	FechaDevolucion time.Time `json:"fechaDevolucion,omitempty" firestore:"fechaDevolucion,omitempty"` // Fecha de devolución (opcional, se llena al devolver)
	Activo          bool      `json:"activo" firestore:"activo"`                                       // true si el préstamo está activo, false si ya se devolvió
}

// Definición de la estructura DatosPagina
type DatosPagina struct {
	Libros            []Libro
	LibrosDisponibles []Libro    // Para el formulario de préstamo
	Personas          []Persona  // Para el formulario de préstamo (ahora solo para referencia, no para selección)
	Prestamos         []Prestamo // Para listar préstamos
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
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Error al iterar libros: %v", err)
			http.Error(w, "Error al cargar libros", http.StatusInternalServerError)
			return
		}
		var libro Libro
		if err := doc.DataTo(&libro); err != nil {
			log.Printf("Error al mapear datos de libro: %v", err)
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
	if c, err := r.Cookie("usuario"); err == nil {
		usuario = c.Value
	}
	if c, err := r.Cookie("rol"); err == nil {
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
	if c, err := r.Cookie("rol"); err == nil {
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
		libro.ID = doc.Ref.ID
		log.Printf("DEBUG: Datos del libro parseados para edición: %+v", libro) // Log con datos parseados

		usuario := ""
		if c, err := r.Cookie("usuario"); err == nil {
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

		log.Printf("DEBUG POST: ID del libro recibido: %s", bookID)
		log.Printf("DEBUG POST: Nombre: %s, Autor: %s, Año: %s, Copias: %s, Disponible: %s", nombre, autor, anoStr, copiasStr, disponibleStr)

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
	if c, err := r.Cookie("rol"); err == nil {
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
	_, err := FirestoreClient.Collection("libro").Doc(bookID).Delete(ctx)
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
	if r.Method == http.MethodGet {
		usuario := ""
		rol := ""
		if c, err := r.Cookie("usuario"); err == nil {
			usuario = c.Value
		}
		if c, err := r.Cookie("rol"); err == nil {
			rol = c.Value
		}

		data := DatosPagina{
			Año:     time.Now().Year(),
			Usuario: usuario,
			Rol:     rol,
		}
		renderTemplate(w, r, "devoluciones.html", data)
		return
	}

	usuario := r.FormValue("usuario")
	libro := r.FormValue("libro")
	fecha := r.FormValue("fecha")

	if usuario == "" || libro == "" || fecha == "" {
		http.Error(w, "Todos los campos son obligatorios", http.StatusBadRequest)
		return
	}

	log.Printf("✅ Devolución registrada (simulada): Usuario '%s' devolvió el libro '%s'", usuario, libro)

	http.Redirect(w, r, "/", http.StatusSeeOther)
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

	ano, err := strconv.Atoi(anoStr)
	if err != nil {
		http.Error(w, "Año de nacimiento inválido", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	// Verificar si el usuario ya existe por cédula o nombre (opcional)
	iter := FirestoreClient.Collection("persona").Where("cedula", "==", cedula).Documents(ctx)
	doc, err := iter.Next()
	if err == nil && doc != nil {
		http.Error(w, "Ya existe un usuario con esa cédula.", http.StatusConflict)
		return
	}

	// Crear nuevo documento de persona
	personaDoc := map[string]interface{}{
		"nombre":     nombre,
		"cedula":     cedula,
		"ano":        ano,
		"contrasena": contrasena, // En un entorno real, la contraseña debería ser hasheada
		"rol":        rol,
	}

	_, _, err = FirestoreClient.Collection("persona").Add(ctx, personaDoc)
	if err != nil {
		log.Printf("Error al registrar persona en Firestore: %v", err)
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

		doc, err := iter.Next()
		if err != nil {
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
	usuarioNombre := "" // Cambiado para evitar confusión con el campo PersonaID
	rol := ""
	if c, err := r.Cookie("usuario"); err == nil {
		usuarioNombre = c.Value
	}
	if c, err := r.Cookie("rol"); err == nil {
		rol = c.Value
	}

	// Obtener mensajes de la URL (si existen)
	mensaje := r.URL.Query().Get("msg")
	tipoMensaje := r.URL.Query().Get("msg_type")

	if r.Method == http.MethodGet {
		// --- Lógica para el método GET: Cargar TODOS los libros y el usuario logueado ---
		var allLibros []Libro
		iterLibros := FirestoreClient.Collection("libro").Documents(ctx) // Obtener todos los libros
		for {
			doc, err := iterLibros.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				log.Printf("Error al iterar libros: %v", err)
				http.Error(w, "Error al cargar libros", http.StatusInternalServerError)
				return
			}
			var libro Libro
			if err := doc.DataTo(&libro); err != nil {
				log.Printf("Error al mapear datos de libro: %v", err)
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
			Usuario:           usuarioNombre, // Se pasa el nombre del usuario logueado
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
		if usuarioNombre == "" {
			http.Redirect(w, r, "/login?msg="+url.QueryEscape("Debes iniciar sesión para registrar un préstamo")+"&msg_type=danger", http.StatusSeeOther)
			return
		}

		// Buscar el ID de la persona en Firestore basado en el nombre de usuario de la cookie
		iterPersona := FirestoreClient.Collection("persona").Where("nombre", "==", usuarioNombre).Documents(ctx)
		personaDoc, err := iterPersona.Next()
		if err != nil {
			log.Printf("Error al buscar persona '%s': %v", usuarioNombre, err)
			http.Redirect(w, r, "/prestamos?msg="+url.QueryEscape("No se encontró tu usuario en la base de datos.")+"&msg_type=danger", http.StatusSeeOther)
			return
		}
		personaID = personaDoc.Ref.ID

		// Iniciar una transacción de Firestore
		err = FirestoreClient.RunTransaction(ctx, func(ctx_tx context.Context, tx *firestore.Transaction) error {
			// 1. Obtener el libro para verificar disponibilidad
			libroRef := FirestoreClient.Collection("libro").Doc(libroID)
			libroDoc, err := tx.Get(libroRef)
			if err != nil {
				return err // Libro no encontrado o error de Firestore
			}
			var libro Libro
			if err := libroDoc.DataTo(&libro); err != nil {
				return err // Error al mapear datos del libro
			}
			// La lógica de disponibilidad se basa en `Copias`
			if libro.Copias <= 0 { // Verificar si hay copias disponibles
				return &http.ProtocolError{ErrorString: "El libro no está disponible para préstamo o no quedan copias."}
			}

			// 2. Crear el nuevo documento de préstamo
			nuevoPrestamo := Prestamo{ // Declarar nuevoPrestamo aquí
				LibroID:       libroID,
				PersonaID:     personaID,     // Usar el ID de la persona logueada
				FechaPrestamo: fechaPrestamo, // Usar la fecha actual del backend
				Activo:        true,          // El préstamo está activo
			}
			// Corregido: tx.Create solo devuelve un error, no un DocumentRef
			err = tx.Create(FirestoreClient.Collection("prestamos").NewDoc(), nuevoPrestamo)
			if err != nil {
				return err // Error al crear el préstamo
			}

			// 3. Actualizar el libro: reducir copias y marcar como no disponible (si las copias llegan a 0)
			nuevasCopias := libro.Copias - 1
			actualizacionesLibro := []firestore.Update{
				{Path: "copias", Value: nuevasCopias},
			}
			// Solo marcar como no disponible si las copias llegan a 0.
			// Si hay más copias, el libro sigue siendo "disponible" para otros préstamos.
			if nuevasCopias <= 0 {
				actualizacionesLibro = append(actualizacionesLibro, firestore.Update{Path: "disponible", Value: false})
				actualizacionesLibro = append(actualizacionesLibro, firestore.Update{Path: "prestadoPorID", Value: personaID}) // Asignar al último que lo prestó
			}

			tx.Update(libroRef, actualizacionesLibro)

			return nil // Transacción exitosa
		})

		if err != nil {
			log.Printf("Error en transacción de préstamo: %v", err)
			// Manejar errores específicos de la transacción
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
		if c, err := r.Cookie("usuario"); err == nil {
			usuario = c.Value
		}
		if c, err := r.Cookie("rol"); err == nil {
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

		ano, err := strconv.Atoi(anoStr)
		if err != nil {
			http.Error(w, "Año inválido", http.StatusBadRequest)
			return
		}
		copias, err := strconv.Atoi(copiasStr)
		if err != nil {
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

		_, _, err = FirestoreClient.Collection("libro").Add(r.Context(), doc)
		if err != nil {
			http.Error(w, "Error al registrar libro", http.StatusInternalServerError)
			log.Println("Error Firestore libro:", err)
			return
		}

		log.Println("✅ Libro registrado:", nombre)
		// Redirige a la página de libros con un parámetro de éxito
		http.Redirect(w, r, "/libros?msg=Libro registrado exitosamente&msg_type=success", http.StatusSeeOther)
		return // Asegúrate de retornar después de la redirección
	}
}

// PersonasHandler fetches and displays the list of persons.
func PersonasHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	usuario := ""
	rol := ""
	if c, err := r.Cookie("usuario"); err == nil {
		usuario = c.Value
	}
	if c, err := r.Cookie("rol"); err == nil {
		rol = c.Value
	}

	var personas []Persona
	iter := FirestoreClient.Collection("persona").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Error al iterar personas: %v", err)
			http.Error(w, "Error al cargar personas", http.StatusInternalServerError)
			return
		}
		var persona Persona
		if err := doc.DataTo(&persona); err != nil {
			log.Printf("Error al mapear datos de persona: %v", err)
			continue
		}
		persona.ID = doc.Ref.ID
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

// EditarPersonaHandler handles displaying the edit form and processing updates for a person.
func EditarPersonaHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("DEBUG: Entrando a EditarPersonaHandler")
	rol := ""
	if c, err := r.Cookie("rol"); err == nil {
		rol = c.Value
	}
	if rol != "admin" {
		http.Error(w, "Acceso denegado. Solo administradores pueden editar usuarios.", http.StatusForbidden)
		return
	}

	if r.Method == http.MethodGet {
		personID := r.URL.Query().Get("id")
		if personID == "" {
			http.Error(w, "ID de persona no proporcionado", http.StatusBadRequest)
			return
		}

		ctx := context.Background()
		doc, err := FirestoreClient.Collection("persona").Doc(personID).Get(ctx)
		if err != nil {
			http.Error(w, "Persona no encontrada: "+err.Error(), http.StatusNotFound)
			return
		}

		var persona Persona
		if err := doc.DataTo(&persona); err != nil {
			http.Error(w, "Error al parsear datos de la persona: "+err.Error(), http.StatusInternalServerError)
			return
		}
		persona.ID = doc.Ref.ID

		usuarioCookie := ""
		if c, err := r.Cookie("usuario"); err == nil {
			usuarioCookie = c.Value
		}

		data := DatosPagina{
			Detalle: &Libro{ // Usamos Detalle de Libro temporalmente para pasar la persona, idealmente sería un DetallePersona
				ID:          persona.ID,
				Nombre:      persona.Nombre,
				Ano:         persona.Ano,
				Descripcion: persona.Cedula, // Usamos Descripcion para la cédula
				Autor:       persona.Rol,    // Usamos Autor para el rol
			},
			Año:     time.Now().Year(),
			Usuario: usuarioCookie,
			Rol:     rol,
		}
		renderTemplate(w, r, "editar_persona.html", data) // Necesitarás crear editar_persona.html
		return
	}

	if r.Method == http.MethodPost {
		personID := r.FormValue("id")
		nombre := r.FormValue("nombre")
		cedula := r.FormValue("cedula")
		anoStr := r.FormValue("ano")
		rol := r.FormValue("rol") // Permitir editar el rol

		ano, err := strconv.Atoi(anoStr)
		if err != nil {
			http.Redirect(w, r, "/personas?msg=Año de nacimiento inválido&msg_type=danger", http.StatusSeeOther)
			return
		}

		updates := []firestore.Update{
			{Path: "nombre", Value: nombre},
			{Path: "cedula", Value: cedula},
			{Path: "ano", Value: ano},
			{Path: "rol", Value: rol},
		}

		ctx := context.Background()
		_, err = FirestoreClient.Collection("persona").Doc(personID).Update(ctx, updates)
		if err != nil {
			log.Printf("Error al actualizar persona %s en Firestore: %v", personID, err)
			http.Redirect(w, r, "/personas?msg=Error al actualizar el usuario&msg_type=danger", http.StatusSeeOther)
			return
		}

		log.Printf("✅ Usuario actualizado exitosamente: %s (ID: %s)", nombre, personID)
		http.Redirect(w, r, "/personas?msg=Usuario actualizado exitosamente&msg_type=success", http.StatusSeeOther)
	}
}

// EliminarPersonaHandler handles deleting a person.
func EliminarPersonaHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("DEBUG: Entrando a EliminarPersonaHandler")
	rol := ""
	if c, err := r.Cookie("rol"); err == nil {
		rol = c.Value
	}
	if rol != "admin" {
		http.Error(w, "Acceso denegado. Solo administradores pueden eliminar usuarios.", http.StatusForbidden)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	personID := r.FormValue("id")
	if personID == "" {
		http.Error(w, "ID de persona no proporcionado", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	_, err := FirestoreClient.Collection("persona").Doc(personID).Delete(ctx)
	if err != nil {
		log.Printf("Error al eliminar persona %s de Firestore: %v", personID, err)
		http.Error(w, "Error al eliminar persona: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Persona eliminada exitosamente: (ID: %s)", personID)
	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/personas", http.StatusSeeOther)
}
