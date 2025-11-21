package tests

import (
	"testing"

	"github.com/google/uuid"

	"match-engine/src/engine"
)

// TestSimpleFullMatch tests Example 1 from PDF Section 3.3
// Reference: PDF Section 3.3 Example 1 (Simple Full Match), Page 3, Line 1
// Initial state: SELL $150.50 (1000 shares), BUY $150.45 (500 shares)
// New order: BUY $150.50 (500 shares)
// Expected: Trade executed at $150.50, 500 shares filled
func TestSimpleFullMatch(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Setup initial order book
	// Reference: PDF Section 3.3 Example 1, Page 3, Line 1
	// SELL order at $150.50 with 1000 shares
	sellOrder1 := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15050, 1000)
	_, _ = matcher.MatchOrder(sellOrder1)

	// BUY order at $150.45 with 500 shares (won't match)
	buyOrder1 := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15045, 500)
	_, _ = matcher.MatchOrder(buyOrder1)

	// New BUY order at $150.50 with 500 shares
	// Reference: PDF Section 3.3 Example 1, Page 3, Line 1
	// This should match against the SELL order at $150.50
	newBuyOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15050, 500)
	result, err := matcher.MatchOrder(newBuyOrder)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify result
	// Reference: PDF Section 3.3 Example 1, Page 3, Line 1
	// Expected: Trade executed: 500 shares at $150.50
	if result.Status != engine.StatusFilled {
		t.Errorf("Expected status FILLED, got: %s", result.Status)
	}

	if result.FilledQuantity != 500 {
		t.Errorf("Expected filled quantity 500, got: %d", result.FilledQuantity)
	}

	if len(result.Trades) != 1 {
		t.Fatalf("Expected 1 trade, got: %d", len(result.Trades))
	}

	trade := result.Trades[0]
	if trade.Price != 15050 {
		t.Errorf("Expected trade price 15050, got: %d", trade.Price)
	}

	if trade.Quantity != 500 {
		t.Errorf("Expected trade quantity 500, got: %d", trade.Quantity)
	}

	// Verify order book state
	// Reference: PDF Section 3.3 Example 1, Page 3, Line 1
	// Order-001 remaining: 500 shares still at $150.50
	orderBook := matcher.GetOrCreateOrderBook(symbol)
	_, qty, _ := orderBook.GetBestAsk()
	if qty != 500 {
		t.Errorf("Expected remaining ask quantity 500, got: %d", qty)
	}
}

// TestSellOrderMatching tests SELL orders matching against BUY orders
// Reference: PDF Section 3.3 (Matching Examples), Page 3, Line 1
// Tests the reverse direction: SELL orders matching against existing BUY orders
func TestSellOrderMatching(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Setup initial order book with BUY orders
	// BUY order at $150.50 with 1000 shares
	buyOrder1 := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15050, 1000)
	_, _ = matcher.MatchOrder(buyOrder1)

	// BUY order at $150.45 with 500 shares (won't match with incoming sell)
	buyOrder2 := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15045, 500)
	_, _ = matcher.MatchOrder(buyOrder2)

	// New SELL order at $150.50 with 500 shares
	// Should match against the BUY order at $150.50
	newSellOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15050, 500)
	result, err := matcher.MatchOrder(newSellOrder)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify result
	if result.Status != engine.StatusFilled {
		t.Errorf("Expected status FILLED, got: %s", result.Status)
	}

	if result.FilledQuantity != 500 {
		t.Errorf("Expected filled quantity 500, got: %d", result.FilledQuantity)
	}

	if len(result.Trades) != 1 {
		t.Fatalf("Expected 1 trade, got: %d", len(result.Trades))
	}

	trade := result.Trades[0]
	if trade.Price != 15050 {
		t.Errorf("Expected trade price 15050, got: %d", trade.Price)
	}

	if trade.Quantity != 500 {
		t.Errorf("Expected trade quantity 500, got: %d", trade.Quantity)
	}

	// Verify order book state - remaining BUY order should have 500 shares
	orderBook := matcher.GetOrCreateOrderBook(symbol)
	_, qty, _ := orderBook.GetBestBid()
	if qty != 500 {
		t.Errorf("Expected remaining bid quantity 500, got: %d", qty)
	}
}

