# Project Rules

The code already in this repo is the standard. New code must look like the owner wrote it: same naming, structure, patterns and tone. Before writing anything, open the nearest existing equivalent (auth stack for HTTP, `user_patient_repository.go` for repositories, `patient_service.go` for services, `auth_test.go` for tests) and copy its shape. If a rule below and the existing code disagree, the existing code wins. Do not introduce a second pattern, and do not rename or reformat existing code unless asked.

Also follow `.github/copilot-instructions.md` and `docs/BACKEND_ARCHITECTURE_GUIDELINES.md` for architecture.

## Layers
- Flow is handler -> controller -> service -> repository. Gin stays in handlers and middleware, GORM stays in repositories.
- Interfaces are declared in `services/` (consumer side), one file per repository interface. Controllers and handlers take concrete types.
- Every dependency is injected through a `NewX(...) (*X, error)` constructor that returns an `apperrors.ErrNilX` sentinel for a nil dependency. Wiring happens only in `container.go`.
- Request identity travels in `appcontext.AppContext` (`PrincipalID`, `PrincipalRole`), set by `middleware.Authenticate` from the JWT. Controllers read them with the private `actingUserID(ctx)` / `actingRole(ctx)` helpers.
- Authorization is by named permission, never by comparing roles in handlers: routes use `authz.RequirePermission(authz.PermXxx)`, and `authz/permissions.go` maps roles to permissions. Admin routes live in the `/admin` group, which authenticates, logs and requires `admin:access` before any route-level permission.
- Models have no tags. GORM records are private to `repositories/` and mapped with a `toXModel` helper.

## Naming
- Receivers are the type's initials: `as`, `ac`, `ah`, `pr`, `ph`, `pc`, `repo`; fakes use `f`.
- DTOs: one file per feature in `dtos/<domain>/`, named by action (`create_patient.go`), types `CreatePatientRequest` / `CreatePatientResponse`, camelCase JSON, imported without an alias (`patient.CreatePatientRequest`). When several actions return the same resource, they share one `<Resource>Response` (`doctor.DoctorResponse`, `doctor.OverrideResponse`) built by one mapper (`toDoctorResponse`); never convert between identical struct types. A response that adds fields to a resource embeds it (`admin.GetDoctorResponse` embeds `doctor.DoctorResponse`) so the JSON stays flat.
- Handler, controller and service methods are short verbs with no noun, because the type already names the domain (`Login`, `Register` in auth; `Create`, `GetByID`, `List`, `Update` for patients). Only DTO types carry the noun (`CreatePatientRequest`). A type that handles several resources names the resource too (`AdminController.VerifyDoctor`, `CreateSpeciality`).
- Handler locals are `createPatientRequest`, `response`, and so on. Controller locals say what they are (`createdPatient`, `userID`).
- Repository "active only" variants are named `GetActiveBy...` / `ListActiveBy...`.
- Errors are `ErrXxx` sentinels in `apperrors/errors.go`, grouped by feature.

