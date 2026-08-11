CREATE TABLE IF NOT EXISTS observations (
    id                  TEXT PRIMARY KEY,
    status              TEXT,
    code                JSONB,
    subject_patient_id  TEXT NOT NULL REFERENCES patients(id),
    effective_date_time TIMESTAMPTZ,
    value_quantity      NUMERIC,
    value_unit          TEXT
);

CREATE INDEX IF NOT EXISTS idx_observations_subject_patient_id ON observations (subject_patient_id);
