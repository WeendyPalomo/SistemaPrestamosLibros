package main

import (
	"html/template"
	"log"
	"net/http"
	"time"
)

// La definición de Libro y DatosPagina se ha movido a handlers.go
// para que sea accesible por todas las funciones que la utilizan.
// Si el proyecto crece, estas estructuras deberían ir en un archivo models.go.
// Se asume que DatosPagina está definida en handlers.go y es accesible.

func Index(w http.ResponseWriter, r *http.Request) {
	usuario := ""
	rol := ""
	if cookie, err := r.Cookie("usuario"); err == nil {
		usuario = cookie.Value
	}
	if cookie, err := r.Cookie("rol"); err == nil {
		rol = cookie.Value
	}

	data := DatosPagina{
		Libros:  nil,
		Detalle: nil,
		Año:     time.Now().Year(),
		Usuario: usuario,
		Rol:     rol,
	}

	tmpl, err := template.ParseFiles("templates/base.html", "templates/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println("Error en plantilla:", err)
		return
	}

	err = tmpl.ExecuteTemplate(w, "base", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println("Error al ejecutar template:", err)
	}
}

func main() {
	InitFirebase() // Asume que esta función inicializa FirestoreClient globalmente

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/", Index)
	http.HandleFunc("/registrar", RegistrarHandler)
	http.HandleFunc("/login", LoginHandler)
	http.HandleFunc("/logout", LogoutHandler)
	http.HandleFunc("/registrar-libro", RegistrarLibroHandler)
	http.HandleFunc("/libros", LibrosHandler)
	http.HandleFunc("/devoluciones", DevolucionesHandler)
	http.HandleFunc("/personas", PersonasHandler)
	http.HandleFunc("/prestamos", PrestamoHandler)
	http.HandleFunc("/editar-libros", EditarLibroHandler)
	http.HandleFunc("/eliminar-libro", EliminarLibroHandler)
	http.HandleFunc("/eliminar-persona", EliminarPersonaHandler)
	log.Println("Servidor corriendo en http://localhost:3000/")
	log.Fatal(http.ListenAndServe(":3000", nil))
}
