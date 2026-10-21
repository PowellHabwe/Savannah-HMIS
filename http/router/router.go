package router

import (
	"context"
	"time"
	"errors"

	httpconfig "github.com/PowellHabwe/Savannah-HMIS/http"
	"github.com/PowellHabwe/Savannah-HMIS/http/resources/appointment"
	"github.com/PowellHabwe/Savannah-HMIS/http/resources/doctor"
	"github.com/PowellHabwe/Savannah-HMIS/http/resources/patient"
	"github.com/PowellHabwe/Savannah-HMIS/libs/response"
	requestId "github.com/PowellHabwe/Savannah-HMIS/middleware/request_id"
	requestlog "github.com/PowellHabwe/Savannah-HMIS/middleware/request_log"
	"github.com/gofiber/fiber/v2"
)

type API interface {
	InitializeRoutes(routeGroup fiber.Router)
}

func New(app *fiber.App, params *httpconfig.ControllerParams) {
	app.Use(requestId.RequestID())

	app.Get("/health", requestlog.NewHandler(func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
		defer cancel()
	if err := params.Pool.Ping(ctx); err != nil {
		params.Logger.Error().Err(err).Msg("health check failed")
		return response.HttpError(fiber.StatusServiceUnavailable, c, errors.New("database unavailable"))
	}
		return response.ReturnData(c, fiber.Map{"status": "ok"})
	}, params.Logger))

	route := app.Group("/api/v1")

	apis := []API{
		doctor.New(params),
		appointment.New(params),
		patient.New(params),
	}
	for _, api := range apis {
		api.InitializeRoutes(route)
	}
}
