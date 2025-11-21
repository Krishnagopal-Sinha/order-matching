package middleware

import (
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func RequestLogger() fiber.Handler {
	disabled := os.Getenv("REQUEST_LOGGING_DISABLED") == "1"
	logLevel := zerolog.GlobalLevel()
	shouldLog := !disabled && logLevel <= zerolog.InfoLevel

	return func(c *fiber.Ctx) error {
		var start time.Time
		if shouldLog {
			start = time.Now()
		}

		err := c.Next()

		if shouldLog {
			latency := time.Since(start)
			log.Info().
				Str("method", c.Method()).
				Str("path", c.Path()).
				Str("ip", c.IP()).
				Int("status", c.Response().StatusCode()).
				Int64("latency_ms", latency.Milliseconds()).
				Int("bytes_in", len(c.Body())).
				Int("bytes_out", len(c.Response().Body())).
				Msg("HTTP request")
		}

		return err
	}
}

