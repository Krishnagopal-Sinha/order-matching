package models

type SubmitOrderRequest struct {
	Symbol   string `json:"symbol"`
	Side     string `json:"side"`
	Type     string `json:"type"`
	Price    int64  `json:"price"` // price in cents, required for LIMIT, 0 for MARKET
	Quantity int64  `json:"quantity"`
}

type SubmitOrderResponse struct {
	OrderID          string      `json:"order_id"`
	Status           string      `json:"status"`
	Message          string      `json:"message,omitempty"`
	FilledQuantity   int64       `json:"filled_quantity,omitempty"`
	RemainingQuantity int64      `json:"remaining_quantity,omitempty"`
	Trades           []TradeInfo `json:"trades,omitempty"`
}

type TradeInfo struct {
	TradeID   string `json:"trade_id"`
	Price     int64  `json:"price"` // price in cents
	Quantity  int64  `json:"quantity"`
	Timestamp int64  `json:"timestamp"` // unix timestamp in milliseconds
}

type CancelOrderResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type OrderBookResponse struct {
	Symbol    string           `json:"symbol"`
	Timestamp int64            `json:"timestamp"` // unix timestamp in milliseconds
	Bids      []PriceLevelInfo `json:"bids"`      // sorted descending (highest first)
	Asks      []PriceLevelInfo `json:"asks"`      // sorted ascending (lowest first)
}

type PriceLevelInfo struct {
	Price    int64 `json:"price"`    // price in cents
	Quantity int64 `json:"quantity"` // aggregated quantity at this price
}

type OrderStatusResponse struct {
	OrderID        string `json:"order_id"`
	Symbol         string `json:"symbol"`
	Side           string `json:"side"`
	Type           string `json:"type"`
	Price          int64  `json:"price"` // price in cents
	Quantity       int64  `json:"quantity"`
	FilledQuantity int64  `json:"filled_quantity"`
	Status         string `json:"status"`
	Timestamp      int64  `json:"timestamp"` // unix timestamp in milliseconds
}

type HealthResponse struct {
	Status          string `json:"status"`
	UptimeSeconds   int64  `json:"uptime_seconds"`
	OrdersProcessed int64  `json:"orders_processed"`
}

type MetricsResponse struct {
	OrdersReceived        int64   `json:"orders_received"`
	OrdersMatched         int64   `json:"orders_matched"`
	OrdersCancelled       int64   `json:"orders_cancelled"`
	OrdersInBook          int64   `json:"orders_in_book"`
	TradesExecuted        int64   `json:"trades_executed"`
	LatencyP50Ms          float64 `json:"latency_p50_ms"`
	LatencyP99Ms          float64 `json:"latency_p99_ms"`
	LatencyP999Ms         float64 `json:"latency_p999_ms"`
	ThroughputOrdersPerSec float64 `json:"throughput_orders_per_sec"`
}


