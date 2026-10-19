-- name: GetPatientById :one
SELECT id, full_name, phone, email
FROM patient
WHERE id = $1;
