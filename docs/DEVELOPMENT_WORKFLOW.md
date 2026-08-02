# Development Workflow

This guide explains where and in what order to write code in the skeleton. The detailed
rules remain in `BACKEND_ARCHITECTURE_GUIDELINES.md`.

## Dependency Direction

Dependencies point inward:

```text
HTTP / consumer / job -> controller or use case -> service -> consumed port
                                                       ^
                                                       |
                                      repository or provider adapter
```

Framework and vendor types stop at their adapter boundary. Domain and service APIs must not
accept Gin, GORM, Redis, Kafka, Viper, or provider SDK types.

## Add an HTTP Appointment Action

Use this order so each layer is driven by required behavior:

1. Define request and response DTOs in `internal/apps/appointment/dtos`.
2. Define domain values or invariants in `internal/apps/appointment/models`.
3. Add a service method and its narrow consumed contracts in
   `internal/apps/appointment/services` or `interfaces` when genuinely shared.
4. Implement required persistence in `repositories` and external capabilities in an
   `adapters/<provider>` child package.
5. Add the use case in `controller`; it authorizes, translates, invokes, and shapes output.
6. Add the HTTP adapter in `handler`; it binds, validates, calls, and maps errors.
7. Wire concrete dependencies in `cmd` or through the app container.
8. Register the versioned route and middleware in `server`.
9. Add focused service, repository integration, handler, and contract tests as applicable.

The normal call path is:

```text
request -> handler -> controller -> service -> repository -> database
response <- handler <- controller <- service <- repository <- database
```

## Write Each Layer

### Handler

- Accept transport values and validate their shape.
- Call exactly the application action required by the endpoint.
- Map typed errors to stable HTTP status codes and response envelopes.
- Do not decide appointment eligibility or open transactions.

### Controller

- Represent one user/application action.
- Perform application authorization and DTO/domain translation.
- Coordinate services without importing the HTTP framework.

### Service

- Enforce appointment invariants and own transaction boundaries.
- Put `context.Context` first for application work and I/O.
- Depend on narrow behavior contracts and validate constructor dependencies.
- Return typed or sentinel application errors and preserve causes with `%w`.

### Repository

- Implement contextual, parameterized, bounded persistence operations.
- Translate database records and errors at the boundary.
- Keep business policy and outbound integrations out of persistence code.

### Provider Adapter

- Keep SDK and provider wire DTOs in the provider child package.
- Translate to a stable application capability interface.
- Enforce timeout, retry, size-limit, redaction, and error-classification policy.

### Composition Root

- Load typed configuration and initialize infrastructure in `cmd`.
- Construct adapters first, then services, controllers, handlers, and the runtime.
- Shut resources down in reverse order and return errors to `main`.

## Tests and Completion

Start with the cheapest test that proves the changed behavior, then run the module checks:

```powershell
gofmt -w <changed-go-files>
go test ./...
go vet ./...
```

Run `go test -race ./...` for concurrency-sensitive or shared-state changes. Before calling
work complete, confirm route/command registration, dependency wiring, typed configuration,
migrations, telemetry, deployment settings, and documentation where the feature needs them.
