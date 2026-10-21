# Appointment System Backend

This repository is a layered Go backend for a real-world appointment management system. It is being developed with production-oriented domain modelling, explicit database migrations, clear application boundaries, and infrastructure adapters.

Before adding code, read these documents:

1. [`docs/BACKEND_ARCHITECTURE_GUIDELINES.md`](docs/BACKEND_ARCHITECTURE_GUIDELINES.md)

2. [`docs/DEVELOPMENT_WORKFLOW.md`](docs/DEVELOPMENT_WORKFLOW.md)

3. [`docs/ADR-0001-full-project-skeleton.md`](docs/ADR-0001-full-project-skeleton.md)

## Structure

```text

.

|-- main.go                         # Delegates process startup to cmd

|-- cmd/                            # Runtime commands and composition roots

|-- server/                         # HTTP engine, routes, health and Swagger route

|-- middleware/                     # Global HTTP middleware (app context, auth, admin log)

|-- config/                         # Typed configuration and validation

|-- envs/                           # Safe local configuration examples only

|-- internal/

|   |-- appcontext/                 # Request-scoped metadata (correlation ID, principal)

|   |-- apps/appointment/

|   |   |-- handler/                # HTTP binding and response mapping

|   |   |-- controller/             # One application action per use case

|   |   |-- services/               # Business rules and transactions

|   |   |-- repositories/           # Persistence adapters

|   |   |-- models/                 # Domain models

|   |   |-- dtos/                   # Transport input and output

|   |   |-- apperrors/              # Sentinel errors and client-facing validation reasons

|   |   |-- authz/                  # Role to permission map and RequirePermission

|   |   |-- constant/               # Central home for fixed values: formats, limits, lifetimes

|   |   |-- utils/                  # Generic helpers: hashing, tokens, JWT, dates, clock times

|   |   |-- interfaces/             # Placeholder: shared application-owned capabilities

|   |   |-- provider/               # Placeholder: provider selection and registration

|   |   |-- adapters/               # Provider implementations: payment/none (development gateway)

|   |   |-- cache/                  # Placeholder: appointment cache policy

|   |   `-- container.go            # App dependency declarations

|   `-- platform/

|       |-- datastore/              # Database connection and pool

|       |-- redis/                  # Placeholder: distributed-cache adapter

|       |-- kafka/                  # Placeholder: broker adapter

|       |-- network/                # Placeholder: outbound HTTP policy

|       |-- logger/                 # Placeholder: structured logging setup

|       `-- telemetry/              # Placeholder: traces, metrics, and profiling

|-- scripts/

|   |-- bootstrap/                  # Creates the first admin (make create-admin)

|   |-- consumer/outbox/            # Publishes outbox events (make consumer)

|   |-- cron/reminders/             # Queues appointment reminders (make reminders)

|   |-- cron/expire_payments/       # Cancels bookings not paid in time (make expire-payments)

|   |-- cron/expire_proposals/      # Closes unanswered reschedule proposals (make expire-proposals)

|   `-- backfill/                   # Placeholder: restartable data repair/import jobs

|-- migrations/                     # Ordered schema migrations

|-- tests/                          # Every test, one folder per package under test

|-- templates/                      # Placeholder: email/document templates

|-- mocks/                          # Placeholder: generated mocks only

|-- deploy/                         # Container/orchestration definitions

`-- docs/                           # Architecture decisions and generated Swagger spec

```

Folders marked "Placeholder" hold only a package description; they are reserved for later features and contain no code yet.

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

admin

```

These lowercase values match the `users.role` CHECK constraint (`000001`, widened by `000007_add_admin_role`) and `models.Role` (`RolePatient`, `RoleDoctor`, `RoleAdmin`). Public registration (`dtos/auth/register.go`) accepts only `oneof=patient doctor`: an admin is never self-registered. The first admin comes from `make create-admin`, later admins from an existing admin through `POST /admin/users`.

What a role may do is decided by named permissions in `internal/apps/appointment/authz`, not by checking the role in handlers. Adding a narrower role later (for example a reviewer who can only verify doctors) is one map entry.

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

doctors

    id

    user_id

    registration_council, registration_number

    verification_status

    ...

specialities, doctor_specialities, doctor_tags, doctor_consultation_modes

    lookup and link tables for a doctor profile

doctor_verifications

    one audit row per verification status change

doctor_availability, doctor_availability_overrides

    weekly working rules and one-off date changes

appointments

    id

    patient_id

    doctor_id

    booked_by_user_id

    starts_at, ends_at

    mode, status

    consultation_fee, slot_duration_min

    cancelled_at, cancelled_by_user_id, cancel_reason

    rescheduled_from_id, idempotency_key

    payment_status, payment_due_at

    ...

