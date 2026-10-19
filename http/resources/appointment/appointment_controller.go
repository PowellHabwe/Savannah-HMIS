package appointment

import (
	"errors"
	"strings"

	httpconfig "github.com/PowellHabwe/Savannah-HMIS/http"
	"github.com/PowellHabwe/Savannah-HMIS/http/errmap"
	"github.com/PowellHabwe/Savannah-HMIS/libs/response"
	requestlog "github.com/PowellHabwe/Savannah-HMIS/middleware/request_log"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type API struct {
	logger     *zerolog.Logger
	validator  *validator.Validate
	repository *AppointmentRepository
}

func New(params *httpconfig.ControllerParams) *API {
	return &API{
		logger:     params.Logger,
		validator:  params.Validator,
		repository: NewAppointmentRepository(params),
	}
}

func (a *API) InitializeRoutes(routeGroup fiber.Router) {
	router := routeGroup.Group("/appointments")
	router.Post("/", requestlog.NewHandler(a.Book, a.logger))
	router.Get("/:id", requestlog.NewHandler(a.GetByID, a.logger))
	router.Patch("/:id/cancel", requestlog.NewHandler(a.Cancel, a.logger))
	router.Patch("/:id/reschedule", requestlog.NewHandler(a.Reschedule, a.logger))
}

var errBadBody = errors.New("invalid request body: expected JSON (slot_start must be RFC 3339, e.g. 2026-10-19T09:00:00+03:00)")

func parseID(c *fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, errors.New("invalid appointment id")
	}
	return id, nil
}

func (a *API) Book(c *fiber.Ctx) error {
	params := &BookParams{}
	if err := c.BodyParser(params); err != nil {
		return response.BadRequest(c, errBadBody)
	}
	if err := a.validator.Struct(params); err != nil {
		return errmap.Validation(c, a.logger, err)
	}

	appt, err := a.repository.Book(c.Context(), params)
	if err != nil {
		return errmap.Respond(c, a.logger, err)
	}
	return response.ReturnCreated(c, appt)
}

func (a *API) GetByID(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return response.BadRequest(c, err)
	}
	appt, err := a.repository.GetByID(c.Context(), id)
	if err != nil {
		return errmap.Respond(c, a.logger, err)
	}
	return response.ReturnData(c, appt)
}

func (a *API) Cancel(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return response.BadRequest(c, err)
	}

	params := &CancelParams{}
	if err := c.BodyParser(params); err != nil {
		return response.BadRequest(c, errors.New("invalid request body: expected JSON with a 'reason'"))
	}
	params.Reason = strings.TrimSpace(params.Reason)
	if err := a.validator.Struct(params); err != nil {
		return errmap.Validation(c, a.logger, err)
	}

	appt, err := a.repository.Cancel(c.Context(), id, params.Reason)
	if err != nil {
		return errmap.Respond(c, a.logger, err)
	}
	return response.ReturnData(c, appt)
}

func (a *API) Reschedule(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return response.BadRequest(c, err)
	}

	params := &RescheduleParams{}
	if err := c.BodyParser(params); err != nil {
		return response.BadRequest(c, errBadBody)
	}
	if err := a.validator.Struct(params); err != nil {
		return errmap.Validation(c, a.logger, err)
	}

	appt, err := a.repository.Reschedule(c.Context(), id, params.SlotStart)
	if err != nil {
		return errmap.Respond(c, a.logger, err)
	}
	return response.ReturnData(c, appt)
}
