package response

import "github.com/gofiber/fiber/v2"


func HttpError(status int, c *fiber.Ctx, err error) error {
	return c.Status(status).JSON(fiber.Map{"message": err.Error()})
}

func ServerError(c *fiber.Ctx, err error) error {
	return HttpError(fiber.StatusInternalServerError, c, err)
}

func BadRequest(c *fiber.Ctx, err error) error {
	return HttpError(fiber.StatusBadRequest, c, err)
}

func NotFound(c *fiber.Ctx, err error) error {
	return HttpError(fiber.StatusNotFound, c, err)
}

func Conflict(c *fiber.Ctx, err error) error {
	return HttpError(fiber.StatusConflict, c, err)
}

func Unprocessable(c *fiber.Ctx, err error) error {
	return HttpError(fiber.StatusUnprocessableEntity, c, err)
}

func ValidationErrors(c *fiber.Ctx, errs []string) error {
	return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
		"message": "validation failed",
		"errors":  errs,
	})
}

func ReturnData(c *fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": data})
}

func ReturnCreated(c *fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": data})
}
