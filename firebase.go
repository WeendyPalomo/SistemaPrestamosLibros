package main

import (
	"context"
	"log"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

var FirestoreClient *firestore.Client

func InitFirebase() {
	ctx := context.Background()

	// Ruta al archivo de credenciales JSON
	sa := option.WithCredentialsFile("prestamolibros-556f1-firebase-adminsdk-fbsvc-1bc548a5b5.json")

	// Configuración explícita con el project ID
	config := &firebase.Config{
		ProjectID: "prestamolibros-556f1",
	}

	// Inicializar la app con configuración y credenciales
	app, err := firebase.NewApp(ctx, config, sa)
	if err != nil {
		log.Fatalf("Error inicializando Firebase: %v", err)
	}

	// Inicializar cliente Firestore
	client, err := app.Firestore(ctx)
	if err != nil {
		log.Fatalf("Error inicializando Firestore: %v", err)
	}

	FirestoreClient = client
	log.Println("✅ Conexión con Firebase Firestore exitosa")
}
