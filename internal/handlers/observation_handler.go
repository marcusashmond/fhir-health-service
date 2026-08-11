package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/marcusashmond/fhir-health-service/internal/models"
	"github.com/marcusashmond/fhir-health-service/internal/repository"
)

type ObservationHandler struct {
	repo        repository.ObservationRepository
	patientRepo repository.PatientRepository
}

func NewObservationHandler(repo repository.ObservationRepository, patientRepo repository.PatientRepository) *ObservationHandler {
	return &ObservationHandler{repo: repo, patientRepo: patientRepo}
}

func (h *ObservationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var observation models.Observation
	if err := json.NewDecoder(r.Body).Decode(&observation); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !h.validateSubject(w, &observation) {
		return
	}

	created, err := h.repo.Create(&observation)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create observation")
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (h *ObservationHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	observation, err := h.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "observation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get observation")
		return
	}

	writeJSON(w, http.StatusOK, observation)
}

func (h *ObservationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var observation models.Observation
	if err := json.NewDecoder(r.Body).Decode(&observation); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !h.validateSubject(w, &observation) {
		return
	}

	updated, err := h.repo.Update(id, &observation)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "observation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update observation")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (h *ObservationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.repo.Delete(id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "observation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete observation")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ObservationHandler) ListByPatient(w http.ResponseWriter, r *http.Request) {
	patientID := chi.URLParam(r, "id")

	if _, err := h.patientRepo.GetByID(patientID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "patient not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to look up patient")
		return
	}

	observations, err := h.repo.ListByPatient(patientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list observations")
		return
	}

	writeJSON(w, http.StatusOK, observations)
}

func (h *ObservationHandler) validateSubject(w http.ResponseWriter, observation *models.Observation) bool {
	patientID := strings.TrimPrefix(observation.Subject.Reference, "Patient/")
	if patientID == "" {
		writeError(w, http.StatusBadRequest, "subject.reference is required, expected format Patient/{id}")
		return false
	}

	if _, err := h.patientRepo.GetByID(patientID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "referenced patient does not exist")
			return false
		}
		writeError(w, http.StatusInternalServerError, "failed to validate patient reference")
		return false
	}

	return true
}
