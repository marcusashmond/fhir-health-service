package repository

import (
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/marcusashmond/fhir-health-service/internal/models"
)

// assertObservationsEqual compares two observations for equality, treating
// EffectiveDateTime as an instant rather than a literal string: Postgres
// returns timestamptz values in the session's timezone, so the RFC3339
// offset can differ from the input even when the instant is identical.
func assertObservationsEqual(t *testing.T, got, want *models.Observation) {
	t.Helper()

	gotTime, wantTime := got.EffectiveDateTime, want.EffectiveDateTime
	gotCopy, wantCopy := *got, *want
	gotCopy.EffectiveDateTime, wantCopy.EffectiveDateTime = "", ""

	if !reflect.DeepEqual(&gotCopy, &wantCopy) {
		t.Errorf("observation = %+v, want %+v", got, want)
	}

	if (gotTime == "") != (wantTime == "") {
		t.Errorf("EffectiveDateTime = %q, want %q", gotTime, wantTime)
		return
	}
	if gotTime == "" {
		return
	}

	gotParsed, err := time.Parse(time.RFC3339, gotTime)
	if err != nil {
		t.Fatalf("failed to parse got EffectiveDateTime %q: %v", gotTime, err)
	}
	wantParsed, err := time.Parse(time.RFC3339, wantTime)
	if err != nil {
		t.Fatalf("failed to parse want EffectiveDateTime %q: %v", wantTime, err)
	}
	if !gotParsed.Equal(wantParsed) {
		t.Errorf("EffectiveDateTime = %q, want instant equal to %q", gotTime, wantTime)
	}
}

func createTestPatient(t *testing.T, repo PatientRepository) *models.Patient {
	t.Helper()
	patient, err := repo.Create(&models.Patient{
		Name:   []models.HumanName{{Family: "Test", Given: []string{"Patient"}}},
		Gender: "unknown",
	})
	if err != nil {
		t.Fatalf("failed to create test patient: %v", err)
	}
	return patient
}

func TestPostgresObservationRepository_CreateAndGetByID(t *testing.T) {
	pool := newTestDB(t)
	patientRepo := NewPostgresPatientRepository(pool)
	repo := NewPostgresObservationRepository(pool)

	patient := createTestPatient(t, patientRepo)

	tests := []struct {
		name        string
		observation *models.Observation
	}{
		{
			name: "full observation",
			observation: &models.Observation{
				Status:            "final",
				Code:              models.CodeableConcept{Coding: []models.Coding{{System: "http://loinc.org", Code: "8480-6", Display: "Systolic blood pressure"}}},
				Subject:           models.Reference{Reference: "Patient/" + patient.ID},
				EffectiveDateTime: "2024-01-01T10:00:00Z",
				ValueQuantity:     &models.Quantity{Value: 120, Unit: "mmHg"},
			},
		},
		{
			name: "minimal observation",
			observation: &models.Observation{
				Status:  "final",
				Subject: models.Reference{Reference: "Patient/" + patient.ID},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, err := repo.Create(tt.observation)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if created.ID == "" {
				t.Fatal("Create() did not assign an ID")
			}

			got, err := repo.GetByID(created.ID)
			if err != nil {
				t.Fatalf("GetByID() error = %v", err)
			}
			assertObservationsEqual(t, got, created)
		})
	}
}

func TestPostgresObservationRepository_GetByID_NotFound(t *testing.T) {
	pool := newTestDB(t)
	repo := NewPostgresObservationRepository(pool)

	_, err := repo.GetByID("does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestPostgresObservationRepository_Update(t *testing.T) {
	pool := newTestDB(t)
	patientRepo := NewPostgresPatientRepository(pool)
	repo := NewPostgresObservationRepository(pool)

	patient := createTestPatient(t, patientRepo)

	created, err := repo.Create(&models.Observation{
		Status:        "preliminary",
		Subject:       models.Reference{Reference: "Patient/" + patient.ID},
		ValueQuantity: &models.Quantity{Value: 100, Unit: "mmHg"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	update := &models.Observation{
		Status:        "final",
		Subject:       models.Reference{Reference: "Patient/" + patient.ID},
		ValueQuantity: &models.Quantity{Value: 150, Unit: "mmHg"},
	}
	updated, err := repo.Update(created.ID, update)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.ID != created.ID {
		t.Errorf("Update() ID = %q, want %q", updated.ID, created.ID)
	}

	got, err := repo.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != "final" {
		t.Errorf("GetByID() Status = %q, want %q", got.Status, "final")
	}
	if got.ValueQuantity == nil || got.ValueQuantity.Value != 150 {
		t.Errorf("GetByID() ValueQuantity = %+v, want Value 150", got.ValueQuantity)
	}
}

func TestPostgresObservationRepository_Update_NotFound(t *testing.T) {
	pool := newTestDB(t)
	patientRepo := NewPostgresPatientRepository(pool)
	repo := NewPostgresObservationRepository(pool)

	patient := createTestPatient(t, patientRepo)

	_, err := repo.Update("does-not-exist", &models.Observation{
		Status:  "final",
		Subject: models.Reference{Reference: "Patient/" + patient.ID},
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestPostgresObservationRepository_Delete(t *testing.T) {
	pool := newTestDB(t)
	patientRepo := NewPostgresPatientRepository(pool)
	repo := NewPostgresObservationRepository(pool)

	patient := createTestPatient(t, patientRepo)

	created, err := repo.Create(&models.Observation{
		Status:  "final",
		Subject: models.Reference{Reference: "Patient/" + patient.ID},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.Delete(created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := repo.GetByID(created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID() after delete error = %v, want ErrNotFound", err)
	}
}

func TestPostgresObservationRepository_Delete_NotFound(t *testing.T) {
	pool := newTestDB(t)
	repo := NewPostgresObservationRepository(pool)

	err := repo.Delete("does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestPostgresObservationRepository_ListByPatient(t *testing.T) {
	pool := newTestDB(t)
	patientRepo := NewPostgresPatientRepository(pool)
	repo := NewPostgresObservationRepository(pool)

	patientA := createTestPatient(t, patientRepo)
	patientB := createTestPatient(t, patientRepo)

	want := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		created, err := repo.Create(&models.Observation{
			Status:  "final",
			Subject: models.Reference{Reference: "Patient/" + patientA.ID},
		})
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		want = append(want, created.ID)
	}

	if _, err := repo.Create(&models.Observation{
		Status:  "final",
		Subject: models.Reference{Reference: "Patient/" + patientB.ID},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	observations, err := repo.ListByPatient(patientA.ID)
	if err != nil {
		t.Fatalf("ListByPatient() error = %v", err)
	}
	if len(observations) != len(want) {
		t.Fatalf("ListByPatient() returned %d observations, want %d", len(observations), len(want))
	}

	got := make([]string, 0, len(observations))
	for _, o := range observations {
		if o.Subject.Reference != "Patient/"+patientA.ID {
			t.Errorf("ListByPatient() returned observation for wrong patient: %v", o.Subject.Reference)
		}
		got = append(got, o.ID)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListByPatient() IDs = %v, want %v", got, want)
	}
}
