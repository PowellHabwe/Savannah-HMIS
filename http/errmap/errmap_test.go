package errmap_test

import (
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PowellHabwe/Savannah-HMIS/http/errmap"
	"github.com/PowellHabwe/Savannah-HMIS/libs/apperr"
	"github.com/PowellHabwe/Savannah-HMIS/utils/slots"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

func TestRespondMapsErrorsToStatusCodes(t *testing.T) {
	l := zerolog.Nop()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"doctor not found", apperr.ErrDoctorNotFound, 404},
		{"patient not found", apperr.ErrPatientNotFound, 404},
		{"appointment not found", apperr.ErrAppointmentNotFound, 404},
		{"slot taken", apperr.ErrSlotTaken, 409},
		{"patient busy", apperr.ErrPatientBusy, 409},
		{"already cancelled", apperr.ErrAlreadyCancelled, 409},
		{"appointment passed", apperr.ErrAppointmentPassed, 422},
		{"not aligned", slots.ErrNotAligned, 422},
		{"in past", slots.ErrInPast, 422},
		{"too soon (wrapped)", fmt.Errorf("%w: need 60 minutes", slots.ErrTooSoon), 422},
		{"outside hours", slots.ErrOutsideWorkingHours, 422},
		{"unknown error", errors.New("db exploded: secret detail"), 500},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/", func(c *fiber.Ctx) error { return errmap.Respond(c, &l, tc.err) })

			resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.want)
			}
			if tc.want == 500 {
				body, _ := io.ReadAll(resp.Body)
				if strings.Contains(string(body), "secret detail") {
					t.Fatalf("internal error leaked to client: %s", body)
				}
			}
		})
	}
}
