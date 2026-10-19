package requestlog

import (
	"time"

	ctxUtil "github.com/PowellHabwe/Savannah-HMIS/utils/ctx"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

func NewHandler(h fiber.Handler, l *zerolog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := h(c)

		l.Info().
			Str("request_id", ctxUtil.RequestID(c)).
			Str("method", c.Method()).
			Str("url", c.OriginalURL()).
			Str("agent", c.Get("User-Agent")).
			Str("remote_ip", c.IP()).
			Int("status", c.Response().StatusCode()).
			Int("req_body_size", len(c.Request().Body())).
			Int("resp_body_size", len(c.Response().Body())).
			Dur("latency", time.Since(start)).
			Msg("request") 
		return err
	}
}
