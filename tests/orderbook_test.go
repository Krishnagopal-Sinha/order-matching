package tests

import (
	"testing"

	"github.com/google/uuid"

	"match-engine/src/engine"
)

// TestOrderBookAddOrder tests adding orders to the order book
// Reference: PDF Section 3.1 (Order Book Concept), Page 3, Line 1
func TestOrderBookAddOrder(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	order := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 100)
	orderBook.AddOrder(order)

	// Verify order is stored
	retrieved, exists := orderBook.GetOrder(order.ID)
	if !exists {
		t.Fatal("Order should exist in order book")
	}

	if retrieved.ID != order.ID {
		t.Errorf("Expected order ID %s, got: %s", order.ID, retrieved.ID)
	}
}

// TestOrderBookBestBidAsk tests getting best bid and ask
// Reference: PDF Section 3.1 (Order Book Concept), Page 3, Line 1
// Best bid: highest buy price, Best ask: lowest sell price
func TestOrderBookBestBidAsk(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	// Add multiple bids (buy orders)
	order1 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 100)
	orderBook.AddOrder(order1)

	order2 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15060, 200)
	orderBook.AddOrder(order2)

	order3 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15040, 300)
	orderBook.AddOrder(order3)

	// Best bid should be highest price: $150.60
	price, qty, ok := orderBook.GetBestBid()
	if !ok {
		t.Fatal("Should have best bid")
	}

	if price != 15060 {
		t.Errorf("Expected best bid price 15060, got: %d", price)
	}

	if qty != 200 {
		t.Errorf("Expected best bid quantity 200, got: %d", qty)
	}

	// Add multiple asks (sell orders)
	sellOrder1 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideSell, engine.TypeLimit, 15070, 100)
	orderBook.AddOrder(sellOrder1)

	sellOrder2 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideSell, engine.TypeLimit, 15080, 200)
	orderBook.AddOrder(sellOrder2)

	sellOrder3 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideSell, engine.TypeLimit, 15065, 300)
	orderBook.AddOrder(sellOrder3)

	// Best ask should be lowest price: $150.65
	price, qty, ok = orderBook.GetBestAsk()
	if !ok {
		t.Fatal("Should have best ask")
	}

	if price != 15065 {
		t.Errorf("Expected best ask price 15065, got: %d", price)
	}

	if qty != 300 {
		t.Errorf("Expected best ask quantity 300, got: %d", qty)
	}
}

// TestOrderBookSnapshot tests getting order book snapshot
// Reference: PDF Section 4.3 (Get Order Book), Page 4, Line 1
func TestOrderBookSnapshot(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	// Add multiple orders at different price levels
	order1 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 100)
	orderBook.AddOrder(order1)

	order2 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15040, 200)
	orderBook.AddOrder(order2)

	sellOrder1 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideSell, engine.TypeLimit, 15060, 150)
	orderBook.AddOrder(sellOrder1)

	sellOrder2 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideSell, engine.TypeLimit, 15070, 250)
	orderBook.AddOrder(sellOrder2)

	// Get snapshot
	bids, asks := orderBook.GetOrderBookSnapshot(10)

	// Verify bids (should be descending: highest first)
	// Reference: PDF Section 4.3 (Get Order Book), Page 4, Line 1
	// Bids sorted by price descending (highest first)
	if len(bids) != 2 {
		t.Fatalf("Expected 2 bid levels, got: %d", len(bids))
	}

	if bids[0].Price != 15050 {
		t.Errorf("Expected first bid price 15050, got: %d", bids[0].Price)
	}

	if bids[1].Price != 15040 {
		t.Errorf("Expected second bid price 15040, got: %d", bids[1].Price)
	}

	// Verify asks (should be ascending: lowest first)
	// Reference: PDF Section 4.3 (Get Order Book), Page 4, Line 1
	// Asks sorted by price ascending (lowest first)
	if len(asks) != 2 {
		t.Fatalf("Expected 2 ask levels, got: %d", len(asks))
	}

	if asks[0].Price != 15060 {
		t.Errorf("Expected first ask price 15060, got: %d", asks[0].Price)
	}

	if asks[1].Price != 15070 {
		t.Errorf("Expected second ask price 15070, got: %d", asks[1].Price)
	}
}

