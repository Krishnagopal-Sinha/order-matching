package engine

import (
	"sync"
	"time"

	"github.com/google/btree"
	"github.com/google/uuid"
)

type Matcher struct {
	OrderBooks map[string]*OrderBook
	mu         sync.RWMutex
}

func NewMatcher() *Matcher {
	return &Matcher{
		OrderBooks: make(map[string]*OrderBook),
	}
}

func (m *Matcher) GetOrderBooksSnapshot() map[string]*OrderBook {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := make(map[string]*OrderBook, len(m.OrderBooks))
	for k, v := range m.OrderBooks {
		snapshot[k] = v
	}
	return snapshot
}

func (m *Matcher) GetOrCreateOrderBook(symbol string) *OrderBook {
	m.mu.RLock()
	if ob, exists := m.OrderBooks[symbol]; exists {
		m.mu.RUnlock()
		return ob
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// edge case: double-check after acquiring write lock
	if ob, exists := m.OrderBooks[symbol]; exists {
		return ob
	}

	ob := NewOrderBook(symbol)
	m.OrderBooks[symbol] = ob
	return ob
}

type MatchResult struct {
	Status          OrderStatus
	FilledQuantity  int64
	RemainingQuantity int64
	Trades          []*Trade
}

func (m *Matcher) MatchOrder(order *Order) (*MatchResult, error) {
	orderBook := m.GetOrCreateOrderBook(order.Symbol)

	if order.Type == TypeMarket {
		return m.matchMarketOrder(order, orderBook)
	}

	return m.matchLimitOrder(order, orderBook)
}

func (m *Matcher) matchLimitOrder(order *Order, orderBook *OrderBook) (*MatchResult, error) {
	result := &MatchResult{
		Status:          StatusAccepted,
		FilledQuantity: 0,
		RemainingQuantity: order.Quantity,
		Trades:          make([]*Trade, 0),
	}

	remainingQty := order.Quantity

	for remainingQty > 0 {
		var bestPrice int64
		var bestPriceLevel *PriceLevel
		var canMatch bool

		if order.Side == SideBuy {
			askPrice, _, hasAsk := orderBook.GetBestAsk()
			if !hasAsk {
				break
			}
			if order.Price < askPrice {
				break
			}
			bestPrice = askPrice
			bestPriceLevel = orderBook.GetPriceLevelForAsk(askPrice)
			canMatch = true
		} else {
			bidPrice, _, hasBid := orderBook.GetBestBid()
			if !hasBid {
				break
			}
			if order.Price > bidPrice {
				break
			}
			bestPrice = bidPrice
			bestPriceLevel = orderBook.GetPriceLevelForBid(bidPrice)
			canMatch = true
		}

		if !canMatch || bestPriceLevel == nil {
			break
		}

		for remainingQty > 0 && !order.IsFilled() {
			// edge case: re-check length to prevent race condition
			if len(bestPriceLevel.Orders) == 0 {
				break
			}
			restingOrder := bestPriceLevel.Orders[0]
			restingRemaining := restingOrder.RemainingQuantity()

			if restingRemaining <= 0 {
				// edge case: order already filled, remove it
				if len(bestPriceLevel.Orders) > 1 {
					bestPriceLevel.Orders = bestPriceLevel.Orders[1:]
				} else {
					bestPriceLevel.Orders = bestPriceLevel.Orders[:0]
				}
				continue
			}

			executionPrice := bestPrice
			executionQty := remainingQty
			if executionQty > restingRemaining {
				executionQty = restingRemaining
			}
			
			if executionQty <= 0 {
				break
			}

			trade := &Trade{
				TradeID:     uuid.New().String(),
				Price:       executionPrice,
				Quantity:    executionQty,
				Timestamp:   time.Now().UnixMilli(),
				BuyOrderID:  "",
				SellOrderID: "",
			}

			if order.Side == SideBuy {
				trade.BuyOrderID = order.ID
				trade.SellOrderID = restingOrder.ID
			} else {
				trade.BuyOrderID = restingOrder.ID
				trade.SellOrderID = order.ID
			}

			result.Trades = append(result.Trades, trade)

			order.Fill(executionQty)
			restingOrder.Fill(executionQty)

			result.FilledQuantity += executionQty
			remainingQty = order.Quantity - order.FilledQuantity

			if remainingQty <= 0 || order.IsFilled() {
				break
			}

			if restingOrder.IsFilled() {
				orderBook.RemoveOrder(restingOrder.ID)
			}

			// edge case: remove empty price level
			if len(bestPriceLevel.Orders) == 0 {
				if order.Side == SideBuy {
					orderBook.Asks.Delete(&PriceLevelItemAscending{PriceLevel: &PriceLevel{Price: bestPrice}})
				} else {
					orderBook.Bids.Delete(&PriceLevelItem{PriceLevel: &PriceLevel{Price: bestPrice}})
				}
				break
			}
		}
		
		if remainingQty <= 0 {
			break
		}
	}

	result.RemainingQuantity = remainingQty

	if result.FilledQuantity == 0 {
		result.Status = StatusAccepted
		orderBook.AddOrder(order)
	} else if remainingQty > 0 {
		result.Status = StatusPartialFill
		orderBook.AddOrder(order)
	} else {
		result.Status = StatusFilled
	}

	return result, nil
}

// edge case: market orders must execute completely or be rejected
func (m *Matcher) matchMarketOrder(order *Order, orderBook *OrderBook) (*MatchResult, error) {
	result := &MatchResult{
		Status:          StatusFilled,
		FilledQuantity: 0,
		RemainingQuantity: 0,
		Trades:          make([]*Trade, 0),
	}

	remainingQty := order.Quantity
	totalAvailable := int64(0)

	if order.Side == SideBuy {
		orderBook.Asks.Ascend(func(item btree.Item) bool {
			priceLevel := item.(*PriceLevelItemAscending).PriceLevel
			for _, o := range priceLevel.Orders {
				totalAvailable += o.RemainingQuantity()
			}
			return true
		})
	} else {
		orderBook.Bids.Ascend(func(item btree.Item) bool {
			priceLevel := item.(*PriceLevelItem).PriceLevel
			for _, o := range priceLevel.Orders {
				totalAvailable += o.RemainingQuantity()
			}
			return true
		})
	}

	// edge case: reject if insufficient liquidity
	if totalAvailable < remainingQty {
		return nil, &InsufficientLiquidityError{
			Requested: remainingQty,
			Available: totalAvailable,
		}
	}

	for remainingQty > 0 {
		var bestPrice int64
		var bestPriceLevel *PriceLevel

		if order.Side == SideBuy {
			askPrice, _, hasAsk := orderBook.GetBestAsk()
			if !hasAsk {
				break
			}
			bestPrice = askPrice
			bestPriceLevel = orderBook.GetPriceLevelForAsk(askPrice)
		} else {
			bidPrice, _, hasBid := orderBook.GetBestBid()
			if !hasBid {
				break
			}
			bestPrice = bidPrice
			bestPriceLevel = orderBook.GetPriceLevelForBid(bidPrice)
		}

		if bestPriceLevel == nil {
			break
		}

		for remainingQty > 0 {
			// edge case: re-check length to prevent race condition
			if len(bestPriceLevel.Orders) == 0 {
				break
			}
			restingOrder := bestPriceLevel.Orders[0]
			restingRemaining := restingOrder.RemainingQuantity()

			if restingRemaining <= 0 {
				if len(bestPriceLevel.Orders) > 1 {
					bestPriceLevel.Orders = bestPriceLevel.Orders[1:]
				} else {
					bestPriceLevel.Orders = bestPriceLevel.Orders[:0]
				}
				continue
			}

			executionPrice := bestPrice
			executionQty := remainingQty
			if executionQty > restingRemaining {
				executionQty = restingRemaining
			}

			trade := &Trade{
				TradeID:     uuid.New().String(),
				Price:       executionPrice,
				Quantity:    executionQty,
				Timestamp:   time.Now().UnixMilli(),
				BuyOrderID:  "",
				SellOrderID: "",
			}

			if order.Side == SideBuy {
				trade.BuyOrderID = order.ID
				trade.SellOrderID = restingOrder.ID
			} else {
				trade.BuyOrderID = restingOrder.ID
				trade.SellOrderID = order.ID
			}

			result.Trades = append(result.Trades, trade)

			order.Fill(executionQty)
			restingOrder.Fill(executionQty)

			result.FilledQuantity += executionQty
			remainingQty -= executionQty

			if restingOrder.IsFilled() {
				orderBook.RemoveOrder(restingOrder.ID)
				if len(bestPriceLevel.Orders) > 1 {
					bestPriceLevel.Orders = bestPriceLevel.Orders[1:]
				} else {
					bestPriceLevel.Orders = bestPriceLevel.Orders[:0]
				}
			}

			if remainingQty <= 0 {
				break
			}

			// edge case: remove empty price level
			if len(bestPriceLevel.Orders) == 0 {
				if order.Side == SideBuy {
					orderBook.Asks.Delete(&PriceLevelItemAscending{PriceLevel: &PriceLevel{Price: bestPrice}})
				} else {
					orderBook.Bids.Delete(&PriceLevelItem{PriceLevel: &PriceLevel{Price: bestPrice}})
				}
				break
			}
		}
	}

	result.RemainingQuantity = remainingQty
	result.Status = StatusFilled

	return result, nil
}

type InsufficientLiquidityError struct {
	Requested int64
	Available int64
}

func (e *InsufficientLiquidityError) Error() string {
	return "Insufficient liquidity"
}

