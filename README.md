# Order Matching Engine

A high-performance order matching engine built in Go that handles stock and cryptocurrency trading orders with low latency. This implementation follows price-time priority matching rules and provides a REST API for order management.

## Building and Running

### Prerequisites

- Go 1.25.4 or later

### Installation

```bash
# Clone the repository
git clone <repository-url>
cd match-engine

# Install dependencies
go mod download

# Build the application
go build -o match-engine main.go

# Run the application
./match-engine
```

Or run directly:

```bash
go run main.go
```

The server starts on port 8080 by default. Change the port using the `PORT` environment variable:

```bash
PORT=3000 go run main.go
```

## Running Tests

Run all tests:

```bash
go test ./...
```

Run tests with coverage:

```bash
go test -cover ./...
```

Run performance tests:

```bash
go test -v -run "Test.*Performance" ./tests
```

## Approach

This order matching engine uses a B-tree data structure (from `github.com/google/btree`) to maintain the order book, providing O(log n) insert and delete operations with O(1) access to the best bid and ask prices. Each price level maintains a FIFO queue of orders to enforce time priority. A hash map provides O(1) lookup for order cancellation by ID. Prices are stored as integers in cents to avoid floating-point precision issues. The matching algorithm implements strict price-time priority: orders match based on best price first, then by arrival time within each price level. The system is fully thread-safe using read-write mutexes for concurrent access, with atomic operations for order state updates.

## Performance Results

The system has been tested against mandatory and stretch goal requirements:

### Mandatory Requirements (All Met)

- **Throughput**: ≥ 30,000 orders/second (sustained 60-second load test)
- **Latency P50**: ≤ 10 ms
- **Latency P99**: ≤ 50 ms
- **Latency P999**: ≤ 100 ms
- **Concurrent Connections**: ≥ 100 connections
- **Correctness**: 100% (no invalid trades, race conditions, or data corruption)

### Stretch Goals

- **Throughput**: ≥ 100,000 orders/second
- **Latency P99**: ≤ 10 ms
- **Latency P999**: ≤ 20 ms

All performance tests are located in the `tests/` directory and can be run individually or as a suite.

## Key Design Decisions

1. **B-tree for Order Book**: Provides O(log n) insert/delete operations and O(1) access to best bid/ask prices. More efficient than a simple sorted list for dynamic order books.

2. **Integer Price Representation**: Prices stored as `int64` in cents (e.g., $150.50 = 15050) eliminates floating-point precision errors and improves performance.

3. **FIFO Queues at Price Levels**: Each price level maintains a slice of orders, ensuring time priority (first-in-first-out) when multiple orders exist at the same price.

4. **Rate Limiting**: Fixed window rate limiting per client IP to prevent abuse and provide back-pressure protection.

5. **Structured Logging**: JSON logging using zerolog for production, with pretty console output for development.

## Concurrency Strategy

The system is designed to handle high concurrency with multiple clients submitting orders simultaneously. The concurrency strategy ensures thread-safety, prevents race conditions, and maintains correctness under load.

### Lock Hierarchy

The implementation uses a two-level locking strategy:

1. **Matcher Level**: A read-write mutex (`sync.RWMutex`) protects the `OrderBooks` map, which stores order books for different symbols. This allows concurrent read access to different symbols while serializing writes.

2. **OrderBook Level**: Each `OrderBook` has its own read-write mutex (`sync.RWMutex`) that protects:
   - The B-tree structures (bids and asks)
   - The orders map (for O(1) order lookup by ID)
   - Price level modifications

### Lock Acquisition Patterns

**Read Operations** (e.g., getting best bid/ask, reading order book):

- Acquire read lock on Matcher's mutex
- Acquire read lock on specific OrderBook's mutex
- Perform read operation
- Release locks (automatic with `defer`)

**Write Operations** (e.g., adding/removing orders, matching):

- Acquire write lock on Matcher's mutex (if needed for OrderBook creation)
- Acquire write lock on specific OrderBook's mutex
- Perform write operation
- Release locks (automatic with `defer`)

### OrderBook Creation Pattern

The `GetOrCreateOrderBook` method uses a double-check locking pattern to avoid race conditions when creating new order books:

1. Acquire read lock and check if order book exists
2. If not found, release read lock and acquire write lock
3. Double-check after acquiring write lock (another goroutine may have created it)
4. Create order book only if still doesn't exist

This pattern minimizes lock contention while ensuring thread-safety.

### Atomic Operations

