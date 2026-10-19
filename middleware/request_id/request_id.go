package requestId

import (
	ctxUtil "github.com/PowellHabwe/Savannah-HMIS/utils/ctx"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/xid"
)

const (
	requestIDHeaderKey = "X-Request-ID"
	maxRequestIDLen    = 64
)

func isSafe(s string) bool {
	if s == "" || len(s) > maxRequestIDLen {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Get(requestIDHeaderKey)
		if !isSafe(id) {
			id = xid.New().String()
		}
		ctxUtil.SetRequestID(c, id)
		c.Set(requestIDHeaderKey, id)
		return c.Next()
	}
}
