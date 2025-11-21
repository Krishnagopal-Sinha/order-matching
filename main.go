package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"match-engine/src/engine"
	"match-engine/src/handlers"
	"match-engine/src/logger"
	"match-engine/src/routes"
)

func main() {
	logger.InitLogger()
	log := logger.GetLogger()

	log.Info().Msg("Initializing Order Matching Engine")

	matcher := engine.NewMatcher()
	orderHandler := handlers.NewOrderHandler(matcher)

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}

			log.Error().
				Str("path", c.Path()).
				Str("method", c.Method()).
				Int("status", code).
				Str("error", err.Error()).
				Msg("Request error")

			return c.Status(code).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	app.Use(recover.New())
	routes.SetupRoutes(app, orderHandler)

	port := ":8080"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = ":" + envPort
	}

	serverError := make(chan error, 1)

	go func() {
		if err := app.Listen(port); err != nil {
			// edge case: ignore shutdown errors, only report real errors
			errStr := err.Error()
			if errStr != "server is shutting down" {
				serverError <- err
			}
		}
	}()

	select {
	case err := <-serverError:
		log.Fatal().
			Err(err).
			Str("port", port).
			Str("hint", "Port may be already in use. Try: PORT=3000 go run main.go").
			Msg("Server failed to start")
	default:
		log.Info().
			Str("port", port).
			Msg("Order Matching Engine started")

		log.Info().
			Strs("endpoints", []string{
				"POST   /api/v1/orders",
				"DELETE /api/v1/orders/:id",
				"GET    /api/v1/orders/:id",
				"GET    /api/v1/orderbook/:symbol",
				"GET    /health",
				"GET    /metrics",
			}).
			Msg("API endpoints registered")
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-quit
	log.Info().Msg("Received shutdown signal, shutting down...")

	shutdownTimeout := 10 * time.Second
	if envTimeout := os.Getenv("SHUTDOWN_TIMEOUT"); envTimeout != "" {
		if parsed, err := time.ParseDuration(envTimeout); err == nil && parsed > 0 {
			shutdownTimeout = parsed
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		// edge case: timeout during shutdown is acceptable
		if errors.Is(err, context.DeadlineExceeded) {
			log.Warn().
				Dur("timeout", shutdownTimeout).
				Msg("Timeout exceeded, shutting down...")
		} else {
			log.Error().
				Err(err).
				Msg("Error during shutdown")
		}
	} else {
		log.Info().Msg("Shutdown complete")
	}

	logger.CloseLogger()
}