appointment_status_history

    one row per appointment status change

appointment_reminders

    one row per reminder window sent, so a reminder is never queued twice

outbox

    events to publish, written in the same transaction as the change

payments

    one row per attempt to collect a fee through a gateway; one live payment per appointment

appointment_reschedule_proposals

    a doctor's offer to move an appointment; the offered time is held by a placeholder appointment

payment_status_history

    one row per payment status change

refunds

    full refunds of captured payments, at most one per payment unless an earlier one failed

```

The `user_patient` table represents the many-to-many relationship and should enforce uniqueness for a `(user_id, patient_id)` pair so the same relationship is not accidentally stored multiple times.

The `patients` table should not use a single `user_id` field to represent ownership or access.

The `appointments.patient_id` field identifies the canonical patient receiving care.

The `appointments.booked_by_user_id` field identifies the user who created the booking and should not be used as the authorization mechanism for accessing the patient.

Authorization should be evaluated using the authenticated user, the relevant user-patient relationship, the user's system role, and the application's authorization rules.

A doctor is a `users` row with role `doctor` plus exactly one `doctors` row (`doctors.user_id` is unique). `(registration_council, registration_number)` is unique, so the same registration cannot belong to two profiles. `doctors.verification_status` is the current state and `doctor_verifications` is its history.

`appointments.starts_at` and `ends_at` are instants (`TIMESTAMPTZ`), not slots. The `appointments_doctor_no_overlap` exclusion constraint (`btree_gist`) lets a doctor hold only one `scheduled` appointment in any time range, so two concurrent bookings of the same slot cannot both succeed; a cancelled appointment frees its time without deleting the row. `consultation_fee` and `slot_duration_min` are copied from the doctor at booking time, so a later profile change never alters a booking. `appointments.status` is the current state and `appointment_status_history` is its history. Appointments are never deleted, only cancelled.

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

{"success": false, "error": "...", "details": "..."}

```

`error` is a stable, generic message for the case (for example `Invalid patient details`). `details` is the exact reason the client can act on, such as `dateOfBirth cannot be in the future`, `email is already registered`, `name is required` or `request body is not valid JSON`. It appears on 4xx responses when there is a reason to give, and is omitted otherwise. A `500` never includes `details`; the real cause is written to the server log next to the request line.

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

Appointments (require `Authorization: Bearer <jwtToken>`):

```text

POST /appointments                 patient  book a free slot for a linked patient

GET  /appointments                 any      the caller's appointments across their patients (?patientId=&status=&from=&to=&limit=&offset=)

GET  /appointments/:id             any      one appointment of a linked patient, or one where the caller is the doctor

POST /appointments/:id/cancel      any      cancel a scheduled appointment (optional {reason})

POST /appointments/:id/reschedule  patient  move to another free slot of the same doctor ({startsAt, mode?})

GET  /appointments/:id/reschedule-proposal          any      the doctor's pending proposal

POST /appointments/:id/reschedule-proposal/accept   patient  move to the offered time

POST /appointments/:id/reschedule-proposal/decline  patient  keep the current time

```

See **Appointments** below for the rules.

Payments (require `Authorization: Bearer <jwtToken>`):

```text

POST /appointments/:id/payments    patient  open a payment for an unpaid booking; returns the gateway clientPayload

GET  /appointments/:id/payment     any      the appointment's latest payment (patient side, its doctor, admins)

POST /payments/:id/verify          patient  confirm a payment with {providerPaymentId, signature}

```

See **Payments** below for the rules.

Doctors:

```text

POST /doctors                     doctor   create own profile (starts pending)

GET  /doctors/me                  doctor   own profile, any status

PUT  /doctors/me                  doctor   replace own profile

GET  /doctors                     any      search verified doctors (?q=&speciality=&tag=&city=&mode=&feeMax=&limit=&offset=)

GET  /doctors/:id                 any      public profile of a verified doctor

GET  /specialities                any      speciality slugs for profiles and search

GET  /doctors/me/availability     doctor   own weekly rules and upcoming overrides

PUT  /doctors/me/availability     doctor   replace the weekly rules

POST /doctors/me/overrides        doctor   add a day off, partial leave or extra session

GET  /doctors/:id/slots?date=     any      free slots of a verified doctor on one date

GET  /doctors/me/appointments     doctor   own appointments on one date (?date=, default today)

POST /doctors/me/appointments/:id/complete   doctor   mark a started appointment completed

POST /doctors/me/appointments/:id/no-show    doctor   mark a started appointment a no-show

POST /doctors/me/appointments/:id/reschedule-proposals  doctor  offer a new time ({startsAt, mode?, reason, note?})

```

