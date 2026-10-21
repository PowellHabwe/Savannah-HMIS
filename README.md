# Savannah-HMIS: Clinic Appointment Booking API

A small REST API for a clinic with 5 doctors. Patients can see the free 30-minute slots for a doctor on a given day, book one, cancel it, or move it to another time.

| | |
|---|---|
| **Public URL** | http://143.198.221.153 (health check: http://143.198.221.153/health) |
| **Repository** | https://github.com/PowellHabwe/Savannah-HMIS |
| **Deploys from** | `main`, automatically, once the tests pass (see [section 3](#3-deployment-and-cicd)) |

Stack: Go, Fiber, PostgreSQL (pgx, sqlc, goose), Docker, GitHub Actions, one DigitalOcean droplet.

Contents: [Quick start](#quick-start-run-it-locally) | [1. System design](#1-system-design) | [2. API and testing it](#2-api-implementation) | [3. Deployment and CI/CD](#3-deployment-and-cicd) | [4. Limitations and next steps](#4-limitations-and-next-steps) | [5. AI reflection](#5-ai-reflection)


## Quick start: run it locally

**You need:** Go 1.25 or newer, Docker (Docker Desktop on Windows or macOS) and Git. GNU Make is optional; every `make` target is just a `go` or `docker` command, and the plain equivalents are listed below.

**1. Get the code and create your config**

```
git clone https://github.com/PowellHabwe/Savannah-HMIS.git
cd Savannah-HMIS
cp .env.example .env
```

The setup defaults in `.env.example` would match the local Postgres container, so you don't need to edit anything. (`cp` works in PowerShell too.)

**2. Start Postgres, create the tables, load demo data, run the API**

```
make db.up          # Postgres in Docker on localhost:5436
make migrate.up     # apply the goose migrations
make seed           # 5 doctors, 3 patients (safe to run again)
make run            # API on http://localhost:8080
```

While you are developing, `make dev` runs the API with live reload instead (it uses [air](https://github.com/air-verse/air), and the config is already in `.air.toml`). Install it once with `go install github.com/air-verse/air@latest`.

**3. Check it works**

```
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/doctors
```

The first should return `{"data":{"status":"ok"}}`, and the second the 5 seeded doctors. Their fixed IDs are used in [Testing the endpoints by hand](#testing-the-endpoints-by-hand), which walks through every endpoint in Postman (with a browser and terminal shortcut) and shows the response shapes.

**Without Make**

| Make target | Plain command |
|---|---|
| `make db.up` | `docker compose up -d postgres` |
| `make migrate.up` | `go run ./cmd/migrate -dir db/migrations up` |
| `make seed` | `go run ./cmd/seed` |
| `make run` | `go run ./cmd/api` |
| `make test` | `go test ./... -race -count=1` |

**Stopping and resetting**

```
docker compose down        # stop Postgres, keep the data
docker compose down -v     # stop Postgres and delete the data (start fresh)
```

**Other targets:** `make dev` (live reload, needs [air](https://github.com/air-verse/air)), `make vet`, `make sqlc` (regenerate the query code, needs [sqlc](https://sqlc.dev); the generated code is already committed, so you only need it after changing a query), `make migrate.status`, `make migrate.create name=...` (needs the `goose` CLI), `make docker.build`.

**If something goes wrong**

- *Port 5436 already in use:* change the left side of `"5436:5432"` in `docker-compose.yml` and set `DB_PORT` in `.env` to match.
- *"connection refused" when migrating:* Postgres takes a few seconds to start. Check `docker compose ps` shows it as healthy, then try again.
- *A booking returns `422` "slot is in the past" or "too soon":* the slot must be a future weekday between 09:00 and 16:30. The testing guide works the date out for you.

To run the whole production stack (Caddy, API, Postgres) on your machine instead, follow step 0 of [`deploy/SETUP.md`](deploy/SETUP.md).

Configuration is by environment variables (see `.env.example`):

| Variable | Default | Meaning |
|---|---|---|
| `ENVIRONMENT` | `development` | `production` switches to JSON logs |
| `SERVER_HOST` / `SERVER_PORT` | `0.0.0.0` / `8080` | Where the API listens |
| `SERVER_TIMEOUT_READ` | `10s` | Request read timeout |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | required | Postgres connection |
| `CLINIC_TIMEZONE` | `Africa/Nairobi` | Timezone for working hours and responses |
| `MIN_LEAD_MINUTES` | `60` | Minimum notice for a booking |

---

## 1. System design

### 1.1 What I took the requirements to be

- 5 doctors, each with set working hours, working in fixed 30-minute slots.
- Patients can list free slots, book, cancel (with a reason) and reschedule.
- A booked slot must disappear for everyone else, even if two people click at the same moment.
- Start small, but don't paint ourselves into a corner: more doctors, more clinics, logins and notifications should all be possible later without a rewrite.

### 1.2 Models

| Table | Purpose | Key columns |
|---|---|---|
| `doctor` | A clinician | `id`, `full_name`, `specialty` |
| `doctor_working_hours` | When a doctor works | `doctor_id`, `weekday` (0 = Sunday ... 6 = Saturday), `start_time`, `end_time`; unique `(doctor_id, weekday)` |
| `patient` | A person who books | `id`, `full_name`, `phone`, `email` |
| `appointment` | A booking | `id`, `doctor_id`, `patient_id`, `slot_start`, `slot_end`, `status` (`BOOKED` or `CANCELLED`), `cancel_reason`, `cancelled_at`, timestamps |

Slots are not a table. A slot is just a doctor and a start time, so I work it out when someone asks:

```
working window for that weekday
  -> split into 30-minute slots
  -> remove slots that have a BOOKED appointment
  -> remove slots that already started or are inside the lead-time window
```

### 1.3 Components and project structure

A request goes through the same path every time:

```
router -> request_id middleware -> request_log wrapper -> controller -> repository -> sqlc / Postgres
                                                              |
                                                        utils/slots (pure logic)
```

- **controller** reads the request, checks its shape and turns errors into HTTP statuses.
- **repository** owns the transactions and the sqlc queries, and returns named domain errors like `ErrSlotTaken`.
- **utils/slots** is plain functions with no database: generating slots, filtering availability, and the booking rules. That is why it is quick to unit test, and it is the one place the rules live.
- **http/errmap** is the only place domain errors become HTTP statuses. Anything unknown becomes a generic 500, and the details go to the log with the request ID, never to the client.
- **libs/response** keeps one response shape: `{"data": ...}` on success, `{"message": "..."}` on error.

The folder layout is deliberate. I wanted something a second or any other dev could add a service to without asking me where things go:

```
cmd/          one folder per binary: api, migrate, seed
db/           migrations, sql queries, generated sqlc code
http/         router, errmap, and resources/<name>/{controller,model,repository}
libs/         shared pieces: apperr, config, constants, response
middleware/   request_id, request_log
utils/        ctx, logger, pgtype, slots, validator
deploy/       docker-compose.prod.yml, Caddyfile, SETUP.md
```

Every binary lives in `cmd/`, and every business area (appointments, doctors, patients) is a self-contained folder under `http/resources/`. To add a new service, say a notifications worker or a billing API, you add a folder in `cmd/`, and its own resource folders, and reuse `libs/`, `middleware/` and `utils/` as they are. Nothing needs to be moved or renamed. It is one Go module today, but this is the same shape I use in my larger backend, where the services are split into a `go.work` monorepo. If this ever grows that way, the folders already line up.

### 1.4 Key decisions

1. **Compute slots, don't store them.** Stored slots need a nightly job to create them, a backlog to keep tidy, and a data migration every time someone's hours change. Computing needs one small query per request (that doctor's hours plus that day's bookings). The cost is a bit more logic at read time, which at 5 doctors is nothing.

2. **The database stops double booking, not the Go code.** I added a partial unique index: `UNIQUE (doctor_id, slot_start) WHERE status = 'BOOKED'`. If you check in Go first and insert second, two requests arriving together can both pass the check. With the index, one insert wins and the other gets a unique violation, which I turn into a `409`. Cancelling just flips the status, so the slot frees itself and the old row stays as history.

3. **Reschedule is one transaction.** I lock the appointment row (`SELECT ... FOR UPDATE`), refuse it if it is cancelled or has already started, validate the new slot exactly like a fresh booking, then update the times. If the new slot is taken the whole thing rolls back and the original booking is untouched.

4. **Time.** Instants are stored as `timestamptz` (UTC). Working hours are wall-clock times in the clinic's timezone (`Africa/Nairobi`, configurable). Requests use RFC 3339, responses are shown in the clinic timezone, and slot starts must land on :00 or :30.

5. **Status codes separate "this isn't valid input" from "this is valid but breaks a rule".**
   - `400` malformed input: bad UUID, bad date, JSON that can't be parsed
   - `404` doctor, patient or appointment not found
   - `409` clashes with current state: slot taken, patient already booked then, already cancelled
   - `422` rule violations: missing fields, unaligned slot, in the past, too soon, outside working hours, appointment already started

6. **Postgres.** I picked it because it can grow with the product instead of forcing a second system early. Today I only use plain relational features plus a partial unique index and row locks. If we later need LLM features, `pgvector` adds embeddings inside the same database. Graph-style queries are possible with recursive queries or an extension like Apache AGE, and JSONB covers flexible data. I'd rather start with one database I understand well and add capabilities to it than start with several.

7. **Goose and hand-written SQL, not an ORM.** Migrations are plain, versioned `.sql` files run by goose, with up and down steps. Queries are plain SQL that sqlc turns into typed Go functions. I chose this because the important behaviour in this project (the partial unique index, `FOR UPDATE`, `timestamptz` handling) is SQL, and I want to see and control it, not hope an ORM generates it. sqlc also checks the queries against the schema when I generate, so a typo fails at build time and not in production. The migration runner is compiled into the Docker image as its own binary, so the server never needs Go or goose installed. The trade-off is that I write more SQL and have to remember to run `make sqlc` after changing a query. For this project that is a good trade.Although its tiny,using raw sql also reduces latency and is pivotal when there are many queries/transactions happening concurrently.Scaling is generally made better with raw SQL

8. **Bonus included.** A 1-hour minimum lead time (`MIN_LEAD_MINUTES`) and `GET /patients/{id}/appointments`, upcoming and soonest first.

### 1.5 Ambiguities, and what I decided

- **Creating doctors and patients.** No endpoints were asked for, so a seed command creates 5 doctors with working hours and 3 patients. The booking request carries a `patient_id`. There is no login.
- **Lunch breaks or several windows per day.** Not supported. One window per doctor per weekday. Supporting breaks later means allowing several rows per `(doctor, weekday)`.
- **One patient, two doctors, same time.** Rejected with `409`, since one person can't be in two rooms.
- **Can someone else take a cancelled slot?** Yes, it is free for everyone straight away.
- **Cancelling or rescheduling something that already started.** Rejected with `422`.
- **Rescheduling to a different doctor.** Not supported. A reschedule stays with the same doctor.
- **A past date or a day the doctor doesn't work.** Availability returns an empty list with `200`, not an error.
- **What "within 1 hour" means.** A slot that starts less than `MIN_LEAD_MINUTES` from now is rejected when booking and hidden from availability. A slot starting exactly at the cut-off is allowed.

### 1.6 Trade-offs and how it grows

| Now | Later |
|---|---|
| No login | JWT, with the patient taken from the token instead of the request body |
| One clinic | Add `clinic_id` to `doctor` and scope every query by it |
| No notifications | Publish booked and cancelled events to a queue, and send email or SMS from a worker (a new service in `cmd/`) |
| Computed slots | Fine for thousands of doctors; cache or materialise slots only if profiling says so |
| One droplet | Managed Postgres, then several API replicas (the API holds no state) |
| One clinic timezone | See below |

**People in different timezones.** The groundwork is already there: every instant is stored in UTC, and only working hours are tied to a timezone. The first step would be a timezone on each clinic, with times shown in the viewer's zone. Only if we later run in several regions, with writes happening in more than one place, does ordering concurrent changes become a real problem, and that is where ideas like vector clocks come in. I wouldn't reach for them before one Postgres primary stops being enough, because today the unique index gives us a single, simple source of truth.

---

## 2. API implementation

Base path: `/api/v1`. Times are RFC 3339 with an offset, for example `2026-11-02T09:00:00+03:00`. Success looks like `{"data": ...}`. Errors look like `{"message": "..."}`, with an extra `"errors": [...]` list for field validation failures.

| Method | Path | Purpose |
|---|---|---|
| POST | `/appointments` | Book a slot |
| GET | `/doctors/{id}/availability?date=YYYY-MM-DD` | Free 30-minute slots for a doctor on a date |
| PATCH | `/appointments/{id}/cancel` | Cancel with a reason |
| PATCH | `/appointments/{id}/reschedule` | Move to a new slot (same doctor) |
| GET | `/patients/{id}/appointments` | Upcoming appointments, soonest first (bonus) |
| GET | `/doctors` | List doctors, to find their IDs |
| GET | `/appointments/{id}` | Fetch one appointment |
| GET | `/health` | Liveness plus a database check (not under `/api/v1`) |

### Book: `POST /appointments`

```json
{ "doctor_id": "d0000000-0000-4000-8000-000000000001",
  "patient_id": "a0000000-0000-4000-8000-000000000001",
  "slot_start": "2026-11-02T09:00:00+03:00" }
```

Returns `201` with the appointment. Checks run in this order: request shape (`422` with a list of field errors), doctor and patient exist (`404`), slot lands on :00 or :30, not in the past, at least an hour away, inside the doctor's working hours (all `422`), and finally the database insert (`409` if the slot is taken or the patient is already booked then).

### Availability: `GET /doctors/{id}/availability?date=2026-11-02`

Returns that doctor's free slots for the date, in the clinic timezone. `400` for a bad doctor ID or date, `404` for an unknown doctor.

### Cancel: `PATCH /appointments/{id}/cancel`

Body: `{"reason": "Patient is travelling"}`. Returns `200`. `409` if it is already cancelled, `422` if it has already started, `404` if unknown. The slot can be booked again straight away.

### Reschedule: `PATCH /appointments/{id}/reschedule`

Body: `{"slot_start": "2026-11-02T10:00:00+03:00"}`. Returns `200`. The new slot goes through the same checks as a fresh booking. `409` if the appointment is cancelled or the new slot is taken (the original stays as it was), `422` for rule violations.

### Status codes

| Code | Meaning |
|---|---|
| 200 / 201 | OK / created |
| 400 | Malformed input: bad UUID, bad date, unparseable JSON |
| 404 | Doctor, patient or appointment not found |
| 409 | Clashes with current state: slot taken, patient already booked then, already cancelled |
| 422 | Rule violation: missing fields, unaligned, past, too soon, outside working hours, already started |
| 500 | Something unexpected. The client gets a generic message; the details are in the logs under the request ID |

### Testing the endpoints by hand (POSTMAN) - Testing Setup is Found Below this Section

You can point it at the live API or your own local one. The demo data is already loaded, with fixed IDs, so the commands work as written:

| Who | ID |
|---|---|
| Dr. Amina Wanjiku (works Mon to Fri, 09:00 to 17:00) | `d0000000-0000-4000-8000-000000000001` |
| Patient Grace Njeri | `a0000000-0000-4000-8000-000000000001` |
| Patient Peter Mwangi | `a0000000-0000-4000-8000-000000000002` |

`GET /api/v1/doctors` lists all five doctors with their IDs. A booking must be on a future working day, so set one date in Postman (`slotDate`) to an upcoming Monday.

Eight checks cover the whole brief:

| # | Do this | You should see |
|---|---|---|
| 1 | Check the API is up | `200`, `{"data":{"status":"ok"}}` |
| 2 | List Dr. Amina's free slots for Monday | `200`, 16 slots from 09:00 to 16:30 |
| 3 | Book 09:00 as Grace | `201`, `"status":"BOOKED"` |
| 4 | Book the same slot as Peter | `409`, `that slot is already booked` |
| 5 | Move Grace's booking to 10:00 | `200`, new times, same `id` |
| 6 | Cancel it with a reason | `200`, `"status":"CANCELLED"` (run check 2 again: 09:00 is free) |
| 7 | Cancel it again | `409`, `appointment is already cancelled` |
| 8 | Book a time in the past | `422`, `cannot book a slot in the past` |

#### Postman (the main way)

##### Step 1. Install and open Postman

Download it from https://www.postman.com/downloads/, install it and open it. If it asks you to sign in, you can create a free account (some versions let you skip). Use the desktop app, not the browser version, if you want to test a local API on `localhost`. The live URL works in both.

##### Step 2. Create an environment (your shortcuts)

An environment is a list of named values. When you write `{{baseUrl}}` inside a request, Postman swaps in the real value. That way you type the server address and the IDs once instead of in every request.

1. In the left sidebar, click **Environments**, then the **+** button to create a new one. Name it `Savannah`.
2. Add these rows. Type each value under **Value** (or **Initial value** if you see both columns):

   | Variable | Value |
   |---|---|
   | `baseUrl` | `http://143.198.221.153` (or use `http://localhost:8080` for your own setup) |
   | `doctorId` | `d0000000-0000-4000-8000-000000000001` |
   | `patientId` | `a0000000-0000-4000-8000-000000000001` |
   | `patientId2` | `a0000000-0000-4000-8000-000000000002` |
   | `slotDate` | an upcoming Monday, for example `2026-11-02` |
   | `appointmentId` | leave empty for now (you paste the booking's ID here after request 3) |

3. Press `Ctrl+S` to save.
4. At the top right there is a dropdown that says **No environment**. Click it and choose `Savannah`. Do this or none of the `{{...}}` shortcuts will work.

The IDs belong to the demo data: `doctorId` is Dr. Amina Wanjiku, `patientId` is Grace, `patientId2` is Peter. `slotDate` must be a future Monday, because Dr. Amina works Monday to Friday and a booking can't be in the past. When that date goes by, change this one value to a later Monday.

##### Step 3. Create a collection

A collection is a folder that keeps your requests together. In the left sidebar, click **Collections**, then **+**, and name it `Savannah`.

##### Step 4. How to add and send any request

You'll do this once per request, and it is the same every time:

1. Hover over the `Savannah` collection, click the **...** menu, and choose **Add request**. Give it a name like `1 Health`.
2. Next to the address bar, pick the **method** (GET, POST or PATCH) from the dropdown. It starts as GET.
3. Paste the **URL** into the address bar. If your environment is selected, `{{baseUrl}}` shows up coloured. If it shows red, go back to Step 2, point 4.
4. If the request has a **body**: click the **Body** tab under the address bar, choose **raw**, then change the small **Text** dropdown to **JSON**. Paste the body into the box. Postman adds the `Content-Type: application/json` header for you.
5. Click the blue **Send** button.
6. Read the result in the panel below. The **status code** (for example `200 OK`) is at the top right of that panel, and the reply is in the **Body** tab. Choose **Pretty** to see it nicely formatted.
7. Press `Ctrl+S` to save the request.

Tip: for requests that look alike (like 3 and 4), right-click the first one, choose **Duplicate**, and change only what's different.

##### Step 5. The eight requests

Send them in this order. The reply for each is shown, so you can compare it with what you get. The API sends compact JSON; Postman's **Pretty** view lays it out like this.

###### Request 1: Is the API up?

- Method and URL: `GET` `{{baseUrl}}/health`
- Body: none
- You should get: status **200 OK**

```json
{ "data": { "status": "ok" } }
```

###### Request 2: List Dr. Amina's free slots

- Method and URL: `GET` `{{baseUrl}}/api/v1/doctors/{{doctorId}}/availability?date={{slotDate}}`
- Body: none. (If you paste the URL with `?date=...` on the end, Postman fills in the **Params** tab itself.)
- You should get: status **200 OK**, and 16 half-hour slots from 09:00 to 16:30. The first two are shown here, and 14 more follow in the same shape.

```json
{
  "data": {
    "doctor_id": "d0000000-0000-4000-8000-000000000001",
    "date": "2026-11-02",
    "timezone": "Africa/Nairobi",
    "slots": [
      { "start": "2026-11-02T09:00:00+03:00", "end": "2026-11-02T09:30:00+03:00" },
      { "start": "2026-11-02T09:30:00+03:00", "end": "2026-11-02T10:00:00+03:00" }
    ]
  }
}
```

###### Request 3: Book 09:00 as Grace

- Method and URL: `POST` `{{baseUrl}}/api/v1/appointments`
- Body (raw, JSON):

```json
{
  "doctor_id": "{{doctorId}}",
  "patient_id": "{{patientId}}",
  "slot_start": "{{slotDate}}T09:00:00+03:00"
}
```

- You should get: status **201 Created**. The `id` is generated by the server, so yours will be different. Notice `status` is `BOOKED`.

```json
{
  "data": {
    "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
    "doctor_id": "d0000000-0000-4000-8000-000000000001",
    "patient_id": "a0000000-0000-4000-8000-000000000001",
    "slot_start": "2026-11-02T09:00:00+03:00",
    "slot_end": "2026-11-02T09:30:00+03:00",
    "status": "BOOKED"
  }
}
```

**Save the booking's ID.** Requests 5 to 7 work on this booking, so they need its `id`. Copy the `id` value from the reply . Then open your `Savannah` environment, paste it into the `appointmentId` row, and press `Ctrl+S`. Requests 5 to 7 already point at `{{appointmentId}}`, so you don't have to edit them.

If you send request 3 a second time, you will get a `409` because 09:00 is now taken. That is the API doing its job. To start over, change `slotDate` to a later Monday.

###### Request 4: Try to book the same slot as Peter

- Method and URL: `POST` `{{baseUrl}}/api/v1/appointments`
- Body: the same as request 3, but with Peter's ID:

```json
{
  "doctor_id": "{{doctorId}}",
  "patient_id": "{{patientId2}}",
  "slot_start": "{{slotDate}}T09:00:00+03:00"
}
```

- You should get: status **409 Conflict**. The slot is already Grace's, so nobody else can have it.

```json
{ "message": "that slot is already booked" }
```

###### Request 5: Move Grace's booking to 10:00

- Method and URL: `PATCH` `{{baseUrl}}/api/v1/appointments/{{appointmentId}}/reschedule`
- Body:

```json
{ "slot_start": "{{slotDate}}T10:00:00+03:00" }
```

- You should get: status **200 OK**. The `id` is the same, and the times have moved to 10:00.

```json
{
  "data": {
    "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
    "doctor_id": "d0000000-0000-4000-8000-000000000001",
    "patient_id": "a0000000-0000-4000-8000-000000000001",
    "slot_start": "2026-11-02T10:00:00+03:00",
    "slot_end": "2026-11-02T10:30:00+03:00",
    "status": "BOOKED"
  }
}
```

###### Request 6: Cancel it

- Method and URL: `PATCH` `{{baseUrl}}/api/v1/appointments/{{appointmentId}}/cancel`
- Body:

```json
{ "reason": "Patient is travelling" }
```

- You should get: status **200 OK**. The status is now `CANCELLED`, and two new fields appear: the reason and the time it was cancelled. The slot is free again, so you can run request 2 once more to see it back in the list.

```json
{
  "data": {
    "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
    "doctor_id": "d0000000-0000-4000-8000-000000000001",
    "patient_id": "a0000000-0000-4000-8000-000000000001",
    "slot_start": "2026-11-02T10:00:00+03:00",
    "slot_end": "2026-11-02T10:30:00+03:00",
    "status": "CANCELLED",
    "cancel_reason": "Patient is travelling",
    "cancelled_at": "2026-10-21T14:05:12.481923+03:00"
  }
}
```

###### Request 7: Cancel it again

- Method and URL: the same as request 6
- Body:

```json
{ "reason": "Again" }
```

- You should get: status **409 Conflict**, because it is already cancelled.

```json
{ "message": "appointment is already cancelled" }
```

###### Request 8: Try to book a time in the past

- Method and URL: `POST` `{{baseUrl}}/api/v1/appointments`
- Body:

```json
{
  "doctor_id": "{{doctorId}}",
  "patient_id": "{{patientId}}",
  "slot_start": "2020-01-06T09:00:00+03:00"
}
```

- You should get: status **422 Unprocessable Entity**. The request is well formed, but it breaks a booking rule.

```json
{ "message": "cannot book a slot in the past" }
```

For one more error, send request 3 with an empty body `{}`. You get **422** and a list saying which fields are missing:

```json
{ "message": "validation failed", "errors": ["doctor_id is required", "patient_id is required", "slot_start is required"] }
```

##### If a request doesn't behave

- **"Could not get response" or it just hangs:** the address is wrong, or the server isn't reachable. Check `baseUrl`, and that the environment is selected (Step 2, point 4).
- **A booking returns `422` about the past or "too soon":** your `slotDate` has gone by. Change it to a later Monday.
- **Request 3 returns `409`:** that slot is already booked from an earlier run. Change `slotDate` to a later Monday, or use a different time.
- **Requests 5 to 7 return `400` or `404`:** `appointmentId` is empty or has the wrong ID. Paste the `id` from request 3's reply into the environment again and save.

#### Just want to look? Use a browser

Anything that only reads data works in a normal browser tab, with nothing to install. Paste these in:

- http://143.198.221.153/health
- http://143.198.221.153/api/v1/doctors
- http://143.198.221.153/api/v1/doctors/d0000000-0000-4000-8000-000000000001/availability?date=2026-11-02

A browser can't book, reschedule or cancel, because those send data. Use Postman for that, or the terminal below. (If the availability date has passed by the time you read this, change it to any future Monday.)

#### Terminal (optional)

If you'd rather not install Postman, one command shows how a booking works. It books Dr. Amina at 09:00 for Grace. Use any future Monday for the date.

Windows PowerShell (type `curl.exe`, not `curl`; in Windows PowerShell plain `curl` is a different command):

```powershell
'{"doctor_id":"d0000000-0000-4000-8000-000000000001","patient_id":"a0000000-0000-4000-8000-000000000001","slot_start":"2026-11-02T09:00:00+03:00"}' | curl.exe -s -X POST "http://143.198.221.153/api/v1/appointments" -H "Content-Type: application/json" --data-binary "@-" -w "`nHTTP %{http_code}`n"
```

Linux and macOS:

```bash
curl -s -X POST http://143.198.221.153/api/v1/appointments -H "Content-Type: application/json" -w '\nHTTP %{http_code}\n' \
  -d '{"doctor_id":"d0000000-0000-4000-8000-000000000001","patient_id":"a0000000-0000-4000-8000-000000000001","slot_start":"2026-11-02T09:00:00+03:00"}'
```

The first run returns `201` with the booking. Run it again and you get `409`, `that slot is already booked`. To cancel or reschedule, send a `PATCH` to `/api/v1/appointments/{id}/cancel` (body `{"reason":"..."}`) or `/reschedule` (body `{"slot_start":"..."}`), using the `id` from the booking response.

#### Response format at a glance

Every successful reply is wrapped in `{"data": ...}`. Every error is `{"message": "..."}`, and field validation errors add an `errors` list. The examples under each Postman request above show the exact shapes.

The bonus list, `GET /api/v1/patients/{id}/appointments`, returns the patient's upcoming bookings, soonest first. Each item has `id`, `doctor_id`, `doctor_name`, `slot_start`, `slot_end` and `status`. It returns an empty list `[]` if there are none.

The status codes and what causes each are in the table under [Status codes](#status-codes).

### Automated tests

```
go test ./...            # unit tests; integration tests skip themselves without a database
```

The integration tests run against real Postgres, because the no-double-booking promise is enforced by a database index and a mock would prove nothing. Point them at the local database:

```powershell
$env:TEST_DATABASE_URL="postgres://clinic:clinicpass@localhost:5436/clinic?sslmode=disable"
go test ./... -count=1 -v
```

Each test makes its own doctor and patients with random IDs and cleans up afterwards, so the seed data is never touched. The booking rules use a fixed clock, so results don't depend on what day you run them.

| Where | What it proves |
|---|---|
| `utils/slots` | Slot generation, availability filtering, and every validation rule, including the boundaries (exactly at the lead-time cut-off, last slot of the day, closing time) |
| `http/errmap` | Each domain error maps to the right status, and internal errors never leak |
| `appointment` (integration) | Double booking is refused; **10 concurrent requests for one slot give exactly 1 winner**; a patient can't be in two places at once; cancelling frees the slot; rescheduling frees the old slot, leaves the original alone on a conflict, validates like a fresh booking, and refuses cancelled or already-started appointments |

---

## 3. Deployment and CI/CD

**Public URL:** http://143.198.221.153

**Branch that triggers a deployment:** `main`. A pull request merged into `main` is a push to `main`, and that starts the Deploy workflow. GitHub branch protection on `main` requires a pull request and a passing `test` check, so nothing reaches production without green tests.

### The pipeline

Two GitHub Actions workflows live in `.github/workflows/`.

**`ci.yml`, on every pull request**
1. Starts a Postgres 16 service container.
2. Runs `go vet ./...`.
3. Applies the goose migrations to that database.
4. Runs `go test ./... -race -count=1` with `TEST_DATABASE_URL` set, so the integration tests (including the concurrent double-booking test) hit real Postgres.

**`deploy.yml`, on push to `main`**
1. Reuses `ci.yml`. If tests fail, nothing is deployed.
2. Connects to the droplet over SSH with a dedicated deploy key.
3. On the droplet, fetches that exact commit from the public GitHub repo and runs `docker compose up -d --build`. A one-shot `migrate` service applies any pending migrations first, and the API only starts once it has succeeded.
4. Smoke test: polls `/health` until it answers, and fails the workflow if it never does.

On the droplet the stack is Caddy (the only thing with published ports), then the API, then Postgres. Postgres and the API aren't reachable from the internet. The database credentials live in a `.env` file on the server only; GitHub holds just the SSH key, host, user and public URL as secrets. The image holds three binaries (`api`, `migrate`, `seed`) on a distroless base.

### Why there's no dev or UAT branch

A common setup is feature branch, then a `dev` branch that deploys to a UAT server where the code gets tested, then `main` to production. I don't have two servers, and paying for a second droplet just to have a staging environment on a project this size didn't make sense. So I made the gate before merge do that job instead:

- Work happens on short-lived branches (`ft-` for features, `bg-` for bugs, `ch-` for chores) and goes to `main` through a pull request.
- The pull request runs the full suite against a real Postgres, which is the closest thing to UAT I can run for free. The deploy runs the same tests again before it touches the server.
- Before the first deploy I tested the production Docker stack on my own machine with the steps in `deploy/SETUP.md`, so the compose file and image were proven before the droplet ever saw them.
- If a deploy goes wrong, the server always checks out an exact commit SHA, so rolling back is a one-line change.

When a second environment is worth paying for, the change is small: adding a `dev` branch, point a second copy of the deploy job at a staging server for pushes to `dev`, and keep `main` for production, ideally behind a GitHub Environments approval. The workflows are already written so that only the target host and branch need to change.

### Other decisions

**Build on the server, no container registry.** For one small service that means fewer moving parts: no registry, tags or package permissions. The costs are that the droplet's CPU and memory do the build (so a 1 GB droplet gets swap), and production runs a fresh build of the commit rather than the exact image CI tested. The next step as it grows is to build once in CI, push to a registry and pull on the server.

**Other safeguards.** Deploys are serialised, so a second merge waits for the one running. Production is plain HTTP on the droplet IP because there is no domain; `deploy/SETUP.md` explains how to get HTTPS with a free `sslip.io` hostname.

The full setup steps are in [`deploy/SETUP.md`](deploy/SETUP.md).

---

## 4. Limitations and next steps

None of these are needed for a 5-doctor clinic on one droplet, so I left them out on purpose and wrote down what I would do next. I would rather ship the brief done well than add features nobody asked for.

- **No login.** Anyone with a patient ID can act for that patient. Next: JWT, with the patient taken from the token.
- **Plain HTTP.** No domain, so no certificate. Next: a domain or `sslip.io` hostname so Caddy can issue HTTPS.
- **No backups.** The Postgres volume lives on the droplet. Next: scheduled dumps to object storage, or managed Postgres.
- **One working window per weekday.** No lunch breaks, leave or public holidays. Next: several windows per day plus an exceptions table.
- **No create or edit endpoints for doctors and patients.** Data comes from the seed command.
- **No pagination or rate limiting** on the list endpoints.
- **No staging environment.** Covered in section 3.
- **Server-side builds.** Build once in CI and use a registry when it matters.
- **Limited observability.** Logs carry a request ID, but there are no metrics or alerts yet.
- **Live reload is set up but optional.** `make dev` uses air for fast feedback while coding. A fuller dev setup (the API running in Docker with the source mounted) could come later.

---

## 5. AI reflection

**1. What I used AI for across the four sections**

- **Section 1, design:** the decisions are mine (they are written out in section 1). I only used AI in this section to help me write the README.
- **Section 2, API:** I used AI to write the tests, including the integration tests and the concurrent double-booking test. I also used it a lot for goose and SQL. I hadn't worked with goose for a long time and I was rusty on writing raw SQL queries for sqlc, so having AI show me the migration commands, the query syntax and the Makefile targets got me going much faster, and the result was better than I would have managed alone in the same time.I used it to generate generic functions with the core function structure being written by me.
- **Section 3, deployment and CI/CD:** AI helped me write the GitHub Actions workflows, the production compose file and the deployment steps. I told it what I wanted (tests on every pull request, deploy on merge, no registry, no domain) and it helped me turn that into working YAML and shell steps.
- **Section 4, this README:** AI helped me draft and tidy it from my notes. I checked it against the code.

**2. One example where AI improved my work**

Goose. I told it, roughly, that I hadn't used goose in a long time and needed migrations that I could run locally, in CI and inside Docker. It suggested compiling a small migration runner into its own binary (`cmd/migrate`) instead of installing goose on the server. That works well with the distroless image, which has no shell or Go, and it means CI, local and production all run migrations the same way. I wouldn't have thought of that as quickly, and it also made the compose `migrate` step simple.

**3. One example where AI output was wrong or incomplete**

The database connection pool. The first version of the API it gave me opened the database without setting any pool limits, so the library's defaults decided how many connections the app would hold. That's fine on a laptop, but on a 1 GB droplet sharing memory with Postgres and Caddy I want to set those numbers on purpose. I noticed it when I went through `main.go` before deploying, and I changed it to set explicit limits (max and min connections, connection lifetime and idle time) myself.Ai wasnt wrong per se,on this other section,but i wanted it to explicitly use my structure for the project,the functions,methods,helpers,so that everything would be designed for posterity

**4. Two decisions I made without AI**

- **How the project is structured.** The monorepo-style layout, the folders (`cmd`, `db`, `http/resources`, `libs`, `middleware`, `utils`, `deploy`) and where each file goes were my call, copied from how I structure my larger backend. I trusted my own judgment because I've used this shape in real projects and I know why each folder exists and what breaks if you skip it. It makes adding a service a matter of adding folders, it keeps the booking rules in one place you can test without a database, and it is a layout other Go teams recognise, so a new person can find things without a tour.Also how to structur the services,basically someone can be able to create a parent directory,and have this current implementation as one service,then many other services running independently. 
- **The tools and the code structure.** Go with Fiber, pgx, sqlc and Postgres, a controller, model and repository layer per resource, domain errors mapped to HTTP in one place, and a shared response shape. I trusted my own judgment here too. These are well-tested choices that I've seen hold up in production, so I know why I'm picking them and when they stop being the right answer. They scale with the project, they are what much of the industry uses, and from my own experience they were the better thing to do for a system that has to start small and keep growing. AI helped me with some of the commands on the way (goose, for example), but the choice of tools and the shape,structure,formatting of the code were decided by me before I asked it anything.