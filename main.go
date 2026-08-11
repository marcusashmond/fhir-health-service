package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/marcusashmond/fhir-health-service/internal/handlers"
	"github.com/marcusashmond/fhir-health-service/internal/repository"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: failed to load .env file: %v", err)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	r := chi.NewRouter()

	r.Get("/health", healthHandler)

	patientRepo := repository.NewPostgresPatientRepository(pool)
	observationRepo := repository.NewPostgresObservationRepository(pool)

	patientHandler := handlers.NewPatientHandler(patientRepo)
	observationHandler := handlers.NewObservationHandler(observationRepo, patientRepo)

	r.Route("/Patient", func(r chi.Router) {
		r.Post("/", patientHandler.Create)
		r.Get("/", patientHandler.List)
		r.Get("/{id}", patientHandler.GetByID)
		r.Put("/{id}", patientHandler.Update)
		r.Delete("/{id}", patientHandler.Delete)
		r.Get("/{id}/Observation", observationHandler.ListByPatient)
	})

	r.Route("/Observation", func(r chi.Router) {
		r.Post("/", observationHandler.Create)
		r.Get("/{id}", observationHandler.GetByID)
		r.Put("/{id}", observationHandler.Update)
		r.Delete("/{id}", observationHandler.Delete)
	})

	log.Println("starting server on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal(err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