Admin (each route needs an admin token and its own permission):

```text

POST /admin/users                          create another admin           admin:manage

GET  /admin/doctors?status=pending         review queue, any status       doctor:verify

GET  /admin/doctors/:id                    profile + verification history doctor:verify

POST /admin/doctors/:id/verify             pending   -> verified          doctor:verify

POST /admin/doctors/:id/reject   {reason}  pending   -> rejected          doctor:verify

POST /admin/doctors/:id/suspend  {reason}  verified  -> suspended         doctor:suspend

POST /admin/doctors/:id/reinstate          suspended -> verified          doctor:suspend

POST /admin/specialities                   add a speciality               speciality:manage

POST /admin/payments/:id/refund  {note?}   full refund of a captured payment  payment:refund

```

`GET /health` (outside `/api/v1`) reports database readiness.

### Doctor onboarding

1. Register with `role = doctor` and log in. The account exists but is not searchable.
2. `POST /doctors` with registration number, council, qualification, specialities (the first is the primary), tags, consultation modes, fee in paise and clinic. The profile is `pending`.
3. An admin finds it in `GET /admin/doctors?status=pending` and verifies or rejects it. Every decision is stored in `doctor_verifications` with the admin and the reason.
4. Only `verified` doctors who accept patients appear in `GET /doctors` and `GET /doctors/:id`.

Status rules:

```text

pending ──verify──▶ verified ──suspend──▶ suspended ──reinstate──▶ verified

   └──reject──▶ rejected ──doctor updates profile──▶ pending

```

A verified doctor who changes the registration number or council also goes back to `pending`. A suspended doctor cannot clear the suspension by editing the profile.

Search: `speciality` and `tag` may repeat and match any of the given values; every other filter must match. The `date` filter needs doctor availability and is not supported yet.

### Availability and slots

Availability is stored as rules, never as one row per slot. Slots are computed when asked for.

- **Weekly rules** (`PUT /doctors/me/availability`): blocks like `{"dayOfWeek": 1, "startTime": "09:00", "endTime": "13:00", "mode": "in_person"}`. `dayOfWeek` is 0 (Sunday) to 6 (Saturday), `endTime` may be `24:00`, blocks on the same day may not overlap, each must fit at least one slot, and the mode must be one the doctor offers. The request replaces the whole week; an empty list clears it.
- **Overrides** (`POST /doctors/me/overrides`) change one date, from today up to a year ahead:
  - not available, no times: whole day off;
  - not available, with times: leave for that part of the day;
  - available, with times and a mode: extra session.
- **Slots** (`GET /doctors/:id/slots?date=YYYY-MM-DD`): the day's rules plus extra sessions, minus leave, merged per mode and cut into `slotDurationMin` pieces. A whole day off returns nothing. Dates from today up to 90 days ahead; slots that already started today are left out. Only verified doctors who accept patients have slots. Any slot that overlaps a `scheduled` appointment is left out.

All times are wall-clock times in the clinic timezone, `APP_TIMEZONE` (default `Asia/Kolkata`). Slot responses also carry `startsAt` / `endsAt` with the UTC offset. Times are stored as minutes from midnight (`540` = 09:00).

### Appointments

1. The patient side picks a time from `GET /doctors/:id/slots?date=` and sends it back unchanged: `{"patientId": 3, "doctorId": 5, "startsAt": "2026-10-26T09:00:00+05:30", "mode": "in_person", "notes": "..."}`.
2. The server recomputes the doctor's free slots and requires an exact `startsAt` + `mode` match. The end time, fee and slot length come from the server, never from the request.
3. The appointment is `scheduled`, and `booked_by_user_id` is the caller from the token.

Rules:

