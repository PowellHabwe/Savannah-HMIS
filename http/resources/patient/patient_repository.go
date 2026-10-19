package patient

import (
	"context"
	"errors"
	"fmt"

	httpconfig "github.com/PowellHabwe/Savannah-HMIS/http"
	"github.com/PowellHabwe/Savannah-HMIS/libs/apperr"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PatientRepository struct {
	*httpconfig.ControllerParams
}

func NewPatientRepository(params *httpconfig.ControllerParams) *PatientRepository {
	return &PatientRepository{params}
}


func (r *PatientRepository) ListUpcomingAppointments(
	ctx context.Context,
	patientID uuid.UUID,
) ([]UpcomingAppointmentResponse, error) {
	if _, err := r.Queries.GetPatientById(ctx, patientID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrPatientNotFound
		}
		return nil, fmt.Errorf("get patient: %w", err)
	}

	rows, err := r.Queries.ListUpcomingAppointmentsForPatient(ctx, patientID)
	if err != nil {
		return nil, fmt.Errorf("list upcoming appointments: %w", err)
	}

	out := make([]UpcomingAppointmentResponse, 0, len(rows))
	for _, a := range rows {
		out = append(out, UpcomingAppointmentResponse{
			ID:         a.ID,
			DoctorID:   a.DoctorID,
			DoctorName: a.DoctorName,
			SlotStart:  a.SlotStart.Time.In(r.Location),
			SlotEnd:    a.SlotEnd.Time.In(r.Location),
			Status:     string(a.Status),
		})
	}
	return out, nil
}
