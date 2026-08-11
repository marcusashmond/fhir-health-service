package models

import "time"

type AuditLog struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	ResourceType string    `json:"resourceType,omitempty"`
	ResourceID   string    `json:"resourceId,omitempty"`
	StatusCode   int       `json:"statusCode"`
}
