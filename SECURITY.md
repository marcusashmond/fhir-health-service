# Security Policy

## Reporting a Vulnerability

Report vulnerabilities privately through GitHub. Open the repository's Security tab and choose "Report a vulnerability." Please do not open a public issue for a security problem.

Include the affected endpoint or file, steps to reproduce, and the impact you expect. If the issue involves patient data, describe it with made up records. Never send real PHI.

## Response Times

* Acknowledgment: within 3 business days
* Initial assessment and severity rating: within 10 business days
* Fix target from the date severity is confirmed: 15 days for critical, 30 days for high, 90 days for moderate, and 180 days for low

You will get an update at least every 14 days until the report is closed. If you want credit in the advisory, say so in the report.

## Supported Versions

Only the latest commit on `main` receives security fixes.

## Scope

This is a personal project and has not been formally authorized to hold real patient data. Reports about the application code, the Dockerfile, the Kubernetes manifests, and the CI workflows are in scope. Weaknesses already listed in [docs/security/POAM.md](docs/security/POAM.md) are known, but reports that show a way to exploit them are still welcome.
