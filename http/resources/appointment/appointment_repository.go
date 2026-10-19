package appointment

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
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	pgUniqueViolation  = "23505"
	doctorSlotIndex    = "appointment_doctor_slot_booked_uq"
	patientSlotIndex   = "appointment_patient_slot_booked_uq"
)

type AppointmentRepository struct {
	*httpconfig.ControllerParams
	now func() time.Time 
}

func NewAppointmentRepository(params *httpconfig.ControllerParams) *AppointmentRepository {
	return &AppointmentRepository{ControllerParams: params, now: time.Now}
}

func (r *AppointmentRepository) minLead() time.Duration {
	return time.Duration(r.Config.Clinic.MinLeadMinutes) * time.Minute
}


func mapInsertError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		switch pgErr.ConstraintName {
		case doctorSlotIndex:
			return apperr.ErrSlotTaken
		case patientSlotIndex:
			return apperr.ErrPatientBusy
		}
	}
	return err
}


func (r *AppointmentRepository) windowFor(
	ctx context.Context,
	q *db.Queries,
	doctorID uuid.UUID,
	start time.Time,
) (*slots.WorkingWindow, error) {
	wh, err := q.GetWorkingHoursForWeekday(ctx, &db.GetWorkingHoursForWeekdayParams{
		DoctorID: doctorID,
		Weekday:  int16(start.In(r.Location).Weekday()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get working hours: %w", err)
	}
	w := slots.WindowFromMicros(wh.StartTime.Microseconds, wh.EndTime.Microseconds)
	return &w, nil
}

func (r *AppointmentRepository) Book(ctx context.Context, p *BookParams) (*AppointmentResponse, error) {
	if _, err := r.Queries.GetDoctorById(ctx, p.DoctorID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrDoctorNotFound
		}
		return nil, fmt.Errorf("get doctor: %w", err)
	}
	if _, err := r.Queries.GetPatientById(ctx, p.PatientID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrPatientNotFound
		}
		return nil, fmt.Errorf("get patient: %w", err)
	}

	window, err := r.windowFor(ctx, r.Queries, p.DoctorID, p.SlotStart)
	if err != nil {
		return nil, err
	}
	if err := slots.ValidateBooking(p.SlotStart, window, r.Location, r.now(), r.minLead()); err != nil {
		return nil, err
	}

	appt, err := r.Queries.CreateAppointment(ctx, &db.CreateAppointmentParams{
		DoctorID:  p.DoctorID,
		PatientID: p.PatientID,
		SlotStart: pgtypeUtil.TimeToPgTimestampTz(p.SlotStart.UTC()),
		SlotEnd:   pgtypeUtil.TimeToPgTimestampTz(p.SlotStart.Add(slots.SlotDuration).UTC()),
	})
	if err != nil {
		if mapped := mapInsertError(err); mapped != err {
			return nil, mapped
		}
		return nil, fmt.Errorf("create appointment: %w", err)
	}
	return toResponse(appt, r.Location), nil
}

func (r *AppointmentRepository) GetByID(ctx context.Context, id uuid.UUID) (*AppointmentResponse, error) {
	appt, err := r.Queries.GetAppointmentById(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrAppointmentNotFound
		}
		return nil, fmt.Errorf("get appointment: %w", err)
	}
	return toResponse(appt, r.Location), nil
}


func (r *AppointmentRepository) lockActive(ctx context.Context, qtx *db.Queries, id uuid.UUID) (*db.Appointment, error) {
	appt, err := qtx.GetAppointmentByIdForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrAppointmentNotFound
		}
		return nil, fmt.Errorf("lock appointment: %w", err)
	}
	if appt.Status == db.AppointmentStatusCANCELLED {
		return nil, apperr.ErrAlreadyCancelled
	}
	if !appt.SlotStart.Time.After(r.now()) {
		return nil, apperr.ErrAppointmentPassed
	}
	return appt, nil
}

func (r *AppointmentRepository) Cancel(ctx context.Context, id uuid.UUID, reason string) (*AppointmentResponse, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := r.Queries.WithTx(tx)

	if _, err := r.lockActive(ctx, qtx, id); err != nil {
		return nil, err
	}

	cancelled, err := qtx.CancelAppointment(ctx, &db.CancelAppointmentParams{
		CancelReason: pgtypeUtil.StringToPgTypeText(reason),
		ID:           id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrAlreadyCancelled
		}
		return nil, fmt.Errorf("cancel appointment: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return toResponse(cancelled, r.Location), nil
}


func (r *AppointmentRepository) Reschedule(ctx context.Context, id uuid.UUID, newStart time.Time) (*AppointmentResponse, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := r.Queries.WithTx(tx)

	appt, err := r.lockActive(ctx, qtx, id)
	if err != nil {
		return nil, err
	}

	window, err := r.windowFor(ctx, qtx, appt.DoctorID, newStart)
	if err != nil {
		return nil, err
	}
	if err := slots.ValidateBooking(newStart, window, r.Location, r.now(), r.minLead()); err != nil {
		return nil, err
	}

	moved, err := qtx.RescheduleAppointment(ctx, &db.RescheduleAppointmentParams{
		SlotStart: pgtypeUtil.TimeToPgTimestampTz(newStart.UTC()),
		SlotEnd:   pgtypeUtil.TimeToPgTimestampTz(newStart.Add(slots.SlotDuration).UTC()),
		ID:        id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrAlreadyCancelled
		}
		if mapped := mapInsertError(err); mapped != err {
			return nil, mapped
		}
		return nil, fmt.Errorf("reschedule appointment: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return toResponse(moved, r.Location), nil
}
