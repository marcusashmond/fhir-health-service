package repository

import (
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/marcusashmond/fhir-health-service/internal/models"
)

var ErrNotFound = errors.New("patient not found")

type PatientRepository interface {
	Create(patient *models.Patient) (*models.Patient, error)
	GetByID(id string) (*models.Patient, error)
	Update(id string, patient *models.Patient) (*models.Patient, error)
	Delete(id string) error
	List() ([]*models.Patient, error)
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
