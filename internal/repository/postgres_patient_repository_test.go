package repository

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/marcusashmond/fhir-health-service/internal/models"
)

func TestPostgresPatientRepository_CreateAndGetByID(t *testing.T) {
	pool := newTestDB(t)
	repo := NewPostgresPatientRepository(pool)

	tests := []struct {
		name    string
		patient *models.Patient
	}{
		{
			name: "full patient",
			patient: &models.Patient{
				Identifier: []models.Identifier{{System: "http://hospital.org/mrn", Value: "12345"}},
				Name:       []models.HumanName{{Family: "Smith", Given: []string{"Jane", "A"}}},
				Gender:     "female",
				BirthDate:  "1990-05-01",
				Address:    []models.Address{{Line: []string{"1 Main St"}, City: "Springfield", State: "IL", PostalCode: "62701", Country: "US"}},
			},
		},
		{
			name:    "minimal patient",
			patient: &models.Patient{Gender: "unknown"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, err := repo.Create(tt.patient)
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
			if !reflect.DeepEqual(got, created) {
				t.Errorf("GetByID() = %+v, want %+v", got, created)
			}
		})
	}
}

func TestPostgresPatientRepository_GetByID_NotFound(t *testing.T) {
	pool := newTestDB(t)
	repo := NewPostgresPatientRepository(pool)

	_, err := repo.GetByID("does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestPostgresPatientRepository_Update(t *testing.T) {
	pool := newTestDB(t)
	repo := NewPostgresPatientRepository(pool)

	created, err := repo.Create(&models.Patient{
		Name:   []models.HumanName{{Family: "Doe", Given: []string{"John"}}},
		Gender: "male",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	update := &models.Patient{
		Name:      []models.HumanName{{Family: "Doe", Given: []string{"Johnathan"}}},
		Gender:    "male",
		BirthDate: "1985-03-15",
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
	if got.BirthDate != "1985-03-15" {
		t.Errorf("GetByID() BirthDate = %q, want %q", got.BirthDate, "1985-03-15")
	}
	if got.Name[0].Given[0] != "Johnathan" {
		t.Errorf("GetByID() Given = %v, want %v", got.Name[0].Given, []string{"Johnathan"})
	}
}

func TestPostgresPatientRepository_Update_NotFound(t *testing.T) {
	pool := newTestDB(t)
	repo := NewPostgresPatientRepository(pool)

	_, err := repo.Update("does-not-exist", &models.Patient{Gender: "male"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestPostgresPatientRepository_Delete(t *testing.T) {
	pool := newTestDB(t)
	repo := NewPostgresPatientRepository(pool)

	created, err := repo.Create(&models.Patient{Gender: "other"})
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

func TestPostgresPatientRepository_Delete_NotFound(t *testing.T) {
	pool := newTestDB(t)
	repo := NewPostgresPatientRepository(pool)

	err := repo.Delete("does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestPostgresPatientRepository_List(t *testing.T) {
	pool := newTestDB(t)
	repo := NewPostgresPatientRepository(pool)

	want := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		created, err := repo.Create(&models.Patient{Gender: "female"})
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		want = append(want, created.ID)
	}

	patients, err := repo.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(patients) != len(want) {
		t.Fatalf("List() returned %d patients, want %d", len(patients), len(want))
	}

	got := make([]string, 0, len(patients))
	for _, p := range patients {
		got = append(got, p.ID)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List() IDs = %v, want %v", got, want)
	}
}
