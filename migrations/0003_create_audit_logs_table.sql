CREATE TABLE IF NOT EXISTS audit_logs (
    id            BIGSERIAL PRIMARY KEY,
    timestamp     TIMESTAMPTZ NOT NULL,
    method        TEXT NOT NULL,
    path          TEXT NOT NULL,
    resource_type TEXT,
    resource_id   TEXT,
    status_code   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs (timestamp DESC);
