package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/marcusashmond/fhir-health-service/internal/models"
	"github.com/marcusashmond/fhir-health-service/internal/repository"
)

type mockObservationRepository struct {
	createFn        func(*models.Observation) (*models.Observation, error)
	getByIDFn       func(string) (*models.Observation, error)
	updateFn        func(string, *models.Observation) (*models.Observation, error)
	deleteFn        func(string) error
	listByPatientFn func(string) ([]*models.Observation, error)
}

func (m *mockObservationRepository) Create(o *models.Observation) (*models.Observation, error) {
	return m.createFn(o)
}

func (m *mockObservationRepository) GetByID(id string) (*models.Observation, error) {
	return m.getByIDFn(id)
}

func (m *mockObservationRepository) Update(id string, o *models.Observation) (*models.Observation, error) {
	return m.updateFn(id, o)
}

func (m *mockObservationRepository) Delete(id string) error {
	return m.deleteFn(id)
}

func (m *mockObservationRepository) ListByPatient(patientID string) ([]*models.Observation, error) {
	return m.listByPatientFn(patientID)
}

type mockEventPublisher struct {
	publishFn func(context.Context, *models.Observation) error
}

func (m *mockEventPublisher) PublishObservationCreated(ctx context.Context, o *models.Observation) error {
	if m.publishFn == nil {
		return nil
	}
	return m.publishFn(ctx, o)
}

// existingPatientRepo returns a patient for any ID, used when tests only
// care about the subject reference existing.
func existingPatientRepo() repository.PatientRepository {
	return &mockPatientRepository{
		getByIDFn: func(id string) (*models.Patient, error) {
			return &models.Patient{ID: id}, nil
		},
	}
}

func newObservationRouter(repo repository.ObservationRepository, patientRepo repository.PatientRepository, publisher EventPublisher) http.Handler {
	h := NewObservationHandler(repo, patientRepo, publisher)
	r := chi.NewRouter()
	r.Post("/Observation", h.Create)
	r.Get("/Observation/{id}", h.GetByID)
	r.Put("/Observation/{id}", h.Update)
	r.Delete("/Observation/{id}", h.Delete)
	r.Get("/Patient/{id}/Observation", h.ListByPatient)
	return r
}

func TestObservationHandler_Create(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		patientRepo repository.PatientRepository
		createFn    func(*models.Observation) (*models.Observation, error)
		wantStatus  int
	}{
		{
			name:        "success",
			body:        `{"status":"final","subject":{"reference":"Patient/p1"}}`,
			patientRepo: existingPatientRepo(),
			createFn: func(o *models.Observation) (*models.Observation, error) {
				o.ID = "obs1"
				return o, nil
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:        "invalid json",
			body:        `not-json`,
			patientRepo: existingPatientRepo(),
			createFn:    func(o *models.Observation) (*models.Observation, error) { return o, nil },
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "missing subject reference",
			body:        `{"status":"final"}`,
			patientRepo: existingPatientRepo(),
			createFn:    func(o *models.Observation) (*models.Observation, error) { return o, nil },
			wantStatus:  http.StatusBadRequest,
		},
		{
			name: "referenced patient does not exist",
			body: `{"status":"final","subject":{"reference":"Patient/missing"}}`,
			patientRepo: &mockPatientRepository{
				getByIDFn: func(id string) (*models.Patient, error) { return nil, repository.ErrNotFound },
			},
			createFn:   func(o *models.Observation) (*models.Observation, error) { return o, nil },
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "repository error",
			body:        `{"status":"final","subject":{"reference":"Patient/p1"}}`,
			patientRepo: existingPatientRepo(),
			createFn: func(o *models.Observation) (*models.Observation, error) {
				return nil, errBoom
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockObservationRepository{createFn: tt.createFn}
			publisher := &mockEventPublisher{}
			router := newObservationRouter(repo, tt.patientRepo, publisher)

			req := httptest.NewRequest(http.MethodPost, "/Observation", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus == http.StatusCreated {
				var got models.Observation
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if got.ID != "obs1" {
					t.Errorf("ID = %q, want %q", got.ID, "obs1")
				}
			}
		})
	}
}

func TestObservationHandler_Create_PublishFailureDoesNotFailRequest(t *testing.T) {
	repo := &mockObservationRepository{
		createFn: func(o *models.Observation) (*models.Observation, error) {
			o.ID = "obs1"
			return o, nil
		},
	}
	publisher := &mockEventPublisher{publishFn: func(ctx context.Context, o *models.Observation) error {
		return errBoom
	}}
	router := newObservationRouter(repo, existingPatientRepo(), publisher)

	body := `{"status":"final","subject":{"reference":"Patient/p1"}}`
	req := httptest.NewRequest(http.MethodPost, "/Observation", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestObservationHandler_GetByID(t *testing.T) {
	tests := []struct {
		name       string
		getByIDFn  func(string) (*models.Observation, error)
		wantStatus int
	}{
		{
			name: "found",
			getByIDFn: func(id string) (*models.Observation, error) {
				return &models.Observation{ID: id, Status: "final"}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "not found",
			getByIDFn:  func(id string) (*models.Observation, error) { return nil, repository.ErrNotFound },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "repository error",
			getByIDFn:  func(id string) (*models.Observation, error) { return nil, errBoom },
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockObservationRepository{getByIDFn: tt.getByIDFn}
			router := newObservationRouter(repo, existingPatientRepo(), &mockEventPublisher{})

			req := httptest.NewRequest(http.MethodGet, "/Observation/xyz", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestObservationHandler_Update(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		patientRepo repository.PatientRepository
		updateFn    func(string, *models.Observation) (*models.Observation, error)
		wantStatus  int
	}{
		{
			name:        "success",
			body:        `{"status":"final","subject":{"reference":"Patient/p1"}}`,
			patientRepo: existingPatientRepo(),
			updateFn: func(id string, o *models.Observation) (*models.Observation, error) {
				o.ID = id
				return o, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "not found",
			body:        `{"status":"final","subject":{"reference":"Patient/p1"}}`,
			patientRepo: existingPatientRepo(),
			updateFn: func(id string, o *models.Observation) (*models.Observation, error) {
				return nil, repository.ErrNotFound
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockObservationRepository{updateFn: tt.updateFn}
			router := newObservationRouter(repo, tt.patientRepo, &mockEventPublisher{})

			req := httptest.NewRequest(http.MethodPut, "/Observation/xyz", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestObservationHandler_Delete(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockObservationRepository{deleteFn: tt.deleteFn}
			router := newObservationRouter(repo, existingPatientRepo(), &mockEventPublisher{})

			req := httptest.NewRequest(http.MethodDelete, "/Observation/xyz", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestObservationHandler_ListByPatient(t *testing.T) {
	tests := []struct {
		name            string
		patientRepo     repository.PatientRepository
		listByPatientFn func(string) ([]*models.Observation, error)
		wantStatus      int
	}{
		{
			name:        "success",
			patientRepo: existingPatientRepo(),
			listByPatientFn: func(patientID string) ([]*models.Observation, error) {
				return []*models.Observation{{ID: "1"}, {ID: "2"}}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "patient not found",
			patientRepo: &mockPatientRepository{
				getByIDFn: func(id string) (*models.Patient, error) { return nil, repository.ErrNotFound },
			},
			listByPatientFn: func(patientID string) ([]*models.Observation, error) { return nil, nil },
			wantStatus:      http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockObservationRepository{listByPatientFn: tt.listByPatientFn}
			router := newObservationRouter(repo, tt.patientRepo, &mockEventPublisher{})

			req := httptest.NewRequest(http.MethodGet, "/Patient/p1/Observation", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