- Booking needs an active `user_patient` link with `full` or `appointments_only` access; no link is `404`, `read_only` is `403`. Only accounts with the `patient` role can book (`appointment:book`).
- A time that is not a free slot is `422`. Losing a race for the same slot is `409` (`details: that time was just booked by someone else`).
- Any active link, including `read_only`, can view the patient's appointments; the appointment's doctor can view it too. Anyone else gets `404`.
- Only a `scheduled` appointment that has not started can be cancelled; anything else is `409`. The patient side must cancel at least 2 hours before the start (`422` after that); the doctor can cancel until the start. `cancelReason` records who cancelled (`patient_request`, `doctor_request`).
- **Reschedule** (`POST /appointments/:id/reschedule`) is for the patient side with `full` or `appointments_only` access, under the same rules as cancelling. In one transaction the old appointment is cancelled with reason `rescheduled` and a new one is created with `rescheduledFromId` pointing back; it keeps the original fee and notes. The new time must be a free slot (`422` otherwise); `mode` defaults to the current one. Doctors get `403` here.
- **Complete / no-show** are for the appointment's doctor, on a `scheduled` appointment that has started; anything else is `409`. Nothing moves out of `completed`, `cancelled` or `no_show`.
- **Doctor reschedule proposals**: the appointment's doctor offers another free slot with a reason (`emergency`, `personal`, `clinic_closed`, `other`). The appointment must be scheduled, not started, and `paid` or `waived`. Both the current time and the offered one are held: the appointment becomes `reschedule_proposed`, and a placeholder appointment in the same status holds the offered slot for the same booker, fee and payment. The proposal expires after 24 hours or at the current start, whichever is first. While it waits, the appointment cannot be cancelled, rescheduled, completed or marked a no-show (`409`).
  - **Accept** (patient side with `full` or `appointments_only` access): the current appointment is cancelled with reason `rescheduled`, the placeholder becomes `scheduled` with `rescheduledFromId`, and the payment moves to it.
  - **Decline**: the placeholder is cancelled (`proposal_declined`) and the appointment is `scheduled` again at its current time.
  - **Expiry**: `make expire-proposals` does the same as a decline (`proposal_expired`) for proposals nobody answered. An answer after expiry is `409`.
  - Outbox events: `appointment.reschedule_proposed`, `appointment.rescheduled` (accept), `appointment.reschedule_declined`, `appointment.reschedule_expired`.
- **Idempotency**: `POST /appointments` and `/reschedule` accept an `Idempotency-Key` header (at most 100 characters). Retrying with the same key and the same request returns the first result instead of failing because the slot is now taken; the same key on a different request is `422`. The key is stored on the appointment it created, unique per user.
- Every status change writes a row to `appointment_status_history` and an `outbox` event (`appointment.booked`, `.cancelled`, `.rescheduled`, `.completed`, `.no_show`) in the same transaction. A reschedule writes one `rescheduled` event, not a cancel plus a booking.

Background jobs:

- `make consumer` (`scripts/consumer/outbox`) runs until stopped and publishes unpublished outbox events in order, at least once. Each batch locks its rows and skips rows another copy holds, so several copies can run. For now an event is written to the log; email, SMS or push replace that later.
- `make reminders` (`scripts/cron/reminders`) queues due reminders once and exits; schedule it every few minutes. It queues an `appointment.reminder` event 2 hours before the start (`two_hours`) and, from 18:00 clinic time, for tomorrow's appointments (`day_before`). `appointment_reminders` makes repeated or overlapping runs safe. An appointment just after midnight that the 2-hour reminder covers gets only that one.
- `make expire-payments` (`scripts/cron/expire_payments`) cancels unpaid bookings past their hold once and exits; schedule it every minute.
- `make expire-proposals` (`scripts/cron/expire_proposals`) closes unanswered reschedule proposals once and exits; schedule it every few minutes.

Responses carry `startsAt` / `endsAt` with the clinic UTC offset, plus `date` and `startTime` in clinic time, `doctor {id, name, speciality}`, `patient {id, name}` and `bookedBy {userId, patientRelationship}`. `patientRelationship` is the booker's `user_patient.patient_relationship` (for example `child`), so a doctor's day can show "booked by child" without another call.

### Payments

A fee is paid online before the booking is confirmed.

1. `POST /appointments` books the slot with `paymentStatus: unpaid` and `paymentDueAt` 10 minutes ahead. The slot is held until then. A free consultation (fee 0) is `waived` and needs no payment.
2. `POST /appointments/:id/payments` opens a gateway order for the fee stored on the appointment (paise, never sent by the client) and returns `clientPayload` for the gateway's payment screen. Anyone with `full` or `appointments_only` access can pay, so a caregiver can pay for a booking someone else made; `payerUserId` records who paid. Asking again returns the same open payment.
3. `POST /payments/:id/verify` with the gateway's `providerPaymentId` and `signature` captures the payment and marks the appointment `paid` in one transaction. A wrong signature, or a `providerPaymentId` that already confirmed another payment, is `422`.

Rules:

