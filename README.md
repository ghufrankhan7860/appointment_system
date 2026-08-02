# Appointment System Backend

This repository is an educational, compile-ready skeleton for a layered Go backend. It
contains package declarations and guidance comments, but no appointment business behavior,
database client, HTTP framework, or external provider implementation yet.

Read these documents before adding code:

1. [`docs/BACKEND_ARCHITECTURE_GUIDELINES.md`](docs/BACKEND_ARCHITECTURE_GUIDELINES.md)
2. [`docs/DEVELOPMENT_WORKFLOW.md`](docs/DEVELOPMENT_WORKFLOW.md)
3. [`docs/ADR-0001-full-project-skeleton.md`](docs/ADR-0001-full-project-skeleton.md)

## Structure

```text
.
|-- main.go                         # Delegates process startup to cmd
|-- cmd/                            # Runtime commands and composition roots
|-- server/                         # HTTP engine, routes, and lifecycle
|-- middleware/                     # Global HTTP middleware
|-- config/                         # Typed configuration and validation
|-- envs/                           # Safe local configuration examples only
|-- internal/
|   |-- apps/appointment/
|   |   |-- handler/                # HTTP binding and response mapping
|   |   |-- controller/             # One application action per use case
|   |   |-- services/               # Business rules and transactions
|   |   |-- repositories/           # Persistence adapters
|   |   |-- models/                 # Domain and persistence representations
|   |   |-- dtos/                   # Transport/application input and output
|   |   |-- interfaces/             # Shared application-owned capabilities
|   |   |-- provider/               # Provider selection and registration
|   |   |-- adapters/               # External provider implementations
|   |   |-- cache/                  # Appointment cache policy
|   |   |-- constant/               # Stable appointment values, sparingly
|   |   `-- container.go            # App dependency declarations
|   `-- platform/
|       |-- datastore/              # Database and migration infrastructure
|       |-- redis/                  # Distributed-cache adapter
|       |-- kafka/                  # Broker adapter
|       |-- network/                # Outbound HTTP policy
|       |-- logger/                 # Structured logging setup
|       `-- telemetry/              # Traces, metrics, and profiling
|-- scripts/
|   |-- consumer/                   # Message consumer loops
|   |-- cron/                       # Scheduled jobs
|   `-- backfill/                   # Restartable data repair/import jobs
|-- migrations/                     # Ordered schema migrations
|-- templates/                      # Email/document templates
|-- tests/                          # Cross-package and end-to-end tests
|-- mocks/                          # Generated mocks only
|-- deploy/                         # Container/orchestration definitions
`-- docs/                           # Architecture and operational decisions
```

## First Commands

```powershell
go test ./...
go vet ./...
```

Replace the temporary module path `appointment_system` in `go.mod` and imports with the
final repository path before publishing the module.
