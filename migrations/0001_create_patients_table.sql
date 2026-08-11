CREATE TABLE IF NOT EXISTS patients (
    id           TEXT PRIMARY KEY,
    identifier   JSONB,
    family_name  TEXT,
    given_names  TEXT[],
    gender       TEXT,
    birth_date   DATE,
    address      JSONB
);
