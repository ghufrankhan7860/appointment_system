# Project Rules

The code already in this repo is the standard. New code must look like the owner wrote it: same naming, structure, patterns and tone. Before writing anything, open the nearest existing equivalent (auth stack for HTTP, `user_patient_repository.go` for repositories, `patient_service.go` for services, `auth_test.go` for tests) and copy its shape. If a rule below and the existing code disagree, the existing code wins. Do not introduce a second pattern, and do not rename or reformat existing code unless asked.

Also follow `.github/copilot-instructions.md` and `docs/BACKEND_ARCHITECTURE_GUIDELINES.md` for architecture.

## Layers
- Flow is handler -> controller -> service -> repository. Gin stays in handlers and middleware, GORM stays in repositories.
- Interfaces are declared in `services/` (consumer side), one file per repository interface. Controllers and handlers take concrete types.
- Every dependency is injected through a `NewX(...) (*X, error)` constructor that returns an `apperrors.ErrNilX` sentinel for a nil dependency. Wiring happens only in `container.go`.
- Request identity travels in `appcontext.AppContext.PrincipalID`, set by `middleware.Authenticate`. Controllers read it with their private `actingUserID(ctx)` helper.
- Models have no tags. GORM records are private to `repositories/` and mapped with a `toXModel` helper.

## Naming
- Receivers are the type's initials: `as`, `ac`, `ah`, `pr`, `ph`, `pc`, `repo`; fakes use `f`.
- DTOs: one file per feature in `dtos/<domain>/`, named by action (`create_patient.go`), types `CreatePatientRequest` / `CreatePatientResponse`, camelCase JSON, imported without an alias (`patient.CreatePatientRequest`). Response fields are duplicated per type; there is no shared base.
- Handler, controller and service methods are short verbs with no noun, because the type already names the domain (`Login`, `Register` in auth; `Create`, `GetByID`, `List`, `Update` for patients). Only DTO types carry the noun (`CreatePatientRequest`).
- Handler locals are `createPatientRequest`, `response`, and so on. Controller locals say what they are (`createdPatient`, `userID`).
- Repository "active only" variants are named `GetActiveBy...` / `ListActiveBy...`.
- Errors are `ErrXxx` sentinels in `apperrors/errors.go`, grouped by feature.

## Code style
- Handlers: bind (`ShouldBindJSON` / `ShouldBindUri` / `ShouldBindQuery`), call the controller with `ctx.Request.Context()`, map errors inline with `errors.Is` in a `switch`, no shared response or error helpers. Failure body is `gin.H{"success": false, "error": "..."}`, success is `gin.H{"success": true, "data": response, "message": "..."}`. Bind failures say `"Invalid request payload"`; unexpected errors say `"Failed to <verb> <noun>"`.
- Controllers build models and responses inline and return service errors unchanged. No HTTP types.
- Services validate input first, then act. Wrap errors as `fmt.Errorf("verb noun: %w", err)`. Private helpers go at the bottom of the file.
- Repositories use `dbFromContext(ctx, repo.db)`, translate `gorm.ErrRecordNotFound` / `ErrDuplicatedKey` to sentinels, and wrap other errors.
- `context.Context` is the first parameter. A blank line goes before `return`. Comments only on exported contracts or non-obvious constraints; no narrating comments.
- Multi-parameter service signatures use one parameter per line.
- New files match the line endings of their neighbours; do not run gofmt over files you did not change.

## API docs
- Every handler method carries swaggo annotations (`// Create godoc`, `@Summary`, `@Tags`, `@Param`, `@Success`, one `@Failure` per status the handler returns, `@Router`; add `@Security BearerAuth` on protected routes). These are the one allowed exception to the "no narrating comments" rule.
- Responses are still written with `gin.H`; `dtos.SuccessResponse` / `dtos.ErrorResponse` exist only to document the envelope.
- After changing a route, DTO or status code, update the annotations and run `go generate ./...`, then commit `docs/swagger/`. The Swagger UI is mounted only outside production (`HTTPConfig.SwaggerEnabled`).

## Tests
- Same package, stdlib `testing` only, no testify, no mock libraries.
- Hand-written `fakeXxx` types with receiver `f`, table-driven tests with `name` / `wantErr` / `t.Run`, `errors.Is` for error checks.
- Messages read like `"NewAuthService returned error: %v"` and `"Refresh error = %v, want %v"`. Test names read like `TestAuthServiceRefreshSessionState`.
- HTTP tests use `httptest` and drive the real handler, controller and service over fake repositories.

## Scope
- Smallest complete change. No new packages, dependencies or abstractions unless the task needs them.
- Run `go build ./...`, `go vet ./...` and `go test ./...` before calling work done.
