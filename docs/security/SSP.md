# System Security Plan: FHIR Health Service

This is a sample System Security Plan written for a personal project. The FHIR Health Service has not been assessed or formally authorized, and no authorizing official has reviewed this plan. The system has never processed real patient data.

* Version: 1.0
* Date: November 7, 2026
* Prepared by: Marcus Ashmond
* Control baseline: NIST SP 800-53 Revision 5, Moderate baseline as defined in NIST SP 800-53B
* Companion document: [Plan of Action and Milestones](POAM.md)

## 1. System Identification

* System name: FHIR Health Service
* System abbreviation: FHS
* System owner: Marcus Ashmond
* Information System Security Officer: Marcus Ashmond. One person holds both roles in this sample, which a production system would not allow.
* Authorizing official: None assigned
* Operational status: Under development. Not in production.
* System type: Application. It runs on Amazon Web Services infrastructure, which this plan treats as the underlying general support system.
* Hosting: AWS commercial region. Amazon EKS runs the application, Amazon RDS for PostgreSQL holds the data, and Amazon MSK provides Kafka.
* Source repository: `github.com/marcusashmond/fhir-health-service`

### 1.1 Purpose

The FHIR Health Service stores and serves patient health records modeled on the HL7 FHIR standard. It exposes a REST API for two resource types: Patient, which holds demographics, and Observation, which holds coded clinical measurements such as blood pressure. Each new Observation is published to a Kafka topic. A consumer reads the topic and flags vital sign values that fall outside a normal range for the observation's LOINC code. The intended users are clinical applications that need a shared place to exchange this data.

## 2. System Description

### 2.1 Components

The system is a single Go binary built from [main.go](../../main.go) and the packages under [internal/](../../internal/). It contains four logical components that run in the same process:

* API server. An HTTP server using the chi router. Handlers in [internal/handlers/](../../internal/handlers/) implement create, read, update, delete, and list operations for Patient and Observation, plus a read endpoint for audit records.
* Audit logger. Middleware in [internal/middleware/audit_log.go](../../internal/middleware/audit_log.go) records each request to the PHI and audit routes. Records go to stdout and to the `audit_logs` table through a bounded queue.
* Kafka producer. [internal/kafka/producer.go](../../internal/kafka/producer.go) publishes each created Observation to the `observation.created` topic.
* Anomaly consumer. [internal/kafka/consumer.go](../../internal/kafka/consumer.go) reads `observation.created` in the consumer group `fhir-health-service-anomaly-detector` and logs an alert when a value is out of range. It runs as a goroutine started in `main`, so each API pod is also one member of the consumer group. It is not a separate deployment.

Supporting components:

* PostgreSQL on Amazon RDS. Tables `patients`, `observations`, and `audit_logs` are defined in [migrations/](../../migrations/). Migrations are applied by hand.
* Kafka on Amazon MSK. One topic, `observation.created`.
* Container image built by the multistage [Dockerfile](../../Dockerfile) on a distroless static base. The image runs as the unprivileged `nonroot` user.
* Kubernetes objects in [k8s/](../../k8s/): a Deployment with 2 replicas, a ClusterIP Service, a NetworkPolicy, and a Secret template for the database and broker connection strings.

### 2.2 Authorization Boundary

Inside the boundary:

* The application binary and its container image
* The Deployment, Service, NetworkPolicy, and Secret in the application's EKS namespace
* The FHS database schema and its data in the RDS instance
* The `observation.created` topic and its messages on the MSK cluster
* Application logs written to stdout by the pods

Outside the boundary, with controls inherited or provided by an external party:

* The AWS infrastructure under the boundary: the EKS control plane and worker nodes, the RDS and MSK services, the VPC, IAM, and KMS. These are inherited from AWS.
* GitHub, which hosts the source repository, and GitHub Actions, which runs CI. They are part of the development environment and are covered under the SA and CM controls, not inside the boundary.
* Client applications that call the API. None are defined yet.

The system has no user accounts. Every endpoint accepts anonymous requests today. Three user roles are planned and described under AC-2: clinical client, auditor, and administrator.

### 2.3 Data Flow

1. A client sends an HTTP request to the Kubernetes Service on port 80, which forwards it to a pod on port 8080. Traffic is plain HTTP. TLS is planned (POAM-09).
2. `http.MaxBytesHandler` rejects any request body over 1 MiB. The server enforces read, write, and idle timeouts.
3. Requests to `/Patient`, `/Observation`, and `/audit-logs` pass through the audit middleware. The handler runs first. The middleware then writes one JSON audit line to stdout and places a copy on the audit queue.
4. Handlers call the repository layer, which sends parameterized SQL to PostgreSQL on TCP 5432. The connection string comes from the Kubernetes Secret `fhir-health-service-secrets`. The Secret template sets `sslmode=require`.
5. When an Observation is created and the database write succeeds, the producer publishes the full Observation as JSON to `observation.created` on TCP 9092, keyed by observation ID. The message includes the patient reference and the measured value. The publish is best effort. A failure is logged and the client still receives 201.
6. The consumer reads the message, looks up the normal range for the LOINC code, and if the value is out of range logs `ALERT: abnormal observation <id> (code <code>)` to stdout. The patient reference and the value are not logged.
7. A background goroutine drains the audit queue and inserts each record into `audit_logs`.
8. `GET /audit-logs` returns audit records newest first, 50 per page by default and at most 500. That request is itself audited.
9. The kubelet probes `GET /health` for readiness and liveness. This route is not audited.

