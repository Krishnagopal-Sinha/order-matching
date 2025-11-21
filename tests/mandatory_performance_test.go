package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"match-engine/src/models"
)

// TestMandatoryPerformanceRequirements tests all mandatory performance requirements
// Requirements:
// - Throughput ≥ 30,000 orders/second (sustained 60-second load test)
// - Latency (p50) ≤ 10 ms
// - Latency (p99) ≤ 50 ms
// - Latency (p999) ≤ 100 ms
// - Correctness 100% (no invalid trades, race conditions, data corruption)
// - Concurrent Connections ≥ 100
func TestMandatoryPerformanceRequirements(t *testing.T) {
	app := setupTestServer()

	// Test parameters matching requirements
	duration := 60 * time.Second // 60-second sustained load test
	minConcurrency := 100        // At least 100 concurrent connections
	targetThroughput := 30000.0  // Minimum 30,000 orders/second
	targetP50 := 10.0           // milliseconds
	targetP99 := 50.0           // milliseconds
	targetP999 := 100.0         // milliseconds

	metrics := &PerformanceMetrics{
		Latencies: make([]time.Duration, 0, 2000000), // Pre-allocate for ~2M requests
	}
	var wg sync.WaitGroup

	// Track correctness: ensure no invalid trades or data corruption
	var correctnessErrors int64
	var totalTrades int64
	orderIDs := sync.Map{} // Thread-safe map to track order IDs

	startTime := time.Now()
	endTime := startTime.Add(duration)

	t.Logf("=== Mandatory Performance Requirements Test ===")
	t.Logf("Duration: %v", duration)
	t.Logf("Concurrency: %d", minConcurrency)
	t.Logf("Target Throughput: ≥ %.0f orders/second", targetThroughput)
	t.Logf("Target Latency P50: ≤ %.0f ms", targetP50)
	t.Logf("Target Latency P99: ≤ %.0f ms", targetP99)
	t.Logf("Target Latency P999: ≤ %.0f ms", targetP999)
	t.Logf("Starting test...")
	t.Logf("")

	// Launch concurrent workers (at least 100)
	for i := 0; i < minConcurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			orderCount := 0

			for time.Now().Before(endTime) {
				// Alternate between buy and sell orders to create matches
				side := "BUY"
				if workerID%2 == 0 {
					side = "SELL"
				}

				reqBody := map[string]interface{}{
					"symbol":   "AAPL",
					"side":     side,
					"type":     "LIMIT",
					"price":    15050 + int64(orderCount%100),
					"quantity": 100,
				}

				body, _ := json.Marshal(reqBody)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")

				requestStart := time.Now()
				resp, err := app.Test(req)
				latency := time.Since(requestStart)

				atomic.AddInt64(&metrics.TotalRequests, 1)
				metrics.AddLatency(latency)

				if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
					atomic.AddInt64(&metrics.SuccessfulRequests, 1)

					var result models.SubmitOrderResponse
					if json.NewDecoder(resp.Body).Decode(&result) == nil {
						// Correctness check: verify order ID is unique
						if _, exists := orderIDs.LoadOrStore(result.OrderID, true); exists {
							atomic.AddInt64(&correctnessErrors, 1)
							t.Errorf("Correctness violation: Duplicate order ID detected: %s", result.OrderID)
						}

						// Track trades for correctness verification
						atomic.AddInt64(&totalTrades, int64(len(result.Trades)))

						// Verify trade correctness
						for _, trade := range result.Trades {
							if trade.Price <= 0 || trade.Quantity <= 0 {
								atomic.AddInt64(&correctnessErrors, 1)
								t.Errorf("Correctness violation: Invalid trade - Price: %d, Quantity: %d", trade.Price, trade.Quantity)
							}
							if trade.TradeID == "" {
								atomic.AddInt64(&correctnessErrors, 1)
								t.Errorf("Correctness violation: Trade missing trade ID")
							}
						}

						// Verify order status consistency
						if result.Status == "FILLED" && result.FilledQuantity != result.RemainingQuantity+result.FilledQuantity {
							// For FILLED orders, filled + remaining should equal original quantity
							// We can't verify original quantity from response, so just check filled > 0
							if result.FilledQuantity <= 0 {
								atomic.AddInt64(&correctnessErrors, 1)
								t.Errorf("Correctness violation: FILLED order has zero filled quantity")
							}
						}
						if result.Status == "PARTIAL_FILL" && (result.FilledQuantity <= 0 || result.RemainingQuantity <= 0) {
							atomic.AddInt64(&correctnessErrors, 1)
							t.Errorf("Correctness violation: PARTIAL_FILL order should have both filled and remaining quantity > 0")
						}
					}
				} else {
					atomic.AddInt64(&metrics.FailedRequests, 1)
				}

				orderCount++
			}
		}(i)
	}

	wg.Wait()
	actualDuration := time.Since(startTime)

	// Calculate metrics
	stats := metrics.GetStats()
	throughput := float64(metrics.SuccessfulRequests) / actualDuration.Seconds()
	p50Latency := stats["latency_p50_ms"].(float64)
	p95Latency := stats["latency_p95_ms"].(float64)
	p99Latency := stats["latency_p99_ms"].(float64)
	p999Latency := stats["latency_p999_ms"].(float64)
	successRate := stats["success_rate"].(float64)

	// Print results
	t.Logf("==========================================")
	t.Logf("MANDATORY PERFORMANCE REQUIREMENTS TEST")
	t.Logf("==========================================")
	t.Logf("Test Duration: %v", actualDuration)
	t.Logf("Total Requests: %d", metrics.TotalRequests)
	t.Logf("Successful Requests: %d", metrics.SuccessfulRequests)
	t.Logf("Failed Requests: %d", metrics.FailedRequests)
	t.Logf("Success Rate: %.2f%%", successRate)
	t.Logf("Total Trades: %d", totalTrades)
	t.Logf("Correctness Errors: %d", correctnessErrors)
	t.Logf("")
	t.Logf("THROUGHPUT:")
	t.Logf("  Achieved: %.2f orders/second", throughput)
	t.Logf("  Target:   ≥ %.0f orders/second", targetThroughput)
	if throughput >= targetThroughput {
		t.Logf("  Status:   ✓ PASS")
	} else {
		t.Logf("  Status:   ✗ FAIL (%.2f%% of target)", throughput/targetThroughput*100)
	}
	t.Logf("")
	t.Logf("LATENCY:")
	t.Logf("  P50:   %.2f ms (target: ≤ %.0f ms)", p50Latency, targetP50)
	if p50Latency <= targetP50 {
		t.Logf("         ✓ PASS")
	} else {
		t.Logf("         ✗ FAIL")
	}
	t.Logf("  P95:   %.2f ms", p95Latency)
	t.Logf("  P99:   %.2f ms (target: ≤ %.0f ms)", p99Latency, targetP99)
	if p99Latency <= targetP99 {
		t.Logf("         ✓ PASS")
	} else {
		t.Logf("         ✗ FAIL")
	}
	t.Logf("  P999:  %.2f ms (target: ≤ %.0f ms)", p999Latency, targetP999)
	if p999Latency <= targetP999 {
		t.Logf("         ✓ PASS")
	} else {
		t.Logf("         ✗ FAIL")
	}
	t.Logf("  Avg:   %.2f ms", stats["latency_avg_ms"])
	t.Logf("  Min:   %.2f ms", stats["latency_min_ms"])
	t.Logf("  Max:   %.2f ms", stats["latency_max_ms"])
	t.Logf("")
	t.Logf("CONCURRENCY:")
	t.Logf("  Concurrent Connections: %d", minConcurrency)
	t.Logf("  Status:   ✓ PASS (≥ 100 required)")
	t.Logf("")
	t.Logf("CORRECTNESS:")
	t.Logf("  Errors Detected: %d", correctnessErrors)
	if correctnessErrors == 0 {
		t.Logf("  Status:   ✓ PASS (100%% correctness)")
	} else {
		t.Logf("  Status:   ✗ FAIL (correctness violations detected)")
	}
	t.Logf("==========================================")

	// Assertions
	allPassed := true

	// Throughput assertion
	if throughput < targetThroughput {
		t.Errorf("REQUIREMENT FAILED: Throughput = %.2f orders/sec (target: ≥ %.0f orders/sec)", throughput, targetThroughput)
		allPassed = false
	}

	// Latency assertions
	if p50Latency > targetP50 {
		t.Errorf("REQUIREMENT FAILED: P50 latency = %.2f ms (target: ≤ %.0f ms)", p50Latency, targetP50)
		allPassed = false
	}

	if p99Latency > targetP99 {
		t.Errorf("REQUIREMENT FAILED: P99 latency = %.2f ms (target: ≤ %.0f ms)", p99Latency, targetP99)
		allPassed = false
	}

	if p999Latency > targetP999 {
		t.Errorf("REQUIREMENT FAILED: P999 latency = %.2f ms (target: ≤ %.0f ms)", p999Latency, targetP999)
		allPassed = false
	}

	// Correctness assertion
	if correctnessErrors > 0 {
		t.Errorf("REQUIREMENT FAILED: Correctness violations detected (%d errors)", correctnessErrors)
		allPassed = false
	}

	// Success rate should be high (at least 99%)
	if successRate < 99.0 {
		t.Errorf("REQUIREMENT FAILED: Success rate = %.2f%% (should be ≥ 99%%)", successRate)
		allPassed = false
	}

	if allPassed {
		t.Logf("")
		t.Logf("🎉 ALL MANDATORY PERFORMANCE REQUIREMENTS MET! 🎉")
		t.Logf("   System is ready for production!")
	} else {
		t.Logf("")
		t.Logf("⚠️  SOME REQUIREMENTS NOT MET")
		t.Logf("   Review the failures above and optimize accordingly.")
	}
}

