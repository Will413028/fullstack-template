## ADDED Requirements

### Requirement: All deployments MUST have resource requests and limits
Every container in every Deployment manifest SHALL define `resources.requests` and `resources.limits` for both CPU and memory.

#### Scenario: Backend deployment has resource constraints
- **WHEN** the backend deployment YAML is applied
- **THEN** the backend container has CPU request 100m, CPU limit 500m, memory request 128Mi, memory limit 512Mi

#### Scenario: Frontend deployment has resource constraints
- **WHEN** the frontend deployment YAML is applied
- **THEN** the frontend container has CPU request 100m, CPU limit 300m, memory request 128Mi, memory limit 384Mi

#### Scenario: Postgres deployment has resource constraints
- **WHEN** the postgres deployment YAML is applied
- **THEN** the postgres container has CPU request 200m, CPU limit 1000m, memory request 256Mi, memory limit 1Gi

#### Scenario: Backend init container has resource constraints
- **WHEN** the backend deployment YAML is applied
- **THEN** the migrate init container has CPU request 100m, CPU limit 300m, memory request 128Mi, memory limit 256Mi

### Requirement: All deployments MUST have securityContext
Every pod and container SHALL run with least-privilege securityContext settings.

#### Scenario: Backend runs as non-root with dropped capabilities
- **WHEN** the backend pod is running
- **THEN** it runs with runAsNonRoot: true, runAsUser: 1001, allowPrivilegeEscalation: false, capabilities drop ALL

#### Scenario: Frontend runs as non-root with dropped capabilities
- **WHEN** the frontend pod is running
- **THEN** it runs with runAsNonRoot: true, runAsUser: 1001, allowPrivilegeEscalation: false, capabilities drop ALL

#### Scenario: Postgres runs as postgres user
- **WHEN** the postgres pod is running
- **THEN** it runs with runAsNonRoot: true, runAsUser: 999, allowPrivilegeEscalation: false, capabilities drop ALL

### Requirement: Frontend deployment MUST have liveness probe
The frontend deployment SHALL include a liveness probe in addition to the existing readiness probe.

#### Scenario: Frontend liveness probe configured
- **WHEN** the frontend deployment YAML is applied
- **THEN** a liveness probe exists with httpGet path /, port 3000, and appropriate timeouts

### Requirement: All probes MUST have timeoutSeconds and failureThreshold
Every readiness and liveness probe SHALL specify timeoutSeconds and failureThreshold to prevent silent hangs.

#### Scenario: Backend probes have timeout and failure threshold
- **WHEN** the backend deployment probes are inspected
- **THEN** each probe has timeoutSeconds and failureThreshold defined

#### Scenario: Frontend probes have timeout and failure threshold
- **WHEN** the frontend deployment probes are inspected
- **THEN** each probe has timeoutSeconds and failureThreshold defined

#### Scenario: Postgres probes have timeout and failure threshold
- **WHEN** the postgres deployment probes are inspected
- **THEN** each probe has timeoutSeconds and failureThreshold defined

### Requirement: PVC MUST specify storageClassName
The postgres PVC SHALL explicitly specify `storageClassName: local-path` for k3d compatibility and cloud-readiness.

#### Scenario: PVC has explicit storage class
- **WHEN** the postgres PVC YAML is applied
- **THEN** storageClassName is set to local-path