### 2.4 Ports, Protocols, and Services

* TCP 8080, HTTP: API listener in the pod. Exposed in the cluster as Service port 80.
* TCP 5432, PostgreSQL wire protocol: pod to RDS. TLS is requested by the deployment template but the server certificate is not verified.
* TCP 9092, Kafka protocol, plaintext: pod to MSK, used by both producer and consumer.
* TCP and UDP 53, DNS: pod to CoreDNS in `kube-system`.

### 2.5 Interconnections

The system has no connections to systems outside the boundary other than the AWS services listed above. No interconnection security agreements are needed.

## 3. Information Types and Security Categorization

### 3.1 Data Types

* Patient demographics (PHI). Name, gender, birth date, postal address, and identifiers such as a medical record number. Stored in the `patients` table. Defined in [internal/models/patient.go](../../internal/models/patient.go).
* Clinical observations (PHI). LOINC coded measurements with a value, unit, effective time, and a reference to the patient. Stored in the `observations` table and carried in full on the `observation.created` topic. Defined in [internal/models/observation.go](../../internal/models/observation.go).
* Audit records. Timestamp, method, path, resource type, resource ID, status code, and client source IP. Stored in `audit_logs` and written to stdout. They hold no clinical content. Resource IDs can still be linked to a patient by anyone with database access.
* Operational logs. Startup, error, and alert lines on stdout. Alert lines carry an observation ID and LOINC code only.
* System credentials. The PostgreSQL connection string, including its password, held in a Kubernetes Secret.

Information types were selected with reference to the health mission area of NIST SP 800-60 Volume II.

### 3.2 FIPS 199 Categorization

Patient demographics and clinical observations:

* Confidentiality: Moderate. Disclosure of identifiable health information is a reportable HIPAA breach. It exposes individuals to embarrassment, discrimination, and identity theft, which is a serious adverse effect. The records are individual patient records, not a population scale data set, so the effect does not rise to severe or catastrophic.
* Integrity: Moderate. A changed blood pressure value or a record attached to the wrong patient could lead a clinician to a wrong decision. Clinicians are expected to confirm critical values at the point of care, so this system is not the only safeguard. The effect is serious but not catastrophic.
* Availability: Moderate. An outage delays access to records and stops abnormal value alerts. Care can continue from source systems during an outage, but alerts would be missed, which is a serious adverse effect.

Audit records: Confidentiality Low, Integrity Moderate, Availability Low. Integrity is the main concern because altered or deleted audit records would hide unauthorized access to PHI.

System credentials: Confidentiality Moderate, Integrity Moderate, Availability Low. The database password grants full access to all PHI.

The overall categorization uses the high water mark across information types:

SC FHIR Health Service = {(confidentiality, MODERATE), (integrity, MODERATE), (availability, MODERATE)}

The Moderate baseline from NIST SP 800-53B applies.

## 4. Control Implementation

### 4.1 Scope and Conventions

Each control below lists its NIST identifier and title, a status, and a statement of how the control is met or what is missing. Statuses mean the following:

* Implemented: The repository contains code or configuration that meets the control. The statement names the file.
* Partially Implemented: Some of the control is met in code or configuration. The gap is described and tracked in the POA&M.
* Planned: Nothing in the system meets the control yet. Tracked in the POA&M.
* Inherited: AWS provides the control through EKS, RDS, MSK, or the underlying infrastructure. Where the protection depends on how the account is configured, the statement says so. That configuration is not in this repository.

This plan covers the AC, AU, IA, SC, SI, CM, CP, RA, and SA families. Within those families it omits the policy and procedure controls (AC-1, AU-1, and so on), because a personal project has no organization to issue policy. It also omits controls that do not apply to a headless API with no interactive sessions or wireless components: AC-8, AC-11, AC-18, AC-19, AC-20, and AC-22. The AT, PE, PL, PS, PM, IR, MA, MP, PT, SR, and CA families are outside the scope of this sample.

### 4.2 Control Status Summary

