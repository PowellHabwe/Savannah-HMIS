-- name: CreateAppointment :one
INSERT INTO appointment (doctor_id, patient_id, slot_start, slot_end)
VALUES (sqlc.arg(doctor_id), sqlc.arg(patient_id), sqlc.arg(slot_start), sqlc.arg(slot_end))
RETURNING *;

-- name: GetAppointmentById :one
SELECT * FROM appointment WHERE id = $1;

-- Used inside a transaction by cancel/reschedule to serialise concurrent changes.
-- name: GetAppointmentByIdForUpdate :one
SELECT * FROM appointment WHERE id = $1 FOR UPDATE;

-- Start times of active bookings for one doctor in [range_start, range_end).
-- name: ListBookedSlotStarts :many
SELECT slot_start
FROM appointment
WHERE doctor_id = sqlc.arg(doctor_id)
  AND status = 'BOOKED'
  AND slot_start >= sqlc.arg(range_start)
  AND slot_start <  sqlc.arg(range_end)
ORDER BY slot_start;

-- name: CancelAppointment :one
UPDATE appointment
SET status = 'CANCELLED',
    cancel_reason = sqlc.arg(cancel_reason),
    cancelled_at = now(),
    updated_on = now()
WHERE id = sqlc.arg(id)
  AND status = 'BOOKED'
RETURNING *;

-- name: RescheduleAppointment :one
UPDATE appointment
SET slot_start = sqlc.arg(slot_start),
    slot_end = sqlc.arg(slot_end),
    updated_on = now()
WHERE id = sqlc.arg(id)
  AND status = 'BOOKED'
RETURNING *;

-- name: ListUpcomingAppointmentsForPatient :many
SELECT
    a.id,
    a.doctor_id,
    d.full_name AS doctor_name,
    a.slot_start,
    a.slot_end,
    a.status
FROM appointment a
JOIN doctor d ON d.id = a.doctor_id
WHERE a.patient_id = $1
  AND a.status = 'BOOKED'
  AND a.slot_start > now()
ORDER BY a.slot_start ASC;
