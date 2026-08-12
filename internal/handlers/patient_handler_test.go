package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/marcusashmond/fhir-health-service/internal/models"
	"github.com/marcusashmond/fhir-health-service/internal/repository"
)

type mockPatientRepository struct {
	createFn  func(*models.Patient) (*models.Patient, error)
	getByIDFn func(string) (*models.Patient, error)
	updateFn  func(string, *models.Patient) (*models.Patient, error)
	deleteFn  func(string) error
	listFn    func() ([]*models.Patient, error)
}

func (m *mockPatientRepository) Create(p *models.Patient) (*models.Patient, error) {
	return m.createFn(p)
}

func (m *mockPatientRepository) GetByID(id string) (*models.Patient, error) {
	return m.getByIDFn(id)
}

func (m *mockPatientRepository) Update(id string, p *models.Patient) (*models.Patient, error) {
	return m.updateFn(id, p)
}

func (m *mockPatientRepository) Delete(id string) error {
	return m.deleteFn(id)
}

func (m *mockPatientRepository) List() ([]*models.Patient, error) {
	return m.listFn()
}

func newPatientRouter(repo repository.PatientRepository) http.Handler {
	h := NewPatientHandler(repo)
	r := chi.NewRouter()
	r.Post("/Patient", h.Create)
	r.Get("/Patient", h.List)
	r.Get("/Patient/{id}", h.GetByID)
	r.Put("/Patient/{id}", h.Update)
	r.Delete("/Patient/{id}", h.Delete)
	return r
}

func TestPatientHandler_Create(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		createFn   func(*models.Patient) (*models.Patient, error)
		wantStatus int
	}{
		{
			name: "success",
			body: `{"gender":"female"}`,
			createFn: func(p *models.Patient) (*models.Patient, error) {
				p.ID = "abc123"
				return p, nil
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid json",
			body:       `not-json`,
			createFn:   func(p *models.Patient) (*models.Patient, error) { return p, nil },
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "repository error",
			body: `{"gender":"male"}`,
			createFn: func(p *models.Patient) (*models.Patient, error) {
				return nil, errBoom
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockPatientRepository{createFn: tt.createFn}
			router := newPatientRouter(repo)

			req := httptest.NewRequest(http.MethodPost, "/Patient", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus == http.StatusCreated {
				var got models.Patient
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if got.ID != "abc123" {
					t.Errorf("ID = %q, want %q", got.ID, "abc123")
				}
			}
		})
	}
}

func TestPatientHandler_GetByID(t *testing.T) {
	tests := []struct {
		name       string
		getByIDFn  func(string) (*models.Patient, error)
		wantStatus int
	}{
		{
			name: "found",
			getByIDFn: func(id string) (*models.Patient, error) {
				return &models.Patient{ID: id, Gender: "male"}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "not found",
			getByIDFn: func(id string) (*models.Patient, error) {
				return nil, repository.ErrNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "repository error",
			getByIDFn: func(id string) (*models.Patient, error) {
				return nil, errBoom
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockPatientRepository{getByIDFn: tt.getByIDFn}
			router := newPatientRouter(repo)

			req := httptest.NewRequest(http.MethodGet, "/Patient/xyz", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestPatientHandler_Update(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		updateFn   func(string, *models.Patient) (*models.Patient, error)
		wantStatus int
	}{
		{
			name: "success",
			body: `{"gender":"male"}`,
			updateFn: func(id string, p *models.Patient) (*models.Patient, error) {
				p.ID = id
				return p, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid json",
			body:       `not-json`,
			updateFn:   func(id string, p *models.Patient) (*models.Patient, error) { return p, nil },
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "not found",
			body: `{"gender":"male"}`,
			updateFn: func(id string, p *models.Patient) (*models.Patient, error) {
				return nil, repository.ErrNotFound
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockPatientRepository{updateFn: tt.updateFn}
			router := newPatientRouter(repo)

			req := httptest.NewRequest(http.MethodPut, "/Patient/xyz", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestPatientHandler_Delete(t *testing.T) {
	tests := []struct {
		name       string
		deleteFn   func(string) error
		wantStatus int
	}{
		{
			name:       "success",
			deleteFn:   func(id string) error { return nil },
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "not found",
			deleteFn:   func(id string) error { return repository.ErrNotFound },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "repository error",
			deleteFn:   func(id string) error { return errBoom },
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockPatientRepository{deleteFn: tt.deleteFn}
			router := newPatientRouter(repo)

			req := httptest.NewRequest(http.MethodDelete, "/Patient/xyz", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestPatientHandler_List(t *testing.T) {
	tests := []struct {
		name       string
		listFn     func() ([]*models.Patient, error)
		wantStatus int
		wantLen    int
	}{
		{
			name: "success",
			listFn: func() ([]*models.Patient, error) {
				return []*models.Patient{{ID: "1"}, {ID: "2"}}, nil
			},
			wantStatus: http.StatusOK,
			wantLen:    2,
		},
		{
			name:       "repository error",
			listFn:     func() ([]*models.Patient, error) { return nil, errBoom },
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockPatientRepository{listFn: tt.listFn}
			router := newPatientRouter(repo)

			req := httptest.NewRequest(http.MethodGet, "/Patient", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus == http.StatusOK {
				var got []*models.Patient
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if len(got) != tt.wantLen {
					t.Errorf("len = %d, want %d", len(got), tt.wantLen)
				}
			}
		})
	}
}
