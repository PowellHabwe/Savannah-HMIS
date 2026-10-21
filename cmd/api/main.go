package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" 

	db "github.com/PowellHabwe/Savannah-HMIS/db/sqlc"
	httpconfig "github.com/PowellHabwe/Savannah-HMIS/http"
	"github.com/PowellHabwe/Savannah-HMIS/http/router"
	"github.com/PowellHabwe/Savannah-HMIS/libs/config"
	"github.com/PowellHabwe/Savannah-HMIS/libs/response"
	"github.com/PowellHabwe/Savannah-HMIS/utils/logger"
	validatorUtil "github.com/PowellHabwe/Savannah-HMIS/utils/validator"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

func errorHandler(c *fiber.Ctx, err error) error {
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return response.HttpError(fe.Code, c, errors.New(fe.Message))
	}
	return response.HttpError(fiber.StatusInternalServerError, c, errors.New("internal server error"))
}

func main() {
	c := config.New()
	l := logger.New(c.Environment)
	v := validatorUtil.New()

	loc, err := c.Clinic.Location()
	if err != nil {
		l.Fatal().Err(err).Msgf("invalid CLINIC_TIMEZONE %q", c.Clinic.Timezone)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	poolCfg, err := pgxpool.ParseConfig(c.DB.DSN())
	if err != nil {
		l.Fatal().Err(err).Msg("invalid database config")
	}
	poolCfg.MaxConns = 10
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		l.Fatal().Err(err).Msg("unable to create database pool")
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		l.Fatal().Err(err).Msg("unable to reach database")
	}

	app := fiber.New(fiber.Config{
		ReadTimeout:           c.Server.TimeoutRead,
		DisableStartupMessage: c.Environment == "production",
		ErrorHandler:          errorHandler,
	})

	router.New(app, &httpconfig.ControllerParams{
		Logger:    l,
		Validator: v,
		Queries:   db.New(pool),
		Pool:      pool,
		Config:    c,
		Location:  loc,
	})

	addr := fmt.Sprintf("%s:%s", c.Server.Host, c.Server.Port)
	errCh := make(chan error, 1)
	go func() { errCh <- app.Listen(addr) }()
	l.Info().Msgf("listening on %s", addr)

	select {
	case err := <-errCh:
		l.Fatal().Err(err).Msg("server stopped unexpectedly")
	case <-ctx.Done():
		l.Info().Msg("shutting down")
		if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
			l.Error().Err(err).Msg("graceful shutdown failed")
		}
	}
}
