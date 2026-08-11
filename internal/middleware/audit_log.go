package middleware

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/marcusashmond/fhir-health-service/internal/models"
	"github.com/marcusashmond/fhir-health-service/internal/repository"
)

// auditQueueSize bounds how many pending audit records can be buffered before
// new ones are dropped, so a slow or unavailable database never backs up
// into (and blocks) the request path.
const auditQueueSize = 1000

type AuditLogger struct {
	repo  repository.AuditLogRepository
	queue chan *models.AuditLog
	done  chan struct{}
}

func NewAuditLogger(repo repository.AuditLogRepository) *AuditLogger {
	a := &AuditLogger{
		repo:  repo,
		queue: make(chan *models.AuditLog, auditQueueSize),
		done:  make(chan struct{}),
	}
	go a.run()
	return a
}

func (a *AuditLogger) run() {
	defer close(a.done)
	for entry := range a.queue {
		if err := a.repo.Insert(entry); err != nil {
			log.Printf("audit log write failed: %v", err)
		}
	}
}

// Close stops accepting new records and waits for the queue to drain.
func (a *AuditLogger) Close() {
	close(a.queue)
	<-a.done
}

// Middleware logs every request it wraps: timestamp, method, path, resource
// ID (if present in the route), and response status code. The record is
// written to stdout as JSON and enqueued for async persistence — the queue
// send is non-blocking, so a full queue or slow DB never delays the response.
func (a *AuditLogger) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		entry := &models.AuditLog{
			Timestamp:    time.Now().UTC(),
			Method:       r.Method,
			Path:         r.URL.Path,
			ResourceType: resourceType(r.URL.Path),
			ResourceID:   chi.URLParam(r, "id"),
			StatusCode:   rec.status,
		}

		if line, err := json.Marshal(entry); err != nil {
			log.Printf("audit log marshal failed: %v", err)
		} else {
			log.Println(string(line))
		}

		select {
		case a.queue <- entry:
		default:
			log.Printf("audit log queue full, dropping record for %s %s", entry.Method, entry.Path)
		}
	})
}

func resourceType(path string) string {
	trimmed := strings.TrimPrefix(path, "/")
	if i := strings.Index(trimmed, "/"); i >= 0 {
		trimmed = trimmed[:i]
	}
	return trimmed
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
