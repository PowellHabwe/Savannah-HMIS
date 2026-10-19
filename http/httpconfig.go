package httpconfig

import (
	"time"

	db "github.com/PowellHabwe/Savannah-HMIS/db/sqlc"
	"github.com/PowellHabwe/Savannah-HMIS/libs/config"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type ControllerParams struct {
	Logger    *zerolog.Logger
	Validator *validator.Validate
	Queries   *db.Queries
	Pool      *pgxpool.Pool
	Config    *config.Config
	Location  *time.Location 
}
