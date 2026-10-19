package ctxUtil

import (
	"github.com/PowellHabwe/Savannah-HMIS/libs/constants"
	"github.com/gofiber/fiber/v2"
)

func RequestID(c *fiber.Ctx) string {
	id, _ := c.Locals(constants.KeyReqID).(string)
	return id
}

func SetRequestID(c *fiber.Ctx, requestID string) {
	c.Locals(constants.KeyReqID, requestID)
}