// TestMarketSellOrder tests market SELL order execution
// Reference: PDF Section 3.3 Example 4 (Market Order Execution), Page 3, Line 1
// Tests that market SELL orders execute at best available BUY prices
func TestMarketSellOrder(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Setup initial order book with BUY orders
	// BUY orders: $150.50 (200), $150.48 (300), $150.45 (400)
	buyOrder1 := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15050, 200)
	_, _ = matcher.MatchOrder(buyOrder1)

	buyOrder2 := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15048, 300)
	_, _ = matcher.MatchOrder(buyOrder2)

	buyOrder3 := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15045, 400)
	_, _ = matcher.MatchOrder(buyOrder3)

	// New MARKET SELL order with 600 shares
	// Should match: 200 at $150.50, 300 at $150.48, 100 at $150.45
	newSellOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeMarket, 0, 600)
	result, err := matcher.MatchOrder(newSellOrder)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify result
	if result.Status != engine.StatusFilled {
		t.Errorf("Expected status FILLED, got: %s", result.Status)
	}

	if result.FilledQuantity != 600 {
		t.Errorf("Expected filled quantity 600, got: %d", result.FilledQuantity)
	}

	// Verify trades
	if len(result.Trades) != 3 {
		t.Fatalf("Expected 3 trades, got: %d", len(result.Trades))
	}

	if result.Trades[0].Price != 15050 || result.Trades[0].Quantity != 200 {
		t.Errorf("Expected first trade: 200 shares at $150.50, got: %d shares at $%d",
			result.Trades[0].Quantity, result.Trades[0].Price)
	}

	if result.Trades[1].Price != 15048 || result.Trades[1].Quantity != 300 {
		t.Errorf("Expected second trade: 300 shares at $150.48, got: %d shares at $%d",
			result.Trades[1].Quantity, result.Trades[1].Price)
	}

	if result.Trades[2].Price != 15045 || result.Trades[2].Quantity != 100 {
		t.Errorf("Expected third trade: 100 shares at $150.45, got: %d shares at $%d",
			result.Trades[2].Quantity, result.Trades[2].Price)
	}
}

// TestMarketSellInsufficientLiquidity tests market SELL order rejection
// Reference: PDF Section 3.3 Example 5 (Insufficient Liquidity), Page 3, Line 1
func TestMarketSellInsufficientLiquidity(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Setup initial order book with small BUY order
	// BUY order: $150.50 (100 shares)
	buyOrder1 := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15050, 100)
	_, _ = matcher.MatchOrder(buyOrder1)

	// New MARKET SELL order with 500 shares
	// Should be rejected: only 100 shares available, requested 500
	newSellOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeMarket, 0, 500)
	result, err := matcher.MatchOrder(newSellOrder)

	// Verify error
	if err == nil {
		t.Fatal("Expected insufficient liquidity error, got nil")
	}

	if _, ok := err.(*engine.InsufficientLiquidityError); !ok {
		t.Errorf("Expected InsufficientLiquidityError, got: %v", err)
	}

	if result != nil {
		t.Errorf("Expected nil result, got: %v", result)
	}
}

// TestPartialFillRestingOrder tests partial fill of resting order
// Reference: PDF Section 3.3 (Matching Examples), Page 3, Line 1
// Tests that when incoming order is larger, resting order gets partially filled
func TestPartialFillRestingOrder(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Setup: SELL order at $150.50 with 300 shares
	sellOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15050, 300)
	_, _ = matcher.MatchOrder(sellOrder)

	// New BUY order at $150.50 with 500 shares
	// Should match: 300 shares filled, 200 remaining in book
	newBuyOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15050, 500)
	result, err := matcher.MatchOrder(newBuyOrder)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify result - incoming order should be partially filled
	if result.Status != engine.StatusPartialFill {
		t.Errorf("Expected status PARTIAL_FILL, got: %s", result.Status)
	}

	if result.FilledQuantity != 300 {
		t.Errorf("Expected filled quantity 300, got: %d", result.FilledQuantity)
	}

	if result.RemainingQuantity != 200 {
		t.Errorf("Expected remaining quantity 200, got: %d", result.RemainingQuantity)
	}

	// Verify resting order is fully filled and removed
	orderBook := matcher.GetOrCreateOrderBook(symbol)
	_, exists := orderBook.GetOrder(sellOrder.ID)
	if exists {
		t.Error("Resting sell order should be fully filled and removed")
	}

	// Verify incoming order remains in book
	_, exists = orderBook.GetOrder(newBuyOrder.ID)
	if !exists {
		t.Error("Incoming buy order should remain in book with remaining quantity")
	}
}

