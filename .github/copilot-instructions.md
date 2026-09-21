# Project Guidelines

This repository follows [the Go backend architecture guide](../docs/BACKEND_ARCHITECTURE_GUIDELINES.md).
Read the relevant section before introducing a package, dependency, runtime mode, or new
integration pattern.

## Working Method

- Before editing, identify the owning layer and inspect one nearby implementation, test, or
  composition root that demonstrates the local convention.
- Make the smallest complete change. Do not mix feature work with unrelated refactoring.
- Preserve public contracts unless the task explicitly requires a breaking change.
- Work with existing uncommitted changes and never discard changes you did not create.
- When a requirement conflicts with this guide, state the tradeoff and document an intentional
  architecture exception rather than silently creating a second pattern.

## Architecture

- Keep dependencies pointing inward: delivery/infrastructure -> application -> domain.
- HTTP requests use handler -> controller/use case -> service -> repository.
- Handlers bind and validate transport input, call a controller, and map output to HTTP.
- Controllers/use cases orchestrate one application action.
- Services contain business rules, coordinate ports, and own transaction boundaries.
- Repositories perform persistence and translate database records/errors; they do not contain
  business policy or call external services.
- Keep Gin, GORM, Redis, Kafka, Viper, and vendor SDK types out of domain and service APIs.
- Define interfaces around behavior consumed by a package. Do not create interfaces only to
  mirror concrete implementations.
- Select concrete implementations and wire constructors only in composition roots.
- Pass narrow dependencies into constructors; do not pass a broad container or request-scoped
  service locator.
- Request-scoped state may contain context, identity/tenant, correlation IDs, and an enriched
  logger, but not every service or raw infrastructure client.
- Use provider adapters for interchangeable external systems. Keep provider DTOs inside their
  adapter and prohibit provider-specific branching in shared workflows.

## Go Conventions

- Run `gofmt` on changed Go files and follow standard package naming.
- Use Go initialisms: `ID`, `HTTP`, `URL`, `API`, and `JSON`.
- Put `context.Context` first in functions that perform I/O or application work. Propagate it
  to database, cache, broker, and outbound HTTP calls; never store it in application-scoped
  structs.
- Use constructors for required dependencies and invariants. Validate dependencies at startup.
- Return typed or sentinel application errors and wrap causes with `%w`. Map errors to HTTP,
  broker retry, or process exit behavior only at the relevant delivery boundary.
- Do not compare error strings, ignore errors, panic for expected failures, or perform network
  initialization in `init()`.
- Keep API DTOs, domain types, persistence models, event schemas, and provider wire DTOs
  separate when their responsibilities differ.
- Avoid mutable globals. A justified in-memory cache must have narrow ownership, synchronization,
  bounded memory, and documented refresh behavior.
- Add concise comments only for exported contracts and non-obvious constraints.

## Data and Integrations

- Parameterize queries and allow-list dynamic identifiers or sort fields.
- Services define transactions; repositories accept a transaction/query abstraction.
- Add ordered migrations for schema changes and separate resumable backfills for large data
  changes.
- Document cache key, scope, TTL, invalidation, staleness, and failure behavior.
- Treat message delivery as at least once. Consumers must be idempotent and define validation,
  retry, dead-letter, commit, cancellation, and duplicate behavior.
- Use an outbox or equivalent when database state and event publication must be atomic.
- Bound external I/O with timeouts. Retries must be limited, jittered where appropriate,
  idempotency-aware, context-aware, and observable.
- Never commit secrets or log credentials, tokens, personal data, or raw sensitive payloads.

## Feature Placement

| Change            | Primary location                                 | Also update                                                   |
| ----------------- | ------------------------------------------------ | ------------------------------------------------------------- |
| HTTP endpoint     | `internal/apps/<app>/handler` and `controller`   | Route, service/repository if needed, DI, tests, API docs      |
| Business rule     | `internal/apps/<app>/services` or domain package | Consumed ports, constructor wiring, unit tests                |
| Database behavior | `internal/apps/<app>/repositories`               | Port, integration test, migration when schema changes         |
| External provider | `internal/apps/<app>/adapters/<provider>`        | Capability registry, typed config, contract tests, telemetry  |
| Consumer          | `scripts/consumer`                               | Event schema, Cobra command, DI, retry/DLQ policy, deployment |
| Cron/backfill     | `scripts/cron` or `scripts/backfill`             | Cobra command, checkpoint/overlap policy, deployment, tests   |
| Technical adapter | `internal/platform/<capability>`                 | Application-owned port, config, health/telemetry              |

Create only packages needed by the change. If a layer becomes large, split it by business
capability while preserving the dependency rules.

## Testing and Completion

- Add tests proportional to behavioral risk: table-driven domain tests, service tests for
  orchestration, repository integration tests for queries, `httptest` handler tests, provider
  contract tests, and consumer idempotency/retry tests as applicable.
- Mock only meaningful boundaries. Prefer a small fake when it communicates stateful behavior
  more clearly than expectation-heavy mocks.
- After edits, run the narrowest relevant test first, then validate the affected module with:

```powershell
go test ./...
go vet ./...
```

- Run `go test -race ./...` for concurrency-sensitive or shared-state changes, plus the
  repository's configured lint, migration, and generation checks.
- Confirm route/command registration, dependency wiring, typed configuration, migrations,
  telemetry, deployment settings, and documentation are complete where applicable.
- Update the architecture guide when adding a reusable pattern or intentionally changing an
  existing one. Do not duplicate detailed guidance in this file.
