package validatorUtil

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

func New() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
		if name == "" || name == "-" {
			return f.Name
		}
		return name
	})
	return v
}


func ValidatorErrors(err error) []string {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return nil
	}
	out := make([]string, 0, len(ve))
	for _, fe := range ve {
		switch fe.Tag() {
		case "required":
			out = append(out, fmt.Sprintf("%s is required", fe.Field()))
		case "uuid", "uuid4":
			out = append(out, fmt.Sprintf("%s must be a valid UUID", fe.Field()))
		case "max":
			out = append(out, fmt.Sprintf("%s must be at most %s characters", fe.Field(), fe.Param()))
		default:
			out = append(out, fmt.Sprintf("%s failed validation (%s)", fe.Field(), fe.Tag()))
		}
	}
	return out
}
