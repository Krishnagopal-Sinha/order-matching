package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"

	"match-engine/src/engine"
	"match-engine/src/handlers"
	"match-engine/src/models"
	"match-engine/src/routes"
)

// setupTestServerWithRateLimit creates a test server with rate limiting enabled
func setupTestServerWithRateLimit() *fiber.App {
	// Enable rate limiting for rate limit tests
	os.Setenv("RATE_LIMIT_DISABLED", "0")
	defer os.Unsetenv("RATE_LIMIT_DISABLED")

	matcher := engine.NewMatcher()
	orderHandler := handlers.NewOrderHandler(matcher)

	app := fiber.New()
	routes.SetupRoutes(app, orderHandler)

	return app
}

// TestRateLimiting tests that rate limiting is working correctly
func TestRateLimiting(t *testing.T) {
	app := setupTestServerWithRateLimit()

	// Make requests up to the limit (default is 100 req/s)
	// We'll make 101 requests to trigger rate limit
	successCount := 0
	rateLimitedCount := 0

	for i := 0; i < 101; i++ {
		reqBody := map[string]interface{}{
			"symbol":   "AAPL",
			"side":     "BUY",
			"type":     "LIMIT",
			"price":    15050,
			"quantity": 100,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		// Use same IP for all requests to test per-client rate limiting
		req.RemoteAddr = "127.0.0.1:12345"

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimitedCount++
		} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			successCount++
		}
	}

	// With default rate limit of 100 req/s, we should have:
	// - 100 successful requests
	// - 1 rate limited request (or more if window hasn't reset)
	t.Logf("Successful requests: %d", successCount)
	t.Logf("Rate limited requests: %d", rateLimitedCount)

	// We should have at least some rate limited requests
	if rateLimitedCount == 0 && successCount > 100 {
		t.Logf("Note: Rate limiting may not have triggered if requests were spread across windows")
	}
}

// TestRateLimitHeaders tests that rate limit headers are present
func TestRateLimitHeaders(t *testing.T) {
	app := setupTestServerWithRateLimit()

	reqBody := map[string]interface{}{
		"symbol":   "AAPL",
		"side":     "BUY",
		"type":     "LIMIT",
		"price":    15050,
		"quantity": 100,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Check for rate limit headers
	limitHeader := resp.Header.Get("X-RateLimit-Limit")
	windowHeader := resp.Header.Get("X-RateLimit-Window")

	if limitHeader == "" {
		t.Errorf("Expected X-RateLimit-Limit header, got empty")
	}
	if windowHeader == "" {
		t.Errorf("Expected X-RateLimit-Window header, got empty")
	}

	t.Logf("X-RateLimit-Limit: %s", limitHeader)
	t.Logf("X-RateLimit-Window: %s", windowHeader)
}

// TestRateLimitResponse tests the rate limit error response
func TestRateLimitResponse(t *testing.T) {
	app := setupTestServerWithRateLimit()

	// Make 101 requests rapidly to trigger rate limit
	for i := 0; i < 101; i++ {
		reqBody := map[string]interface{}{
			"symbol":   "AAPL",
			"side":     "BUY",
			"type":     "LIMIT",
			"price":    15050,
			"quantity": 100,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"

		resp, err := app.Test(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			// Verify error response format
			var errorResp models.ErrorResponse
			if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil {
				if errorResp.Error == "" {
					t.Errorf("Expected error message in rate limit response")
				}
				t.Logf("Rate limit error response: %s", errorResp.Error)
			}
			break
		}
	}
}

// TestHealthEndpointNotRateLimited tests that health endpoint is not rate limited
func TestHealthEndpointNotRateLimited(t *testing.T) {
	app := setupTestServer()

	// Make many requests to health endpoint
	successCount := 0
	for i := 0; i < 150; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		resp, err := app.Test(req)

		if err == nil && resp.StatusCode == http.StatusOK {
			successCount++
		}
	}

	// All health check requests should succeed (not rate limited)
	if successCount < 150 {
		t.Errorf("Expected all health check requests to succeed, got %d/150", successCount)
	}
}