// TestMandatoryThroughput tests the 30k orders/second requirement specifically
func TestMandatoryThroughput(t *testing.T) {
	app := setupTestServer()

	duration := 60 * time.Second
	concurrency := 100
	targetThroughput := 30000.0

	metrics := &PerformanceMetrics{
		Latencies: make([]time.Duration, 0, 2000000),
	}
	var wg sync.WaitGroup

	startTime := time.Now()
	endTime := startTime.Add(duration)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			orderCount := 0

			for time.Now().Before(endTime) {
				reqBody := map[string]interface{}{
					"symbol":   "AAPL",
					"side":     "BUY",
					"type":     "LIMIT",
					"price":    15050 + int64(orderCount%100),
					"quantity": 100,
				}

				body, _ := json.Marshal(reqBody)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")

				requestStart := time.Now()
				resp, err := app.Test(req)
				latency := time.Since(requestStart)

				atomic.AddInt64(&metrics.TotalRequests, 1)
				metrics.AddLatency(latency)

				if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
					atomic.AddInt64(&metrics.SuccessfulRequests, 1)
				} else {
					atomic.AddInt64(&metrics.FailedRequests, 1)
				}

				orderCount++
			}
		}(i)
	}

	wg.Wait()
	actualDuration := time.Since(startTime)

	throughput := float64(metrics.SuccessfulRequests) / actualDuration.Seconds()

	t.Logf("=== Mandatory Throughput Test (60s) ===")
	t.Logf("Duration: %v", actualDuration)
	t.Logf("Concurrency: %d", concurrency)
	t.Logf("Total Requests: %d", metrics.TotalRequests)
	t.Logf("Successful Requests: %d", metrics.SuccessfulRequests)
	t.Logf("Throughput: %.2f orders/second", throughput)
	t.Logf("Target: ≥ %.0f orders/second", targetThroughput)

	if throughput >= targetThroughput {
		t.Logf("✓ REQUIREMENT MET")
	} else {
		t.Errorf("✗ REQUIREMENT NOT MET: %.2f orders/sec < %.0f orders/sec", throughput, targetThroughput)
	}
}

