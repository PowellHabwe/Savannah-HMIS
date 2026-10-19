package patient

import (
	"errors"

	httpconfig "github.com/PowellHabwe/Savannah-HMIS/http"
	"github.com/PowellHabwe/Savannah-HMIS/http/errmap"
	"github.com/PowellHabwe/Savannah-HMIS/libs/response"
	requestlog "github.com/PowellHabwe/Savannah-HMIS/middleware/request_log"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type API struct {
	logger     *zerolog.Logger
	repository *PatientRepository
}

func New(params *httpconfig.ControllerParams) *API {
	return &API{
		logger:     params.Logger,
		repository: NewPatientRepository(params),
	}
}

func (a *API) InitializeRoutes(routeGroup fiber.Router) {
	router := routeGroup.Group("/patients")
	router.Get("/:id/appointments", requestlog.NewHandler(a.ListUpcomingAppointments, a.logger))
}

func (a *API) ListUpcomingAppointments(c *fiber.Ctx) error {
	patientID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, errors.New("invalid patient id"))
	}

	data, err := a.repository.ListUpcomingAppointments(c.Context(), patientID)
	if err != nil {
		return errmap.Respond(c, a.logger, err)
	}
	return response.ReturnData(c, data)
}
