package repository

import "github.com/marcusashmond/fhir-health-service/internal/models"

type AuditLogRepository interface {
	Insert(entry *models.AuditLog) error
	List(limit, offset int) ([]*models.AuditLog, error)
}
