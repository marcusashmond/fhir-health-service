package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marcusashmond/fhir-health-service/internal/models"
)

type PostgresAuditLogRepository struct {
	db *pgxpool.Pool
}

func NewPostgresAuditLogRepository(db *pgxpool.Pool) *PostgresAuditLogRepository {
	return &PostgresAuditLogRepository{db: db}
}

func (r *PostgresAuditLogRepository) Insert(entry *models.AuditLog) error {
	_, err := r.db.Exec(context.Background(), `
		INSERT INTO audit_logs (timestamp, method, path, resource_type, resource_id, status_code)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, entry.Timestamp, entry.Method, entry.Path, entry.ResourceType, entry.ResourceID, entry.StatusCode)
	return err
}

func (r *PostgresAuditLogRepository) List(limit, offset int) ([]*models.AuditLog, error) {
	rows, err := r.db.Query(context.Background(), `
		SELECT id, timestamp, method, path, resource_type, resource_id, status_code
		FROM audit_logs
		ORDER BY id DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]*models.AuditLog, 0)
	for rows.Next() {
		var (
			entry                    models.AuditLog
			resourceType, resourceID *string
		)
		if err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.Method, &entry.Path, &resourceType, &resourceID, &entry.StatusCode); err != nil {
			return nil, err
		}
		if resourceType != nil {
			entry.ResourceType = *resourceType
		}
		if resourceID != nil {
			entry.ResourceID = *resourceID
		}
		entries = append(entries, &entry)
	}
	return entries, rows.Err()
}