| Control | Title | Status |
|---|---|---|
| AC-2 | Account Management | Planned |
| AC-3 | Access Enforcement | Planned |
| AC-4 | Information Flow Enforcement | Partially Implemented |
| AC-6 | Least Privilege | Partially Implemented |
| AC-6(9) | Log Use of Privileged Functions | Planned |
| AC-6(10) | Prohibit Non-privileged Users from Executing Privileged Functions | Planned |
| AC-7 | Unsuccessful Logon Attempts | Planned |
| AC-12 | Session Termination | Planned |
| AC-14 | Permitted Actions Without Identification or Authentication | Planned |
| AC-17 | Remote Access | Inherited |
| AU-2 | Event Logging | Partially Implemented |
| AU-3 | Content of Audit Records | Partially Implemented |
| AU-3(1) | Additional Audit Information | Planned |
| AU-4 | Audit Log Storage Capacity | Inherited |
| AU-5 | Response to Audit Logging Process Failures | Partially Implemented |
| AU-6 | Audit Record Review, Analysis, and Reporting | Partially Implemented |
| AU-6(1) | Automated Process Integration | Planned |
| AU-7 | Audit Record Reduction and Report Generation | Partially Implemented |
| AU-8 | Time Stamps | Implemented |
| AU-9 | Protection of Audit Information | Planned |
| AU-11 | Audit Record Retention | Planned |
| AU-12 | Audit Record Generation | Partially Implemented |
| IA-2 | Identification and Authentication (Organizational Users) | Planned |
| IA-2(1) | Multi-factor Authentication to Privileged Accounts | Planned |
| IA-2(2) | Multi-factor Authentication to Non-privileged Accounts | Planned |
| IA-3 | Device Identification and Authentication | Planned |
| IA-4 | Identifier Management | Planned |
| IA-5 | Authenticator Management | Partially Implemented |
| IA-8 | Identification and Authentication (Non-organizational Users) | Planned |
| SC-2 | Separation of System and User Functionality | Planned |
| SC-5 | Denial-of-service Protection | Partially Implemented |
| SC-7 | Boundary Protection | Partially Implemented |
| SC-8 | Transmission Confidentiality and Integrity | Planned |
| SC-8(1) | Cryptographic Protection | Planned |
| SC-10 | Network Disconnect | Implemented |
| SC-12 | Cryptographic Key Establishment and Management | Inherited |
| SC-13 | Cryptographic Protection | Planned |
| SC-23 | Session Authenticity | Planned |
| SC-28 | Protection of Information at Rest | Inherited |
| SC-28(1) | Cryptographic Protection | Inherited |
| SC-39 | Process Isolation | Inherited |
| SI-2 | Flaw Remediation | Partially Implemented |
| SI-2(2) | Automated Flaw Remediation Status | Planned |
| SI-3 | Malicious Code Protection | Planned |
| SI-4 | System Monitoring | Planned |
| SI-5 | Security Alerts, Advisories, and Directives | Planned |
| SI-7 | Software, Firmware, and Information Integrity | Partially Implemented |
| SI-10 | Information Input Validation | Partially Implemented |
| SI-11 | Error Handling | Implemented |
| SI-12 | Information Management and Retention | Planned |
| CM-2 | Baseline Configuration | Partially Implemented |
| CM-3 | Configuration Change Control | Partially Implemented |
| CM-4 | Impact Analyses | Planned |
| CM-5 | Access Restrictions for Change | Planned |
| CM-6 | Configuration Settings | Partially Implemented |
| CM-7 | Least Functionality | Partially Implemented |
| CM-8 | System Component Inventory | Partially Implemented |
| CP-2 | Contingency Plan | Planned |
| CP-4 | Contingency Plan Testing | Planned |
| CP-6 | Alternate Storage Site | Inherited |
| CP-7 | Alternate Processing Site | Inherited |
| CP-9 | System Backup | Inherited |
| CP-10 | System Recovery and Reconstitution | Partially Implemented |
| RA-2 | Security Categorization | Partially Implemented |
| RA-3 | Risk Assessment | Planned |
| RA-5 | Vulnerability Monitoring and Scanning | Partially Implemented |
| RA-5(2) | Update Vulnerabilities to Be Scanned | Implemented |
| RA-5(11) | Public Disclosure Program | Planned |
| RA-7 | Risk Response | Partially Implemented |
| SA-3 | System Development Life Cycle | Partially Implemented |
| SA-5 | System Documentation | Partially Implemented |
| SA-8 | Security and Privacy Engineering Principles | Partially Implemented |
| SA-9 | External System Services | Planned |
| SA-10 | Developer Configuration Management | Partially Implemented |
| SA-11 | Developer Testing and Evaluation | Partially Implemented |
| SA-22 | Unsupported System Components | Partially Implemented |

Totals: 4 Implemented, 29 Partially Implemented, 34 Planned, 9 Inherited.

### 4.3 Access Control (AC)

#### AC-2 Account Management

Status: Planned

The system has no accounts. No code creates, disables, or reviews users, and the README states that authentication and authorization are not built. The plan defines three roles. Clinical clients may read and write Patient and Observation resources. Auditors may read `/audit-logs` and nothing else. Administrators manage the deployment and do not use the API for PHI. Accounts will live in an external OIDC identity provider, and the API will map token claims to these roles. Tracked as POAM-02.

#### AC-3 Access Enforcement

Status: Planned

No route checks who is calling. Any client that can reach port 8080 can read, change, or delete any patient record and can read the full audit trail. Enforcement will be a chi middleware that rejects requests without a valid token and checks the caller's role against the route before the handler runs. Tracked as POAM-02.

#### AC-4 Information Flow Enforcement

Status: Partially Implemented

[k8s/networkpolicy.yaml](../../k8s/networkpolicy.yaml) selects the API pods for both ingress and egress. Ingress is limited to TCP 8080 from pods in the same namespace. Egress is limited to DNS in `kube-system` and to TCP 5432 and 9092 inside one CIDR for RDS and MSK. [k8s/service.yaml](../../k8s/service.yaml) uses type ClusterIP, so the service has no address outside the cluster.

Gaps: EKS enforces NetworkPolicy only when network policy support is turned on in the Amazon VPC CNI, and that setting is not part of this repository. The egress CIDR is a placeholder (`10.0.0.0/16`) that has to be narrowed to the RDS and MSK subnets. Tracked as POAM-12.

#### AC-6 Least Privilege

Status: Partially Implemented