// TestMultipleSymbols tests matching orders for different symbols
// Reference: PDF FAQ (What symbols should I support?) - support any string symbol
func TestMultipleSymbols(t *testing.T) {
	matcher := engine.NewMatcher()

	// Add orders for different symbols
	symbol1 := "AAPL"
	symbol2 := "GOOGL"

	// AAPL: SELL order
	sellOrder1 := engine.NewOrder(uuid.New().String(), symbol1, engine.SideSell, engine.TypeLimit, 15050, 100)
	_, _ = matcher.MatchOrder(sellOrder1)

	// GOOGL: SELL order
	sellOrder2 := engine.NewOrder(uuid.New().String(), symbol2, engine.SideSell, engine.TypeLimit, 25000, 200)
	_, _ = matcher.MatchOrder(sellOrder2)

	// Match BUY order for AAPL
	buyOrder1 := engine.NewOrder(uuid.New().String(), symbol1, engine.SideBuy, engine.TypeLimit, 15050, 100)
	result1, err1 := matcher.MatchOrder(buyOrder1)

	if err1 != nil {
		t.Fatalf("Expected no error for AAPL, got: %v", err1)
	}

	if result1.Status != engine.StatusFilled {
		t.Errorf("Expected AAPL order to be FILLED, got: %s", result1.Status)
	}

	// Verify GOOGL order book is unaffected
	orderBook2 := matcher.GetOrCreateOrderBook(symbol2)
	_, exists := orderBook2.GetOrder(sellOrder2.ID)
	if !exists {
		t.Error("GOOGL order should still exist")
	}
}

// TestLimitOrderPriceCrossing tests that limit orders only match when price crosses
// Reference: PDF Section 3.2 (Price-Time Priority Rules), Page 3, Line 1
func TestLimitOrderPriceCrossing(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Setup: SELL order at $150.50
	sellOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15050, 1000)
	_, _ = matcher.MatchOrder(sellOrder)

	// BUY order at $150.49 (too low, won't match)
	buyOrder1 := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15049, 500)
	result1, err1 := matcher.MatchOrder(buyOrder1)

	if err1 != nil {
		t.Fatalf("Expected no error, got: %v", err1)
	}

	if result1.Status != engine.StatusAccepted {
		t.Errorf("Expected status ACCEPTED, got: %s", result1.Status)
	}

	if result1.FilledQuantity != 0 {
		t.Errorf("Expected filled quantity 0, got: %d", result1.FilledQuantity)
	}

	// BUY order at $150.50 (exact match, should match)
	buyOrder2 := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15050, 500)
	result2, err2 := matcher.MatchOrder(buyOrder2)

	if err2 != nil {
		t.Fatalf("Expected no error, got: %v", err2)
	}

	if result2.Status != engine.StatusFilled {
		t.Errorf("Expected status FILLED, got: %s", result2.Status)
	}

	if result2.FilledQuantity != 500 {
		t.Errorf("Expected filled quantity 500, got: %d", result2.FilledQuantity)
	}
}

// TestOrderCancellationAfterPartialFill tests cancelling a partially filled order
// Reference: PDF Section 4.2 (Cancel Order), Page 4, Line 1
func TestOrderCancellationAfterPartialFill(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Setup: SELL order at $150.50 with 300 shares
	sellOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15050, 300)
	_, _ = matcher.MatchOrder(sellOrder)

	// BUY order at $150.50 with 500 shares (partially fills)
	buyOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15050, 500)
	result, _ := matcher.MatchOrder(buyOrder)

	if result.Status != engine.StatusPartialFill {
		t.Fatalf("Expected partial fill, got: %s", result.Status)
	}

	// Cancel the partially filled order
	orderBook := matcher.GetOrCreateOrderBook(symbol)
	removed := orderBook.RemoveOrder(buyOrder.ID)
	if !removed {
		t.Fatal("Order should be removed")
	}

	// Verify order is removed from book
	_, exists := orderBook.GetOrder(buyOrder.ID)
	if exists {
		t.Fatal("Order should not exist after cancellation")
	}

	// Verify order status is updated
	if buyOrder.Status != engine.StatusPartialFill {
		t.Errorf("Order status should remain PARTIAL_FILL, got: %s", buyOrder.Status)
	}
}

