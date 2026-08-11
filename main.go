package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/marcusashmond/fhir-health-service/internal/handlers"
	"github.com/marcusashmond/fhir-health-service/internal/kafka"
	"github.com/marcusashmond/fhir-health-service/internal/middleware"
	"github.com/marcusashmond/fhir-health-service/internal/repository"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: failed to load .env file: %v", err)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	kafkaBroker := os.Getenv("KAFKA_BROKER")
	if kafkaBroker == "" {
		log.Fatal("KAFKA_BROKER environment variable is required")
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	producer := kafka.NewProducer(kafkaBroker)
	defer producer.Close()

	consumer := kafka.NewConsumer(kafkaBroker)
	defer consumer.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		consumer.Run(ctx)
	}()

	r := chi.NewRouter()

	r.Get("/health", healthHandler)

	patientRepo := repository.NewPostgresPatientRepository(pool)
	observationRepo := repository.NewPostgresObservationRepository(pool)
	auditLogRepo := repository.NewPostgresAuditLogRepository(pool)

	patientHandler := handlers.NewPatientHandler(patientRepo)
	observationHandler := handlers.NewObservationHandler(observationRepo, patientRepo, producer)
	auditLogHandler := handlers.NewAuditLogHandler(auditLogRepo)

	auditLogger := middleware.NewAuditLogger(auditLogRepo)
	defer auditLogger.Close()

	r.Get("/audit-logs", auditLogHandler.List)

	r.Route("/Patient", func(r chi.Router) {
		r.Use(auditLogger.Middleware)
		r.Post("/", patientHandler.Create)
		r.Get("/", patientHandler.List)
		r.Get("/{id}", patientHandler.GetByID)
		r.Put("/{id}", patientHandler.Update)
		r.Delete("/{id}", patientHandler.Delete)
		r.Get("/{id}/Observation", observationHandler.ListByPatient)
	})

	r.Route("/Observation", func(r chi.Router) {
		r.Use(auditLogger.Middleware)
		r.Post("/", observationHandler.Create)
		r.Get("/{id}", observationHandler.GetByID)
		r.Put("/{id}", observationHandler.Update)
		r.Delete("/{id}", observationHandler.Delete)
	})

	srv := &http.Server{Addr: ":8080", Handler: r}

	go func() {
		log.Println("starting server on :8080")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}

	wg.Wait()
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
