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

|   |   |-- adapters/               # Provider implementations: payment/none, payment/razorpay, storage/local

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

|   |-- cron/reconcile_refunds/     # Settles refunds still waiting for the gateway (make reconcile-refunds)

|   |-- cron/purge_sessions/        # Deletes sessions ended more than 90 days ago (make purge-sessions)

|   |-- cron/expire_calls/          # Ends call rooms whose time is over (make expire-calls)

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

medical_records

    one clinical note per appointment: complaint, diagnosis, advice, vitals, follow-up; draft or finalized

prescription_items

    the medicines on a record's prescription

medical_attachments

    uploaded reports and scans; only the file's storage key, never the file or a public URL

medical_record_access_log

    who created, edited, finalized, viewed, uploaded or downloaded clinical data, with the request's correlation id

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

11. A patient has at most one active self link (the patient's own

    account), it always has full access, and a patient always keeps

    at least one active full access holder.

12. An appointment has at most one live call room, and a call room

    grants no access to appointments, payments or records.

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

Sessions (require `Authorization: Bearer <jwtToken>`):

```text

GET    /auth/sessions                 the caller's active sessions, one per device login; current marks this one

DELETE /auth/sessions/:id             sign out one of the caller's sessions (404 if it is not theirs)

POST   /auth/sessions/revoke-others   sign out every other session of the caller ({revoked})

```

A login opens one session row and a refresh rotates its token in place, so one login on one device stays one session until logout, revocation or expiry. Each session records the client IP at login and the last-seen IP and time, the User-Agent (raw and parsed into browser, OS and device by `ua-parser`) and the first `Accept-Language` tag. The app may also send these optional headers on login and refresh:

```text

X-Device-Id        random ID the app keeps per install or browser (max 64)

X-Device-Name      device name (max 100)

X-Device-Platform  web | android | ios

X-App-Version      app version (max 30)

X-Device-Screen    WxH, such as 1080x2400

X-Device-Timezone  IANA zone, such as Asia/Kolkata

```

A bad value returns `400` with `details` (for example `X-Device-Platform must be one of web, android, ios`). On refresh the last-seen IP, User-Agent, app version and timezone are updated; the login IP and device ID never change, and a refresh sending a different `X-Device-Id` is allowed but logged as `refresh device mismatch`. The `jwtToken` carries the session ID (`sid`). Revoking a session stops its refresh token at once; its access token keeps working until it expires (15 minutes). Session times are UTC.

Patients (require `Authorization: Bearer <jwtToken>`):

```text

POST /patients           create a patient and link it to the caller (relationship, full access)

GET  /patients           list the caller's patients (?limit=&offset=)

GET  /patients/:id       get one patient the caller has active access to

PUT  /patients/:id       update a patient (requires full access)

```

`dateOfBirth` is sent as `YYYY-MM-DD`. A patient the caller has no active link to is reported as `404`, so patient IDs are not revealed. Updating with a non-full access level returns `403`. Get and list also return the caller's own `relationship` and `accessLevel` for each patient.

Patient sharing (require `Authorization: Bearer <jwtToken>`):

```text

POST   /patients/:id/invites              full     create an invite ({relationship, accessLevel, expiresInHours?}); the code is returned only here

GET    /patients/:id/invites              full     pending invites, without codes

DELETE /patients/:id/invites/:inviteId    full     revoke a pending invite

POST   /patients/claim                    any      redeem a code ({code}); links the caller and returns the patient

GET    /patients/:id/members              full     who has access, with a masked mobile; current marks the caller

PATCH  /patients/:id/members/:userId      full     change another member's access level ({accessLevel})

DELETE /patients/:id/members/:userId      full     revoke another member's access

DELETE /patients/:id/membership           any      leave: remove the caller's own access

```

Sharing never creates a patient: redeeming an invite adds a `user_patient` link, so the patient's appointments, payments and records follow at once.