// TestMultiplePriceLevels tests Example 2 from PDF Section 3.3
// Reference: PDF Section 3.3 Example 2 (Multiple Price Levels - Walking the Book), Page 3, Line 1
// Tests partial fills across multiple price levels
func TestMultiplePriceLevels(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Setup initial order book
	// Reference: PDF Section 3.3 Example 2, Page 3, Line 1
	// SELL $150.50 (300 shares), SELL $150.52 (400 shares), SELL $150.55 (600 shares)
	sellOrder1 := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15050, 300)
	_, _ = matcher.MatchOrder(sellOrder1)

	sellOrder2 := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15052, 400)
	_, _ = matcher.MatchOrder(sellOrder2)

	sellOrder3 := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15055, 600)
	_, _ = matcher.MatchOrder(sellOrder3)

	// New BUY order at $150.53 with 800 shares
	// Reference: PDF Section 3.3 Example 2, Page 3, Line 1
	// Should match: 300 at $150.50, 400 at $150.52, then stop (can't match at $150.55)
	newBuyOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15053, 800)
	result, err := matcher.MatchOrder(newBuyOrder)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify result
	// Reference: PDF Section 3.3 Example 2, Page 3, Line 1
	// Expected: Total filled: 700 shares, Remaining: 100 shares
	if result.FilledQuantity != 700 {
		t.Errorf("Expected filled quantity 700, got: %d", result.FilledQuantity)
	}

	if result.RemainingQuantity != 100 {
		t.Errorf("Expected remaining quantity 100, got: %d", result.RemainingQuantity)
	}

	if result.Status != engine.StatusPartialFill {
		t.Errorf("Expected status PARTIAL_FILL, got: %s", result.Status)
	}

	// Verify trades
	// Reference: PDF Section 3.3 Example 2, Page 3, Line 1
	// Expected: Trade 1: 300 shares at $150.50, Trade 2: 400 shares at $150.52
	if len(result.Trades) != 2 {
		t.Fatalf("Expected 2 trades, got: %d", len(result.Trades))
	}

	if result.Trades[0].Price != 15050 || result.Trades[0].Quantity != 300 {
		t.Errorf("Expected first trade: 300 shares at $150.50, got: %d shares at $%d",
			result.Trades[0].Quantity, result.Trades[0].Price)
	}

	if result.Trades[1].Price != 15052 || result.Trades[1].Quantity != 400 {
		t.Errorf("Expected second trade: 400 shares at $150.52, got: %d shares at $%d",
			result.Trades[1].Quantity, result.Trades[1].Price)
	}
}

