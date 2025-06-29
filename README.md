# 📚 Sistema de Préstamo de Libros – Biblioteca PUCE

Este proyecto es un sistema web desarrollado en **Go (Golang)** que permite gestionar una biblioteca, incluyendo **registro de libros**, **préstamos**, **devoluciones** y **usuarios con roles (admin / usuario)**.

---

## 🚀 Funcionalidades principales

- 📖 Registro y edición de libros
- 👥 Registro de usuarios con validación de rol
- 🔐 Inicio de sesión con control de acceso
- 📦 Préstamo de libros (disminuye copias)
- 🔄 Devolución de libros (aumenta copias)
- 🔍 Listado de libros disponibles
- 🧾 Listado de usuarios y eliminación (solo admins)
- 📊 Pruebas de rendimiento (K6)
- 🧪 Pruebas unitarias con `net/http/httptest`

---

## 🛠️ Tecnologías usadas

- **Lenguaje:** Go (Golang)
- **Base de datos:** Firebase Firestore
- **Frontend:** HTML + Bootstrap
- **Routing y templates:** Go HTML templates
- **Autenticación:** Cookies
- **Pruebas:** K6 (estrés) + `testing handlers` (unitarias)

## 🧑‍💻 Instalación y ejecución

1. **Clona el proyecto**
```bash
git clone https://github.com/WeendyPalomo/SistemaPrestamosLibros.git
cd SistemaPrestamosLibros```

2. **Ejecuta la aplicación**
```fresh```

3. **Accede desde tu navegador en:**
👉 http://localhost:3000

