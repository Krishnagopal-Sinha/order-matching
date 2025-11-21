package routes

import (
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"match-engine/src/handlers"
	"match-engine/src/middleware"
)

func SetupRoutes(app *fiber.App, orderHandler *handlers.OrderHandler) {
	rateLimitDisabled := os.Getenv("RATE_LIMIT_DISABLED") == "1"
	
	maxRequests := 100
	if envMax := os.Getenv("RATE_LIMIT_MAX"); envMax != "" {
		if parsed, err := strconv.Atoi(envMax); err == nil && parsed > 0 {
			maxRequests = parsed
		}
	}

	windowDuration := time.Second
	if envWindow := os.Getenv("RATE_LIMIT_WINDOW"); envWindow != "" {
		if parsed, err := time.ParseDuration(envWindow); err == nil && parsed > 0 {
			windowDuration = parsed
		}
	}

	serviceAvailability := middleware.DefaultServiceAvailability()
	app.Use(serviceAvailability.Middleware())
	app.Use(middleware.RequestLogger())

	api := app.Group("/api/v1")

	if !rateLimitDisabled {
		rateLimiter := middleware.NewRateLimiter(maxRequests, windowDuration)
		api.Use(rateLimiter.Middleware())
	}

	api.Post("/orders", orderHandler.SubmitOrder)
	api.Delete("/orders/:id", orderHandler.CancelOrder)
	api.Get("/orders/:id", orderHandler.GetOrderStatus)
	api.Get("/orderbook/:symbol", orderHandler.GetOrderBook)

	app.Get("/health", orderHandler.HealthCheck)
	app.Get("/metrics", orderHandler.Metrics)
}