// TestTimePriority tests Example 3 from PDF Section 3.3
// Reference: PDF Section 3.3 Example 3 (Time Priority at Same Price - FIFO), Page 3, Line 1
// Tests that orders at the same price match in FIFO order (timestamp order)
func TestTimePriority(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Setup initial order book
	// Reference: PDF Section 3.3 Example 3, Page 3, Line 1
	// Three SELL orders at $150.50: 200, 300, 400 shares (in that order)
	sellOrder1 := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15050, 200)
	_, _ = matcher.MatchOrder(sellOrder1)

	sellOrder2 := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15050, 300)
	_, _ = matcher.MatchOrder(sellOrder2)

	sellOrder3 := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15050, 400)
	_, _ = matcher.MatchOrder(sellOrder3)

	// New BUY order at $150.50 with 500 shares
	// Reference: PDF Section 3.3 Example 3, Page 3, Line 1
	// Should match: 200 from order-007, 300 from order-008, stop (order-009 untouched)
	newBuyOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15050, 500)
	result, err := matcher.MatchOrder(newBuyOrder)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify result
	// Reference: PDF Section 3.3 Example 3, Page 3, Line 1
	// Expected: Trade 1: 200 shares, Trade 2: 300 shares
	if result.FilledQuantity != 500 {
		t.Errorf("Expected filled quantity 500, got: %d", result.FilledQuantity)
	}

	if result.Status != engine.StatusFilled {
		t.Errorf("Expected status FILLED, got: %s", result.Status)
	}

	// Verify trades
	// Reference: PDF Section 3.3 Example 3, Page 3, Line 1
	// Expected: Two trades matching in FIFO order
	if len(result.Trades) != 2 {
		t.Fatalf("Expected 2 trades, got: %d", len(result.Trades))
	}

	if result.Trades[0].Quantity != 200 {
		t.Errorf("Expected first trade quantity 200, got: %d", result.Trades[0].Quantity)
	}

	if result.Trades[1].Quantity != 300 {
		t.Errorf("Expected second trade quantity 300, got: %d", result.Trades[1].Quantity)
	}

	// Verify order book state
	// Reference: PDF Section 3.3 Example 3, Page 3, Line 1
	// Order-009 should remain untouched (400 shares)
	orderBook := matcher.GetOrCreateOrderBook(symbol)
	
	// Verify the third sell order still exists with 400 shares
	order3, exists := orderBook.GetOrder(sellOrder3.ID)
	if !exists {
		t.Fatal("Third sell order should still exist")
	}
	if order3.RemainingQuantity() != 400 {
		t.Errorf("Expected third order remaining quantity 400, got: %d", order3.RemainingQuantity())
	}
	
	// Verify best ask
	_, qty, _ := orderBook.GetBestAsk()
	if qty != 400 {
		t.Errorf("Expected remaining ask quantity 400, got: %d", qty)
	}
}

// TestMarketOrderExecution tests Example 4 from PDF Section 3.3
// Reference: PDF Section 3.3 Example 4 (Market Order Execution), Page 3, Line 1
// Tests that market orders execute at best available prices
func TestMarketOrderExecution(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Setup initial order book
	// Reference: PDF Section 3.3 Example 4, Page 3, Line 1
	// SELL orders: $150.50 (200), $150.52 (300), $150.55 (400)
	sellOrder1 := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15050, 200)
	_, _ = matcher.MatchOrder(sellOrder1)

	sellOrder2 := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15052, 300)
	_, _ = matcher.MatchOrder(sellOrder2)

	sellOrder3 := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15055, 400)
	_, _ = matcher.MatchOrder(sellOrder3)

	// New MARKET BUY order with 600 shares
	// Reference: PDF Section 3.3 Example 4, Page 3, Line 1
	// Should match: 200 at $150.50, 300 at $150.52, 100 at $150.55
	newBuyOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeMarket, 0, 600)
	result, err := matcher.MatchOrder(newBuyOrder)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify result
	// Reference: PDF Section 3.3 Example 4, Page 3, Line 1
	// Expected: Fully filled, 600 shares total
	if result.Status != engine.StatusFilled {
		t.Errorf("Expected status FILLED, got: %s", result.Status)
	}

	if result.FilledQuantity != 600 {
		t.Errorf("Expected filled quantity 600, got: %d", result.FilledQuantity)
	}

	// Verify trades
	// Reference: PDF Section 3.3 Example 4, Page 3, Line 1
	// Expected: Three trades at different prices
	if len(result.Trades) != 3 {
		t.Fatalf("Expected 3 trades, got: %d", len(result.Trades))
	}

	if result.Trades[0].Price != 15050 || result.Trades[0].Quantity != 200 {
		t.Errorf("Expected first trade: 200 shares at $150.50, got: %d shares at $%d",
			result.Trades[0].Quantity, result.Trades[0].Price)
	}

	if result.Trades[1].Price != 15052 || result.Trades[1].Quantity != 300 {
		t.Errorf("Expected second trade: 300 shares at $150.52, got: %d shares at $%d",
			result.Trades[1].Quantity, result.Trades[1].Price)
	}

	if result.Trades[2].Price != 15055 || result.Trades[2].Quantity != 100 {
		t.Errorf("Expected third trade: 100 shares at $150.55, got: %d shares at $%d",
			result.Trades[2].Quantity, result.Trades[2].Price)
	}
}