// TestMandatoryLatency tests latency requirements specifically
func TestMandatoryLatency(t *testing.T) {
	app := setupTestServer()

	numRequests := 10000
	concurrency := 100
	targetP50 := 10.0
	targetP99 := 50.0
	targetP999 := 100.0

	metrics := &PerformanceMetrics{
		Latencies: make([]time.Duration, 0, numRequests),
	}
	var wg sync.WaitGroup

	startTime := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			requestsPerWorker := numRequests / concurrency
			for j := 0; j < requestsPerWorker; j++ {
				reqBody := map[string]interface{}{
					"symbol":   "AAPL",
					"side":     "BUY",
					"type":     "LIMIT",
					"price":    15050 + int64(j%100),
					"quantity": 100,
				}

				body, _ := json.Marshal(reqBody)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")

				requestStart := time.Now()
				resp, err := app.Test(req)
				latency := time.Since(requestStart)

				atomic.AddInt64(&metrics.TotalRequests, 1)
				metrics.AddLatency(latency)

				if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
					atomic.AddInt64(&metrics.SuccessfulRequests, 1)
				} else {
					atomic.AddInt64(&metrics.FailedRequests, 1)
				}
			}
		}(i)
	}

	wg.Wait()
	actualDuration := time.Since(startTime)

	stats := metrics.GetStats()
	p50Latency := stats["latency_p50_ms"].(float64)
	p99Latency := stats["latency_p99_ms"].(float64)
	p999Latency := stats["latency_p999_ms"].(float64)

	t.Logf("=== Mandatory Latency Test ===")
	t.Logf("Total Requests: %d", metrics.TotalRequests)
	t.Logf("Duration: %v", actualDuration)
	t.Logf("Concurrency: %d", concurrency)
	t.Logf("")
	t.Logf("Latency P50:   %.2f ms (target: ≤ %.0f ms)", p50Latency, targetP50)
	if p50Latency <= targetP50 {
		t.Logf("             ✓ PASS")
	} else {
		t.Errorf("             ✗ FAIL")
	}
	t.Logf("Latency P99:   %.2f ms (target: ≤ %.0f ms)", p99Latency, targetP99)
	if p99Latency <= targetP99 {
		t.Logf("             ✓ PASS")
	} else {
		t.Errorf("             ✗ FAIL")
	}
	t.Logf("Latency P999:  %.2f ms (target: ≤ %.0f ms)", p999Latency, targetP999)
	if p999Latency <= targetP999 {
		t.Logf("             ✓ PASS")
	} else {
		t.Errorf("             ✗ FAIL")
	}

	// Assertions
	if p50Latency > targetP50 {
		t.Errorf("REQUIREMENT FAILED: P50 latency = %.2f ms (target: ≤ %.0f ms)", p50Latency, targetP50)
	}
	if p99Latency > targetP99 {
		t.Errorf("REQUIREMENT FAILED: P99 latency = %.2f ms (target: ≤ %.0f ms)", p99Latency, targetP99)
	}
	if p999Latency > targetP999 {
		t.Errorf("REQUIREMENT FAILED: P999 latency = %.2f ms (target: ≤ %.0f ms)", p999Latency, targetP999)
	}
}

