package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"match-engine/src/models"
)

// PerformanceMetrics tracks performance metrics during testing
type PerformanceMetrics struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	Latencies          []time.Duration
	mu                 sync.Mutex
}

// AddLatency adds a latency measurement
func (pm *PerformanceMetrics) AddLatency(latency time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Latencies = append(pm.Latencies, latency)
}

// GetPercentile returns the latency at the given percentile (0-100)
func (pm *PerformanceMetrics) GetPercentile(percentile float64) time.Duration {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if len(pm.Latencies) == 0 {
		return 0
	}

	sorted := make([]time.Duration, len(pm.Latencies))
	copy(sorted, pm.Latencies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	index := int(float64(len(sorted)) * percentile / 100.0)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// GetStats returns performance statistics
func (pm *PerformanceMetrics) GetStats() map[string]interface{} {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	stats := make(map[string]interface{})
	stats["total_requests"] = pm.TotalRequests
	stats["successful_requests"] = pm.SuccessfulRequests
	stats["failed_requests"] = pm.FailedRequests
	stats["success_rate"] = float64(pm.SuccessfulRequests) / float64(pm.TotalRequests) * 100

	if len(pm.Latencies) > 0 {
		sorted := make([]time.Duration, len(pm.Latencies))
		copy(sorted, pm.Latencies)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i] < sorted[j]
		})

		stats["latency_p50_ms"] = float64(sorted[len(sorted)*50/100]) / float64(time.Millisecond)
		stats["latency_p95_ms"] = float64(sorted[len(sorted)*95/100]) / float64(time.Millisecond)
		stats["latency_p99_ms"] = float64(sorted[len(sorted)*99/100]) / float64(time.Millisecond)
		stats["latency_p999_ms"] = float64(sorted[len(sorted)*999/1000]) / float64(time.Millisecond)
		stats["latency_min_ms"] = float64(sorted[0]) / float64(time.Millisecond)
		stats["latency_max_ms"] = float64(sorted[len(sorted)-1]) / float64(time.Millisecond)
		avg := pm.calculateAverage(sorted)
		stats["latency_avg_ms"] = float64(avg) / float64(time.Millisecond)
	}

	return stats
}

func (pm *PerformanceMetrics) calculateAverage(latencies []time.Duration) time.Duration {
	var sum time.Duration
	for _, l := range latencies {
		sum += l
	}
	return sum / time.Duration(len(latencies))
}

// TestOrderSubmissionThroughput tests order submission throughput
// Target: Should handle at least 10,000 orders/second
func TestOrderSubmissionThroughput(t *testing.T) {
	app := setupTestServer()

	// Test parameters
	concurrency := 100
	duration := 5 * time.Second

	metrics := &PerformanceMetrics{}
	var wg sync.WaitGroup

	// Start load generation
	startTime := time.Now()
	endTime := startTime.Add(duration)

	// Launch concurrent workers
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

	// Calculate throughput
	stats := metrics.GetStats()
	throughput := float64(metrics.SuccessfulRequests) / actualDuration.Seconds()

	t.Logf("=== Order Submission Throughput Test ===")
	t.Logf("Duration: %v", actualDuration)
	t.Logf("Total Requests: %d", metrics.TotalRequests)
	t.Logf("Successful Requests: %d", metrics.SuccessfulRequests)
	t.Logf("Failed Requests: %d", metrics.FailedRequests)
	t.Logf("Success Rate: %.2f%%", stats["success_rate"])
	t.Logf("Throughput: %.2f orders/second", throughput)
	t.Logf("Latency P50: %.2f ms", stats["latency_p50_ms"])
	t.Logf("Latency P95: %.2f ms", stats["latency_p95_ms"])
	t.Logf("Latency P99: %.2f ms", stats["latency_p99_ms"])
	t.Logf("Latency P999: %.2f ms", stats["latency_p999_ms"])
	t.Logf("Latency Avg: %.2f ms", stats["latency_avg_ms"])

	// Performance assertions (adjust thresholds based on requirements)
	if throughput < 5000 {
		t.Errorf("Throughput too low: %.2f orders/sec (target: 5000+)", throughput)
	}

	if stats["latency_p99_ms"].(float64) > 100 {
		t.Errorf("P99 latency too high: %.2f ms (target: <100ms)", stats["latency_p99_ms"])
	}
}