- `make expire-payments` cancels bookings still unpaid after `paymentDueAt` with cancel reason `payment_expired`, which frees the slot. A payment that arrives after that is refunded automatically and verify returns `409` (`details: the booking expired before the payment arrived; the payment has been refunded`). Payment and expiry use conditional updates, so when they race exactly one wins.
- Cancelling a paid appointment refunds it in full (`patient_cancelled` or `doctor_cancelled`); cancelling an unpaid one expires its open order.
- Rescheduling keeps the payment: the new appointment takes over the payment status, due time and payments.
- A doctor can complete an appointment only when it is `paid` or `waived` (`422 Payment is pending` otherwise). Reminders are sent only for paid or free appointments.
- Admins refund a captured payment in full with `POST /admin/payments/:id/refund`. A refund is recorded as `requested` before the gateway is called and finished with its answer, so no gateway call holds a database transaction open.
- Outbox events: `payment.captured`, `payment.refunded`.

Gateways sit behind the `PaymentGateway` port in `services/`; `PAYMENT_PROVIDER` picks the adapter. The only adapter so far is `none` (`adapters/payment/none`), which moves no money: its `clientPayload` includes the `signature` to verify with, so the whole flow can be run from Swagger. `none` is refused when `APP_ENV` is production, so a production deploy needs a real gateway adapter.

### First admin

```powershell

$env:ADMIN_PASSWORD = "<at least 12 characters>"

make create-admin NAME="Ops Admin" EMAIL=ops@example.com MOBILE=9876543210

```

It uses the same database settings as the API and refuses to run once any admin exists.

### Swagger UI

With the API running, open:

`http://localhost:8080/swagger/index.html`

To call the protected patient and appointment endpoints: run **POST /auth/register** then **POST /auth/login**, copy the `jwtToken` from the response, click **Authorize**, and enter `Bearer <jwtToken>`.

Swagger is chosen at build time with the `swagger` Go build tag, not by an environment variable. Local builds include it: `make up`, `make run` and the VS Code **Debug Appointment API** launch all build with `-tags swagger`. A plain `go build` or `docker build` (what production uses) compiles it out, so the binary contains no Swagger code or route and no setting can turn it on.

```powershell

make up                    # Docker, Swagger on

make run                   # go run -tags swagger .

go run .                   # no tag, no Swagger

```

The spec is generated from the `// @...` annotations above each handler and is committed in `docs/swagger/`. After changing a route, DTO or status code, update the annotations and regenerate:

```powershell

go install github.com/swaggo/swag/cmd/swag@v1.16.6

go generate ./...

```

## First Commands

The Makefile wraps the common commands. On Windows, where `make` is not installed, use `mingw32-make` with the same targets.

```powershell

make test                  # go test for the normal build and the swagger build

make vet                   # go vet for both builds

make cover                 # coverage of the application code, run from tests/

```

| Target | What it does |
|---|---|
| `make db` | Start Postgres and apply migrations |
| `make run` | Run the API locally with Swagger (`go run -tags swagger .`) |
| `make up` / `make down` | Build and start / stop the whole Docker stack |
| `make down-v` | Stop the stack and delete the database volume |
| `make db-reset` | Delete the database volume, then start Postgres and migrate again |
| `make migrate` | Apply pending migrations |
| `make swagger` | Regenerate `docs/swagger` from the handler annotations |
| `make create-admin` | Create the first admin (see **First admin**) |
| `make logs` / `make ps` | Follow API logs / list containers |
| `make consumer` | Publish outbox events until stopped (Ctrl+C) |
| `make reminders` | Queue due appointment reminders once |
| `make expire-payments` | Cancel bookings not paid within the hold, once |
| `make expire-proposals` | Close reschedule proposals nobody answered in time, once |

## Tests

Every test lives in `tests/`, one folder per package under test (`tests/services`, `tests/handler`, `tests/repositories`, ...). Each folder is an external `<package>_test` package, so tests exercise exported behaviour only, and shared fakes live in that folder's `fakes_test.go`.

The tests in `tests/repositories` run against the local Postgres. Start it with `make db` first; without a database they are skipped, not failed.

## Run and Debug

Start PostgreSQL and run the API normally:

```powershell

make db

make run

```

Or without the Makefile:

```powershell

docker compose -f deploy/compose.yaml up -d --wait postgres

docker compose -f deploy/compose.yaml run --rm migrate

go run .

```

The `migrate` service applies every ordered migration in `migrations/` (`000001` onwards) and skips the ones already applied, so it is safe to run again after pulling new migrations. Schema migrations are explicit and are not run automatically when the application starts.

Copy `envs/.env.example` for the local settings. `JWT_SECRET` must be at least 32 characters and must be replaced outside local development. `PAYMENT_PROVIDER` defaults to `none`, which is for local development only.

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
