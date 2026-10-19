package patient

import (
	"time"

	"github.com/google/uuid"
)

type UpcomingAppointmentResponse struct {
	ID         uuid.UUID `json:"id"`
	DoctorID   uuid.UUID `json:"doctor_id"`
	DoctorName string    `json:"doctor_name"`
	SlotStart  time.Time `json:"slot_start"`
	SlotEnd    time.Time `json:"slot_end"`
	Status     string    `json:"status"`
}
