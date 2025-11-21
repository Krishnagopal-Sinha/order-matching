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
)

// TestStretchGoalThroughput tests if system can handle 100,000+ orders/second
// Stretch Goal: Throughput ≥ 100,000 orders/second
func TestStretchGoalThroughput(t *testing.T) {
	app := setupTestServer()

	// Test parameters for stretch goal
	duration := 10 * time.Second
	targetThroughput := 100000.0 // orders per second
	minConcurrency := 500        // Need high concurrency for 100k ops/sec
	maxConcurrency := 2000       // Try up to 2000 concurrent workers

	metrics := &PerformanceMetrics{}
	var wg sync.WaitGroup

	// Test with increasing concurrency to find optimal level
	for concurrency := minConcurrency; concurrency <= maxConcurrency; concurrency += 200 {
		metrics = &PerformanceMetrics{
			Latencies: make([]time.Duration, 0, 100000),
		}

		startTime := time.Now()
		endTime := startTime.Add(duration)

		// Launch concurrent workers
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				orderCount := 0

				for time.Now().Before(endTime) {
					// Alternate between buy and sell to create matches
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
					} else {
						atomic.AddInt64(&metrics.FailedRequests, 1)
					}

					orderCount++
				}
			}(i)
		}

		wg.Wait()
		actualDuration := time.Since(startTime)

		stats := metrics.GetStats()
		throughput := float64(metrics.SuccessfulRequests) / actualDuration.Seconds()

		t.Logf("Concurrency %d: Throughput = %.2f orders/sec, Success Rate = %.2f%%",
			concurrency, throughput, stats["success_rate"])

		// If we achieved target throughput, break
		if throughput >= targetThroughput {
			t.Logf("✓ Achieved stretch goal throughput: %.2f orders/sec (target: %.0f)", throughput, targetThroughput)
			break
		}
	}

	stats := metrics.GetStats()
	throughput := float64(metrics.SuccessfulRequests) / time.Since(time.Now().Add(-duration)).Seconds()

	t.Logf("=== Stretch Goal Throughput Test ===")
	t.Logf("Total Requests: %d", metrics.TotalRequests)
	t.Logf("Successful Requests: %d", metrics.SuccessfulRequests)
	t.Logf("Failed Requests: %d", metrics.FailedRequests)
	t.Logf("Success Rate: %.2f%%", stats["success_rate"])
	t.Logf("Throughput: %.2f orders/second", throughput)

	// Assert stretch goal
	if throughput < targetThroughput {
		t.Errorf("Stretch goal NOT achieved: %.2f orders/sec (target: %.0f orders/sec)", throughput, targetThroughput)
	} else {
		t.Logf("✓ STRETCH GOAL ACHIEVED: Throughput = %.2f orders/sec", throughput)
	}
}

// TestStretchGoalLatencyP99 tests if p99 latency is ≤ 10ms
// Stretch Goal: Latency (p99) ≤ 10 ms
func TestStretchGoalLatencyP99(t *testing.T) {
	app := setupTestServer()

	// Test parameters
	numRequests := 10000
	concurrency := 200
	targetP99 := 10.0 // milliseconds

	metrics := &PerformanceMetrics{
		Latencies: make([]time.Duration, 0, numRequests),
	}
	var wg sync.WaitGroup

	startTime := time.Now()

	// Launch concurrent requests
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
	duration := time.Since(startTime)

	stats := metrics.GetStats()
	p99Latency := stats["latency_p99_ms"].(float64)

	t.Logf("=== Stretch Goal P99 Latency Test ===")
	t.Logf("Total Requests: %d", metrics.TotalRequests)
	t.Logf("Duration: %v", duration)
	t.Logf("Latency P50: %.2f ms", stats["latency_p50_ms"])
	t.Logf("Latency P95: %.2f ms", stats["latency_p95_ms"])
	t.Logf("Latency P99: %.2f ms", p99Latency)
	t.Logf("Latency P999: %.2f ms", stats["latency_p999_ms"])
	t.Logf("Latency Avg: %.2f ms", stats["latency_avg_ms"])

	// Assert stretch goal
	if p99Latency > targetP99 {
		t.Errorf("Stretch goal NOT achieved: P99 latency = %.2f ms (target: ≤ %.0f ms)", p99Latency, targetP99)
	} else {
		t.Logf("✓ STRETCH GOAL ACHIEVED: P99 latency = %.2f ms", p99Latency)
	}
}

