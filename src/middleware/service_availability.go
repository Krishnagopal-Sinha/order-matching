package middleware

import (
	"os"
	"strconv"
	"sync/atomic"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

type ServiceAvailability struct {
	maintenanceMode        atomic.Bool
	maxConcurrentRequests  int64
	inFlightRequests       atomic.Int64
}

func NewServiceAvailability(maxConcurrentRequests int64) *ServiceAvailability {
	sa := &ServiceAvailability{
		maxConcurrentRequests: maxConcurrentRequests,
	}

	if os.Getenv("MAINTENANCE_MODE") == "1" {
		sa.maintenanceMode.Store(true)
		log.Warn().Msg("Service is in maintenance mode - all requests will return 503")
	}

	return sa
}

func (sa *ServiceAvailability) SetMaintenanceMode(enabled bool) {
	sa.maintenanceMode.Store(enabled)
	if enabled {
		log.Warn().Msg("Service maintenance mode enabled")
	} else {
		log.Info().Msg("Service maintenance mode disabled")
	}
}

func (sa *ServiceAvailability) IsMaintenanceMode() bool {
	return sa.maintenanceMode.Load()
}

func (sa *ServiceAvailability) GetInFlightRequests() int64 {
	return sa.inFlightRequests.Load()
}

func (sa *ServiceAvailability) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// edge case: health check always available
		if c.Path() == "/health" {
			return c.Next()
		}

		if sa.maintenanceMode.Load() {
			log.Warn().
				Str("path", c.Path()).
				Str("method", c.Method()).
				Str("ip", c.IP()).
				Msg("Request rejected: service in maintenance mode")
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error":   "Service unavailable",
				"message": "The service is currently undergoing maintenance. Please try again later.",
				"code":    503,
			})
		}

		// edge case: check server overload if limit is set
		if sa.maxConcurrentRequests > 0 {
			currentRequests := sa.inFlightRequests.Load()
			if currentRequests >= sa.maxConcurrentRequests {
				log.Warn().
					Str("path", c.Path()).
					Str("method", c.Method()).
					Int64("current_requests", currentRequests).
					Int64("max_requests", sa.maxConcurrentRequests).
					Msg("Request rejected: server overload")
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"error":   "Service unavailable",
					"message": "The service is currently overloaded. Please try again later.",
					"code":    503,
				})
			}
		}

		sa.inFlightRequests.Add(1)
		defer sa.inFlightRequests.Add(-1)

		return c.Next()
	}
}

func DefaultServiceAvailability() *ServiceAvailability {
	maxConcurrent := int64(0)

	if envMax := os.Getenv("MAX_CONCURRENT_REQUESTS"); envMax != "" {
		if parsed, err := parseEnvInt64(envMax); err == nil && parsed > 0 {
			maxConcurrent = parsed
			log.Info().
				Int64("max_concurrent_requests", maxConcurrent).
				Msg("Server overload detection enabled")
		}
	}

	return NewServiceAvailability(maxConcurrent)
}

func parseEnvInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

