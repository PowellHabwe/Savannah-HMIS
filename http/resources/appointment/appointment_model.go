package appointment

import (
	"time"

	db "github.com/PowellHabwe/Savannah-HMIS/db/sqlc"
	"github.com/google/uuid"
)

type BookParams struct {
	DoctorID  uuid.UUID `json:"doctor_id"  validate:"required"`
	PatientID uuid.UUID `json:"patient_id" validate:"required"`
	SlotStart time.Time `json:"slot_start" validate:"required"`
}

type CancelParams struct {
	Reason string `json:"reason" validate:"required,max=500"`
}

type RescheduleParams struct {
	SlotStart time.Time `json:"slot_start" validate:"required"`
}

type AppointmentResponse struct {
	ID           uuid.UUID  `json:"id"`
	DoctorID     uuid.UUID  `json:"doctor_id"`
	PatientID    uuid.UUID  `json:"patient_id"`
	SlotStart    time.Time  `json:"slot_start"`
	SlotEnd      time.Time  `json:"slot_end"`
	Status       string     `json:"status"`
	CancelReason *string    `json:"cancel_reason,omitempty"`
	CancelledAt  *time.Time `json:"cancelled_at,omitempty"`
}

func toResponse(a *db.Appointment, loc *time.Location) *AppointmentResponse {
	r := &AppointmentResponse{
		ID:        a.ID,
		DoctorID:  a.DoctorID,
		PatientID: a.PatientID,
		SlotStart: a.SlotStart.Time.In(loc),
		SlotEnd:   a.SlotEnd.Time.In(loc),
		Status:    string(a.Status),
	}
	if a.CancelReason.Valid {
		reason := a.CancelReason.String
		r.CancelReason = &reason
	}
	if a.CancelledAt.Valid {
		t := a.CancelledAt.Time.In(loc)
		r.CancelledAt = &t
	}
	return r
}
