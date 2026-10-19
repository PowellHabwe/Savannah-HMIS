package doctor

import (
	"context"
	"errors"
	"fmt"
	"time"

	db "github.com/PowellHabwe/Savannah-HMIS/db/sqlc"
	httpconfig "github.com/PowellHabwe/Savannah-HMIS/http"
	"github.com/PowellHabwe/Savannah-HMIS/libs/apperr"
	pgtypeUtil "github.com/PowellHabwe/Savannah-HMIS/utils/pgtype"
	"github.com/PowellHabwe/Savannah-HMIS/utils/slots"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type DoctorRepository struct {
	*httpconfig.ControllerParams
	now func() time.Time 
}

func NewDoctorRepository(params *httpconfig.ControllerParams) *DoctorRepository {
	return &DoctorRepository{ControllerParams: params, now: time.Now}
}

func (r *DoctorRepository) ListDoctors(ctx context.Context) ([]DoctorResponse, error) {
	rows, err := r.Queries.ListDoctors(ctx)
	if err != nil {
		return nil, fmt.Errorf("list doctors: %w", err)
	}
	out := make([]DoctorResponse, 0, len(rows))
	for _, d := range rows {
		out = append(out, DoctorResponse{ID: d.ID, FullName: d.FullName, Specialty: d.Specialty})
	}
	return out, nil
}


func (r *DoctorRepository) GetAvailability(
	ctx context.Context,
	doctorID uuid.UUID,
	date time.Time,
) (*AvailabilityResponse, error) {
	if _, err := r.Queries.GetDoctorById(ctx, doctorID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrDoctorNotFound
		}
		return nil, fmt.Errorf("get doctor: %w", err)
	}

	resp := &AvailabilityResponse{
		DoctorID: doctorID,
		Date:     date.Format("2006-01-02"),
		Timezone: r.Location.String(),
		Slots:    []SlotResponse{},
	}

	wh, err := r.Queries.GetWorkingHoursForWeekday(ctx, &db.GetWorkingHoursForWeekdayParams{
		DoctorID: doctorID,
		Weekday:  int16(date.Weekday()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return resp, nil
		}
		return nil, fmt.Errorf("get working hours: %w", err)
	}

	window := slots.WindowFromMicros(wh.StartTime.Microseconds, wh.EndTime.Microseconds)
	all := slots.Generate(date, window, r.Location)

	dayStart, dayEnd := slots.DayBounds(date, r.Location)
	bookedRows, err := r.Queries.ListBookedSlotStarts(ctx, &db.ListBookedSlotStartsParams{
		DoctorID:   doctorID,
		RangeStart: pgtypeUtil.TimeToPgTimestampTz(dayStart.UTC()),
		RangeEnd:   pgtypeUtil.TimeToPgTimestampTz(dayEnd.UTC()),
	})
	if err != nil {
		return nil, fmt.Errorf("list booked slots: %w", err)
	}
	booked := make([]time.Time, 0, len(bookedRows))
	for _, b := range bookedRows {
		booked = append(booked, b.Time)
	}

	lead := time.Duration(r.Config.Clinic.MinLeadMinutes) * time.Minute
	for _, s := range slots.Available(all, booked, r.now(), lead) {
		resp.Slots = append(resp.Slots, SlotResponse{Start: s, End: s.Add(slots.SlotDuration)})
	}
	return resp, nil
}
