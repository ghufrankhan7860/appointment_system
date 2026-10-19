# Appointment System Backend

This repository is a layered Go backend for a real-world appointment management system. It is being developed with production-oriented domain modelling, explicit database migrations, clear application boundaries, and infrastructure adapters.

Before adding code, read these documents:

1. [`docs/BACKEND_ARCHITECTURE_GUIDELINES.md`](docs/BACKEND_ARCHITECTURE_GUIDELINES.md)

2. [`docs/DEVELOPMENT_WORKFLOW.md`](docs/DEVELOPMENT_WORKFLOW.md)

3. [`docs/ADR-0001-full-project-skeleton.md`](docs/ADR-0001-full-project-skeleton.md)

## Structure

```text

.

|-- main.go                         # Delegates process startup to cmd

|-- cmd/                            # Runtime commands and composition roots

|-- server/                         # HTTP engine, routes, and lifecycle

|-- middleware/                     # Global HTTP middleware

|-- config/                         # Typed configuration and validation

|-- envs/                           # Safe local configuration examples only

|-- internal/

|   |-- apps/appointment/

|   |   |-- handler/                # HTTP binding and response mapping

|   |   |-- controller/             # One application action per use case

|   |   |-- services/               # Business rules and transactions

|   |   |-- repositories/           # Persistence adapters

|   |   |-- models/                 # Domain and persistence representations

|   |   |-- dtos/                   # Transport/application input and output

|   |   |-- interfaces/             # Shared application-owned capabilities

|   |   |-- provider/               # Provider selection and registration

|   |   |-- adapters/               # External provider implementations

|   |   |-- cache/                  # Appointment cache policy

|   |   |-- constant/               # Stable appointment values, sparingly

|   |   `-- container.go            # App dependency declarations

|   `-- platform/

|       |-- datastore/              # Database and migration infrastructure

|       |-- redis/                  # Distributed-cache adapter

|       |-- kafka/                  # Broker adapter

|       |-- network/                # Outbound HTTP policy

|       |-- logger/                 # Structured logging setup

|       `-- telemetry/              # Traces, metrics, and profiling

|-- scripts/

|   |-- consumer/                   # Message consumer loops

|   |-- cron/                       # Scheduled jobs

|   `-- backfill/                   # Restartable data repair/import jobs

|-- migrations/                     # Ordered schema migrations

|-- templates/                      # Email/document templates

|-- tests/                          # Cross-package and end-to-end tests

|-- mocks/                          # Generated mocks only

|-- deploy/                         # Container/orchestration definitions

`-- docs/                           # Architecture and operational decisions

```

## Domain Model

The appointment system deliberately separates the concepts of **User**, **Patient**, **User-Patient relationship**, and **Appointment**.

A `User` represents an authenticated application account and actor. A `Patient` represents the real-world person receiving healthcare. A patient may be managed by one or more users, and a user may manage one or more patients.

This separation allows the system to support scenarios such as:

- A patient booking an appointment for themselves

- A user booking appointments for family members

- Multiple users managing the same patient

- A caregiver managing a patient's appointments

- A staff member creating an appointment on behalf of a patient

- A patient eventually having their own application account while family members retain authorized access

### User

A `User` represents an authenticated account and an actor in the system.

The `users.role` field defines the **system-level role of the account**.

For the current application, the supported roles are:

```text

patient

doctor

```

These lowercase values match the `users.role` CHECK constraint in
`migrations/000001_create_users.up.sql`, `models.Role` (`RolePatient`,
`RoleDoctor`) and the `oneof=patient doctor` validation in
`dtos/auth/register.go`.

The role answers:

```text

What kind of actor is this account within the application?

```

For example:

```text

User 1

role = patient

User 2

role = doctor

```

`users.role` must not be used to represent a user's relationship with a particular patient. Those are separate concepts.

A future account can also have a more specialized profile without changing the fundamental `User` model. For example, doctor-specific fields can be kept in a doctor/practitioner profile rather than being added directly to `users`.

### Patient

A `Patient` represents the **real-world person receiving healthcare**.

The patient record is independent of the account that is currently using the application.

A patient should have one canonical record in the system. A new patient record must not be created merely because another user needs to manage or book appointments for that same person.

For example:

```text

User 1 -> Patient 10 (self)

User 1 -> Patient 11 (mother)

User 1 -> Patient 12 (father)

User 2 -> Patient 11 (mother)

```

There is still only one `Patient 11`.

A patient can also have their own account:

```text

User 3 -> Patient 11 (self)

