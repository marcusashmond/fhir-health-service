package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marcusashmond/fhir-health-service/internal/models"
)

type PostgresObservationRepository struct {
	db *pgxpool.Pool
}

func NewPostgresObservationRepository(db *pgxpool.Pool) *PostgresObservationRepository {
	return &PostgresObservationRepository{db: db}
}

func (r *PostgresObservationRepository) Create(observation *models.Observation) (*models.Observation, error) {
	observation.ID = generateID()

	codeJSON, subjectPatientID, effectiveDateTime, valueQuantity, valueUnit, err := encodeObservation(observation)
	if err != nil {
		return nil, err
	}

	_, err = r.db.Exec(context.Background(), `
		INSERT INTO observations (id, status, code, subject_patient_id, effective_date_time, value_quantity, value_unit)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, observation.ID, observation.Status, codeJSON, subjectPatientID, effectiveDateTime, valueQuantity, valueUnit)
	if err != nil {
		return nil, err
	}

	return observation, nil
}

func (r *PostgresObservationRepository) GetByID(id string) (*models.Observation, error) {
	row := r.db.QueryRow(context.Background(), `
		SELECT id, status, code, subject_patient_id, effective_date_time, value_quantity, value_unit
		FROM observations WHERE id = $1
	`, id)

	observation, err := scanObservation(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return observation, nil
}

func (r *PostgresObservationRepository) Update(id string, observation *models.Observation) (*models.Observation, error) {
	observation.ID = id

	codeJSON, subjectPatientID, effectiveDateTime, valueQuantity, valueUnit, err := encodeObservation(observation)
	if err != nil {
		return nil, err
	}

	tag, err := r.db.Exec(context.Background(), `
		UPDATE observations
		SET status = $2, code = $3, subject_patient_id = $4, effective_date_time = $5, value_quantity = $6, value_unit = $7
		WHERE id = $1
	`, id, observation.Status, codeJSON, subjectPatientID, effectiveDateTime, valueQuantity, valueUnit)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}

	return observation, nil
}

func (r *PostgresObservationRepository) Delete(id string) error {
	tag, err := r.db.Exec(context.Background(), `DELETE FROM observations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresObservationRepository) ListByPatient(patientID string) ([]*models.Observation, error) {
	rows, err := r.db.Query(context.Background(), `
		SELECT id, status, code, subject_patient_id, effective_date_time, value_quantity, value_unit
		FROM observations WHERE subject_patient_id = $1 ORDER BY id
	`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	observations := make([]*models.Observation, 0)
	for rows.Next() {
		observation, err := scanObservation(rows)
		if err != nil {
			return nil, err
		}
		observations = append(observations, observation)
	}
	return observations, rows.Err()
}

func encodeObservation(observation *models.Observation) (codeJSON []byte, subjectPatientID string, effectiveDateTime, valueQuantity, valueUnit any, err error) {
	codeJSON, err = json.Marshal(observation.Code)
	if err != nil {
		return nil, "", nil, nil, nil, err
	}

	subjectPatientID = strings.TrimPrefix(observation.Subject.Reference, "Patient/")

	if observation.EffectiveDateTime != "" {
		effectiveDateTime = observation.EffectiveDateTime
	}

	if observation.ValueQuantity != nil {
		valueQuantity = observation.ValueQuantity.Value
		if observation.ValueQuantity.Unit != "" {
			valueUnit = observation.ValueQuantity.Unit
		}
	}

	return codeJSON, subjectPatientID, effectiveDateTime, valueQuantity, valueUnit, nil
}

func scanObservation(row pgx.Row) (*models.Observation, error) {
	var (
		id, status, subjectPatientID string
		codeJSON                     []byte
		effectiveDateTime            *time.Time
		valueQuantity                *float64
		valueUnit                    *string
	)

	if err := row.Scan(&id, &status, &codeJSON, &subjectPatientID, &effectiveDateTime, &valueQuantity, &valueUnit); err != nil {
		return nil, err
	}

	observation := &models.Observation{
		ID:      id,
		Status:  status,
		Subject: models.Reference{Reference: "Patient/" + subjectPatientID},
	}

	if len(codeJSON) > 0 {
		if err := json.Unmarshal(codeJSON, &observation.Code); err != nil {
			return nil, err
		}
	}

	if effectiveDateTime != nil {
		observation.EffectiveDateTime = effectiveDateTime.Format(time.RFC3339)
	}

	if valueQuantity != nil {
		observation.ValueQuantity = &models.Quantity{Value: *valueQuantity}
		if valueUnit != nil {
			observation.ValueQuantity.Unit = *valueUnit
		}
	}

	return observation, nil
}
