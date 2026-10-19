package doctor

import (
	"time"

	"github.com/google/uuid"
)

type DoctorResponse struct {
	ID        uuid.UUID `json:"id"`
	FullName  string    `json:"full_name"`
	Specialty string    `json:"specialty"`
}

type SlotResponse struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type AvailabilityResponse struct {
	DoctorID uuid.UUID      `json:"doctor_id"`
	Date     string         `json:"date"`
	Timezone string         `json:"timezone"`
	Slots    []SlotResponse `json:"slots"`
}