## Code style
- Handlers: bind (`ShouldBindJSON` / `ShouldBindUri` / `ShouldBindQuery`), call the controller with `ctx.Request.Context()`, map errors inline with `errors.Is` in a `switch`. Success is `gin.H{"success": true, "data": response, "message": "..."}`. Failures are inline `gin.H{"success": false, "error": "<generic message>"}`; on branches that can carry a reason add a third key, written one key per line: `"details": apperrors.Reason(err)` for service validation errors, `"details": bindErrorDetail(err)` for JSON/query bind failures. Every 500 calls `_ = ctx.Error(err)` (logged once by `middleware.LogRequest`) and returns only `"Failed to <verb> <noun>"`, never internals. No shared response helpers.
- Client mistakes are `apperrors.Invalid(apperrors.ErrInvalidXInput, "<jsonField>", "<short reason>")`, read as "field reason": `Invalid(ErrInvalidPatientInput, "dateOfBirth", "cannot be in the future")`. Use an empty field when the rule spans fields. Do not echo the bad value back. Internal failures (missing ids, acting user) keep `fmt.Errorf("...: %w", err)` and carry no reason.
- Controllers build models and responses inline and return service errors unchanged. No HTTP types.
- Services validate input first, then act. Wrap errors as `fmt.Errorf("verb noun: %w", err)`. Private helpers go at the bottom of the file; when a service file grows long, move its validation or calculation helpers into a sibling file of the same package (`doctor_validation.go`, `availability_slots.go`).
- `utils` holds only generic, domain-free helpers (hashing, tokens, `CivilDate`, `ParseClockTime`, `NormalizeValues`, `EscapeLike`). Business rules never go there. 
- No hardcoded values: fixed formats, limits, sizes and lifetimes live in the central `constant` package, one file per topic (`auth.go`, `time.go`, `doctor.go`, `availability.go`, `pagination.go`), and code uses the name (`constant.DateLayout`, `constant.MaxDoctorTags`). Enum values stay next to their type in `models`. Infrastructure packages that must not import the app (`config`, `server`, `middleware`, `scripts`) keep their own named `const` block at the top of the file instead. Struct tags and Swagger comments cannot use constants and stay literal.
- Repositories use `dbFromContext(ctx, repo.db)`, translate `gorm.ErrRecordNotFound` to sentinels, detect unique clashes with the typed constraint constants in `repositories/postgres_errors.go`: `if emailUnique.ViolatedBy(err) { ... }` (it reads the constraint name from the Postgres error via `errors.As`; GORM error translation is off so that name survives; EXCLUDE clashes work the same way through the `exclusionConstraint` constants, as in `doctorNoOverlapExclusion.ViolatedBy(err)`), and wrap other errors. Repositories return facts (`ErrEmailAlreadyExists`); services choose the client wording (`apperrors.Invalid`).
- `context.Context` is the first parameter. A blank line goes before `return`. Comments only on exported contracts or non-obvious constraints; no narrating comments.
- Multi-parameter service signatures use one parameter per line.
- New files match the line endings of their neighbours; do not run gofmt over files you did not change.

## API docs
- Every handler method carries swaggo annotations (`// Create godoc`, `@Summary`, `@Tags`, `@Param`, `@Success`, one `@Failure` per status the handler returns, `@Router`; add `@Security BearerAuth` on protected routes). These are the one allowed exception to the "no narrating comments" rule.
- Responses are still written with `gin.H`; `dtos.SuccessResponse` / `dtos.ErrorResponse` exist only to document the envelope.
- After changing a route, DTO or status code, update the annotations and run `go generate ./...`, then commit `docs/swagger/`. The Swagger UI exists only in builds with `-tags swagger` (`server/swagger.go`); `server/swagger_off.go` is the default no-op, so production builds compile it out. Never gate Swagger with a runtime flag or env var. Run `make test` and `make vet`, which cover both builds.

## Tests
- All tests live in `tests/<package>/` as the external package `<package>_test` (never beside the code). They test exported behaviour only: inject collaborators (repositories, clock) through constructors rather than reading unexported fields. Stdlib `testing` only, no testify, no mock libraries.
- Hand-written `fakeXxx` types with receiver `f`, kept once per folder in `fakes_test.go`; table-driven tests with `name` / `wantErr` / `t.Run`, `errors.Is` for error checks.
- Messages read like `"NewAuthService returned error: %v"` and `"Refresh error = %v, want %v"`. Test names read like `TestAuthServiceRefreshSessionState`.
- HTTP tests use `httptest` and drive the real handler, controller and service over fake repositories.
- Database tests (`tests/repositories`) use the local Postgres from `make db` and `t.Skip` when it is not reachable. `make cover` reports coverage of the application code.

## Scope
- Smallest complete change. No new packages, dependencies or abstractions unless the task needs them.
- Run `go build ./...`, `go vet ./...` and `go test ./...` before calling work done.