// TestOrderBookPriceLevelAggregation tests quantity aggregation at price levels
// Reference: PDF Section 4.3 (Get Order Book), Page 4, Line 1
// Quantities at each price level are aggregated
func TestOrderBookPriceLevelAggregation(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	// Add multiple orders at the same price level
	order1 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 100)
	orderBook.AddOrder(order1)

	order2 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 200)
	orderBook.AddOrder(order2)

	order3 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 300)
	orderBook.AddOrder(order3)

	// Best bid should aggregate all quantities
	_, qty, ok := orderBook.GetBestBid()
	if !ok {
		t.Fatal("Should have best bid")
	}

	expectedQty := int64(600) // 100 + 200 + 300
	if qty != expectedQty {
		t.Errorf("Expected aggregated quantity %d, got: %d", expectedQty, qty)
	}
}

// TestOrderBookRemoveOrder tests removing orders from the order book
// Reference: PDF Section 4.2 (Cancel Order), Page 4, Line 1
func TestOrderBookRemoveOrder(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	order := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 100)
	orderBook.AddOrder(order)

	// Verify order exists
	_, exists := orderBook.GetOrder(order.ID)
	if !exists {
		t.Fatal("Order should exist")
	}

	// Remove order
	removed := orderBook.RemoveOrder(order.ID)
	if !removed {
		t.Fatal("Order should be removed")
	}

	// Verify order is removed
	_, exists = orderBook.GetOrder(order.ID)
	if exists {
		t.Fatal("Order should not exist after removal")
	}
}

// TestOrderBookEmptyPriceLevelRemoval tests that empty price levels are removed
// Reference: PDF Section 3.1 (Order Book Concept), Page 3, Line 1
func TestOrderBookEmptyPriceLevelRemoval(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	order := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 100)
	orderBook.AddOrder(order)

	// Verify price level exists
	_, qty, ok := orderBook.GetBestBid()
	if !ok || qty != 100 {
		t.Fatal("Price level should exist with quantity 100")
	}

	// Remove order
	orderBook.RemoveOrder(order.ID)

	// Verify price level is removed
	_, _, ok = orderBook.GetBestBid()
	if ok {
		t.Fatal("Price level should be removed when empty")
	}
}

// TestOrderBookSnapshotDepth tests order book snapshot with depth limit
// Reference: PDF Section 4.3 (Get Order Book), Page 4, Line 1
func TestOrderBookSnapshotDepth(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	// Add multiple orders at different price levels
	for i := 0; i < 15; i++ {
		order := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15000+int64(i*10), 100)
		orderBook.AddOrder(order)
	}

	// Get snapshot with depth=5
	bids, _ := orderBook.GetOrderBookSnapshot(5)

	// Should return at most 5 bid levels
	if len(bids) > 5 {
		t.Errorf("Expected at most 5 bid levels, got: %d", len(bids))
	}

	// Verify bids are sorted descending (highest first)
	// Reference: PDF Section 4.3 (Get Order Book), Page 4, Line 1
	if len(bids) > 1 {
		for i := 0; i < len(bids)-1; i++ {
			if bids[i].Price < bids[i+1].Price {
				t.Errorf("Bids should be sorted descending, but %d < %d", bids[i].Price, bids[i+1].Price)
			}
		}
	}
}

// TestOrderBookSnapshotAsksAscending tests that asks are sorted ascending
// Reference: PDF Section 4.3 (Get Order Book), Page 4, Line 1
func TestOrderBookSnapshotAsksAscending(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	// Add multiple sell orders at different prices
	prices := []int64{15070, 15060, 15080, 15065, 15075}
	for _, price := range prices {
		order := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideSell, engine.TypeLimit, price, 100)
		orderBook.AddOrder(order)
	}

	// Get snapshot
	_, asks := orderBook.GetOrderBookSnapshot(10)

	// Verify asks are sorted ascending (lowest first)
	// Reference: PDF Section 4.3 (Get Order Book), Page 4, Line 1
	if len(asks) < 2 {
		t.Fatal("Expected at least 2 ask levels")
	}

	for i := 0; i < len(asks)-1; i++ {
		if asks[i].Price > asks[i+1].Price {
			t.Errorf("Asks should be sorted ascending, but %d > %d", asks[i].Price, asks[i+1].Price)
		}
	}

	// First ask should be lowest price
	if asks[0].Price != 15060 {
		t.Errorf("Expected first ask price 15060, got: %d", asks[0].Price)
	}
}

// TestOrderBookPartialFillAggregation tests quantity aggregation after partial fills
// Reference: PDF Section 4.3 (Get Order Book), Page 4, Line 1
func TestOrderBookPartialFillAggregation(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	// Add multiple orders at same price level
	order1 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 100)
	orderBook.AddOrder(order1)

	order2 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 200)
	orderBook.AddOrder(order2)

	order3 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 300)
	orderBook.AddOrder(order3)

	// Partially fill one order
	order1.Fill(50)

	// Best bid should aggregate remaining quantities
	_, qty, ok := orderBook.GetBestBid()
	if !ok {
		t.Fatal("Should have best bid")
	}

	// Expected: 50 (order1 remaining) + 200 (order2) + 300 (order3) = 550
	expectedQty := int64(550)
	if qty != expectedQty {
		t.Errorf("Expected aggregated quantity %d, got: %d", expectedQty, qty)
	}
}

