package engine

import (
	"sync"
	"sync/atomic"
	"time"
)

type OrderSide string

const (
	SideBuy  OrderSide = "BUY"
	SideSell OrderSide = "SELL"
)

type OrderType string

const (
	TypeLimit  OrderType = "LIMIT"
	TypeMarket OrderType = "MARKET"
)

type OrderStatus string

const (
	StatusAccepted    OrderStatus = "ACCEPTED"
	StatusPartialFill OrderStatus = "PARTIAL_FILL"
	StatusFilled     OrderStatus = "FILLED"
	StatusCancelled   OrderStatus = "CANCELLED"
)

// edge case: price stored as int64 in cents to avoid floating-point precision errors
type Order struct {
	ID            string
	Symbol        string
	Side          OrderSide
	Type          OrderType
	Price         int64 // price in cents, required for LIMIT, 0 for MARKET
	Quantity      int64
	FilledQuantity int64 // atomic for thread-safety
	Status        OrderStatus
	Timestamp     int64
	statusMu      sync.Mutex
}

type Trade struct {
	TradeID     string
	Price       int64
	Quantity    int64
	Timestamp   int64
	BuyOrderID  string
	SellOrderID string
}

type PriceLevel struct {
	Price  int64
	Orders []*Order // fifo ordering for time priority
}

func NewOrder(id, symbol string, side OrderSide, orderType OrderType, price, quantity int64) *Order {
	return &Order{
		ID:            id,
		Symbol:        symbol,
		Side:          side,
		Type:          orderType,
		Price:         price,
		Quantity:      quantity,
		FilledQuantity: 0,
		Status:        StatusAccepted,
		Timestamp:     time.Now().UnixMilli(),
	}
}

func (o *Order) GetFilledQuantity() int64 {
	return atomic.LoadInt64(&o.FilledQuantity)
}

func (o *Order) RemainingQuantity() int64 {
	filled := atomic.LoadInt64(&o.FilledQuantity)
	return o.Quantity - filled
}

func (o *Order) IsFilled() bool {
	filled := atomic.LoadInt64(&o.FilledQuantity)
	return filled >= o.Quantity
}

func (o *Order) Fill(quantity int64) {
	newFilled := atomic.AddInt64(&o.FilledQuantity, quantity)
	
	o.statusMu.Lock()
	if newFilled >= o.Quantity {
		o.Status = StatusFilled
	} else {
		o.Status = StatusPartialFill
	}
	o.statusMu.Unlock()
}

func (o *Order) GetStatus() OrderStatus {
	o.statusMu.Lock()
	defer o.statusMu.Unlock()
	return o.Status
}

func (o *Order) SetStatus(status OrderStatus) {
	o.statusMu.Lock()
	defer o.statusMu.Unlock()
	o.Status = status
}

