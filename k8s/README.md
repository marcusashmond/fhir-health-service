# Kubernetes manifests

Basic manifests for running the API server in a cluster:

- `deployment.yaml` — Deployment with resource requests/limits and readiness/liveness probes against `/health`
- `service.yaml` — ClusterIP Service in front of the pods
- `secret.yaml` — template for `DATABASE_URL` / `KAFKA_BROKER` (placeholder values — see the comment in the file)

## Postgres and Kafka are not included here

`docker-compose.yml` at the repo root runs Postgres and Kafka as plain containers for **local development only**. This directory does not attempt to containerize them for Kubernetes — running stateful Postgres/Kafka as raw pods (no persistent storage strategy, no clustering, no backups) isn't something to reach for here.

A real deployment should point `DATABASE_URL` / `KAFKA_BROKER` (in `secret.yaml`) at:

- **Managed services** — e.g. AWS RDS for Postgres, MSK for Kafka (or GCP Cloud SQL / Confluent Cloud, etc.), or
- **In-cluster instances run via a proper operator/Helm chart** — e.g. the [Bitnami Postgres chart](https://github.com/bitnami/charts/tree/main/bitnami/postgresql) or [Strimzi](https://strimzi.io/) for Kafka — rather than bare Deployments.

## Usage

```bash
docker build -t fhir-health-service:latest .
# push to your registry, then update the image in deployment.yaml

kubectl apply -f k8s/secret.yaml     # or generate via your secret manager, see comment in the file
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
```