- An invite code is 8 characters from `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (no I, O, 0 or 1), single use, and stored only as its SHA-256. Redeeming ignores case, spaces and dashes. An invite lives 48 hours unless `expiresInHours` (1–168) says otherwise, and a patient can have at most 5 pending.
- `relationship` is the redeemer's relationship to the patient. A `self` invite must grant `full` and is refused (`409`) while the patient already has a self link; it is how a grown-up child takes over their own record.
- An unknown, used, revoked or expired code is one `404`. After 5 wrong codes within an hour the caller gets `429` until the hour passes (`patient_invite_attempts`). A caller already linked gets `409`. A user whose access was revoked, or who left, gets the same link back with the new invite's relationship and access level.
- The self link is the top authority: nobody else can change or revoke it, and the self user can change or revoke anyone, including whoever created the record. Other full holders manage everyone except the self link. Nobody changes their own access through the member routes; they leave instead.
- Every change keeps at least one active full holder (`409` otherwise), including leaving.
- Revoking access keeps past and upcoming appointments; `booked_by_user_id` stays as the record of who booked.
- Each change writes a `user_patient_history` row (`created`, `invite_redeemed`, `access_changed`, `revoked_by_holder`, `left`) and an outbox event (`patient.access_granted`, `patient.access_changed`, `patient.access_revoked`).
- Claiming a record by an OTP to the patient's own mobile or email (no existing holder needed) waits for email or SMS delivery.

Calls (audio and video; `/calls/*` routes are public, the rest require `Authorization: Bearer <jwtToken>`):

```text

POST   /appointments/:id/call                      doctor, patient side  join the appointment's call ({mediaMode?: audio|video}), opening the room if nobody has

GET    /appointments/:id/call                      doctor, patient side  the live room and who joined; inRoom marks who is connected now

POST   /appointments/:id/call/end                  doctor, patient side  hang up for everyone

POST   /appointments/:id/call/guest-links          doctor               a link for someone without an account ({displayName?, maxUses?}); the token is returned only here

DELETE /appointments/:id/call/guest-links/:linkId  doctor               revoke a guest link

POST   /calls/guest/:token                         public               join an appointment's call with a guest link ({displayName, mediaMode?})

POST   /calls/code/:code                           public               join a code room ({displayName, mediaMode?})

GET    /calls/ws?token=                            public               the signaling WebSocket, opened with the wsToken of a join response

POST   /doctors/me/call-rooms                      verified doctor      create a code room ({title?, maxParticipants?, requiresApproval?, scheduledAt?}); the code is returned only here

GET    /doctors/me/call-rooms                      verified doctor      the caller's rooms, newest first

POST   /doctors/me/call-rooms/:id/join             verified doctor      join as the host

POST   /doctors/me/call-rooms/:id/end              verified doctor      end the room for everyone

GET    /doctors/me/call-rooms/:id/participants     verified doctor      everyone who joined, waiting ones included

```

Calls are WebRTC peer to peer: media goes browser to browser and the API only decides who may join, relays the signaling messages and hands out ICE servers. Every join answers `{roomId, peerId, role, waiting, wsToken, wsTokenExpiresAt, iceServers}`.

- An appointment's call is one room at a time (`call_rooms`, `kind = appointment`) holding the doctor and one patient-side peer. Only the appointment's doctor and the patient side with `full` or `appointments_only` access join; anyone else, `read_only` included, gets `404`. Every appointment mode can have a call.
- The call opens 10 minutes before the start and closes 30 minutes after the end, for a `scheduled` appointment that is `paid` or `waived` (`409` with the reason otherwise). Each side has one seat; a third person gets `409`, and the same user joining again takes their own seat back.
- A guest link joins someone without an account as the patient side, so it is refused while the patient side is in. It works until the call closes, up to `maxUses` joins (3 by default, at most 5), and only its SHA-256 is stored. The frontend builds the join URL from the token.
- A code room (`kind = code`) is a doctor's room outside any appointment: a family briefing, a follow-up group, a second opinion. It holds 2 to 5 people including the host (5 by default) and never grants access to the host's appointments or records. Only doctors with `call:host` and `verification_status = verified` host it (`403`); a host keeps at most 3 live rooms and creates at most 20 a day (`409`).
- A room code is 6 random digits and a [Damm](https://en.wikipedia.org/wiki/Damm_algorithm) check digit, shown once and stored as its SHA-256; spaces and dashes are ignored. A wrong check digit is a typo (`400`, not counted). Without `scheduledAt` a room lasts 24 hours; with it, joins open 10 minutes before and the room lasts until 2 hours after.
- With a lobby (`requiresApproval`, on by default) a joiner waits (`waiting: true`) until the host admits them. The room's size is checked when someone is admitted, and the host's seat is always kept free.
- An unknown guest link or room code is one `404`. 5 of them from one IP within 10 minutes answer `429` until the window passes (`call_join_attempts`).
- Completing an appointment, marking it no-show or cancelling it ends its room (`appointment_closed`). The host leaving a code room ends it after 5 minutes (`host_ended`).
- Each change writes a `call_events` row (`opened`, `joined`, `left`, `admitted`, `denied`, `removed`, `muted`, `unmuted`, `media_changed`, `connection_state`, `ended`), and a room starting or ending writes an outbox event (`call.started`, `call.ended`).

The WebSocket carries JSON messages `{"type": ...}`. The server sends `welcome` (your `peerId` and the `peers` already in), `peer-joined`, `peer-left`, `lobby` (you are waiting; for the host, `peer` is someone waiting), `offer` / `answer` / `ice` with `from`, `media`, `host`, `ended` and `error`. A client sends `offer`, `answer` and `ice` with `to`, `media {mode}`, `state {state: connected|relay|failed}`, `leave`, and, as a code room's host, `host {action: admit|deny|remove|mute|unmute|end, peerId}`. The newcomer sends the offers to everyone already in, which avoids both sides offering at once. Audio-to-video is a renegotiation between the peers; the server only passes it on.

- The `wsToken` (15 minutes) is a JWT for one participant of one room, signed with a key derived from `JWT_SECRET` and its own audience, so it never works as an access token and an access token never opens a socket. It is checked before the upgrade, so a bad token is a plain `401` and a full room `409`. Browsers may open the socket only from the site that serves the API.
- The database is the source of truth for who is in a room: a participant is in while `left_at` is empty and `last_seen_at` is under 60 seconds old. The server pings every 20 seconds; each pong updates `last_seen_at` and closes the socket once its room has ended, so an end from another request or the job reaches open sockets within 20 seconds.
- Sockets live in the API process (`adapters/callhub/memory`, behind the `CallHub` port), which is enough while each environment runs one API container; several would need a relay between them, such as Redis pub/sub.
- `iceServers` always has STUN (`STUN_URLS`, default `stun:stun.l.google.com:19302`). TURN, which relays a call the peers cannot connect directly (common on mobile networks), is added only when `TURN_URLS` and `TURN_SECRET` are set, with credentials valid for one hour in coturn's `use-auth-secret` scheme. See [Calls: TURN](#calls-turn).

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

POST /appointments/:id/payments    patient  open a payment for an unpaid booking with {method?: razorpay|upi}; returns the clientPayload

GET  /appointments/:id/payment     any      the appointment's latest payment (patient side, its doctor, admins)

POST /payments/:id/verify          patient  confirm a payment with {providerPaymentId, signature?}

GET  /appointments/:id/payment-options  patient  the methods this booking can be paid by, with the doctor's UPI details

POST /appointments/:id/payment/clinic   patient  pay at the clinic instead: the booking is confirmed as due_at_clinic

GET  /appointments/:id/payment/upi-qr   patient  the doctor's UPI QR image

POST /payments/:id/claim                patient  "I've paid" for a direct UPI payment (multipart: reference, proof?)

GET  /payments/:id/proof                any      the claim's screenshot (patient side, its doctor, admins)

PUT  /doctors/me/upi                    doctor   set or clear the UPI ID patients pay at ({upiId})

GET|PUT|DELETE /doctors/me/upi-qr       doctor   the doctor's UPI QR image (PUT is multipart: file)

GET  /doctors/me/payments/claims        doctor   direct UPI payments waiting for a check, oldest first

POST /doctors/me/payments/:id/verify    doctor   the money arrived ({note?}): the booking is paid

POST /doctors/me/payments/:id/reject    doctor   the money never arrived ({reason}): the booking is cancelled

POST /doctors/me/appointments/:id/collected  doctor  the fee was collected at the clinic ({note?})

GET  /doctors/me/refunds                doctor   direct UPI refunds the doctor still owes

POST /doctors/me/refunds/:id/close      doctor   {outcome: refunded|not_received, note?}

GET  /admin/payments/claims             admin    every claim waiting for a check

POST /admin/payments/:id/claim/verify   admin    verify a claim as backup ({note?})

POST /admin/payments/:id/claim/reject   admin    reject a claim as backup ({reason})

GET  /admin/refunds/manual              admin    every direct UPI refund still owed

POST /admin/refunds/:id/close           admin    close a direct UPI refund ({outcome, note?})

POST /webhooks/payment             gateway  Razorpay's signed callback; no token, X-Razorpay-Signature instead

```

See **Payments** below for the rules.

Medical records (require `Authorization: Bearer <jwtToken>`):

```text

POST /doctors/me/appointments/:id/record      doctor   open the record of a started appointment

PUT  /doctors/me/records/:id                  doctor   edit a draft: notes, vitals, medicines

POST /doctors/me/records/:id/finalize         doctor   lock the record and share it with the patient side

POST /doctors/me/records/:id/attachments      doctor   add a file to a draft (multipart)

GET  /records/:id                             any      one record with its files

GET  /patients/:id/records                    any      the patient's finalized records

GET  /patients/:id/attachments                any      the patient's files

POST /patients/:id/attachments                any      upload a report outside a visit (multipart)

GET  /attachments/:id/content                 any      download a file

GET  /doctors/me/patients/:id/records         doctor   a treated patient's finalized records plus own drafts

GET  /doctors/me/patients/:id/attachments     doctor   a treated patient's files

```

See **Medical records** below for the rules.

Doctors:

```text

POST /doctors                     doctor   create own profile (starts pending)

GET  /doctors/me                  doctor   own profile, any status

PUT  /doctors/me                  doctor   replace own profile

GET  /doctors                     any      search verified doctors (?q=&speciality=&tag=&city=&mode=&feeMax=&date=&limit=&offset=)

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

GET  /doctors/me/patients        doctor   own patients, next visit first (?q=&limit=&offset=)

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

GET  /admin/refunds?status=      refunds, oldest first; default requested = still with Razorpay  payment:refund

GET  /admin/users/:id/sessions?limit=&offset=     every session of a user (active, revoked, expired)  session:manage

DELETE /admin/users/:id/sessions/:sessionId     revoke one session of a user    session:manage

POST /admin/users/:id/sessions/revoke-all      sign a user out everywhere ({revoked})  session:manage


GET  /admin/records/:id/access-log         who touched a record, never what it says  record:audit

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

Search: `speciality` and `tag` may repeat and match any of the given values; every other filter must match. `date=YYYY-MM-DD` keeps only doctors with at least one free slot that day (in `mode`, when given), checked exactly against their slots; the date must be from today up to 90 days ahead, otherwise `400`.

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
- `make expire-payments` (`scripts/cron/expire_payments`) cancels unpaid bookings past their hold once and exits; schedule it every minute. Before cancelling, it asks the gateway whether the order was paid after all, in case the webhook was lost.
- `make expire-proposals` (`scripts/cron/expire_proposals`) closes unanswered reschedule proposals once and exits; schedule it every few minutes.
- `make reconcile-refunds` (`scripts/cron/reconcile_refunds`) settles refunds still `requested` with what the gateway reports, once, and exits; schedule it every 15 minutes. A refund the gateway has no record of after 15 minutes is marked `failed`, so an admin can retry it.
- `make purge-sessions` (`scripts/cron/purge_sessions`) deletes sessions that expired or were revoked more than 90 days ago, once, and exits; schedule it daily. Sessions hold the client IP and device details, which are personal data, so they are not kept longer.
- `make expire-calls` (`scripts/cron/expire_calls`) ends call rooms whose time is over, once, and exits; schedule it every 5 minutes. An appointment room ends once its appointment closed or 30 minutes after it ended (`no_answer` if the second peer never came, `timeout` otherwise); a code room past its expiry, 5 minutes after its host left, or after 30 minutes with nobody in. Sockets still open see the end on their next heartbeat.

Responses carry `startsAt` / `endsAt` with the clinic UTC offset, plus `date` and `startTime` in clinic time, `doctor {id, name, speciality}`, `patient {id, name}` and `bookedBy {userId, patientRelationship}`. `patientRelationship` is the booker's `user_patient.patient_relationship` (for example `child`), so a doctor's day can show "booked by child" without another call.

### Payments

A fee is paid online before the booking is confirmed.

1. `POST /appointments` books the slot with `paymentStatus: unpaid` and `paymentDueAt` 10 minutes ahead. The slot is held until then. A free consultation (fee 0) is `waived` and needs no payment.
2. `POST /appointments/:id/payments` opens a gateway order for the fee stored on the appointment (paise, never sent by the client) and returns `clientPayload` for the gateway's payment screen. Anyone with `full` or `appointments_only` access can pay, so a caregiver can pay for a booking someone else made; `payerUserId` records who paid. Asking again returns the same open payment.
3. The payment is captured and the appointment marked `paid` in one transaction, either by `POST /payments/:id/verify` with the gateway's `providerPaymentId` and `signature`, or by the gateway's webhook. Proof the gateway does not accept, or a `providerPaymentId` that already confirmed another payment, is `422`.

Rules:

- `make expire-payments` cancels bookings still unpaid after `paymentDueAt` with cancel reason `payment_expired`, which frees the slot. Lateness is judged on when the payer paid, as the gateway reports it, not on when the server heard of it. A payment made after the hold is refunded automatically and verify returns `409` (`details: the booking expired before the payment arrived; the payment is being refunded`). Payment and expiry use conditional updates, so when they race exactly one wins.
- Cancelling a paid appointment refunds it in full (`patient_cancelled` or `doctor_cancelled`); cancelling an unpaid one expires its open payment.
- Rescheduling keeps the payment: the new appointment takes over the payment status, due time and payments.
- A doctor can complete an appointment only when it is `paid`, `waived` or `due_at_clinic` (`422 Payment is pending` otherwise). Reminders are sent only for paid, free or pay-at-clinic appointments. A doctor cannot propose a new time while the payment is `unpaid` or `pending_verification`.
- Admins refund a captured payment in full with `POST /admin/payments/:id/refund`. A refund is recorded as `requested` before the gateway is called and settled with its answer, so no gateway call holds a database transaction open.
- A refund the gateway is still processing stays `requested`: the refund route answers `202` with `refundPending: true` and the payment stays `captured` until the gateway's refund webhook, or `make reconcile-refunds`, marks the payment and the appointment `refunded`. A gateway error also leaves it `requested`, because the gateway may have received it.
- A refund the gateway refuses is `failed`, the refund route answers `409`, and a `refund.failed` event is written. Refunding the payment again retries it. `GET /admin/refunds?status=failed` lists them.
- Outbox events: `payment.captured`, `payment.refunded`, `refund.failed`.

Gateways sit behind the `PaymentGateway` port in `services/`; `PAYMENT_PROVIDER` picks the adapter:

- `none` (`adapters/payment/none`) moves no money: its `clientPayload` includes the `signature` to verify with, so the whole flow can be run from Swagger. It is refused when `APP_ENV` is production.
- `razorpay` (`adapters/payment/razorpay`) uses [Razorpay Standard Checkout](https://razorpay.com/docs/payments/payment-gateway/web-integration/standard/) through the official `razorpay-go` SDK. See **Razorpay** below.

### Payments: methods

`PAYMENT_METHODS` lists the ways a booking can be paid, comma-separated, in any combination: `razorpay` (the gateway above), `upi` (paid straight to the doctor's UPI ID and checked by hand) and `clinic` (paid at the visit). It defaults to `razorpay`. An empty value is none mode: every booking is paid at the clinic. Unknown or repeated entries stop the server at startup.

- The gateway is built and checked only when `razorpay` is listed: then `PAYMENT_PROVIDER=razorpay` with its keys is required in production, as before. Without it, production starts with no Razorpay keys, and `POST /appointments/:id/payments` with method `razorpay` is `409`.
- `GET /appointments/:id/payment-options` gives the methods a booking offers: `razorpay` when listed, `upi` when listed and the doctor has set a UPI ID, `clinic` when listed. When no online method applies (none mode, or `upi` alone and a doctor with no UPI ID), `clinic` is the only one, whatever the list says.
- Locally, `make up-upi`, `make up-clinic`, `make up-upi-clinic`, `make up-all` and `make up-none` start the Docker stack with that list (`make up` keeps `razorpay`, or takes `PAYMENT_METHODS` from the shell). Switching recreates only the api container. The local stack runs `PAYMENT_PROVIDER=none`, which the web client can't pay through, so `razorpay` needs the Razorpay test settings in the shell (see **Razorpay**).
- A booking that can be paid online starts `unpaid` with the 10-minute hold. A booking that can only be paid at the clinic is `due_at_clinic` at once, with no hold.

Direct UPI:

1. The doctor sets a UPI ID (`PUT /doctors/me/upi`, `name@bank`) and may upload a QR image (`PUT /doctors/me/upi-qr`). Clearing the UPI ID removes the QR.
2. `POST /appointments/:id/payments` with `{"method":"upi"}` opens a `created` payment with provider `upi_direct`. Its `clientPayload` has `upiId`, `payeeName`, `amount` (rupees), `note`, `hasQr` and `upiLink`, a `upi://pay` link with the amount fixed that UPI apps open. An open payment by another method is expired first, so the patient can switch.
3. Within the hold the patient sends `POST /payments/:id/claim` with the UPI reference (UTR, 6 to 35 letters or digits; spaces are dropped) and an optional screenshot. The payment becomes `claimed` and the booking `pending_verification`, with `paymentDueAt` moved to the start: the slot stays held until then.
4. The doctor, or an admin as backup, checks their bank app. Verify captures the payment and marks the booking `paid`. Reject needs a reason: the payment is `rejected` and the booking cancelled with `payment_rejected` and back to `unpaid`, which frees the slot. The reason is shown to the patient.
5. `make expire-payments` cancels a claim still unchecked at the start with `payment_unverified`.

Pay at the clinic:

- `POST /appointments/:id/payment/clinic` turns an `unpaid` booking `due_at_clinic` when `clinic` is offered. The booking is confirmed and its open online payment expired.
- The doctor records the fee with `POST /doctors/me/appointments/:id/collected`, before or after the visit: a captured payment with provider `clinic` is written and the booking becomes `paid`.
- `due_at_clinic` counts as settled for the video call window, completing the visit and reschedule proposals. `pending_verification` does not.

Refunds of direct UPI payments:

- Cancelling a booking whose UPI payment is `captured` or `claimed`, or expiring an unchecked claim, writes a `requested` refund with no gateway call. `make reconcile-refunds` leaves it alone.
- The doctor sends the money back and closes it with `POST /doctors/me/refunds/:id/close`: `refunded` marks the payment and the booking refunded; `not_received`, only for a claim never verified, fails the refund and rejects the claim. Admins can close them too (`/admin/refunds/:id/close`).
- Screenshots and QR images are JPEG or PNG of at most 5 MB, checked by content, and stored through `STORAGE_PROVIDER`. They are served with `Cache-Control: no-store`.
- Outbox events: `payment.claimed` and `payment.rejected`, besides the ones above.

### Razorpay

1. `POST /appointments/:id/payments` creates a Razorpay order for the fee. `clientPayload` holds `keyId`, `orderId`, `amount` (paise) and `currency`, which the client opens Checkout with. The key secret never leaves the server.
2. Checkout hands the browser `razorpay_payment_id` and `razorpay_signature`, which the client sends to `POST /payments/:id/verify` as `providerPaymentId` and `signature`. The server checks the signature, HMAC-SHA256 of `<stored order id>|<payment id>` keyed with the key secret, against the order it stored, never one the client names. It then fetches the payment from Razorpay and requires the same order, amount and currency. An `authorized` payment is captured.
3. Razorpay also calls `POST /api/v1/webhooks/payment`. `X-Razorpay-Signature` (hex HMAC-SHA256 of the raw body, keyed with `RAZORPAY_WEBHOOK_SECRET`) must match or the answer is `401`. A reported capture is fetched from Razorpay again before anything is recorded, so a replayed body changes nothing.
   - `payment.captured` and `order.paid` capture the payment.
   - `refund.processed` and `refund.failed` settle the refund.
   - Every other event, `payment.failed` included (the payer can retry on the same order), repeats, and orders this API did not open get `200` and change nothing. The reason is logged; the body never is.
4. Refunds are normal speed. Each request carries an `X-Refund-Idempotency` key derived from the refund's id, so a retried request cannot refund twice.

Razorpay orders cannot be cancelled. Money paid on an order whose booking already expired or was cancelled is captured and refunded.

Setup (test mode first):

- In the Razorpay Dashboard, in test mode, generate API keys (Account & Settings > API Keys). Set `PAYMENT_PROVIDER=razorpay`, `RAZORPAY_KEY_ID` (`rzp_test_...`) and `RAZORPAY_KEY_SECRET`.
- Set Payment capture to automatic (Account & Settings > Payment capture). The API also captures an `authorized` payment itself.
- Add a webhook (Accounts & Settings > Webhooks) at `https://<api host>/api/v1/webhooks/payment` with the events `payment.captured`, `order.paid`, `refund.processed` and `refund.failed`. Choose a secret of at least 32 characters and set it as `RAZORPAY_WEBHOOK_SECRET`. Test and live mode each need their own webhook.
- Razorpay only calls public HTTPS URLs. For a local API, run a tunnel such as `cloudflared tunnel --url http://localhost:8080` and register the `https://<name>.trycloudflare.com/api/v1/webhooks/payment` URL it prints. Razorpay blocks some ngrok domains, so don't use ngrok. A quick tunnel's name changes on every start, so update the webhook URL each time.
- The Docker stack passes the three `RAZORPAY_*` values through from the shell. Run `make expire-payments` and `make reconcile-refunds` with the same settings, because they ask Razorpay too.
- Outside production `RAZORPAY_KEY_ID` must start with `rzp_test_`, and in production with `rzp_live_`, so a local run cannot take real money and production cannot take test money. Keep both secrets out of the repository and out of logs.
- Test payments: UPI `success@razorpay` or `failure@razorpay`, or one of Razorpay's test cards.

### Medical records

A record is the doctor's note for one appointment, with the visit's prescription. A visit has one prescription, so its medicines are part of the record.

1. The appointment's doctor opens the record once the appointment has started and is not cancelled (`409` otherwise). Opening it again returns the same record.
2. The doctor edits the draft with `PUT`: chief complaint, symptoms, diagnosis, examination notes, advice, prescription notes, follow-up date, vitals and up to 30 medicines. Vitals must be plausible (pulse 20-300, temperature 77-113 °F, weight 0.5-500 kg, SpO2 50-100) and each medicine needs a name and a dosage.
3. Finalizing needs a diagnosis and locks the record for good (`409` on any later change). It writes a `medical_record.finalized` outbox event that carries ids only, no clinical content.

Who can read:

- The record's own doctor, in any status.
- A doctor treating the patient (`patient:view`): someone with a completed, no-show or paid upcoming booking with them. They read the patient's finalized records from any doctor and download the patient's files, through `GET /doctors/me/patients/:id/records` and `/attachments`, `GET /records/:id` and the download route. Other doctors get `404`.
- The patient side with **`full`** access, finalized records only. `appointments_only` and `read_only` links get `404`, the same as no link: they can manage appointments but not read diagnoses.
- Admins see the access log (`record:audit`), never the record itself.

Files:

- The patient side with `full` access uploads reports outside a visit; the doctor adds files to their own draft. Uploads are `multipart/form-data` with `file`, `kind` (`lab_report`, `imaging`, `prescription_scan`, `referral`, `other`), `title` and an optional `takenOn`.
- Only PDF, JPEG and PNG are kept, detected from the file's content, not its name or the client's content type (`415` otherwise). The limit is 10 MB, counted on the bytes received (`413`).
- Downloads stream through the API after the same access check, as `Content-Disposition: attachment` with `X-Content-Type-Options: nosniff`.
- Files are stored behind the `FileStorage` port in `services/`; `STORAGE_PROVIDER` picks the adapter. The `local` adapter (`adapters/storage/local`) writes under `STORAGE_LOCAL_DIR` (default `var/storage`, ignored by git); the Docker stack keeps it on the `appointment_storage_data` volume. Cloudflare R2 will be another adapter with no service change.

Every read and write of a record or file, including lists, writes a row to `medical_record_access_log` with the user, the action and the request's correlation id, in the same transaction as the read or write.

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
| `make up-upi` / `up-clinic` / `up-upi-clinic` / `up-all` / `up-none` | `make up` with `PAYMENT_METHODS` set to `upi`, `clinic`, `upi,clinic`, all three, or empty (see **Payments: methods**) |
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
| `make reconcile-refunds` | Settle refunds still waiting for the gateway, once |
| `make purge-sessions` | Delete sessions ended more than 90 days ago, once |
| `make expire-calls` | End call rooms whose time is over, once |

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

Copy `envs/.env.example` for the local settings. `JWT_SECRET` must be at least 32 characters and must be replaced outside local development. `PAYMENT_PROVIDER` defaults to `none`, which is for local development only; `razorpay` needs the three `RAZORPAY_*` settings (see **Razorpay**). `PAYMENT_METHODS` (default `razorpay`) picks how bookings can be paid; see **Payments: methods**. `STORAGE_PROVIDER=local` with `STORAGE_LOCAL_DIR` keeps uploaded medical files on disk; outside development, put that folder on a persistent volume.

Logs always go to stdout through `log/slog`. Set `LOG_FILE` to a path (for example `var/log/appointment.log`) to also append every line to that file; its folder is created when missing, and empty `LOG_FILE` writes no file. The file is not rotated, so rotate it with logrotate (`copytruncate`) or prune it to the retention below; in Docker, put its folder on a volume or the file is lost with the container. `LOG_FORMAT` is `json` (default, for production, where the container runtime or a collector ships them) or `text` (local), `LOG_LEVEL` is `debug`, `info` (default), `warn` or `error`, and `LOG_SOURCE=true` adds the file and line. Every line logged during a request carries `correlation_id` (also returned as `X-Correlation-ID`) and, once authenticated, `user_id` and `role`. Each request writes one `request served` line with the route pattern, never the raw path; a 500 line carries the real error. Services log domain events with IDs only, and the keys `password`, `token`, `email`, `mobile`, `notes`, `diagnosis` and similar are redacted. Never log free text or personal data. Request lines hold `client_ip` and admin lines are an audit trail next to medical data, so keep logs for at most 30 days unless a legal hold requires longer.

The client IP comes from the connection unless `TRUSTED_PROXIES` lists the reverse proxies (IPs or CIDRs, comma-separated) whose `X-Forwarded-For` header can be believed; empty trusts none, which is right without a proxy. Behind Caddy in Docker, set it to the subnet of the network Caddy reaches the api on; on the EC2 host that is `appointment-edge` (`docker network inspect appointment-edge`, `172.18.0.0/16`) for both staging and production. Otherwise every session records Caddy's IP, or any client could fake its IP if all proxies were trusted.

The client IP also keys the limit on wrong call codes and guest links, so a wrong `TRUSTED_PROXIES` either counts everyone as Caddy or lets a client pick its own IP.

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

## Deploy (EC2)

Staging and production share one EC2 host (13.55.81.62) as separate compose projects, each with its own Postgres, volumes, image tag, server folder and env file, so each deploys on its own. The edge Caddy is the only container that publishes ports:

```text
internet ─▶ edge Caddy :80/:443 (appointment-edge, /home/ubuntu/edge)
             ├─ medappoint.in          /api/*, /health ─▶ api-production:8080   (appointment-production, /home/ubuntu/app-production)
             │                         everything else ─▶ web-production:80     (appointment-web-production, /home/ubuntu/web-production)
             ├─ www.medappoint.in      301 ─▶ https://medappoint.in
             └─ staging.medappoint.in  /api/*, /health ─▶ api-staging:8080      (appointment-staging, /home/ubuntu/app)
                                       everything else ─▶ web-staging:80        (appointment-web, /home/ubuntu/web)
```

Until Razorpay activates live mode, `medappoint.in` temporarily serves the staging stack (test keys, staging database) so Razorpay can review a working site; `deploy/edge/Caddyfile` marks the line to switch back once production runs with live keys.

The api and web containers join the shared `appointment-edge` network under those aliases; Postgres and the consumer stay on their own project network, so staging cannot reach the production database. Production Postgres listens on server-local `127.0.0.1:5433` (staging `5432`) for an SSH tunnel.

| Script | Deploys | Env file |
|---|---|---|
| `deploy-staging.ps1` | `deploy/compose.staging.yaml` | `envs/.env.staging` |
| `deploy-production.ps1` | `deploy/compose.production.yaml` | `envs/.env.production` |
| `deploy-edge.ps1` | `deploy/edge/` (Caddy), only when a domain or route changes | none |

`deploy-production.ps1` refuses uncommitted changes, an env file whose `APP_ENV` is not `production`, a Razorpay key that is not `rzp_live_` and any `REPLACE_ME` value, then asks you to type `production` (`-Yes` skips the prompt). `deploy-edge.ps1` validates the Caddyfile in a throwaway container and reloads Caddy in place. Production's scheduled jobs and daily database dump are in `deploy/crontab.production`, added by hand to the server crontab next to the staging block.


### Calls: TURN

TURN is built but off: with `TURN_URLS` and `TURN_SECRET` empty the API hands out STUN only, and calls that cannot connect directly fail. `deploy/turn/` holds a coturn stack for the EC2 host; no deploy script ships it. To turn it on:

1. Open the EC2 security group for `3478` TCP and UDP, `5349` TCP and UDP `49152-49400`. coturn runs with host networking, the one exception to "only the edge Caddy publishes ports".
2. Point `turn.medappoint.in` at the Elastic IP.
3. Put a certificate for `turn.medappoint.in` in `/home/ubuntu/turn-certs/turn.crt` and `turn.key` (for example a `turn.medappoint.in { respond 404 }` block in the edge Caddyfile and a copy of the files from Caddy's data volume), and renew it the same way.
4. Copy `deploy/turn/turn.env.example` to `deploy/turn/.env` on the server with a new `TURN_SECRET` (at least 32 characters) and `TURN_EXTERNAL_IP` (`<Elastic IP>/<private IP>`), then run `docker compose -f deploy/turn/compose.turn.yaml up -d`.
5. Set the same `TURN_SECRET` and `TURN_URLS=turn:turn.medappoint.in:3478,turns:turn.medappoint.in:5349` in `envs/.env.staging` and `envs/.env.production`, and deploy both APIs.

Check it with the WebRTC Trickle ICE page: a `relay` candidate means TURN works. One relay serves both environments.

Production's `deploy/crontab.production` runs `expire_calls` every 5 minutes; add the same line to the staging block with `$C`.

Replace the temporary module path `appointment_system` in `go.mod` and imports with the final repository path before publishing the module.
