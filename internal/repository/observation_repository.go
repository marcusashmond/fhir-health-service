package repository

import "github.com/marcusashmond/fhir-health-service/internal/models"

type ObservationRepository interface {
	Create(observation *models.Observation) (*models.Observation, error)
	GetByID(id string) (*models.Observation, error)
	Update(id string, observation *models.Observation) (*models.Observation, error)
	Delete(id string) error
	ListByPatient(patientID string) ([]*models.Observation, error)
}
