package handlers

import (
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"match-engine/src/engine"
	"match-engine/src/models"
)

type OrderHandler struct {
	Matcher          *engine.Matcher
	StartTime        time.Time
	OrdersReceived   int64
	OrdersMatched    int64
	OrdersCancelled  int64
	TradesExecuted   int64
	
	latencies        []time.Duration
	latenciesMu      sync.RWMutex
	maxLatencies     int
}

func NewOrderHandler(matcher *engine.Matcher) *OrderHandler {
	maxLatencies := 10000
	if envMax := os.Getenv("METRICS_MAX_LATENCIES"); envMax != "" {
		if parsed, err := strconv.Atoi(envMax); err == nil && parsed > 0 {
			maxLatencies = parsed
		}
	}
	
	return &OrderHandler{
		Matcher:      matcher,
		StartTime:    time.Now(),
		latencies:    make([]time.Duration, 0, maxLatencies),
		maxLatencies: maxLatencies,
	}
}

func (h *OrderHandler) SubmitOrder(c *fiber.Ctx) error {
	var req models.SubmitOrderRequest

	if err := c.BodyParser(&req); err != nil {
		log.Warn().
			Err(err).
			Str("ip", c.IP()).
			Str("path", c.Path()).
			Msg("Invalid request: malformed JSON")
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "Invalid request: malformed JSON",
		})
	}

	if err := validateSubmitOrderRequest(&req); err != nil {
		log.Warn().
			Err(err).
			Str("symbol", req.Symbol).
			Str("side", req.Side).
			Str("type", req.Type).
			Str("ip", c.IP()).
			Msg("Invalid order request")
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: err.Error(),
		})
	}

	orderID := uuid.New().String()

	var side engine.OrderSide
	var orderType engine.OrderType

	if req.Side == "BUY" {
		side = engine.SideBuy
	} else {
		side = engine.SideSell
	}

	if req.Type == "LIMIT" {
		orderType = engine.TypeLimit
	} else {
		orderType = engine.TypeMarket
	}

	order := engine.NewOrder(orderID, req.Symbol, side, orderType, req.Price, req.Quantity)

	startTime := time.Now()

	log.Info().
		Str("order_id", orderID).
		Str("symbol", req.Symbol).
		Str("side", req.Side).
		Str("type", req.Type).
		Int64("price", req.Price).
		Int64("quantity", req.Quantity).
		Str("ip", c.IP()).
		Msg("Order submitted")

	atomic.AddInt64(&h.OrdersReceived, 1)

	result, err := h.Matcher.MatchOrder(order)
	
	latency := time.Since(startTime)
	h.recordLatency(latency)

	// edge case: handle insufficient liquidity for market orders
	if err != nil {
		if _, ok := err.(*engine.InsufficientLiquidityError); ok {
			orderBook := h.Matcher.GetOrCreateOrderBook(req.Symbol)
			var totalAvailable int64
			if side == engine.SideBuy {
				_, qty, _ := orderBook.GetBestAsk()
				totalAvailable = qty
			} else {
				_, qty, _ := orderBook.GetBestBid()
				totalAvailable = qty
			}
			log.Warn().
				Str("order_id", orderID).
				Str("symbol", req.Symbol).
				Int64("requested", req.Quantity).
				Int64("available", totalAvailable).
				Msg("Insufficient liquidity for market order")
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error: "Insufficient liquidity: only " + strconv.FormatInt(totalAvailable, 10) + " shares available, requested " + strconv.FormatInt(req.Quantity, 10),
			})
		}
		log.Error().
			Err(err).
			Str("order_id", orderID).
			Str("symbol", req.Symbol).
			Msg("Error matching order")
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "Internal server error",
		})
	}

	trades := make([]models.TradeInfo, 0, len(result.Trades))
	for _, trade := range result.Trades {
		trades = append(trades, models.TradeInfo{
			TradeID:   trade.TradeID,
			Price:     trade.Price,
			Quantity:  trade.Quantity,
			Timestamp: trade.Timestamp,
		})
	}

	response := models.SubmitOrderResponse{
		OrderID:          orderID,
		Status:           string(result.Status),
		FilledQuantity:   result.FilledQuantity,
		RemainingQuantity: result.RemainingQuantity,
		Trades:           trades,
	}

	if result.Status == engine.StatusPartialFill || result.Status == engine.StatusFilled {
		atomic.AddInt64(&h.OrdersMatched, 1)
	}
	atomic.AddInt64(&h.TradesExecuted, int64(len(trades)))

	log.Info().
		Str("order_id", orderID).
		Str("status", string(result.Status)).
		Int64("filled_quantity", result.FilledQuantity).
		Int64("remaining_quantity", result.RemainingQuantity).
		Int("trades_count", len(result.Trades)).
		Msg("Order processed")

	if result.Status == engine.StatusAccepted {
		response.Message = "Order added to book"
		return c.Status(fiber.StatusCreated).JSON(response)
	} else if result.Status == engine.StatusPartialFill {
		return c.Status(fiber.StatusAccepted).JSON(response)
	} else {
		return c.Status(fiber.StatusOK).JSON(response)
	}
}

