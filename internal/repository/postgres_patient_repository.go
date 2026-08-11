package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marcusashmond/fhir-health-service/internal/models"
)

type PostgresPatientRepository struct {
	db *pgxpool.Pool
}

func NewPostgresPatientRepository(db *pgxpool.Pool) *PostgresPatientRepository {
	return &PostgresPatientRepository{db: db}
}

func (r *PostgresPatientRepository) Create(patient *models.Patient) (*models.Patient, error) {
	patient.ID = generateID()

	identifierJSON, familyName, givenNames, birthDate, addressJSON, err := encodePatient(patient)
	if err != nil {
		return nil, err
	}

	_, err = r.db.Exec(context.Background(), `
		INSERT INTO patients (id, identifier, family_name, given_names, gender, birth_date, address)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, patient.ID, identifierJSON, familyName, givenNames, patient.Gender, birthDate, addressJSON)
	if err != nil {
		return nil, err
	}

	return patient, nil
}

func (r *PostgresPatientRepository) GetByID(id string) (*models.Patient, error) {
	row := r.db.QueryRow(context.Background(), `
		SELECT id, identifier, family_name, given_names, gender, birth_date, address
		FROM patients WHERE id = $1
	`, id)

	patient, err := scanPatient(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return patient, nil
}

func (r *PostgresPatientRepository) Update(id string, patient *models.Patient) (*models.Patient, error) {
	patient.ID = id

	identifierJSON, familyName, givenNames, birthDate, addressJSON, err := encodePatient(patient)
	if err != nil {
		return nil, err
	}

	tag, err := r.db.Exec(context.Background(), `
		UPDATE patients
		SET identifier = $2, family_name = $3, given_names = $4, gender = $5, birth_date = $6, address = $7
		WHERE id = $1
	`, id, identifierJSON, familyName, givenNames, patient.Gender, birthDate, addressJSON)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}

	return patient, nil
}

func (r *PostgresPatientRepository) Delete(id string) error {
	tag, err := r.db.Exec(context.Background(), `DELETE FROM patients WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresPatientRepository) List() ([]*models.Patient, error) {
	rows, err := r.db.Query(context.Background(), `
		SELECT id, identifier, family_name, given_names, gender, birth_date, address
		FROM patients ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	patients := make([]*models.Patient, 0)
	for rows.Next() {
		patient, err := scanPatient(rows)
		if err != nil {
			return nil, err
		}
		patients = append(patients, patient)
	}
	return patients, rows.Err()
}

func encodePatient(patient *models.Patient) (identifierJSON []byte, familyName string, givenNames []string, birthDate any, addressJSON []byte, err error) {
	identifierJSON, err = json.Marshal(patient.Identifier)
	if err != nil {
		return nil, "", nil, nil, nil, err
	}

	addressJSON, err = json.Marshal(patient.Address)
	if err != nil {
		return nil, "", nil, nil, nil, err
	}

	givenNames = []string{}
	if len(patient.Name) > 0 {
		familyName = patient.Name[0].Family
		if patient.Name[0].Given != nil {
			givenNames = patient.Name[0].Given
		}
	}

	if patient.BirthDate != "" {
		birthDate = patient.BirthDate
	}

	return identifierJSON, familyName, givenNames, birthDate, addressJSON, nil
}

func scanPatient(row pgx.Row) (*models.Patient, error) {
	var (
		id, familyName, gender      string
		givenNames                  []string
		birthDate                   *time.Time
		identifierJSON, addressJSON []byte
	)

	if err := row.Scan(&id, &identifierJSON, &familyName, &givenNames, &gender, &birthDate, &addressJSON); err != nil {
		return nil, err
	}

	patient := &models.Patient{
		ID:     id,
		Gender: gender,
	}

	if birthDate != nil {
		patient.BirthDate = birthDate.Format("2006-01-02")
	}

	if familyName != "" || len(givenNames) > 0 {
		patient.Name = []models.HumanName{{Family: familyName, Given: givenNames}}
	}

	if len(identifierJSON) > 0 {
		if err := json.Unmarshal(identifierJSON, &patient.Identifier); err != nil {
			return nil, err
		}
	}

	if len(addressJSON) > 0 {
		if err := json.Unmarshal(addressJSON, &patient.Address); err != nil {
			return nil, err
		}
	}

	return patient, nil
}
