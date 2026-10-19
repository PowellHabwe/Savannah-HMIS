# Savannah-HMIS: Clinic Appointment Booking API

A small REST API for a clinic with 5 doctors. Patients see free 30-minute slots for a doctor on a given day, book one, cancel, or reschedule.

**Public URL:** _TBD (added after deployment)_
**Deploys from:** `main`, automatically when a pull request is merged (see [CI/CD](#cicd))

Stack: Go, Fiber, PostgreSQL (pgx, sqlc, goose), Docker, GitHub Actions, DigitalOcean droplet.

---

## 1. System design

### 1.1 Requirements I derived
- 5 doctors, each with set working hours, working in fixed 30-minute slots.
- Patients can: list free slots, book, cancel (with reason), reschedule.
- A booked slot must be unavailable to everyone else.
- Must start small but be able to grow (more doctors, more clinics, auth, notifications).

### 1.2 Models

| Table | Purpose | Key columns |
|---|---|---|
| `doctor` | A clinician | `id`, `full_name`, `specialty` |
| `doctor_working_hours` | When a doctor works | `doctor_id`, `weekday` (0-6), `start_time`, `end_time`; unique `(doctor_id, weekday)` |
| `patient` | A person who books | `id`, `full_name`, `phone`, `email` |
| `appointment` | A booking | `id`, `doctor_id`, `patient_id`, `slot_start`, `slot_end`, `status` (`BOOKED`/`CANCELLED`), `cancel_reason`, `cancelled_at`, timestamps |

**Slots are not a table.** A slot is `(doctor, start time)`. Availability is computed:

```
working window for that weekday
  -> split into 30-minute slots
  -> remove slots that have a BOOKED appointment
  -> (for today) remove slots that already started or are inside the lead-time window
```

### 1.3 Components (layers, mirroring the Sofia backend)

```
router -> request_id mw -> request_log wrapper -> controller -> repository -> sqlc/Postgres
                                                      |
                                                utils/slots (pure logic)
```
- **controller:** parse the request, validate shape, map errors to HTTP status codes.
- **repository:** transactions and sqlc queries; returns typed domain errors (`ErrSlotTaken`, `ErrOutsideHours`, ...).
- **utils/slots:** pure functions (generate slots, check alignment, past, lead time, within working hours). No DB, so it is fast to unit test, and it is where the booking rules live.
- **libs/response:** one response envelope: `{"data": ...}` on success, `{"message": "..."}` on error.

### 1.4 Key decisions

1. **Compute slots instead of storing them.** Storing slots means a nightly job, a backlog to maintain, and a data migration whenever hours change. Computing needs one small query per request (working hours + that day's bookings). Trade-off: more logic at read time. For 5 doctors this is negligible.
2. **The database prevents double booking, not the application.** A partial unique index:
   `UNIQUE (doctor_id, slot_start) WHERE status = 'BOOKED'`.
   A check-then-insert in Go has a race window; two simultaneous requests could both pass the check. With the index, one insert wins and the other gets a unique violation, which I map to `409 Conflict`. Cancelling flips the status, so the slot is bookable again automatically while the cancelled row stays for history.
3. **Reschedule is one transaction.** Lock the appointment row (`SELECT ... FOR UPDATE`), reject if cancelled, validate the new slot exactly like a fresh booking, update `slot_start/slot_end`. If the new slot is taken the transaction rolls back and the original booking is untouched. The old slot frees itself because the row has moved.
4. **Time handling.** Instants are stored as `timestamptz` (UTC). Working hours are wall-clock times interpreted in the clinic timezone (`Africa/Nairobi`, configurable). Requests use RFC 3339 timestamps. Slot starts must be aligned to :00 or :30.
5. **Validation vs conflict status codes.**
   - `400` malformed input (bad UUID, bad date format)
   - `404` doctor/patient/appointment not found
   - `409` state conflicts: slot already taken, appointment already cancelled
   - `422` business-rule violations: outside working hours, in the past, within 1 hour of now, unaligned slot
6. **Bonus included.** 1-hour minimum lead time (configurable via `MIN_LEAD_MINUTES`) and `GET /patients/{id}/appointments` (upcoming, sorted ascending).
7. **One Go module** rather than a `go.work` monorepo. The folder layout (`libs`, `middleware`, `utils`, `http/resources/*/{controller,model,repository}`) follows my larger backend so it can later be split or merged into a workspace easily.

### 1.5 Ambiguities and the decision I made
- **No endpoints to create doctors/patients are required**, so a seed command creates 5 doctors with working hours and a few patients. `patient_id` is supplied in the booking request; there is no authentication.
- **Lunch breaks / multiple windows per day:** not supported. One window per doctor per weekday. Adding breaks later means allowing multiple rows per `(doctor, weekday)`.
- **A patient booking two doctors at the same time:** rejected (unique `(patient_id, slot_start) WHERE BOOKED`), since one person cannot attend both.
- **Cancelled appointment can be rebooked by someone else?** Yes, the slot becomes free for everyone.
- **Cancelling past appointments:** allowed only while the slot is still in the future.
- **Availability for a past date or a non-working day:** returns an empty list with `200` (not an error).

### 1.6 Trade-offs and how it grows
| Now | Later |
|---|---|
| No auth | JWT + patient identity from the token (my Sofia backend already has this pattern) |
| Single clinic | add `clinic_id` to `doctor` and scope queries |
| No notifications | publish "booked/cancelled" events to a queue and email/SMS from a worker |
| Computed slots | still fine at thousands of doctors; add a cache or materialise slots only if profiling demands it |
| One droplet | managed Postgres, then horizontal API replicas (the API is stateless) |

---

## 2. API

Base path: `/api/v1`. Times are RFC 3339 with offset (e.g. `2026-10-19T09:00:00+03:00`); responses are rendered in the clinic timezone.
Success: `{"data": ...}`. Error: `{"message": "..."}`, plus `"errors": [...]` for field validation failures.

| Method | Path | Purpose |
|---|---|---|
| POST | `/appointments` | Book a slot |
| GET | `/doctors/{id}/availability?date=YYYY-MM-DD` | Free 30-minute slots for a doctor on a date |
| PATCH | `/appointments/{id}/cancel` | Cancel with a reason |
| PATCH | `/appointments/{id}/reschedule` | Move to a new slot (same doctor) |
| GET | `/patients/{id}/appointments` | Upcoming appointments, soonest first (bonus) |
| GET | `/doctors` | List doctors (helper, to discover IDs) |
| GET | `/appointments/{id}` | Fetch one appointment (helper) |
| GET | `/health` | Liveness + database check |

### Book: `POST /appointments`
```json
{ "doctor_id": "d0000000-0000-4000-8000-000000000001",
  "patient_id": "a0000000-0000-4000-8000-000000000001",
  "slot_start": "2026-10-19T09:00:00+03:00" }
```
`201` with the appointment. Rules checked, in order: shape (`422` with a list of field errors), doctor/patient exist (`404`), slot aligned to :00/:30, not in the past, at least 1 hour from now, inside the doctor's working hours (all `422`), then the database insert (`409` if the slot is taken).

### Cancel: `PATCH /appointments/{id}/cancel`
`{"reason": "Patient is travelling"}` returns `200`. `409` if already cancelled, `422` if the appointment has already started, `404` if unknown. The slot is immediately bookable again.

### Reschedule: `PATCH /appointments/{id}/reschedule`
`{"slot_start": "2026-10-19T10:00:00+03:00"}` returns `200`. The new slot is validated exactly like a new booking. `409` if the appointment is cancelled or the new slot is taken (the original booking is left untouched), `422` for rule violations.

### Status codes
| Code | Meaning |
|---|---|
| 200 / 201 | OK / created |
| 400 | Malformed input: bad UUID, bad date, unparseable JSON |
| 404 | Doctor, patient or appointment not found |
| 409 | Conflict with current state: slot taken, patient already booked then, already cancelled |
| 422 | Rule violation: missing fields, unaligned, past, inside 1-hour window, outside working hours, appointment already started |
| 500 | Unexpected; generic message to the client, details in the logs with the request ID |

## 3. Running locally
```
cp .env.example .env
make db.up
make migrate.up
make seed
make run          # http://localhost:8080/health
make test
```

## Testing
```
go test ./...            # unit tests; integration tests are skipped without a database
```
Integration tests run against real Postgres, because the no-double-booking guarantee is enforced by a database index. Point them at the local database:
```
$env:TEST_DATABASE_URL="postgres://clinic:clinicpass@localhost:5436/clinic?sslmode=disable"
go test ./... -count=1 -v
```
They create their own doctor and patients with random IDs and clean up after themselves.

| Where | What it proves |
|---|---|
| `utils/slots` | Slot generation, availability filtering, every validation rule (alignment, past, 1-hour window, working hours) |
| `http/errmap` | Each domain error maps to the right HTTP status; internal errors never leak |
| `appointment` (integration) | Double booking is refused; **10 concurrent requests for one slot produce exactly 1 winner**; cancel frees the slot; reschedule frees the old slot, leaves the original untouched on conflict, and validates like a fresh booking |

## CI/CD

Two GitHub Actions workflows in `.github/workflows/`:

**`ci.yml`: runs on every pull request**
1. Starts a Postgres 16 service container.
2. `go vet ./...`
3. Applies the goose migrations to that database.
4. `go test ./... -race -count=1`, with `TEST_DATABASE_URL` set so the integration tests (including the concurrent double-booking test) run against real Postgres.

**`deploy.yml`: runs when a PR is merged into `main`** (a merge is a push to `main`)
1. Reuses `ci.yml`. If tests fail, nothing is deployed.
2. Connects to the droplet over SSH with a dedicated deploy key.
3. On the droplet: fetches that exact commit from the public GitHub repo, then `docker compose up -d --build`. A one-shot `migrate` service applies pending migrations first; the API starts only after it succeeds.
4. Smoke test: polls `/health` until it answers, and fails the workflow if it never does.

**Runtime on the droplet (Docker Compose):** Caddy (the only service with published ports) -> API -> Postgres. Postgres and the API are not reachable from the internet. Database credentials live only in a `.env` file on the server; GitHub only holds the SSH key, host, user and public URL as secrets.

**Decision: build on the server, no container registry.** For one small service this means fewer moving parts (no registry, tags or package permissions). The trade-off is that the build uses the droplet's CPU and memory (a 1 GB droplet gets swap) and production runs a fresh build of the commit rather than the exact image CI tested. As the system grows, the next step is to build once in CI, push to a registry and have the server pull that image.

**Safeguards:** deploys are serialised (a second merge waits for the running one), the server always checks out an exact commit SHA so rollback is a one-line change, and `main` should be branch-protected to require a pull request and a passing `test` check. Full setup steps are in [`deploy/SETUP.md`](deploy/SETUP.md).

## 4. AI reflection
_Filled in at the end, in my own words._