```

while User 1 and User 2 may continue to have authorized relationships with the same patient.

The `patients` table therefore must not contain a single `user_id` field as the mechanism for determining who can manage the patient.

### User-Patient Relationship

The relationship between users and patients is many-to-many and is represented by a dedicated `user_patient` table.

Conceptually:

```text

users

  |

  | M:N

  v

user_patient

  |

  | M:N

  v

patients

```

This allows:

```text

One user  -> many patients

Many users -> one patient

```

For example:

```text

User 1 -> Patient 10

User 1 -> Patient 11

User 1 -> Patient 12

User 2 -> Patient 11

```

The `user_patient` record contains information about the relationship between that particular user and that particular patient.

The main fields have distinct meanings:

```text

user_patient.relationship

    = The user's relationship to the patient.

user_patient.access_level

    = What level of access or management is this user permitted to have

      for this patient?

user_patient.status

    = Whether the relationship is active, pending, revoked, etc.

```

Example:

```text

user_id | patient_id | patient_relationship | access_level

--------+------------+--------------+------------

1       | 10         | self         | full

1       | 11         | mother          | full

2       | 11         | daughter     | full

3       | 11         | caregiver    | appointments_only

```

`relationship` and `access_level` must remain conceptually separate.

For example, being a patient's `son` does not by itself determine what information or actions the user is authorized to access.

Likewise, `access_level` must not be confused with `users.role`.

The distinction is:

```text

users.role

    = Account/system role

user_patient.relationship

    = The user's relationship to this particular patient

user_patient.access_level

    = Permitted access/management level for this particular patient

```

Authorization rules should be enforced through the application's authorization model and should not rely solely on a family relationship string.

### Appointment

An `Appointment` belongs to the **Patient receiving care**, not directly to the user who happens to book it.

The primary relationship is:

```text

Appointment -> Patient

```

Therefore an appointment should contain:

```text

appointments.patient_id

```

An appointment should also record the authenticated actor who created the booking:

```text

appointments.booked_by_user_id

```

These fields represent two different facts:

```text

patient_id

    = Who is receiving care?

booked_by_user_id

    = Which authenticated user created the appointment?

```

`booked_by_user_id` is therefore not redundant data copied from the patient relationship. It represents the booking actor and provides a useful audit trail.

For example:

```text

Patient 11 = Mother

Appointment 100

patient_id = 11

booked_by_user_id = 1

Appointment 101

patient_id = 11

booked_by_user_id = 2

```

Both appointments belong to the same patient, even though different users created the bookings.

A staff member may also create an appointment:

```text

Appointment 102

patient_id = 11

booked_by_user_id = 50

```

This still does not change the identity of the patient.

The user's authority to book an appointment for the patient must be determined through the appropriate authorization and user-patient relationship rules. `booked_by_user_id` identifies the actor; it does not itself grant access.

### High-Level Relationship

```text

                         +-------------+

                         |    users    |

                         |-------------|

                         | id          |

                         | role        |

                         +------+------+

                                |

                                | M:N

                                v

                         +-------------+

                         |user_patient|

                         |-------------|

                         | user_id     |

                         | patient_id  |

                         | relationship|

                         |access_level |

                         | status      |

                         +------+------+

                                |

                                | M:N

                                v

                         +-------------+

                         |   patients  |

                         |-------------|

                         | id          |

                         | ...         |

                         | created_at  |

                         | updated_at  |

                         +------+------+

                                |

                                | 1:N

                                v

                         +-------------+

                         |appointments |

                         |-------------|

                         | id          |

                         | patient_id  |

                         | doctor_id   |

                         |booked_by... |

                         | ...         |

                         +------+------+

                                |

                                |

                                +---- booked_by_user_id ----> users

```

The model should therefore be understood as:

```text

User

    = authenticated application actor

Patient

    = real-world person receiving healthcare

UserPatient

    = relationship and access between a user and a patient

Appointment

    = scheduled healthcare interaction for a patient

```

This separation is a core domain invariant and must be respected by repositories, services, controllers, migrations, and authorization logic.

## Persistence Modelling

Persistence records should reflect the domain relationships above.

The core schema is expected to follow this structure:

```text

users

    id

    role

    ...

patients

    id

    ...

    created_at

    updated_at

user_patient

    user_id

    patient_id

    relationship

    access_level

    status

    created_at

    updated_at

appointments

    id

    patient_id

    doctor_id

    booked_by_user_id

    ...