func (h *OrderHandler) CancelOrder(c *fiber.Ctx) error {
	orderID := c.Params("id")

	var foundOrder *engine.Order
	var foundOrderBook *engine.OrderBook

	orderBooks := h.Matcher.GetOrderBooksSnapshot()
	for _, orderBook := range orderBooks {
		if order, exists := orderBook.GetOrder(orderID); exists {
			foundOrder = order
			foundOrderBook = orderBook
			break
		}
	}

	if foundOrder == nil {
		log.Warn().
			Str("order_id", orderID).
			Str("ip", c.IP()).
			Msg("Cancel order: order not found")
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error: "Order not found",
		})
	}

	// edge case: cannot cancel already filled orders
	if foundOrder.GetStatus() == engine.StatusFilled {
		log.Warn().
			Str("order_id", orderID).
			Str("status", string(foundOrder.GetStatus())).
			Msg("Cancel order: order already filled")
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "Cannot cancel: order already filled",
		})
	}

	foundOrderBook.RemoveOrder(orderID)
	foundOrder.SetStatus(engine.StatusCancelled)

	atomic.AddInt64(&h.OrdersCancelled, 1)

	log.Info().
		Str("order_id", orderID).
		Str("symbol", foundOrder.Symbol).
		Str("ip", c.IP()).
		Msg("Order cancelled")

	return c.Status(fiber.StatusOK).JSON(models.CancelOrderResponse{
		OrderID: orderID,
		Status:  "CANCELLED",
	})
}

func (h *OrderHandler) GetOrderBook(c *fiber.Ctx) error {
	symbol := c.Params("symbol")

	defaultDepth := 10
	if envDepth := os.Getenv("ORDERBOOK_DEFAULT_DEPTH"); envDepth != "" {
		if parsed, err := strconv.Atoi(envDepth); err == nil && parsed > 0 {
			defaultDepth = parsed
		}
	}
	
	maxDepth := 1000
	if envMaxDepth := os.Getenv("ORDERBOOK_MAX_DEPTH"); envMaxDepth != "" {
		if parsed, err := strconv.Atoi(envMaxDepth); err == nil && parsed > 0 {
			maxDepth = parsed
		}
	}
	
	depthStr := c.Query("depth", strconv.Itoa(defaultDepth))
	depth, err := strconv.Atoi(depthStr)
	if err != nil || depth <= 0 {
		depth = defaultDepth
	}

	// edge case: enforce maximum depth limit
	if depth > maxDepth {
		depth = maxDepth
	}

	orderBook := h.Matcher.GetOrCreateOrderBook(symbol)

	bidsLevels, asksLevels := orderBook.GetOrderBookSnapshot(depth)

	bids := make([]models.PriceLevelInfo, 0, len(bidsLevels))
	for _, level := range bidsLevels {
		bids = append(bids, models.PriceLevelInfo{
			Price:    level.Price,
			Quantity: level.Quantity,
		})
	}

	asks := make([]models.PriceLevelInfo, 0, len(asksLevels))
	for _, level := range asksLevels {
		asks = append(asks, models.PriceLevelInfo{
			Price:    level.Price,
			Quantity: level.Quantity,
		})
	}

	return c.Status(fiber.StatusOK).JSON(models.OrderBookResponse{
		Symbol:    symbol,
		Timestamp: time.Now().UnixMilli(),
		Bids:      bids,
		Asks:      asks,
	})
}

func (h *OrderHandler) GetOrderStatus(c *fiber.Ctx) error {
	orderID := c.Params("id")

	var foundOrder *engine.Order
	orderBooks := h.Matcher.GetOrderBooksSnapshot()
	for _, orderBook := range orderBooks {
		if order, exists := orderBook.GetOrder(orderID); exists {
			foundOrder = order
			break
		}
	}

	if foundOrder == nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error: "Order not found",
		})
	}

	return c.Status(fiber.StatusOK).JSON(models.OrderStatusResponse{
		OrderID:        foundOrder.ID,
		Symbol:         foundOrder.Symbol,
		Side:           string(foundOrder.Side),
		Type:           string(foundOrder.Type),
		Price:          foundOrder.Price,
		Quantity:       foundOrder.Quantity,
		FilledQuantity: foundOrder.GetFilledQuantity(),
		Status:         string(foundOrder.GetStatus()),
		Timestamp:      foundOrder.Timestamp,
	})
}

