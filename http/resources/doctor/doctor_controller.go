package doctor

import (
	"errors"

	httpconfig "github.com/PowellHabwe/Savannah-HMIS/http"
	"github.com/PowellHabwe/Savannah-HMIS/http/errmap"
	"github.com/PowellHabwe/Savannah-HMIS/libs/response"
	requestlog "github.com/PowellHabwe/Savannah-HMIS/middleware/request_log"
	"github.com/PowellHabwe/Savannah-HMIS/utils/slots"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type API struct {
	logger     *zerolog.Logger
	validator  *validator.Validate
	repository *DoctorRepository
	params     *httpconfig.ControllerParams
}

func New(params *httpconfig.ControllerParams) *API {
	return &API{
		logger:     params.Logger,
		validator:  params.Validator,
		repository: NewDoctorRepository(params),
		params:     params,
	}
}

func (a *API) InitializeRoutes(routeGroup fiber.Router) {
	router := routeGroup.Group("/doctors")
	router.Get("/", requestlog.NewHandler(a.ListDoctors, a.logger))
	router.Get("/:id/availability", requestlog.NewHandler(a.GetAvailability, a.logger))
}

func (a *API) ListDoctors(c *fiber.Ctx) error {
	data, err := a.repository.ListDoctors(c.Context())
	if err != nil {
		return errmap.Respond(c, a.logger, err)
	}
	return response.ReturnData(c, data)
}

func (a *API) GetAvailability(c *fiber.Ctx) error {
	doctorID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, errors.New("invalid doctor id"))
	}

	dateStr := c.Query("date")
	if dateStr == "" {
		return response.BadRequest(c, errors.New("query parameter 'date' is required (YYYY-MM-DD)"))
	}
	date, err := slots.ParseDate(dateStr, a.params.Location)
	if err != nil {
		return response.BadRequest(c, err)
	}

	data, err := a.repository.GetAvailability(c.Context(), doctorID, date)
	if err != nil {
		return errmap.Respond(c, a.logger, err)
	}
	return response.ReturnData(c, data)
}