// TestStretchGoalLatencyP999 tests if p999 latency is ≤ 20ms
// Stretch Goal: Latency (p999) ≤ 20 ms
func TestStretchGoalLatencyP999(t *testing.T) {
	app := setupTestServer()

	// Test parameters
	numRequests := 20000 // Need more requests to get accurate p999
	concurrency := 300
	targetP999 := 20.0 // milliseconds

	metrics := &PerformanceMetrics{
		Latencies: make([]time.Duration, 0, numRequests),
	}
	var wg sync.WaitGroup

	startTime := time.Now()

	// Launch concurrent requests
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
	duration := time.Since(startTime)

	stats := metrics.GetStats()
	p999Latency := stats["latency_p999_ms"].(float64)

	t.Logf("=== Stretch Goal P999 Latency Test ===")
	t.Logf("Total Requests: %d", metrics.TotalRequests)
	t.Logf("Duration: %v", duration)
	t.Logf("Latency P50: %.2f ms", stats["latency_p50_ms"])
	t.Logf("Latency P95: %.2f ms", stats["latency_p95_ms"])
	t.Logf("Latency P99: %.2f ms", stats["latency_p99_ms"])
	t.Logf("Latency P999: %.2f ms", p999Latency)
	t.Logf("Latency Avg: %.2f ms", stats["latency_avg_ms"])

	// Assert stretch goal
	if p999Latency > targetP999 {
		t.Errorf("Stretch goal NOT achieved: P999 latency = %.2f ms (target: ≤ %.0f ms)", p999Latency, targetP999)
	} else {
		t.Logf("✓ STRETCH GOAL ACHIEVED: P999 latency = %.2f ms", p999Latency)
	}
}

// TestAllStretchGoals tests all stretch goals together
// This is the comprehensive test for bonus points
func TestAllStretchGoals(t *testing.T) {
	app := setupTestServer()

	// Stretch goal targets
	targetThroughput := 100000.0 // orders per second
	targetP99 := 10.0            // milliseconds
	targetP999 := 20.0           // milliseconds

	// Test parameters
	duration := 15 * time.Second
	concurrency := 1500 // High concurrency for throughput

	metrics := &PerformanceMetrics{
		Latencies: make([]time.Duration, 0, 2000000),
	}
	var wg sync.WaitGroup

	startTime := time.Now()
	endTime := startTime.Add(duration)

	// Launch high-concurrency load
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			orderCount := 0

			for time.Now().Before(endTime) {
				// Mix of buy and sell orders
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
				} else {
					atomic.AddInt64(&metrics.FailedRequests, 1)
				}

				orderCount++
			}
		}(i)
	}

	wg.Wait()
	actualDuration := time.Since(startTime)

	stats := metrics.GetStats()
	throughput := float64(metrics.SuccessfulRequests) / actualDuration.Seconds()
	p99Latency := stats["latency_p99_ms"].(float64)
	p999Latency := stats["latency_p999_ms"].(float64)

	t.Logf("==========================================")
	t.Logf("STRETCH GOALS COMPREHENSIVE TEST")
	t.Logf("==========================================")
	t.Logf("Duration: %v", actualDuration)
	t.Logf("Total Requests: %d", metrics.TotalRequests)
	t.Logf("Successful Requests: %d", metrics.SuccessfulRequests)
	t.Logf("Failed Requests: %d", metrics.FailedRequests)
	t.Logf("Success Rate: %.2f%%", stats["success_rate"])
	t.Logf("")
	t.Logf("THROUGHPUT:")
	t.Logf("  Achieved: %.2f orders/second", throughput)
	t.Logf("  Target:   ≥ %.0f orders/second", targetThroughput)
	if throughput >= targetThroughput {
		t.Logf("  Status:   ✓ ACHIEVED")
	} else {
		t.Logf("  Status:   ✗ NOT ACHIEVED (%.2f%% of target)", throughput/targetThroughput*100)
	}
	t.Logf("")
	t.Logf("LATENCY:")
	t.Logf("  P50:   %.2f ms", stats["latency_p50_ms"])
	t.Logf("  P95:   %.2f ms", stats["latency_p95_ms"])
	t.Logf("  P99:   %.2f ms (target: ≤ %.0f ms)", p99Latency, targetP99)
	if p99Latency <= targetP99 {
		t.Logf("         ✓ ACHIEVED")
	} else {
		t.Logf("         ✗ NOT ACHIEVED")
	}
	t.Logf("  P999:  %.2f ms (target: ≤ %.0f ms)", p999Latency, targetP999)
	if p999Latency <= targetP999 {
		t.Logf("         ✓ ACHIEVED")
	} else {
		t.Logf("         ✗ NOT ACHIEVED")
	}
	t.Logf("  Avg:   %.2f ms", stats["latency_avg_ms"])
	t.Logf("==========================================")

	// Assertions
	allAchieved := true

	if throughput < targetThroughput {
		t.Errorf("Stretch goal NOT achieved: Throughput = %.2f orders/sec (target: ≥ %.0f)", throughput, targetThroughput)
		allAchieved = false
	}

	if p99Latency > targetP99 {
		t.Errorf("Stretch goal NOT achieved: P99 latency = %.2f ms (target: ≤ %.0f ms)", p99Latency, targetP99)
		allAchieved = false
	}

	if p999Latency > targetP999 {
		t.Errorf("Stretch goal NOT achieved: P999 latency = %.2f ms (target: ≤ %.0f ms)", p999Latency, targetP999)
		allAchieved = false
	}

	if allAchieved {
		t.Logf("")
		t.Logf("🎉 ALL STRETCH GOALS ACHIEVED! 🎉")
		t.Logf("   Eligible for bonus points!")
	}
}