// TestOrderSubmissionLatency tests order submission latency
// Target: P99 latency should be < 50ms, P50 latency should be < 10ms
func TestOrderSubmissionLatency(t *testing.T) {
	app := setupTestServer()

	// Test parameters
	numRequests := 1000
	concurrency := 50

	metrics := &PerformanceMetrics{}
	var wg sync.WaitGroup

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

	stats := metrics.GetStats()

	t.Logf("=== Order Submission Latency Test ===")
	t.Logf("Total Requests: %d", metrics.TotalRequests)
	t.Logf("Successful Requests: %d", metrics.SuccessfulRequests)
	t.Logf("Failed Requests: %d", metrics.FailedRequests)
	t.Logf("Latency P50: %.2f ms", stats["latency_p50_ms"])
	t.Logf("Latency P95: %.2f ms", stats["latency_p95_ms"])
	t.Logf("Latency P99: %.2f ms", stats["latency_p99_ms"])
	t.Logf("Latency P999: %.2f ms", stats["latency_p999_ms"])
	t.Logf("Latency Min: %.2f ms", stats["latency_min_ms"])
	t.Logf("Latency Max: %.2f ms", stats["latency_max_ms"])
	t.Logf("Latency Avg: %.2f ms", stats["latency_avg_ms"])

	// Performance assertions
	if stats["latency_p50_ms"].(float64) > 10 {
		t.Errorf("P50 latency too high: %.2f ms (target: <10ms)", stats["latency_p50_ms"])
	}

	if stats["latency_p99_ms"].(float64) > 50 {
		t.Errorf("P99 latency too high: %.2f ms (target: <50ms)", stats["latency_p99_ms"])
	}
}

// TestMatchingPerformance tests order matching performance under load
func TestMatchingPerformance(t *testing.T) {
	app := setupTestServer()

	// Pre-populate order book with sell orders
	numSellOrders := 1000
	for i := 0; i < numSellOrders; i++ {
		reqBody := map[string]interface{}{
			"symbol":   "AAPL",
			"side":     "SELL",
			"type":     "LIMIT",
			"price":    15050 + int64(i%50),
			"quantity": 100,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		app.Test(req)
	}

	// Test matching performance
	numBuyOrders := 500
	concurrency := 20

	metrics := &PerformanceMetrics{}
	var wg sync.WaitGroup

	startTime := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			ordersPerWorker := numBuyOrders / concurrency
			for j := 0; j < ordersPerWorker; j++ {
				reqBody := map[string]interface{}{
					"symbol":   "AAPL",
					"side":     "BUY",
					"type":     "LIMIT",
					"price":    15100, // Higher than sell orders, should match
					"quantity": 50,
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
	throughput := float64(metrics.SuccessfulRequests) / duration.Seconds()

	t.Logf("=== Matching Performance Test ===")
	t.Logf("Pre-populated Orders: %d", numSellOrders)
	t.Logf("Matching Orders: %d", numBuyOrders)
	t.Logf("Duration: %v", duration)
	t.Logf("Throughput: %.2f orders/second", throughput)
	t.Logf("Latency P50: %.2f ms", stats["latency_p50_ms"])
	t.Logf("Latency P99: %.2f ms", stats["latency_p99_ms"])
}

// TestConcurrentOperationsPerformance tests performance under mixed concurrent operations
func TestConcurrentOperationsPerformance(t *testing.T) {
	app := setupTestServer()

	// Test parameters
	duration := 10 * time.Second
	concurrency := 50

	metrics := &PerformanceMetrics{
		Latencies: make([]time.Duration, 0, 10000),
	}
	var wg sync.WaitGroup

	startTime := time.Now()
	endTime := startTime.Add(duration)

	// Launch mixed operations
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			orderIDs := make([]string, 0)

			for time.Now().Before(endTime) {
				// Submit order
				reqBody := map[string]interface{}{
					"symbol":   "AAPL",
					"side":     "BUY",
					"type":     "LIMIT",
					"price":    15050 + int64(workerID%100),
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
						orderIDs = append(orderIDs, result.OrderID)
					}
				} else {
					atomic.AddInt64(&metrics.FailedRequests, 1)
				}

				// Occasionally get order book
				if workerID%5 == 0 {
					req = httptest.NewRequest(http.MethodGet, "/api/v1/orderbook/AAPL?depth=10", nil)
					requestStart = time.Now()
					app.Test(req)
					latency = time.Since(requestStart)
					metrics.AddLatency(latency)
				}

				// Occasionally cancel an order
				if len(orderIDs) > 0 && workerID%10 == 0 {
					orderID := orderIDs[0]
					orderIDs = orderIDs[1:]
					req = httptest.NewRequest(http.MethodDelete, "/api/v1/orders/"+orderID, nil)
					requestStart = time.Now()
					app.Test(req)
					latency = time.Since(requestStart)
					metrics.AddLatency(latency)
				}
			}
		}(i)
	}

	wg.Wait()
	actualDuration := time.Since(startTime)

	stats := metrics.GetStats()
	throughput := float64(metrics.SuccessfulRequests) / actualDuration.Seconds()

	t.Logf("=== Concurrent Operations Performance Test ===")
	t.Logf("Duration: %v", actualDuration)
	t.Logf("Total Requests: %d", metrics.TotalRequests)
	t.Logf("Successful Requests: %d", metrics.SuccessfulRequests)
	t.Logf("Throughput: %.2f operations/second", throughput)
	t.Logf("Latency P50: %.2f ms", stats["latency_p50_ms"])
	t.Logf("Latency P95: %.2f ms", stats["latency_p95_ms"])
	t.Logf("Latency P99: %.2f ms", stats["latency_p99_ms"])
	t.Logf("Latency P999: %.2f ms", stats["latency_p999_ms"])
}