For order state that can be updated concurrently during matching:

- **FilledQuantity**: Uses `atomic.AddInt64` and `atomic.LoadInt64` for lock-free updates. This allows multiple matching operations to update the filled quantity concurrently without blocking.

- **Status Updates**: Protected by a mutex (`statusMu`) on each order to ensure atomic status transitions (e.g., ACCEPTED → PARTIAL_FILL → FILLED).

### Safe Iteration

When iterating over order books (e.g., for metrics or snapshots), the system uses snapshot patterns:

- `GetOrderBooksSnapshot()` creates a copy of the OrderBooks map while holding a read lock, allowing safe iteration without holding locks for extended periods.

### Deadlock Prevention

The lock ordering is consistent and prevents deadlocks:

1. Always acquire Matcher lock before OrderBook lock
2. Never hold multiple OrderBook locks simultaneously (each operation works on a single symbol)
3. Locks are always released via `defer` statements to prevent lock leaks

### Performance Characteristics

- **Read-heavy workloads**: Read-write mutexes allow multiple concurrent readers, improving throughput for order book queries
- **Write operations**: Write locks serialize modifications, ensuring data consistency
- **Lock granularity**: Fine-grained locking at the OrderBook level allows concurrent operations on different symbols
- **Lock-free reads**: Atomic operations for `FilledQuantity` avoid lock contention during hot-path matching operations

### Testing

The concurrency strategy has been validated through:

- Concurrent test suites with 100+ simultaneous goroutines
- Race condition detection via Go's race detector
- Load testing with sustained high throughput (30,000+ orders/second)
- Correctness verification ensuring no data corruption or invalid trades

## API Endpoints

### Submit Order

**POST** `/api/v1/orders`

Submit a new limit or market order.

```bash
curl -X POST http://localhost:8080/api/v1/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "AAPL",
    "side": "BUY",
    "type": "LIMIT",
    "price": 15050,
    "quantity": 100
  }'
```

### Cancel Order

**DELETE** `/api/v1/orders/{order_id}`

Cancel an active order.

### Get Order Book

**GET** `/api/v1/orderbook/{symbol}?depth=10`

Get the order book for a symbol with optional depth parameter.

### Get Order Status

**GET** `/api/v1/orders/{order_id}`

Get the status of an order.

### Health Check

**GET** `/health`

Check if the service is healthy.

### Metrics

**GET** `/metrics`

Get system metrics including latency percentiles and throughput.

## Configuration

Configuration is done via environment variables:

| Variable                  | Default | Description                                               |
| ------------------------- | ------- | --------------------------------------------------------- |
| `PORT`                    | `:8080` | Server port                                               |
| `LOG_LEVEL`               | `info`  | Log level (trace, debug, info, warn, error, fatal, panic) |
| `LOG_FORMAT`              | `json`  | Log format (json or pretty)                               |
| `RATE_LIMIT_MAX`          | `100`   | Maximum requests per window                               |
| `RATE_LIMIT_WINDOW`       | `1s`    | Rate limit window duration                                |
| `MAINTENANCE_MODE`        | `0`     | Set to `1` to enable maintenance mode                     |
| `MAX_CONCURRENT_REQUESTS` | `0`     | Max concurrent requests (0 = disabled)                    |

## Assumptions and Limitations

**Assumptions:**

- Orders are processed in-memory with no persistence (orders are lost on restart)
- Single symbol per order book (multiple symbols are supported via separate order books)
- Market orders require sufficient liquidity or will be rejected
- Order IDs are generated server-side using UUIDs (extremely unlikely to collide)

**Limitations:**

- No order persistence or recovery after restart
- Basic metrics tracking (can be enhanced with proper instrumentation)
- No advanced order types (stop loss, fill-or-kill, immediate-or-cancel)
- No WebSocket streaming for real-time updates

## What Would Be Improved With More Time

1. **Persistence**: Add database or file-based persistence for order recovery after restarts
2. **Advanced Order Types**: Implement stop loss, fill-or-kill, and immediate-or-cancel orders
3. **WebSocket API**: Real-time order book updates and trade notifications via WebSocket
4. **Observability**: Integrate Prometheus metrics, distributed tracing, and better monitoring
5. **Performance Optimizations**:
   - Lock-free data structures where possible
   - Memory pool for order allocations
   - Batch processing for high-throughput scenarios
6. **Multi-Symbol Optimization**: Optimize for scenarios with many active symbols
7. **Historical Data**: Time-series storage for historical order book snapshots and trade history
