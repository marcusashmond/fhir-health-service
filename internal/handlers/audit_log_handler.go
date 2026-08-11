package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/marcusashmond/fhir-health-service/internal/repository"
)

const (
	defaultAuditLogLimit = 50
	maxAuditLogLimit     = 500
)

type AuditLogHandler struct {
	repo repository.AuditLogRepository
}

func NewAuditLogHandler(repo repository.AuditLogRepository) *AuditLogHandler {
	return &AuditLogHandler{repo: repo}
}

// List returns audit trail entries, most recent first, paginated via
// ?limit=&offset= query params so the trail is inspectable for compliance
// review without dumping the whole table at once.
func (h *AuditLogHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := defaultAuditLogLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsed > maxAuditLogLimit {
			parsed = maxAuditLogLimit
		}
		limit = parsed
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "offset must be a non-negative integer")
			return
		}
		offset = parsed
	}

	entries, err := h.repo.List(limit, offset)
	if err != nil {
		log.Printf("audit log list failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list audit logs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"limit":   limit,
		"offset":  offset,
	})
}
