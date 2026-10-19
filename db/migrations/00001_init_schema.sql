-- +goose Up
CREATE TYPE appointment_status AS ENUM ('BOOKED', 'CANCELLED');

CREATE TABLE doctor (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name  text NOT NULL,
    specialty  text NOT NULL,
    created_on timestamptz NOT NULL DEFAULT now(),
    updated_on timestamptz NOT NULL DEFAULT now()
);

-- weekday follows Go's time.Weekday: 0 = Sunday ... 6 = Saturday.
-- start_time / end_time are wall-clock times in the clinic timezone.
CREATE TABLE doctor_working_hours (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    doctor_id  uuid NOT NULL REFERENCES doctor (id) ON DELETE CASCADE,
    weekday    smallint NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_time time NOT NULL,
    end_time   time NOT NULL,
    CONSTRAINT working_hours_order CHECK (end_time > start_time),
    CONSTRAINT working_hours_doctor_weekday_uq UNIQUE (doctor_id, weekday)
);

CREATE TABLE patient (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name  text NOT NULL,
    phone      text NOT NULL,
    email      text,
    created_on timestamptz NOT NULL DEFAULT now(),
    updated_on timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE appointment (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    doctor_id     uuid NOT NULL REFERENCES doctor (id) ON DELETE RESTRICT,
    patient_id    uuid NOT NULL REFERENCES patient (id) ON DELETE RESTRICT,
    slot_start    timestamptz NOT NULL,
    slot_end      timestamptz NOT NULL,
    status        appointment_status NOT NULL DEFAULT 'BOOKED',
    cancel_reason text,
    cancelled_at  timestamptz,
    created_on    timestamptz NOT NULL DEFAULT now(),
    updated_on    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT appointment_slot_length CHECK (slot_end = slot_start + interval '30 minutes'),
    CONSTRAINT appointment_cancel_consistency CHECK ((status = 'CANCELLED') = (cancelled_at IS NOT NULL))
);

-- The core guarantee: a doctor's slot can have at most ONE active booking.
-- Cancelled rows are excluded, so cancelling frees the slot automatically
-- while keeping the history.
CREATE UNIQUE INDEX appointment_doctor_slot_booked_uq
    ON appointment (doctor_id, slot_start)
    WHERE status = 'BOOKED';

-- A patient cannot be in two places at the same time.
CREATE UNIQUE INDEX appointment_patient_slot_booked_uq
    ON appointment (patient_id, slot_start)
    WHERE status = 'BOOKED';

CREATE INDEX appointment_patient_start_idx ON appointment (patient_id, slot_start);
CREATE INDEX appointment_doctor_start_idx  ON appointment (doctor_id, slot_start);

-- +goose Down
DROP TABLE IF EXISTS appointment;
DROP TABLE IF EXISTS patient;
DROP TABLE IF EXISTS doctor_working_hours;
DROP TABLE IF EXISTS doctor;
DROP TYPE IF EXISTS appointment_status;