// TestOrderBookQueryPerformance tests order book query performance
func TestOrderBookQueryPerformance(t *testing.T) {
	app := setupTestServer()

	// Pre-populate order book
	numOrders := 5000
	for i := 0; i < numOrders; i++ {
		side := "SELL"
		if i%2 == 0 {
			side = "BUY"
		}
		reqBody := map[string]interface{}{
			"symbol":   "AAPL",
			"side":     side,
			"type":     "LIMIT",
			"price":    15000 + int64(i%500),
			"quantity": 100,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		app.Test(req)
	}

	// Test query performance
	numQueries := 1000
	concurrency := 20

	metrics := &PerformanceMetrics{}
	var wg sync.WaitGroup

	startTime := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			queriesPerWorker := numQueries / concurrency
			for j := 0; j < queriesPerWorker; j++ {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/orderbook/AAPL?depth=20", nil)

				requestStart := time.Now()
				resp, err := app.Test(req)
				latency := time.Since(requestStart)

				atomic.AddInt64(&metrics.TotalRequests, 1)
				metrics.AddLatency(latency)

				if err == nil && resp.StatusCode == http.StatusOK {
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
	throughput := float64(metrics.SuccessfulRequests) / duration.Seconds()

	t.Logf("=== Order Book Query Performance Test ===")
	t.Logf("Pre-populated Orders: %d", numOrders)
	t.Logf("Queries: %d", numQueries)
	t.Logf("Duration: %v", duration)
	t.Logf("Throughput: %.2f queries/second", throughput)
	t.Logf("Latency P50: %.2f ms", stats["latency_p50_ms"])
	t.Logf("Latency P99: %.2f ms", stats["latency_p99_ms"])

	// Performance assertions
	if stats["latency_p99_ms"].(float64) > 20 {
		t.Errorf("Order book query P99 latency too high: %.2f ms (target: <20ms)", stats["latency_p99_ms"])
	}
}

// BenchmarkOrderSubmission benchmarks order submission
func BenchmarkOrderSubmission(b *testing.B) {
	app := setupTestServer()

	reqBody := map[string]interface{}{
		"symbol":   "AAPL",
		"side":     "BUY",
		"type":     "LIMIT",
		"price":    15050,
		"quantity": 100,
	}

	body, _ := json.Marshal(reqBody)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			app.Test(req)
		}
	})
}

// BenchmarkOrderMatching benchmarks order matching
func BenchmarkOrderMatching(b *testing.B) {
	app := setupTestServer()

	// Pre-populate with sell orders
	for i := 0; i < 100; i++ {
		reqBody := map[string]interface{}{
			"symbol":   "AAPL",
			"side":     "SELL",
			"type":     "LIMIT",
			"price":    15050 + int64(i),
			"quantity": 100,
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		app.Test(req)
	}

	reqBody := map[string]interface{}{
		"symbol":   "AAPL",
		"side":     "BUY",
		"type":     "LIMIT",
		"price":    15100,
		"quantity": 50,
	}
	body, _ := json.Marshal(reqBody)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			app.Test(req)
		}
	})
}

// PrintPerformanceReport prints a formatted performance report
func PrintPerformanceReport(metrics *PerformanceMetrics, testName string) {
	stats := metrics.GetStats()
	fmt.Printf("\n=== %s ===\n", testName)
	fmt.Printf("Total Requests: %d\n", stats["total_requests"])
	fmt.Printf("Successful: %d\n", stats["successful_requests"])
	fmt.Printf("Failed: %d\n", stats["failed_requests"])
	fmt.Printf("Success Rate: %.2f%%\n", stats["success_rate"])
	if latency, ok := stats["latency_p50_ms"].(float64); ok {
		fmt.Printf("Latency P50: %.2f ms\n", latency)
		if p95, ok := stats["latency_p95_ms"].(float64); ok {
			fmt.Printf("Latency P95: %.2f ms\n", p95)
		}
		if p99, ok := stats["latency_p99_ms"].(float64); ok {
			fmt.Printf("Latency P99: %.2f ms\n", p99)
		}
		if p999, ok := stats["latency_p999_ms"].(float64); ok {
			fmt.Printf("Latency P999: %.2f ms\n", p999)
		}
		if avg, ok := stats["latency_avg_ms"].(float64); ok {
			fmt.Printf("Latency Avg: %.2f ms\n", avg)
		}
	}
	fmt.Println()
}