```

The `user_patient` table represents the many-to-many relationship and should enforce uniqueness for a `(user_id, patient_id)` pair so the same relationship is not accidentally stored multiple times.

The `patients` table should not use a single `user_id` field to represent ownership or access.

The `appointments.patient_id` field identifies the canonical patient receiving care.

The `appointments.booked_by_user_id` field identifies the user who created the booking and should not be used as the authorization mechanism for accessing the patient.

Authorization should be evaluated using the authenticated user, the relevant user-patient relationship, the user's system role, and the application's authorization rules.

## Timestamps

Persistent records should generally maintain their own lifecycle timestamps where applicable:

```text

created_at

updated_at

```

These timestamps describe the lifecycle of the individual record.

For example:

```text

users.created_at

    = When the user account was created

patients.created_at

    = When the patient record was created

user_patient.created_at

    = When the relationship between the user and patient was created

appointments.created_at

    = When the appointment record was created

```

Likewise:

```text

updated_at

```

represents when that particular record was last modified.

These are independent lifecycle events even when multiple records are created as part of the same business workflow or transaction.

## Core Domain Invariants

The following invariants should be preserved throughout the application:

```text

1. One real-world patient should have one canonical patient record

   within the system.

2. Multiple users may be associated with the same patient.

3. One user may be associated with multiple patients.

4. users.role represents the account/system role and is separate from

   the user's relationship with a specific patient.

5. user_patient.relationship represents the relationship between a

   specific user and a specific patient.

6. user_patient.access_level represents the permitted level of

   access/management for that relationship.

7. appointments.patient_id identifies the patient receiving care.

8. appointments.booked_by_user_id identifies the authenticated actor

   who created the appointment.

9. booked_by_user_id does not replace the user-patient relationship

   and does not by itself grant authorization to access the patient.

10. Creating an additional user account or granting another user access

    to an existing patient must not create a duplicate patient record.

```

## API

All routes are served under `/api/v1`. Responses use one envelope:

```text

{"success": true, "data": {...}, "message": "..."}

{"success": false, "error": "..."}

```

Auth (public):

```text

POST /auth/register      create an account

POST /auth/login         returns jwtToken and refreshToken

POST /auth/refresh       rotates the refresh token

POST /auth/logout        revokes the refresh token

```

Patients (require `Authorization: Bearer <jwtToken>`):

```text

POST /patients           create a patient and link it to the caller (relationship, full access)

GET  /patients           list the caller's patients (?limit=&offset=)

GET  /patients/:id       get one patient the caller has active access to

PUT  /patients/:id       update a patient (requires full access)

```

`dateOfBirth` is sent as `YYYY-MM-DD`. A patient the caller has no active link to is reported as `404`, so patient IDs are not revealed. Updating with a non-full access level returns `403`.

`GET /health` (outside `/api/v1`) reports database readiness.

Appointment endpoints are not implemented yet.

### Swagger UI

With the API running, open:

`http://localhost:8080/swagger/index.html`

To call the protected patient endpoints: run **POST /auth/register** then **POST /auth/login**, copy the `jwtToken` from the response, click **Authorize**, and enter `Bearer <jwtToken>`.

The UI is served only outside production. It is disabled when `APP_ENV` (or, if unset, `GIN_MODE`) is `production`, `prod` or `release`, which includes the `api` service in `deploy/compose.yaml`.

The spec is generated from the `// @...` annotations above each handler and is committed in `docs/swagger/`. After changing a route, DTO or status code, update the annotations and regenerate:

```powershell

go install github.com/swaggo/swag/cmd/swag@v1.16.6

go generate ./...

```

## First Commands

```powershell

go test ./...

go vet ./...

```

## Run and Debug

Start PostgreSQL and run the API normally:

```powershell

docker compose -f deploy/compose.yaml up -d --wait postgres

docker compose -f deploy/compose.yaml run --rm migrate

go run .

```

The `migrate` service applies every ordered migration in `migrations/` (`000001` onwards) and skips the ones already applied, so it is safe to run again after pulling new migrations. Schema migrations are explicit and are not run automatically when the application starts.

Copy `envs/.env.example` for the local settings. `JWT_SECRET` must be at least 32 characters and must be replaced outside local development.

As the schema evolves, create ordered migrations for domain changes such as:

```text

create users

create patients

create user_patient

create appointments

...

```

Do not rely on application startup to implicitly mutate the production schema.

To debug in VS Code, install the official Go extension, open **Run and Debug**, select **Debug Appointment API**, and press `F5`.

The pre-launch task starts PostgreSQL, then Delve launches the application with the local development environment. Set breakpoints in handlers, controllers, services, or repositories and call:

`http://localhost:8080/health`

Stop the database with the **Stop PostgreSQL** task or:

```powershell

docker compose -f deploy/compose.yaml down

```

Replace the temporary module path `appointment_system` in `go.mod` and imports with the final repository path before publishing the module.
