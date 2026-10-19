-- name: GetDoctorById :one
SELECT id, full_name, specialty
FROM doctor
WHERE id = $1;

-- name: ListDoctors :many
SELECT id, full_name, specialty
FROM doctor
ORDER BY full_name;

-- name: GetWorkingHoursForWeekday :one
SELECT doctor_id, weekday, start_time, end_time
FROM doctor_working_hours
WHERE doctor_id = sqlc.arg(doctor_id)
  AND weekday = sqlc.arg(weekday);