// TestInsufficientLiquidity tests Example 5 from PDF Section 3.3
// Reference: PDF Section 3.3 Example 5 (Insufficient Liquidity), Page 3, Line 1
// Tests that market orders are rejected if insufficient liquidity exists
func TestInsufficientLiquidity(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Setup initial order book
	// Reference: PDF Section 3.3 Example 5, Page 3, Line 1
	// SELL order: $150.50 (100 shares)
	sellOrder1 := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15050, 100)
	_, _ = matcher.MatchOrder(sellOrder1)

	// New MARKET BUY order with 500 shares
	// Reference: PDF Section 3.3 Example 5, Page 3, Line 1
	// Should be rejected: only 100 shares available, requested 500
	newBuyOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeMarket, 0, 500)
	result, err := matcher.MatchOrder(newBuyOrder)

	// Verify error
	// Reference: PDF Section 3.3 Example 5, Page 3, Line 1
	// Expected: Insufficient liquidity error
	if err == nil {
		t.Fatal("Expected insufficient liquidity error, got nil")
	}

	if _, ok := err.(*engine.InsufficientLiquidityError); !ok {
		t.Errorf("Expected InsufficientLiquidityError, got: %v", err)
	}

	if result != nil {
		t.Errorf("Expected nil result, got: %v", result)
	}
}

// TestLimitOrderNoMatch tests that limit orders don't match if price doesn't cross
// Reference: PDF Section 3.2 (Price-Time Priority Rules), Page 3, Line 1
func TestLimitOrderNoMatch(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Setup: SELL order at $150.50
	sellOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideSell, engine.TypeLimit, 15050, 1000)
	_, _ = matcher.MatchOrder(sellOrder)

	// BUY order at $150.45 (too low, won't match)
	buyOrder := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15045, 500)
	result, err := matcher.MatchOrder(buyOrder)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should be accepted but not filled
	if result.Status != engine.StatusAccepted {
		t.Errorf("Expected status ACCEPTED, got: %s", result.Status)
	}

	if result.FilledQuantity != 0 {
		t.Errorf("Expected filled quantity 0, got: %d", result.FilledQuantity)
	}
}

// TestOrderCancellation tests order cancellation
// Reference: PDF Section 4.2 (Cancel Order), Page 4, Line 1
func TestOrderCancellation(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Add an order to the book
	order := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15050, 100)
	_, _ = matcher.MatchOrder(order)

	orderBook := matcher.GetOrCreateOrderBook(symbol)

	// Verify order exists
	_, exists := orderBook.GetOrder(order.ID)
	if !exists {
		t.Fatal("Order should exist before cancellation")
	}

	// Cancel the order
	removed := orderBook.RemoveOrder(order.ID)
	if !removed {
		t.Fatal("Order should be removed")
	}

	// Verify order is removed
	_, exists = orderBook.GetOrder(order.ID)
	if exists {
		t.Fatal("Order should not exist after cancellation")
	}
}

// TestEmptyOrderBook tests matching against empty order book
// Reference: PDF Section 3.3, edge case handling
func TestEmptyOrderBook(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Try to match against empty book
	order := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15050, 100)
	result, err := matcher.MatchOrder(order)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should be accepted but not filled
	if result.Status != engine.StatusAccepted {
		t.Errorf("Expected status ACCEPTED, got: %s", result.Status)
	}
}

