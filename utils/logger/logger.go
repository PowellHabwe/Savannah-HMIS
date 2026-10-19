package logger

import (
	"io"
	"os"
	"time"

	"github.com/PowellHabwe/Savannah-HMIS/libs/constants"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

func New(env string) *zerolog.Logger {
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	var w io.Writer = os.Stdout
	level := zerolog.InfoLevel
	if env == constants.EnvironmentDevelopment {
		w = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
		level = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(level)

	l := zerolog.New(w).With().Timestamp().Caller().Logger()
	return &l
}
