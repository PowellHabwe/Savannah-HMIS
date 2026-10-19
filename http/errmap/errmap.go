package errmap

import (
	"errors"

	"github.com/PowellHabwe/Savannah-HMIS/libs/apperr"
	"github.com/PowellHabwe/Savannah-HMIS/libs/constants"
	"github.com/PowellHabwe/Savannah-HMIS/libs/response"
	ctxUtil "github.com/PowellHabwe/Savannah-HMIS/utils/ctx"
	"github.com/PowellHabwe/Savannah-HMIS/utils/slots"
	validatorUtil "github.com/PowellHabwe/Savannah-HMIS/utils/validator"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)


func Respond(c *fiber.Ctx, l *zerolog.Logger, err error) error {
	switch {
	case errors.Is(err, apperr.ErrDoctorNotFound),
		errors.Is(err, apperr.ErrPatientNotFound),
		errors.Is(err, apperr.ErrAppointmentNotFound):
		return response.NotFound(c, err)

	case errors.Is(err, apperr.ErrSlotTaken),
		errors.Is(err, apperr.ErrPatientBusy),
		errors.Is(err, apperr.ErrAlreadyCancelled):
		return response.Conflict(c, err)

	case errors.Is(err, apperr.ErrAppointmentPassed),
		errors.Is(err, slots.ErrNotAligned),
		errors.Is(err, slots.ErrInPast),
		errors.Is(err, slots.ErrTooSoon),
		errors.Is(err, slots.ErrOutsideWorkingHours):
		return response.Unprocessable(c, err)
	}

	l.Error().
		Stack().
		Err(err).
		Str(constants.KeyReqID, ctxUtil.RequestID(c)).
		Msg("unexpected error")
	return response.ServerError(c, errors.New("Something went wrong. Please try again later."))
}

func Validation(c *fiber.Ctx, l *zerolog.Logger, err error) error {
	if msgs := validatorUtil.ValidatorErrors(err); len(msgs) > 0 {
		return response.ValidationErrors(c, msgs)
	}
	return Respond(c, l, err)
}