func (h *OrderHandler) HealthCheck(c *fiber.Ctx) error {
	uptime := time.Since(h.StartTime).Seconds()

	var ordersProcessed int64
	orderBooks := h.Matcher.GetOrderBooksSnapshot()
	for _, orderBook := range orderBooks {
		ordersProcessed += int64(len(orderBook.Orders))
	}

	return c.Status(fiber.StatusOK).JSON(models.HealthResponse{
		Status:         "healthy",
		UptimeSeconds:  int64(uptime),
		OrdersProcessed: ordersProcessed,
	})
}

func (h *OrderHandler) Metrics(c *fiber.Ctx) error {
	var ordersInBook int64
	orderBooks := h.Matcher.GetOrderBooksSnapshot()
	for _, orderBook := range orderBooks {
		ordersInBook += int64(len(orderBook.Orders))
	}

	p50, p99, p999 := h.calculateLatencyPercentiles()
	throughput := h.calculateThroughput()

	return c.Status(fiber.StatusOK).JSON(models.MetricsResponse{
		OrdersReceived:      atomic.LoadInt64(&h.OrdersReceived),
		OrdersMatched:       atomic.LoadInt64(&h.OrdersMatched),
		OrdersCancelled:     atomic.LoadInt64(&h.OrdersCancelled),
		OrdersInBook:        ordersInBook,
		TradesExecuted:      atomic.LoadInt64(&h.TradesExecuted),
		LatencyP50Ms:        p50,
		LatencyP99Ms:        p99,
		LatencyP999Ms:       p999,
		ThroughputOrdersPerSec: throughput,
	})
}

func (h *OrderHandler) recordLatency(latency time.Duration) {
	h.latenciesMu.Lock()
	defer h.latenciesMu.Unlock()
	
	h.latencies = append(h.latencies, latency)

	// edge case: maintain rolling window by removing oldest measurements
	if len(h.latencies) > h.maxLatencies {
		removeCount := len(h.latencies) - h.maxLatencies
		h.latencies = h.latencies[removeCount:]
	}
}

func (h *OrderHandler) calculateLatencyPercentiles() (p50, p99, p999 float64) {
	h.latenciesMu.RLock()
	defer h.latenciesMu.RUnlock()
	
	if len(h.latencies) == 0 {
		return 0, 0, 0
	}
	
	latenciesCopy := make([]time.Duration, len(h.latencies))
	copy(latenciesCopy, h.latencies)
	
	sort.Slice(latenciesCopy, func(i, j int) bool {
		return latenciesCopy[i] < latenciesCopy[j]
	})
	
	p50Index := int(float64(len(latenciesCopy)) * 0.50)
	p99Index := int(float64(len(latenciesCopy)) * 0.99)
	p999Index := int(float64(len(latenciesCopy)) * 0.999)

	// edge case: ensure indices are within bounds
	if p50Index >= len(latenciesCopy) {
		p50Index = len(latenciesCopy) - 1
	}
	if p99Index >= len(latenciesCopy) {
		p99Index = len(latenciesCopy) - 1
	}
	if p999Index >= len(latenciesCopy) {
		p999Index = len(latenciesCopy) - 1
	}
	
	p50 = float64(latenciesCopy[p50Index].Nanoseconds()) / 1e6
	p99 = float64(latenciesCopy[p99Index].Nanoseconds()) / 1e6
	p999 = float64(latenciesCopy[p999Index].Nanoseconds()) / 1e6
	
	return p50, p99, p999
}

func (h *OrderHandler) calculateThroughput() float64 {
	uptime := time.Since(h.StartTime).Seconds()
	if uptime <= 0 {
		return 0
	}
	
	ordersReceived := atomic.LoadInt64(&h.OrdersReceived)
	return float64(ordersReceived) / uptime
}

func validateSubmitOrderRequest(req *models.SubmitOrderRequest) error {
	if req.Symbol == "" {
		return &ValidationError{Message: "Invalid order: symbol is required"}
	}

	if req.Side != "BUY" && req.Side != "SELL" {
		return &ValidationError{Message: "Invalid order: side must be BUY or SELL"}
	}

	if req.Type != "LIMIT" && req.Type != "MARKET" {
		return &ValidationError{Message: "Invalid order: type must be LIMIT or MARKET"}
	}

	if req.Quantity <= 0 {
		return &ValidationError{Message: "Invalid order: quantity must be positive"}
	}

	// edge case: price required for limit orders
	if req.Type == "LIMIT" {
		if req.Price <= 0 {
			return &ValidationError{Message: "Invalid order: price must be positive for LIMIT orders"}
		}
	}

	return nil
}

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

