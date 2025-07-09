# 📚 Sistema de Préstamos de Libros

Este proyecto es una aplicación web desarrollada en Go (Golang) que permite gestionar libros, préstamos, devoluciones y usuarios dentro de una biblioteca académica. Utiliza Firebase Firestore como base de datos y HTML con Bootstrap para la interfaz web. Está pensado para ejecutarse tanto localmente como en entornos de despliegue como Render.

## 🚀 Funcionalidades

- Registro y autenticación de usuarios
- Roles de usuario: administrador y usuario regular
- Registro, edición y eliminación de libros (solo admin)
- Préstamo y devolución de libros
- Búsqueda de libros en tiempo real
- Control de disponibilidad por número de copias
- Gestión de personas (usuarios registrados)

## 🛠️ Tecnologías utilizadas

- **Backend:** Go (Golang)
- **Frontend:** HTML, Bootstrap 5, JavaScript
- **Base de datos:** Firebase Firestore
- **Plantillas:** `html/template`
- **Despliegue:** Render.com

## 🔐 Configuración de Firebase en Render

Para conectar correctamente Firebase Firestore en Render, es necesario usar una variable de entorno que contenga las credenciales del service account:

1. Desde Firebase, genera un archivo JSON de tipo `Admin SDK`.
2. Copia todo su contenido.
3. En el panel de Render, ve a:

Environment > Environment Variables

4. Agrega una variable con:
- **Name:** `GOOGLE_APPLICATION_CREDENTIALS_JSON`
- **Value:** (pega el contenido del JSON sin saltos de línea)

> Render cargará automáticamente esta variable y tu app podrá autenticar con Firestore.

## 🧪 Pruebas de rendimiento

Se han realizado pruebas de carga con [k6](https://k6.io/) para medir el tiempo de respuesta de la ruta `/libros` con múltiples usuarios concurrentes.  
Resultados: tiempo promedio ≈ 199ms, 0% de fallos, 1011 iteraciones completadas en 1 minuto.

## 📦 Estructura del proyecto
├── main.go # Punto de entrada de la aplicación
├── handlers.go # Lógica principal y controladores
├── firebase.go # Conexión a Firebase Firestore
├── templates/ # Archivos HTML base + vistas
├── static/ # Archivos estáticos (CSS, JS, imágenes)
├── db.go # Archivo base para conexión (si se amplía a SQL)
├── test/ # Scripts de prueba (ej. test_libros.js para k6)
├── go.mod / go.sum # Dependencias del proyecto


## 📄 Licencia

Este proyecto es académico y libre de uso con fines educativos.