// TestOrderBookGetOrderNotFound tests getting non-existent order
// Reference: PDF Section 4.2 (Cancel Order), Page 4, Line 1
func TestOrderBookGetOrderNotFound(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	// Try to get non-existent order
	_, exists := orderBook.GetOrder("non-existent-id")
	if exists {
		t.Error("Order should not exist")
	}
}

// TestOrderBookRemoveNonExistentOrder tests removing non-existent order
// Reference: PDF Section 4.2 (Cancel Order), Page 4, Line 1
func TestOrderBookRemoveNonExistentOrder(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	// Try to remove non-existent order
	removed := orderBook.RemoveOrder("non-existent-id")
	if removed {
		t.Error("Should return false for non-existent order")
	}
}

// TestOrderBookBestBidAskEmpty tests best bid/ask when order book is empty
// Reference: PDF Section 3.1 (Order Book Concept), Page 3, Line 1
func TestOrderBookBestBidAskEmpty(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	// Get best bid from empty book
	_, _, ok := orderBook.GetBestBid()
	if ok {
		t.Error("Should not have best bid in empty book")
	}

	// Get best ask from empty book
	_, _, ok = orderBook.GetBestAsk()
	if ok {
		t.Error("Should not have best ask in empty book")
	}
}

// TestOrderBookMultiplePriceLevelsSnapshot tests snapshot with multiple price levels
// Reference: PDF Section 4.3 (Get Order Book), Page 4, Line 1
func TestOrderBookMultiplePriceLevelsSnapshot(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	// Add bids at different price levels
	bidPrices := []int64{15050, 15040, 15060, 15045, 15055}
	for _, price := range bidPrices {
		order := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, price, 100)
		orderBook.AddOrder(order)
	}

	// Add asks at different price levels
	askPrices := []int64{15070, 15080, 15065, 15075, 15085}
	for _, price := range askPrices {
		order := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideSell, engine.TypeLimit, price, 100)
		orderBook.AddOrder(order)
	}

	// Get snapshot
	bids, asks := orderBook.GetOrderBookSnapshot(10)

	// Verify all price levels are included
	if len(bids) != 5 {
		t.Errorf("Expected 5 bid levels, got: %d", len(bids))
	}

	if len(asks) != 5 {
		t.Errorf("Expected 5 ask levels, got: %d", len(asks))
	}

	// Verify bids are sorted descending
	if bids[0].Price != 15060 {
		t.Errorf("Expected highest bid 15060, got: %d", bids[0].Price)
	}

	// Verify asks are sorted ascending
	if asks[0].Price != 15065 {
		t.Errorf("Expected lowest ask 15065, got: %d", asks[0].Price)
	}
}

// TestOrderBookFIFOOrdering tests that orders at same price maintain FIFO order
// Reference: PDF Section 3.2 (Price-Time Priority Rules), Page 3, Line 1
func TestOrderBookFIFOOrdering(t *testing.T) {
	orderBook := engine.NewOrderBook("AAPL")

	// Add multiple orders at same price in sequence
	order1 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 100)
	orderBook.AddOrder(order1)

	order2 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 200)
	orderBook.AddOrder(order2)

	order3 := engine.NewOrder(uuid.New().String(), "AAPL", engine.SideBuy, engine.TypeLimit, 15050, 300)
	orderBook.AddOrder(order3)

	// Get price level
	priceLevel := orderBook.GetPriceLevelForBid(15050)
	if priceLevel == nil {
		t.Fatal("Price level should exist")
	}

	// Verify orders are in FIFO order (order1, order2, order3)
	if len(priceLevel.Orders) != 3 {
		t.Fatalf("Expected 3 orders, got: %d", len(priceLevel.Orders))
	}

	if priceLevel.Orders[0].ID != order1.ID {
		t.Errorf("Expected first order to be order1, got: %s", priceLevel.Orders[0].ID)
	}

	if priceLevel.Orders[1].ID != order2.ID {
		t.Errorf("Expected second order to be order2, got: %s", priceLevel.Orders[1].ID)
	}

	if priceLevel.Orders[2].ID != order3.ID {
		t.Errorf("Expected third order to be order3, got: %s", priceLevel.Orders[2].ID)
	}
}