The container runs with minimal privilege. The [Dockerfile](../../Dockerfile) uses `distroless/static-debian12:nonroot` and sets `USER nonroot:nonroot`. [k8s/deployment.yaml](../../k8s/deployment.yaml) sets `runAsNonRoot: true`, `runAsUser: 65532`, `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, drops all Linux capabilities, and applies the `RuntimeDefault` seccomp profile.

Gaps: The application connects to PostgreSQL with one role for every table and every operation, so a flaw in any handler has write access to the audit table. The producer is allowed to create topics (`AllowAutoTopicCreation: true` in [producer.go](../../internal/kafka/producer.go)). The pod mounts the default service account token even though it never calls the Kubernetes API. There are no API roles at all (see AC-2). Tracked as POAM-13.

#### AC-6(9) Log Use of Privileged Functions

Status: Planned

Every request to the PHI and audit routes is logged, deletes included. The system has no notion of a privileged function or privileged user yet, so the records cannot show who used one. Once roles exist, administrative operations will be marked as privileged in the audit record. Tracked as POAM-02.

#### AC-6(10) Prohibit Non-privileged Users from Executing Privileged Functions

Status: Planned

Without roles, any caller can run every function, including deleting patients and reading the audit trail. The role check described under AC-3 will block these functions for callers who lack the right role. Tracked as POAM-02.

#### AC-7 Unsuccessful Logon Attempts

Status: Planned

There is no logon. When OIDC authentication is added, lockout after repeated failures will be enforced by the identity provider. The API will log each rejected token as an audit event. Tracked as POAM-01.

#### AC-12 Session Termination

Status: Planned

The API is stateless and has no sessions. The plan is to accept only short lived access tokens (15 minutes or less) and reject expired tokens on every request, which ends a session when its token expires. Tracked as POAM-01.

#### AC-14 Permitted Actions Without Identification or Authentication

Status: Planned

Today every action is permitted without identification, which is the opposite of what this control intends. The only action meant to stay anonymous is `GET /health`, which the kubelet uses for probes and which returns no data. All other routes will require authentication. Tracked as POAM-01.

#### AC-17 Remote Access

Status: Inherited

All administrative access is remote and goes through AWS. Operators reach the cluster through the EKS API, which authenticates with AWS IAM, and they manage RDS and MSK through the AWS APIs. The design has no SSH access to worker nodes or bastion hosts. MFA on IAM identities and the EKS access entries are part of the AWS account configuration, which is not in this repository.

### 4.4 Audit and Accountability (AU)

#### AU-2 Event Logging

Status: Partially Implemented

The audit middleware in [internal/middleware/audit_log.go](../../internal/middleware/audit_log.go) logs every request to `/Patient`, `/Observation`, and `/audit-logs`, including failed ones. [main.go](../../main.go) attaches it with `r.Use(auditLogger.Middleware)` on the Patient and Observation route groups and `r.With(auditLogger.Middleware)` on the audit route. This covers every read, create, update, and delete of PHI and every read of the audit trail. `TestAuditLogger_Middleware_RecordsRequest` in [audit_log_test.go](../../internal/middleware/audit_log_test.go) verifies each of these route types.

Gaps: There are no authentication events because there is no authentication. The Kafka consumer does not write audit records when it processes a message. The event list has not been reviewed with anyone outside the project. Tracked as POAM-05.

#### AU-3 Content of Audit Records

Status: Partially Implemented

Each record, defined in [internal/models/audit_log.go](../../internal/models/audit_log.go), contains:

* What happened: method, path, resource type
* When: UTC timestamp
* Where: the path and resource ID
* Source: client IP, taken from the TCP peer address by `sourceIP`. The `X-Forwarded-For` header is ignored on purpose, because no trusted proxy is configured and any client could set it.
* Outcome: HTTP status code

Gap: The record does not identify the user, because the system has no users. Tracked as POAM-03.

#### AU-3(1) Additional Audit Information

Status: Planned

The plan is to add the authenticated subject, the role used, a request ID that ties the audit record to application logs, and the user agent. Tracked as POAM-03.

#### AU-4 Audit Log Storage Capacity

Status: Inherited

Audit records persist in the `audit_logs` table on RDS. RDS storage autoscaling grows the volume as it fills, when autoscaling is enabled on the instance. The application side has a fixed bound: the in memory queue holds at most 1,000 records (`auditQueueSize`). Stdout copies of audit records sit on the node only until kubelet log rotation removes them, so the database is the durable store until log shipping is in place (POAM-06).

#### AU-5 Response to Audit Logging Process Failures

Status: Partially Implemented

Audit failures do not stop request handling and are logged. If the database insert fails, the background writer logs `audit log write failed`. If the queue is full, the middleware logs `audit log queue full, dropping record` and moves on without blocking the request. The JSON audit line is written to stdout before the record is queued, so a stdout copy exists even when the database copy is lost. `TestAuditLogger_Middleware_InsertFailureIsLogged` and `TestAuditLogger_Middleware_FullQueueDropsWithoutBlocking` cover both paths.

Gaps: Nobody is alerted when these messages appear. Dropped database records are not retried. The design favors availability of the API over completeness of the database copy, and that trade has not been accepted by anyone with authority to accept it. Tracked as POAM-04.

#### AU-6 Audit Record Review, Analysis, and Reporting

Status: Partially Implemented

[internal/handlers/audit_log_handler.go](../../internal/handlers/audit_log_handler.go) serves `GET /audit-logs` so a reviewer can page through records newest first. There is no review schedule, no named reviewer, and no defined list of findings that require a report. Tracked as POAM-06.

#### AU-6(1) Automated Process Integration

Status: Planned

Audit records are not sent to any central log service or SIEM. The plan is to ship pod stdout to Amazon CloudWatch Logs with Fluent Bit and to alarm on audit write failures, dropped records, and spikes in 4xx responses. Tracked as POAM-06.

#### AU-7 Audit Record Reduction and Report Generation

Status: Partially Implemented

`GET /audit-logs` supports `limit` and `offset`, capped at 500 records per page, and the table has an index on `timestamp`. It does not filter by time range, resource, status, or source IP, so a reviewer cannot narrow the trail through the API. Tracked as POAM-06.

#### AU-8 Time Stamps

Status: Implemented

The middleware stamps each record with `time.Now().UTC()`, and the `audit_logs.timestamp` column is `TIMESTAMPTZ`, which stores microsecond precision. `TestAuditLogger_Middleware_RecordsRequest` checks that the timestamp is in UTC. Clock synchronization on the EKS worker nodes comes from the Amazon Time Sync Service.

#### AU-9 Protection of Audit Information

Status: Planned

Audit records are not protected today. `GET /audit-logs` is open to any client. The application's database role can update and delete rows in `audit_logs`. Stdout copies have no integrity protection. Auditing the audit endpoint (AU-2) shows who read the trail but does not stop anyone. The plan is to restrict the endpoint to the auditor role, write audit records through a database role that has INSERT and SELECT only on that table, and keep a second copy in CloudWatch Logs encrypted with KMS. Tracked as POAM-07.

#### AU-11 Audit Record Retention

Status: Planned

No retention period is defined. Nothing deletes old records, and nothing makes sure they are kept. The plan sets six years, which matches the HIPAA documentation retention requirement, applied to both the database table and the CloudWatch log group. Tracked as POAM-08.

#### AU-12 Audit Record Generation

Status: Partially Implemented

The application generates audit records for the events in AU-2 on every API pod. `TestAuditLogger_Close_DrainsQueue` confirms that queued records are written during shutdown, and `main` calls `auditLogger.Close()` before the database pool closes.

Gaps: The set of audited routes is fixed in code, so changing which events are logged needs a code change and a deploy. The consumer generates no audit records. Tracked as POAM-05.

### 4.5 Identification and Authentication (IA)

#### IA-2 Identification and Authentication (Organizational Users)

Status: Planned

The API does not identify or authenticate anyone. The plan is to validate OIDC bearer tokens (JWT) on every request except `/health`, checking signature, issuer, audience, and expiry against the identity provider's published keys. Tracked as POAM-01.

#### IA-2(1) Multi-factor Authentication to Privileged Accounts

Status: Planned

Administrators and auditors will be required to use MFA at the identity provider before a token is issued. Tracked as POAM-01.

#### IA-2(2) Multi-factor Authentication to Non-privileged Accounts

Status: Planned

Human users of clinical client applications will also be required to use MFA at the identity provider. Service clients will use the OAuth client credentials grant with private key JWT client authentication. Tracked as POAM-01.

#### IA-3 Device Identification and Authentication

Status: Planned

The pods do not authenticate to Kafka at all. They authenticate to PostgreSQL with a password, but they do not verify the server's certificate. The plan is MSK IAM authentication on port 9098, or mutual TLS on 9094, for Kafka, and `sslmode=verify-full` with the RDS certificate bundle for PostgreSQL. Tracked as POAM-10.

#### IA-4 Identifier Management

Status: Planned

There are no user identifiers. User identifiers will be issued and retired by the identity provider, and the API will record the token subject in audit records. Tracked as POAM-01.

#### IA-5 Authenticator Management

Status: Partially Implemented

The application holds no credentials in code. [main.go](../../main.go) reads `DATABASE_URL` and `KAFKA_BROKER` from the environment and refuses to start if either is missing. In Kubernetes these come from the Secret `fhir-health-service-secrets` through `envFrom`. [.gitignore](../../.gitignore) and [.dockerignore](../../.dockerignore) keep `.env` files out of the repository and the image. The comment in [k8s/secret.yaml](../../k8s/secret.yaml) says to generate the Secret from a secret manager and never commit real values.

Gaps: The database password is static and never rotated. Kafka has no authenticator. Kubernetes Secrets are stored base64 encoded unless EKS envelope encryption with KMS is in use. [docker-compose.yml](../../docker-compose.yml) uses a fixed password, which is acceptable only because that file is for local development. Tracked as POAM-11.

#### IA-8 Identification and Authentication (Non-organizational Users)

Status: Planned

External clients, such as partner clinical applications, will authenticate through the same OIDC provider using the client credentials grant. No external users exist today. Tracked as POAM-01.

### 4.6 System and Communications Protection (SC)

#### SC-2 Separation of System and User Functionality

Status: Planned

The audit review function is served by the same router and on the same port as the clinical API, with no difference in who can reach it. Separation will come from role checks (AC-3) and, later, from moving `/audit-logs` behind a separate internal listener. Tracked as POAM-02.

#### SC-5 Denial-of-service Protection

Status: Partially Implemented

These protections are in place:

* The HTTP server in [main.go](../../main.go) sets `ReadTimeout` to 10 seconds, `WriteTimeout` to 15 seconds, and `IdleTimeout` to 60 seconds. This limits slow client attacks that hold connections open.
* `http.MaxBytesHandler` caps every request body at 1 MiB (`maxRequestBodyBytes`).
* `/audit-logs` caps a page at 500 records.
* The audit queue is bounded and never blocks the request path.
* [k8s/deployment.yaml](../../k8s/deployment.yaml) sets CPU and memory limits (500m and 256Mi), so one pod cannot starve the node.

Gaps: There is no rate limiting. `GET /Patient` returns every patient in one response with no pagination, so its cost grows with the table. Tracked as POAM-14.

#### SC-7 Boundary Protection

Status: Partially Implemented

The service is reachable only inside the cluster (ClusterIP), and the NetworkPolicy described under AC-4 denies all other ingress and egress for the API pods. Because the policy selects the pods for both directions, any traffic it does not list is denied. VPC security groups and subnets around RDS and MSK are inherited from AWS.

Gaps: No external entry point is defined yet, so there is no load balancer, TLS termination point, or AWS WAF in front of the API. NetworkPolicy enforcement on EKS has to be switched on. Tracked as POAM-12.

#### SC-8 Transmission Confidentiality and Integrity

Status: Planned

PHI moves in clear text on two of three paths. The API serves plain HTTP. The Kafka producer and consumer use a plaintext listener with no TLS settings in [producer.go](../../internal/kafka/producer.go) or [consumer.go](../../internal/kafka/consumer.go), and each message carries a full Observation. The database template uses `sslmode=require`, which encrypts but does not check the server certificate, and [.env.example](../../.env.example) uses `sslmode=disable` for local development. Tracked as POAM-09 for HTTP and PostgreSQL and POAM-10 for Kafka.

#### SC-8(1) Cryptographic Protection

Status: Planned

The plan is TLS 1.2 or later on every path. The API will sit behind an AWS Application Load Balancer with an ACM certificate, with TLS between the load balancer and the pods. PostgreSQL will use `sslmode=verify-full`, and Kafka will use the MSK TLS listener. Tracked as POAM-09 and POAM-10.

#### SC-10 Network Disconnect

Status: Implemented

`IdleTimeout: 60 * time.Second` on the HTTP server in [main.go](../../main.go) closes any keep alive connection that has been idle for 60 seconds. `ReadTimeout` closes connections that stop sending a request part way through. The API has no other kind of session.

#### SC-12 Cryptographic Key Establishment and Management

Status: Inherited

Keys that protect data at rest in RDS and MSK are created and stored in AWS KMS, which also handles rotation for customer managed keys when rotation is enabled. TLS certificates for the planned load balancer will come from AWS Certificate Manager. The application itself creates and stores no keys.

#### SC-13 Cryptographic Protection

Status: Planned

The application does no cryptography that protects PHI today. `crypto/rand` is used only to generate resource IDs. Once TLS is added, the binary should be built with `GOFIPS140` so it uses the Go Cryptographic Module. Before the system claims FIPS validated cryptography, the module's CMVP certificate status has to be confirmed. Tracked as POAM-09.

#### SC-23 Session Authenticity

Status: Planned

Without TLS, a client cannot verify it is talking to the real service, and responses can be changed in transit. TLS (SC-8) and signed tokens (IA-2) together will provide this. Tracked as POAM-09.

#### SC-28 Protection of Information at Rest

Status: Inherited

PHI at rest lives in RDS and MSK. RDS encrypts storage, snapshots, and backups with KMS when encryption is chosen at instance creation, and it cannot be added later without a snapshot and restore. MSK encrypts data at rest with KMS by default. The application writes nothing to local disk, and the container root filesystem is read only.

#### SC-28(1) Cryptographic Protection

Status: Inherited

RDS and MSK use 256 bit AES through KMS for data at rest, as described under SC-28.

#### SC-39 Process Isolation

Status: Inherited

Each pod runs in its own Linux namespaces under the containerd runtime on EKS worker nodes. The pod security context adds a non root user, no added capabilities, and the default seccomp profile, which further limits what the process can reach.

### 4.7 System and Information Integrity (SI)

#### SI-2 Flaw Remediation

Status: Partially Implemented

[.github/workflows/ci.yml](../../.github/workflows/ci.yml) runs `govulncheck` version 1.9.0 on every push and pull request to `main`, and the job fails if a known vulnerability is reachable from the code. When that step was added, it found 14 reachable vulnerabilities in the Go standard library, `klauspost/compress`, and `golang.org/x/text`. They were fixed by moving to Go 1.26.10 and updating both modules, and the scan is now clean.

Gaps: Scans run only when code changes, so a new advisory against an unchanged release goes unnoticed. The container base images are not scanned. No remediation time frame is defined. Tracked as POAM-15.

#### SI-2(2) Automated Flaw Remediation Status

Status: Planned

Nothing reports the flaw status of deployed images. The plan adds a weekly scheduled `govulncheck` run and Dependabot for Go modules and GitHub Actions. Amazon Inspector will scan images in ECR. Tracked as POAM-15.

#### SI-3 Malicious Code Protection

Status: Planned

The distroless image has no shell or package manager, which leaves little room for malicious code to run. That is not detection. The plan is Amazon GuardDuty Runtime Monitoring for EKS and Amazon Inspector scanning of images in ECR. Tracked as POAM-06.

#### SI-4 System Monitoring

Status: Planned

The system is not monitored for attacks or unauthorized use. The anomaly consumer looks for abnormal vital signs. It is a clinical feature and gives no security monitoring. Readiness and liveness probes detect a dead process, not misuse. Monitoring will come from CloudWatch alarms on the audit stream and from GuardDuty EKS Protection. Tracked as POAM-06.

#### SI-5 Security Alerts, Advisories, and Directives

Status: Planned

No one is subscribed to security advisories for the components this system uses. The plan is to turn on GitHub security advisories and Dependabot alerts for the repository and to subscribe to the Go, PostgreSQL, and AWS security bulletins. Tracked as POAM-15.

#### SI-7 Software, Firmware, and Information Integrity

Status: Partially Implemented

Go module downloads are checked against `go.sum` and the public Go checksum database, so a changed dependency fails the build. The runtime image contains only the static binary.

Gaps: The base images (`golang:1.26-alpine` and `distroless/static-debian12:nonroot`) are pulled by tag, not digest. The Deployment uses the mutable tag `fhir-health-service:latest`. Images are not signed, and GitHub Actions are pinned by version tag rather than commit SHA. Tracked as POAM-16.

#### SI-10 Information Input Validation

Status: Partially Implemented

These checks are in place:

* Every handler decodes JSON with `encoding/json` and returns 400 on malformed input.
* `validateSubject` in [observation_handler.go](../../internal/handlers/observation_handler.go) rejects an Observation whose `subject.reference` is empty or names a patient that does not exist. The foreign key on `observations.subject_patient_id` enforces the same rule in the database.
* All SQL in `internal/repository/postgres_*.go` uses `$n` bind parameters, which prevents SQL injection.
* The audit handler rejects a `limit` or `offset` that is not a valid integer.
* Bodies over 1 MiB are rejected.

Gaps: Field values are not validated. Gender, status, and date formats are passed through to PostgreSQL, where a bad date becomes a 500 instead of a 400. Unknown JSON fields are silently ignored. Tracked as POAM-17.

#### SI-11 Error Handling

Status: Implemented

Clients receive short fixed messages such as `failed to create patient` or `patient not found` through `writeError` in [patient_handler.go](../../internal/handlers/patient_handler.go). Database errors, stack details, and SQL never reach the response. Internal detail goes to the server log only. The anomaly alert in `checkAnomaly` logs the observation ID and LOINC code and leaves out the patient reference and value. `TestCheckAnomaly_AlertOmitsPHI` in [consumer_test.go](../../internal/kafka/consumer_test.go) checks this.

#### SI-12 Information Management and Retention

Status: Planned

No retention or disposal rule exists for PHI. Deletes through the API are permanent, with no record of what was removed beyond the audit entry. Observation messages stay on the Kafka topic for the broker's default retention period, which this system does not set. Tracked as POAM-08.

### 4.8 Configuration Management (CM)

#### CM-2 Baseline Configuration

Status: Partially Implemented

The baseline is kept in git. It consists of the [Dockerfile](../../Dockerfile), the manifests in [k8s/](../../k8s/), the SQL in [migrations/](../../migrations/), and the dependency versions in `go.mod` and `go.sum`. The Go version is fixed at 1.26.10 in `go.mod`, and CI builds with that version.

Gaps: Deployments use the `latest` image tag, so the running version cannot be tied to a commit. Base images are not pinned by digest. Tracked as POAM-16.

#### CM-3 Configuration Change Control

Status: Partially Implemented

All changes go through git, and CI builds, vets, checks formatting, tests, and scans every push and pull request to `main`. Gaps: Branch protection is not configured, changes have been pushed straight to `main`, and no second person reviews them. Tracked as POAM-18.

#### CM-4 Impact Analyses

Status: Planned

Changes are not reviewed for security impact before merge. A pull request template with a security impact section will be added and required for changes to authentication, audit, network, or data handling code. Tracked as POAM-18.

#### CM-5 Access Restrictions for Change

Status: Planned

The repository has one owner and no rules about who may merge. The cluster has no documented limits on who may apply manifests. Branch protection, CODEOWNERS, and EKS access entries that limit `kubectl apply` in the namespace to a deploy role will address this. Tracked as POAM-18.

#### CM-6 Configuration Settings

Status: Partially Implemented

Security relevant settings are set in code and manifests and reviewed with the code: server timeouts and body limit in [main.go](../../main.go), the pod and container security context and resource limits in [k8s/deployment.yaml](../../k8s/deployment.yaml), and the NetworkPolicy. Gap: The cluster is not checked against a published benchmark such as the CIS Amazon EKS Benchmark. Tracked as POAM-19.

#### CM-7 Least Functionality

Status: Partially Implemented

The runtime image has no shell, package manager, or other binaries. The pod exposes one port. The router serves only the routes registered in `main`, and anything else returns 404.

Gaps: The producer can create Kafka topics. The pod mounts a service account token it does not use. Tracked as POAM-13.

#### CM-8 System Component Inventory

Status: Partially Implemented

`go.mod` and `go.sum` list every Go module and version. The Dockerfile names both base images, and section 2.1 lists the runtime components. Gap: No SBOM is produced for the image, so packages from the base layers are not inventoried. Tracked as POAM-19.

### 4.9 Contingency Planning (CP)

#### CP-2 Contingency Plan

Status: Planned

There is no contingency plan. It will define recovery objectives, restore steps for RDS, redeploy steps for the application, and who does what. Tracked as POAM-20.

#### CP-4 Contingency Plan Testing

Status: Planned

Restores have never been tested. The plan calls for a yearly test that restores RDS to a point in time in a separate environment and runs the application against it. Tracked as POAM-20.

#### CP-6 Alternate Storage Site

Status: Inherited

RDS automated backups and snapshots are stored by AWS in Amazon S3, separate from the database instance. Copying snapshots to a second region is an account setting and is not configured here.

#### CP-7 Alternate Processing Site

Status: Inherited

EKS node groups and RDS can run across several Availability Zones in a region. This gives an alternate processing location when node groups span at least two zones and RDS runs as a Multi AZ deployment. Both are account settings outside this repository.

#### CP-9 System Backup

Status: Inherited

RDS automated backups provide daily snapshots and point in time recovery for 1 to 35 days, depending on the configured retention. The database is the system of record. Kafka messages are transient copies of Observations already stored in PostgreSQL, so they are not backed up. The application is rebuilt from git rather than backed up.

#### CP-10 System Recovery and Reconstitution

Status: Partially Implemented

The application holds no state and can be redeployed from the Dockerfile and manifests. On SIGTERM, `main` stops the HTTP server, stops the consumer, and drains the audit queue before exit. The Deployment runs 2 replicas, and the liveness probe restarts a pod that stops answering.

Gaps: Migrations are applied by hand with `psql`, so rebuilding the schema is a manual step. The Deployment has no PodDisruptionBudget or topology spread, so both replicas can land on one node. Tracked as POAM-20.

### 4.10 Risk Assessment (RA)

#### RA-2 Security Categorization

Status: Partially Implemented

Section 3 of this plan documents the information types and the FIPS 199 categorization with its rationale. Gap: No authorizing official has reviewed or approved it. Tracked as POAM-21.

#### RA-3 Risk Assessment

Status: Planned

No formal risk assessment or threat model exists. The gap analysis in this plan is a starting point. The plan is a STRIDE threat model of the data flow in section 2.3 and a risk assessment following NIST SP 800-30. Tracked as POAM-22.

#### RA-5 Vulnerability Monitoring and Scanning

Status: Partially Implemented

`govulncheck` scans the Go code and its dependencies in CI and reports only vulnerabilities that the code can actually reach. Gaps: Container images and the cluster configuration are not scanned, and scans do not run on a schedule. Tracked as POAM-15.

#### RA-5(2) Update Vulnerabilities to Be Scanned

Status: Implemented

`govulncheck` downloads the current Go vulnerability database from `vuln.go.dev` on every run, so each CI scan checks against the latest published entries.

#### RA-5(11) Public Disclosure Program

Status: Planned

The repository has no `SECURITY.md` and no way to report a vulnerability privately. A security policy will be added, and GitHub private vulnerability reporting will be turned on. Tracked as POAM-23.

#### RA-7 Risk Response

Status: Partially Implemented

Weaknesses found while writing this plan are recorded in [POAM.md](POAM.md), each with a fix and a target date. Weaknesses fixed before the POA&M was opened are listed at the end of that document. Gap: There is no set schedule for reviewing and updating the POA&M. Tracked as POAM-21.

### 4.11 System and Services Acquisition (SA)

#### SA-3 System Development Life Cycle

Status: Partially Implemented

Development follows a simple life cycle. Changes are made in git, checked by CI, and deployed from a container image. Security checks in the life cycle are `go vet`, the test suite, and `govulncheck`. Gap: Security activities such as threat modeling and security review are not defined for each stage. Tracked as POAM-22.

#### SA-5 System Documentation

Status: Partially Implemented

The [README](../../README.md) covers architecture, setup, the audit log, and deployment. [k8s/README.md](../../k8s/README.md) covers the manifests, including the NetworkPolicy prerequisites. This plan describes the security design. Gaps: There is no API reference and no administrator guide covering secure configuration of the AWS services. Tracked as POAM-24.

#### SA-8 Security and Privacy Engineering Principles

Status: Partially Implemented

Several principles show up in the code. Least privilege in the container (AC-6). Minimal attack surface through a distroless image (CM-7). Bounded resources on the request path (SC-5). Data minimization in logs, where alerts carry IDs, not PHI (SI-11). Default deny networking (SC-7). Gap: The design has never had a security review, and authentication, the most basic principle for PHI, is missing. Tracked as POAM-22.

#### SA-9 External System Services

Status: Planned

The system depends on AWS for EKS, RDS, MSK, and KMS. All four are HIPAA eligible services. A Business Associate Addendum with AWS has to be accepted through AWS Artifact before any PHI is stored, and the AWS FedRAMP package covering these services has to be recorded as the basis for inherited controls. Neither has been done. Tracked as POAM-25.

#### SA-10 Developer Configuration Management

Status: Partially Implemented

All source, manifests, and migrations are under git version control, and `go.sum` locks dependency content. Gaps: Commits are not signed, and there is no branch protection. Tracked as POAM-18.

#### SA-11 Developer Testing and Evaluation

Status: Partially Implemented

CI runs unit tests for the handlers, the Kafka consumer, and the audit middleware. It also runs integration tests for the repositories against a real PostgreSQL 16 container started with testcontainers, using the project migrations. The middleware tests cover record content, source IP handling, `X-Forwarded-For` rejection, insert failures, and the queue full path. `go vet` and `govulncheck` run on every change. Gaps: No static application security testing tool such as gosec or CodeQL runs, and there is no coverage target. Tracked as POAM-26.

#### SA-22 Unsupported System Components

Status: Partially Implemented

The runtime components are on supported versions: Go 1.26.10, PostgreSQL 16, and a current distroless base. Gap: [docker-compose.yml](../../docker-compose.yml) runs Kafka from `bitnamilegacy/kafka`, an image repository that no longer receives updates. It is used only for local development but should be replaced. Tracked as POAM-27.

## 5. Plan of Action and Milestones

Every control marked Partially Implemented or Planned is tracked in [POAM.md](POAM.md) with its weakness, planned fix, milestones, and target date.
