package apperr

import "errors"

var (
	ErrDoctorNotFound      = errors.New("doctor not found")
	ErrPatientNotFound     = errors.New("patient not found")
	ErrAppointmentNotFound = errors.New("appointment not found")
	ErrSlotTaken        = errors.New("that slot is already booked")
	ErrPatientBusy      = errors.New("the patient already has an appointment at that time")
	ErrAlreadyCancelled = errors.New("appointment is already cancelled")
	ErrAppointmentPassed = errors.New("appointment has already started and can no longer be changed")
)
