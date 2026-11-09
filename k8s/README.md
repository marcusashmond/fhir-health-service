# Kubernetes manifests

Basic manifests for running the API server in a cluster:

- `deployment.yaml`: Deployment with resource requests and limits, and readiness and liveness probes against `/health`. Pods run as UID 65532 with a read only root filesystem, all capabilities dropped, privilege escalation disabled, and no service account token mounted.
- `service.yaml`: ClusterIP Service in front of the pods
- `networkpolicy.yaml`: limits ingress to port 8080 from the same namespace and egress to DNS, PostgreSQL, and Kafka. Set the CIDR in the file to your RDS and MSK subnets. On EKS, enforcement requires network policy support in the VPC CNI or another policy engine.
- `secret.yaml`: template for `DATABASE_URL` and `KAFKA_BROKER`. The values are placeholders. See the comment in the file.

## Postgres and Kafka are not included here

`docker-compose.yml` at the repo root runs Postgres and Kafka as plain containers for local development only. This directory does not run them in Kubernetes. Stateful Postgres and Kafka as bare pods would have no persistent storage strategy, no clustering, and no backups.

A real deployment should point `DATABASE_URL` / `KAFKA_BROKER` (in `secret.yaml`) at:

- Managed services, such as Amazon RDS for Postgres and Amazon MSK for Kafka (or GCP Cloud SQL, Confluent Cloud, and similar), or
- Instances in the cluster run by an operator or Helm chart, such as the [Bitnami Postgres chart](https://github.com/bitnami/charts/tree/main/bitnami/postgresql) or [Strimzi](https://strimzi.io/) for Kafka, rather than bare Deployments.

Create the `observation.created` topic before deploying. The app does not create topics, and MSK has topic auto creation turned off by default.

## Usage

```bash
docker build -t fhir-health-service:latest .
# push to your registry, then update the image in deployment.yaml

kubectl apply -f k8s/secret.yaml     # or generate via your secret manager, see comment in the file
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
kubectl apply -f k8s/networkpolicy.yaml
```