// TestMatchingOnNewOrderArrival tests requirement 3.5.2
// Reference: PDF Section 3.5 (Your Implementation Requirements), Requirement 2
// Requirement: "Check for matches whenever a new order arrives"
// This test explicitly verifies that matching is attempted for every new order,
// both when no match exists and when a match does exist
func TestMatchingOnNewOrderArrival(t *testing.T) {
	matcher := engine.NewMatcher()
	symbol := "AAPL"

	// Test Case 1: New order arrives with no matching orders in book
	// Matching should be checked, but no match found, order should be accepted
	// Reference: PDF Section 3.5, Requirement 2 - matching checked even if no match
	order1 := engine.NewOrder(uuid.New().String(), symbol, engine.SideBuy, engine.TypeLimit, 15050, 100)
	result1, err1 := matcher.MatchOrder(order1)

	if err1 != nil {
		t.Fatalf("Expected no error when checking for matches on new order, got: %v", err1)
	}

	// Matching was checked - order should be processed and accepted (no match found)
	if result1.Status != engine.StatusAccepted {
		t.Errorf("Expected status ACCEPTED after matching check (no match), got: %s", result1.Status)
	}

	if result1.FilledQuantity != 0 {
		t.Errorf("Expected filled quantity 0 when no match exists, got: %d", result1.FilledQuantity)
	}

	// Verify order was added to book (matching was attempted)
	orderBook := matcher.GetOrCreateOrderBook(symbol)
	_, exists := orderBook.GetOrder(order1.ID)
	if !exists {
		t.Error("Order should exist in book after matching check, even if no match found")
	}

	// Test Case 2: New order arrives with matching order in book
	// Matching should be checked and match should be found immediately
	// Reference: PDF Section 3.5, Requirement 2 - matching checked and executed when match exists
	// Use a different symbol to avoid interference from Test Case 1
	symbol2 := "GOOGL"
	sellOrder := engine.NewOrder(uuid.New().String(), symbol2, engine.SideSell, engine.TypeLimit, 15050, 200)
	_, _ = matcher.MatchOrder(sellOrder)

	// New buy order arrives - matching should be checked and match should occur
	buyOrder := engine.NewOrder(uuid.New().String(), symbol2, engine.SideBuy, engine.TypeLimit, 15050, 150)
	result2, err2 := matcher.MatchOrder(buyOrder)

	if err2 != nil {
		t.Fatalf("Expected no error when checking for matches on new order, got: %v", err2)
	}

	// Matching was checked and match was found - order should be filled
	if result2.Status != engine.StatusFilled {
		t.Errorf("Expected status FILLED after matching check found match, got: %s", result2.Status)
	}

	if result2.FilledQuantity != 150 {
		t.Errorf("Expected filled quantity 150 when match found, got: %d", result2.FilledQuantity)
	}

	if len(result2.Trades) == 0 {
		t.Error("Expected trade records when match found during matching check")
	}

	// Test Case 3: New market order arrives - matching should be checked immediately
	// Reference: PDF Section 3.5, Requirement 2 - applies to all order types including market orders
	// Use a different symbol to avoid interference
	symbol3 := "MSFT"
	sellOrder2 := engine.NewOrder(uuid.New().String(), symbol3, engine.SideSell, engine.TypeLimit, 15055, 100)
	_, _ = matcher.MatchOrder(sellOrder2)

	marketBuyOrder := engine.NewOrder(uuid.New().String(), symbol3, engine.SideBuy, engine.TypeMarket, 0, 50)
	result3, err3 := matcher.MatchOrder(marketBuyOrder)

	if err3 != nil {
		t.Fatalf("Expected no error when checking for matches on new market order, got: %v", err3)
	}

	// Matching was checked for market order and match was found
	if result3.Status != engine.StatusFilled {
		t.Errorf("Expected status FILLED after matching check on market order, got: %s", result3.Status)
	}

	if result3.FilledQuantity != 50 {
		t.Errorf("Expected filled quantity 50 for market order, got: %d", result3.FilledQuantity)
	}

	// Test Case 4: New order arrives with price that doesn't cross - matching still checked
	// Reference: PDF Section 3.5, Requirement 2 - matching checked even when price doesn't cross
	// Use a different symbol to avoid interference
	symbol4 := "TSLA"
	sellOrder3 := engine.NewOrder(uuid.New().String(), symbol4, engine.SideSell, engine.TypeLimit, 15100, 100)
	_, _ = matcher.MatchOrder(sellOrder3)

	// Buy order at lower price - matching checked but no match (price doesn't cross)
	buyOrder2 := engine.NewOrder(uuid.New().String(), symbol4, engine.SideBuy, engine.TypeLimit, 15050, 100)
	result4, err4 := matcher.MatchOrder(buyOrder2)

	if err4 != nil {
		t.Fatalf("Expected no error when checking for matches, got: %v", err4)
	}

	// Matching was checked but no match (price doesn't cross) - order accepted
	if result4.Status != engine.StatusAccepted {
		t.Errorf("Expected status ACCEPTED when matching checked but price doesn't cross, got: %s", result4.Status)
	}

	// Verify all orders were processed (matching was checked for each)
	// This confirms that matching is attempted whenever a new order arrives
	if result1 == nil || result2 == nil || result3 == nil || result4 == nil {
		t.Error("All orders should have been processed with matching checks")
	}
}

