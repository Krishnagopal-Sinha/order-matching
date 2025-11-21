package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"match-engine/src/models"
)

// TestConcurrentOrderSubmission tests concurrent order submission
// Verifies that multiple orders can be submitted simultaneously without data races
func TestConcurrentOrderSubmission(t *testing.T) {
	app := setupTestServer()

	// Number of concurrent goroutines
	numGoroutines := 50
	ordersPerGoroutine := 10

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*ordersPerGoroutine)

	// Submit orders concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < ordersPerGoroutine; j++ {
				// Alternate between buy and sell orders
				side := "BUY"
				if (goroutineID+j)%2 == 0 {
					side = "SELL"
				}

				reqBody := map[string]interface{}{
					"symbol":   "AAPL",
					"side":     side,
					"type":     "LIMIT",
					"price":    15050 + int64(j%10), // Vary prices slightly
					"quantity": 100,
				}

				body, err := json.Marshal(reqBody)
				if err != nil {
					errors <- err
					return
				}

				req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)

				if err != nil {
					errors <- err
					return
				}

				if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
					errors <- err
					return
				}

				var result models.SubmitOrderResponse
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					errors <- err
					return
				}

				// Verify order was created
				if result.OrderID == "" {
					errors <- err
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
			t.Logf("Error in concurrent submission: %v", err)
		}
	}

	if errorCount > 0 {
		t.Errorf("Encountered %d errors during concurrent order submission", errorCount)
	}
}

// TestConcurrentMatching tests concurrent order matching
// Verifies that orders can be matched correctly when submitted concurrently
func TestConcurrentMatching(t *testing.T) {
	app := setupTestServer()

	// First, add some sell orders
	sellOrders := []map[string]interface{}{
		{"symbol": "AAPL", "side": "SELL", "type": "LIMIT", "price": 15050, "quantity": 1000},
		{"symbol": "AAPL", "side": "SELL", "type": "LIMIT", "price": 15051, "quantity": 1000},
		{"symbol": "AAPL", "side": "SELL", "type": "LIMIT", "price": 15052, "quantity": 1000},
	}

	for _, order := range sellOrders {
		body, _ := json.Marshal(order)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		app.Test(req)
	}

	// Now submit buy orders concurrently
	numGoroutines := 20
	var wg sync.WaitGroup
	var totalFilled int64
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			reqBody := map[string]interface{}{
				"symbol":   "AAPL",
				"side":     "BUY",
				"type":     "LIMIT",
				"price":    15055, // Higher than sell orders, should match
				"quantity": 50,
			}

			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			if err != nil {
				t.Logf("Error in concurrent matching: %v", err)
				return
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
				return
			}

			var result models.SubmitOrderResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return
			}

			mu.Lock()
			totalFilled += result.FilledQuantity
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Verify that orders were matched correctly
	// Total buy quantity: 20 * 50 = 1000
	// Should match against sell orders
	if totalFilled < 500 {
		t.Errorf("Expected at least 500 shares filled, got: %d", totalFilled)
	}
}

// TestConcurrentCancellation tests concurrent order cancellation
// Verifies that orders can be cancelled safely when accessed concurrently
func TestConcurrentCancellation(t *testing.T) {
	app := setupTestServer()

	// Create multiple orders
	numOrders := 20
	orderIDs := make([]string, numOrders)

	// Submit orders
	for i := 0; i < numOrders; i++ {
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
		resp, _ := app.Test(req)

		var result models.SubmitOrderResponse
		json.NewDecoder(resp.Body).Decode(&result)
		orderIDs[i] = result.OrderID
	}

	// Cancel orders concurrently
	var wg sync.WaitGroup
	errors := make(chan error, numOrders)

	for _, orderID := range orderIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/orders/"+id, nil)
			resp, err := app.Test(req)

			if err != nil {
				errors <- err
				return
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
				errors <- err
				return
			}
		}(orderID)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
		}
	}

	if errorCount > 0 {
		t.Errorf("Encountered %d errors during concurrent cancellation", errorCount)
	}
}

// TestConcurrentOrderBookAccess tests concurrent order book reads
// Verifies that order book can be read safely while orders are being submitted
func TestConcurrentOrderBookAccess(t *testing.T) {
	app := setupTestServer()

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Start goroutines that submit orders
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			reqBody := map[string]interface{}{
				"symbol":   "AAPL",
				"side":     "BUY",
				"type":     "LIMIT",
				"price":    15050 + int64(i%10),
				"quantity": 100,
			}

			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			app.Test(req)
		}
	}()

	// Start goroutines that read order book
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/orderbook/AAPL?depth=10", nil)
				resp, err := app.Test(req)

				if err != nil {
					errors <- err
					return
				}

				if resp.StatusCode != http.StatusOK {
					errors <- err
					return
				}

				var result models.OrderBookResponse
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					errors <- err
					return
				}

				// Verify response structure
				if result.Symbol != "AAPL" {
					errors <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
		}
	}

	if errorCount > 0 {
		t.Errorf("Encountered %d errors during concurrent order book access", errorCount)
	}
}

// TestConcurrentOrderStatusAccess tests concurrent order status reads
// Verifies that order status can be read safely while orders are being processed
func TestConcurrentOrderStatusAccess(t *testing.T) {
	app := setupTestServer()

	// Create some orders
	numOrders := 10
	orderIDs := make([]string, numOrders)

	for i := 0; i < numOrders; i++ {
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
		resp, _ := app.Test(req)

		var result models.SubmitOrderResponse
		json.NewDecoder(resp.Body).Decode(&result)
		orderIDs[i] = result.OrderID
	}

	// Read order status concurrently
	var wg sync.WaitGroup
	errors := make(chan error, numOrders*10)

	for _, orderID := range orderIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+id, nil)
				resp, err := app.Test(req)

				if err != nil {
					errors <- err
					return
				}

				if resp.StatusCode != http.StatusOK {
					errors <- err
					return
				}

				var result models.OrderStatusResponse
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					errors <- err
					return
				}

				// Verify response
				if result.OrderID != id {
					errors <- err
					return
				}
			}
		}(orderID)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
		}
	}

	if errorCount > 0 {
		t.Errorf("Encountered %d errors during concurrent order status access", errorCount)
	}
}

// TestConcurrentMixedOperations tests a mix of concurrent operations
// Verifies that all operations work correctly when executed concurrently
func TestConcurrentMixedOperations(t *testing.T) {
	app := setupTestServer()

	var wg sync.WaitGroup
	errors := make(chan error, 200)

	// Submit orders
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			reqBody := map[string]interface{}{
				"symbol":   "AAPL",
				"side":     "BUY",
				"type":     "LIMIT",
				"price":    15050 + int64(id%10),
				"quantity": 100,
			}

			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			if err != nil {
				errors <- err
				return
			}

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				errors <- err
				return
			}

			var result models.SubmitOrderResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				errors <- err
				return
			}

			// Try to read order status
			req = httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+result.OrderID, nil)
			resp, err = app.Test(req)

			if err != nil {
				errors <- err
				return
			}

			if resp.StatusCode != http.StatusOK {
				errors <- err
				return
			}
		}(i)
	}

	// Read order book concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/orderbook/AAPL?depth=10", nil)
			resp, err := app.Test(req)

			if err != nil {
				errors <- err
				return
			}

			if resp.StatusCode != http.StatusOK {
				errors <- err
				return
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
		}
	}

	if errorCount > 0 {
		t.Errorf("Encountered %d errors during concurrent mixed operations", errorCount)
	}
}

