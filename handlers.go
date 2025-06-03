package main

import (
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// Definición de la estructura Libro
type Libro struct {
	ID          string `json:"id"`
	Nombre      string `json:"nombre"`
	Autor       string `json:"autor"`
	Ano         int    `json:"ano"`
	Descripcion string `json:"descripcion"`
	ImagenURL   string `json:"imagenURL"`
	Copias      int    `json:"copias"` // Nuevo campo para el número de copias
}

// Definición de la estructura DatosPagina
type DatosPagina struct {
	Libros      []Libro
	Detalle     *Libro
	Año         int
	Usuario     string
	Rol         string
	SearchQuery string
	Mensaje     string // Nuevo campo para mensajes de éxito/error
	TipoMensaje string // "success" o "danger"
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
		data := doc.Data()

		nombre, _ := data["nombre"].(string)
		autor, _ := data["autor"].(string)
		descripcion, _ := data["descripcion"].(string)
		imagen, _ := data["imagen"].(string)
		anoFloat, ok := data["ano"].(int64)
		ano := 0
		if ok {
			ano = int(anoFloat)
		}
		copiasFloat, ok := data["copias"].(int64) // Obtener copias
		copias := 0
		if ok {
			copias = int(copiasFloat)
		}

		allLibros = append(allLibros, Libro{
			ID:          doc.Ref.ID,
			Nombre:      nombre,
			Autor:       autor,
			Descripcion: descripcion,
			Ano:         ano,
			ImagenURL:   imagen,
			Copias:      copias, // Asignar copias
		})
	}

	// FIX: Cambiado de '[]Libros' a '[]Libro'
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

		log.Printf("DEBUG POST: ID del libro recibido: %s", bookID)
		log.Printf("DEBUG POST: Nombre: %s, Autor: %s, Año: %s, Copias: %s", nombre, autor, anoStr, copiasStr)

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

		updates := []firestore.Update{
			{Path: "nombre", Value: nombre},
			{Path: "autor", Value: autor},
			{Path: "descripcion", Value: descripcion},
			{Path: "imagen", Value: imagen},
			{Path: "ano", Value: ano},
			{Path: "copias", Value: copias},
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
		log.Println("DEBUG: Respondiendo 200 OK para petición AJAX de eliminación.")
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
	ano := r.FormValue("ano")
	contrasena := r.FormValue("contrasena")

	if nombre == "" || cedula == "" || ano == "" || contrasena == "" {
		http.Error(w, "Todos los campos son obligatorios", http.StatusBadRequest)
		return
	}

	// NOTA: Esta sección no usaba Firestore para 'persona' en este punto.
	// Si tu aplicación ya tenía una colección "persona", esto no la usaba para registro en esta versión.
	log.Println("✅ Usuario registrado (simulado):", nombre) // Esto era una simulación antes de integrar Firestore para personas.
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
			Where("contrasena", "==", contrasena).
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

func PrestamoHandler(w http.ResponseWriter, r *http.Request) {
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
		renderTemplate(w, r, "prestamos.html", data)
		return
	}

	usuario := r.FormValue("usuario")
	libro := r.FormValue("libro")
	fecha := r.FormValue("fecha")

	if usuario == "" || libro == "" || fecha == "" {
		http.Error(w, "Todos los campos son obligatorios", http.StatusBadRequest)
		return
	}

	log.Printf("✅ Préstamo registrado (simulado): Usuario '%s' devolvió el libro '%s'", usuario, libro)

	http.Redirect(w, r, "/", http.StatusSeeOther)
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

		doc := map[string]interface{}{
			"nombre":      nombre,
			"autor":       autor,
			"descripcion": descripcion,
			"ano":         ano,
			"imagen":      imagen,
			"copias":      copias,
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
