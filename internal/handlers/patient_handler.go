package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/marcusashmond/fhir-health-service/internal/models"
	"github.com/marcusashmond/fhir-health-service/internal/repository"
)

type PatientHandler struct {
	repo repository.PatientRepository
}

func NewPatientHandler(repo repository.PatientRepository) *PatientHandler {
	return &PatientHandler{repo: repo}
}

func (h *PatientHandler) Create(w http.ResponseWriter, r *http.Request) {
	var patient models.Patient
	if err := json.NewDecoder(r.Body).Decode(&patient); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	created, err := h.repo.Create(&patient)
	if err != nil {
		log.Printf("patient create failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create patient")
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (h *PatientHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	patient, err := h.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "patient not found")
			return
		}
		log.Printf("patient get failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get patient")
		return
	}

	writeJSON(w, http.StatusOK, patient)
}

func (h *PatientHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var patient models.Patient
	if err := json.NewDecoder(r.Body).Decode(&patient); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updated, err := h.repo.Update(id, &patient)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "patient not found")
			return
		}
		log.Printf("patient update failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update patient")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (h *PatientHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.repo.Delete(id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "patient not found")
			return
		}
		log.Printf("patient delete failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete patient")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *PatientHandler) List(w http.ResponseWriter, r *http.Request) {
	patients, err := h.repo.List()
	if err != nil {
		log.Printf("patient list failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list patients")
		return
	}

	writeJSON(w, http.StatusOK, patients)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
