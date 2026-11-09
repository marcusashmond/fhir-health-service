package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/marcusashmond/fhir-health-service/internal/models"
)

type fakeAuditLogRepository struct {
	mu        sync.Mutex
	entries   []*models.AuditLog
	insertErr error
	started   chan struct{} // if set, signaled once when the first Insert begins
	release   chan struct{} // if set, Insert blocks until it is closed
	startOnce sync.Once
}

func (f *fakeAuditLogRepository) Insert(entry *models.AuditLog) error {
	if f.started != nil {
		f.startOnce.Do(func() { close(f.started) })
	}
	if f.release != nil {
		<-f.release
	}
	if f.insertErr != nil {
		return f.insertErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, entry)
	return nil
}

func (f *fakeAuditLogRepository) List(limit, offset int) ([]*models.AuditLog, error) {
	return nil, nil
}

func (f *fakeAuditLogRepository) persisted() []*models.AuditLog {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*models.AuditLog(nil), f.entries...)
}

// captureLog redirects the standard logger for the duration of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	flags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(flags)
	})
	return &buf
}

// newAuditedRouter wires the middleware the same way main.go does.
func newAuditedRouter(a *AuditLogger) http.Handler {
	r := chi.NewRouter()
	r.With(a.Middleware).Get("/audit-logs", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"entries":[]}`))
	})
	r.Route("/Patient", func(r chi.Router) {
		r.Use(a.Middleware)
		r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		r.Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
	})
	r.Route("/Observation", func(r chi.Router) {
		r.Use(a.Middleware)
		r.Post("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})
	})
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {})
	return r
}

func TestAuditLogger_Middleware_RecordsRequest(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		remoteAddr string
		want       models.AuditLog
	}{
		{
			name:       "read patient",
			method:     http.MethodGet,
			path:       "/Patient/p1",
			remoteAddr: "192.0.2.10:51234",
			want:       models.AuditLog{Method: "GET", Path: "/Patient/p1", ResourceType: "Patient", ResourceID: "p1", StatusCode: 200, SourceIP: "192.0.2.10"},
		},
		{
			name:       "failed delete keeps handler status",
			method:     http.MethodDelete,
			path:       "/Patient/missing",
			remoteAddr: "192.0.2.11:40000",
			want:       models.AuditLog{Method: "DELETE", Path: "/Patient/missing", ResourceType: "Patient", ResourceID: "missing", StatusCode: 404, SourceIP: "192.0.2.11"},
		},
		{
			name:       "create observation",
			method:     http.MethodPost,
			path:       "/Observation",
			remoteAddr: "[2001:db8::1]:443",
			want:       models.AuditLog{Method: "POST", Path: "/Observation", ResourceType: "Observation", StatusCode: 201, SourceIP: "2001:db8::1"},
		},
		{
			name:       "audit log access is itself audited",
			method:     http.MethodGet,
			path:       "/audit-logs",
			remoteAddr: "198.51.100.7:1234",
			want:       models.AuditLog{Method: "GET", Path: "/audit-logs", ResourceType: "audit-logs", StatusCode: 200, SourceIP: "198.51.100.7"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captureLog(t)
			repo := &fakeAuditLogRepository{}
			a := NewAuditLogger(repo)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.RemoteAddr = tt.remoteAddr
			rec := httptest.NewRecorder()
			before := time.Now().UTC()
			newAuditedRouter(a).ServeHTTP(rec, req)
			a.Close()

			if rec.Code != tt.want.StatusCode {
				t.Errorf("response status = %d, want %d", rec.Code, tt.want.StatusCode)
			}

			entries := repo.persisted()
			if len(entries) != 1 {
				t.Fatalf("persisted %d audit records, want 1", len(entries))
			}
			got := entries[0]
			if got.Timestamp.Before(before) || got.Timestamp.Location() != time.UTC {
				t.Errorf("Timestamp = %v, want a UTC time at or after %v", got.Timestamp, before)
			}
			got.Timestamp = time.Time{}
			if *got != tt.want {
				t.Errorf("audit record = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestAuditLogger_Middleware_UnwrappedRouteNotAudited(t *testing.T) {
	captureLog(t)
	repo := &fakeAuditLogRepository{}
	a := NewAuditLogger(repo)

	newAuditedRouter(a).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	a.Close()

	if n := len(repo.persisted()); n != 0 {
		t.Errorf("persisted %d audit records for /health, want 0", n)
	}
}

func TestAuditLogger_Middleware_IgnoresForwardedFor(t *testing.T) {
	captureLog(t)
	repo := &fakeAuditLogRepository{}
	a := NewAuditLogger(repo)

	req := httptest.NewRequest(http.MethodGet, "/Patient/p1", nil)
	req.RemoteAddr = "192.0.2.10:51234"
	req.Header.Set("X-Forwarded-For", "203.0.113.99")
	newAuditedRouter(a).ServeHTTP(httptest.NewRecorder(), req)
	a.Close()

	entries := repo.persisted()
	if len(entries) != 1 {
		t.Fatalf("persisted %d audit records, want 1", len(entries))
	}
	if entries[0].SourceIP != "192.0.2.10" {
		t.Errorf("SourceIP = %q, want the connection peer 192.0.2.10", entries[0].SourceIP)
	}
}

func TestAuditLogger_Middleware_WritesJSONLine(t *testing.T) {
	buf := captureLog(t)
	a := NewAuditLogger(&fakeAuditLogRepository{})

	req := httptest.NewRequest(http.MethodGet, "/Patient/p1", nil)
	req.RemoteAddr = "192.0.2.10:51234"
	newAuditedRouter(a).ServeHTTP(httptest.NewRecorder(), req)
	a.Close()

	var line string
	for _, l := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(l, "{") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("no JSON audit line in log output: %q", buf.String())
	}

	var got models.AuditLog
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("audit line is not valid JSON: %v (%q)", err, line)
	}
	if got.Method != "GET" || got.Path != "/Patient/p1" || got.ResourceID != "p1" || got.StatusCode != 200 || got.SourceIP != "192.0.2.10" {
		t.Errorf("logged audit record = %+v", got)
	}
}

func TestAuditLogger_Middleware_InsertFailureIsLogged(t *testing.T) {
	buf := captureLog(t)
	a := NewAuditLogger(&fakeAuditLogRepository{insertErr: errors.New("db down")})

	rec := httptest.NewRecorder()
	newAuditedRouter(a).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/Patient/p1", nil))
	a.Close()

	if rec.Code != http.StatusOK {
		t.Errorf("response status = %d, want 200 even when the audit insert fails", rec.Code)
	}
	if !strings.Contains(buf.String(), "audit log write failed: db down") {
		t.Errorf("insert failure not logged: %q", buf.String())
	}
}

func TestAuditLogger_Middleware_FullQueueDropsWithoutBlocking(t *testing.T) {
	buf := captureLog(t)
	repo := &fakeAuditLogRepository{started: make(chan struct{}), release: make(chan struct{})}
	a := NewAuditLogger(repo)
	router := newAuditedRouter(a)

	// The first record is taken off the queue and held inside Insert, so the
	// queue itself is empty once started fires.
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/Patient/p1", nil))
	<-repo.started

	const extra = 10
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < auditQueueSize+extra; i++ {
			router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/Patient/p1", nil))
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		close(repo.release)
		t.Fatal("requests blocked on a full audit queue")
	}

	close(repo.release)
	a.Close()

	if got, want := len(repo.persisted()), 1+auditQueueSize; got != want {
		t.Errorf("persisted %d audit records, want %d", got, want)
	}
	if got := strings.Count(buf.String(), "audit log queue full, dropping record for GET /Patient/p1"); got != extra {
		t.Errorf("logged %d dropped records, want %d", got, extra)
	}
}

func TestAuditLogger_Close_DrainsQueue(t *testing.T) {
	captureLog(t)
	repo := &fakeAuditLogRepository{}
	a := NewAuditLogger(repo)
	router := newAuditedRouter(a)

	const n = 25
	for i := 0; i < n; i++ {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/Observation", nil))
	}
	a.Close()

	if got := len(repo.persisted()); got != n {
		t.Errorf("persisted %d audit records after Close, want %d", got, n)
	}
}

func TestSourceIP(t *testing.T) {
	tests := []struct {
		remoteAddr string
		want       string
	}{
		{"192.0.2.10:51234", "192.0.2.10"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"not-a-host-port", "not-a-host-port"},
		{"", ""},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = tt.remoteAddr
		if got := sourceIP(r); got != tt.want {
			t.Errorf("sourceIP(%q) = %q, want %q", tt.remoteAddr, got, tt.want)
		}
	}
}

func TestResourceType(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/Patient", "Patient"},
		{"/Patient/p1", "Patient"},
		{"/Patient/p1/Observation", "Patient"},
		{"/Observation/o1", "Observation"},
		{"/audit-logs", "audit-logs"},
		{"/", ""},
	}
	for _, tt := range tests {
		if got := resourceType(tt.path); got != tt.want {
			t.Errorf("resourceType(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
