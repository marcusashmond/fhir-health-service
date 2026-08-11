# fhir-health-service

A FHIR-compliant health data interoperability service in Go, with Kafka-based event streaming and real-time anomaly detection.

## Overview

This is a backend service that ingests and serves patient health data modeled on the FHIR standard (`Patient` and `Observation` resources), backed by PostgreSQL. Every created `Observation` is published to Kafka, where a consumer checks it against known-normal ranges for its LOINC code and logs an alert if the value is out of range — giving the system a real-time path for flagging abnormal vitals as they're recorded, separate from the request/response path.

## Architecture

```text
Client --HTTP--> REST API (chi) --SQL--> PostgreSQL
                     |
                     | on Observation create
                     v
              Kafka Producer --> [observation.created topic] --> Kafka Consumer --> anomaly check --> ALERT log
```

```mermaid
flowchart LR
    Client -->|HTTP| API["REST API (chi router)"]
    API -->|CRUD| PG[(PostgreSQL)]
    API -->|on Observation create| Producer[Kafka Producer]
    Producer -->|publish| Topic[["observation.created"]]
    Topic --> Consumer[Kafka Consumer]
    Consumer --> Check{"Out of normal range?"}
    Check -->|yes| Alert["log.Printf ALERT"]
    Check -->|no| NoOp[no action]
```

The Kafka publish happens after the Postgres write succeeds and is best-effort: if it fails, the error is logged but the HTTP request still returns success, since the write of record already landed in Postgres.

## Tech Stack

- **Go** (1.26)
- **[chi](https://github.com/go-chi/chi)** — HTTP router
- **PostgreSQL** — persistence, via [pgx](https://github.com/jackc/pgx)
- **Kafka** ([kafka-go](https://github.com/segmentio/kafka-go)) — event streaming, KRaft mode (no Zookeeper)
- **Docker Compose** — local Postgres + Kafka
- **[godotenv](https://github.com/joho/godotenv)** — `.env` file loading

## Features

- Patient CRUD (`Create`, `Read`, `Update`, `Delete`, `List`)
- Observation CRUD with patient linkage — `subject.reference` is validated against existing patients at write time (returns `400` if the referenced patient doesn't exist)
- List observations by patient (`GET /Patient/{id}/Observation`)
- Kafka event publishing on Observation creation (`observation.created` topic)
- Real-time anomaly-detection consumer for out-of-range vital signs — currently covers systolic blood pressure (LOINC `8480-6`, normal range 90–140), structured as a lookup table so more LOINC codes are a one-line addition
- Graceful shutdown on `SIGINT`/`SIGTERM` — HTTP server and Kafka consumer both drain cleanly
- Health check endpoint (`GET /health`)
- HIPAA-aligned audit logging middleware on all `/Patient` and `/Observation` routes — logs timestamp, method, path, resource ID, and status code as JSON to stdout and asynchronously to the `audit_logs` table (a full queue or DB hiccup drops the persisted record rather than blocking the request). Inspectable via `GET /audit-logs?limit=&offset=`

Not yet implemented: authentication/authorization.

## Getting Started

### Prerequisites

- Go 1.26+
- Docker and Docker Compose

### Setup

```bash
git clone https://github.com/marcusashmond/fhir-health-service.git
cd fhir-health-service

cp .env.example .env

docker compose up -d

# apply migrations (no migration runner yet — applied directly via psql)
docker compose exec -T postgres psql -U fhir -d fhir < migrations/0001_create_patients_table.sql
docker compose exec -T postgres psql -U fhir -d fhir < migrations/0002_create_observations_table.sql
docker compose exec -T postgres psql -U fhir -d fhir < migrations/0003_create_audit_logs_table.sql

go run main.go
```

The server listens on `:8080`.

### Example: create a patient

```bash
curl -X POST http://localhost:8080/Patient \
  -H 'Content-Type: application/json' \
  -d '{"name":[{"family":"Doe","given":["Jane"]}],"gender":"female","birthDate":"1990-01-01"}'
```

Response includes a server-generated `id`, e.g. `"id":"a1b2c3d4e5f6a7b8"`.

### Example: create an observation (triggers an anomaly alert)

```bash
curl -X POST http://localhost:8080/Observation \
  -H 'Content-Type: application/json' \
  -d '{
    "status":"final",
    "code":{"coding":[{"system":"http://loinc.org","code":"8480-6","display":"Systolic blood pressure"}]},
    "subject":{"reference":"Patient/a1b2c3d4e5f6a7b8"},
    "valueQuantity":{"value":150,"unit":"mmHg"}
  }'
```

The consumer picks this up off `observation.created` and logs:

```text
ALERT: abnormal observation for patient Patient/a1b2c3d4e5f6a7b8: value 150 outside normal range [90, 140] (code 8480-6)
```

### Example: inspect the audit trail

```bash
curl "http://localhost:8080/audit-logs?limit=10"
```

Returns the most recent accesses to `/Patient` and `/Observation`, newest first:

```json
{
  "entries": [
    {"id": 3, "timestamp": "2026-08-11T18:21:30.335Z", "method": "POST", "path": "/Observation", "resourceType": "Observation", "statusCode": 201},
    {"id": 2, "timestamp": "2026-08-11T18:21:30.298Z", "method": "GET", "path": "/Patient/a1b2c3d4e5f6a7b8", "resourceType": "Patient", "resourceId": "a1b2c3d4e5f6a7b8", "statusCode": 200}
  ],
  "limit": 10,
  "offset": 0
}
```

## Project Structure

```text
internal/
  handlers/    HTTP handlers (chi) for Patient, Observation, and audit-log endpoints
  models/      FHIR resource structs — Patient, Observation, and supporting types (HumanName, Address, CodeableConcept, ...)
  repository/  Postgres-backed persistence behind PatientRepository / ObservationRepository / AuditLogRepository interfaces
  kafka/       Kafka producer (publishes observation.created) and consumer (anomaly detection)
  middleware/  Audit-logging middleware for Patient/Observation routes
migrations/    Raw SQL migration files, applied manually against Postgres
k8s/           Kubernetes manifests (Deployment, Service, Secret) for deploying the app — see k8s/README.md
Dockerfile     Multi-stage build producing a minimal distroless runtime image
```

## Deployment

The app has a multi-stage `Dockerfile` (Go build stage → `distroless/static` runtime, no shell, non-root) and basic Kubernetes manifests under [`k8s/`](k8s/README.md).

```bash
docker build -t fhir-health-service:latest .
docker run -p 8080:8080 \
  -e DATABASE_URL='postgres://fhir:fhir@host.docker.internal:5432/fhir?sslmode=disable' \
  -e KAFKA_BROKER='host.docker.internal:9092' \
  fhir-health-service:latest
```

`host.docker.internal` lets a standalone container reach the Postgres/Kafka started by `docker compose` on the host — useful for testing the image locally. In Kubernetes, `DATABASE_URL`/`KAFKA_BROKER` are supplied via `k8s/secret.yaml` and should point at managed or in-cluster instances (RDS/MSK, or a Postgres/Kafka Helm chart) rather than `docker-compose.yml`'s containers, which are dev-only. See [`k8s/README.md`](k8s/README.md) for details.

## Roadmap

Not committed to, but the natural next steps given the current structure:

- Additional LOINC code ranges in the anomaly consumer (heart rate, temperature, etc.)
- A migration runner instead of applying SQL files by hand
- Authentication/authorization
- In-cluster or managed Postgres/Kafka for the Kubernetes deployment (currently BYO per `k8s/README.md`)
