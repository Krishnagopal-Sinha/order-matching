package engine

import (
	"sync"

	"github.com/google/btree"
)

type PriceLevelItem struct {
	PriceLevel *PriceLevel
}

func (p *PriceLevelItem) Less(than btree.Item) bool {
	other := than.(*PriceLevelItem)
	return p.PriceLevel.Price > other.PriceLevel.Price
}

type PriceLevelItemAscending struct {
	PriceLevel *PriceLevel
}

func (p *PriceLevelItemAscending) Less(than btree.Item) bool {
	other := than.(*PriceLevelItemAscending)
	return p.PriceLevel.Price < other.PriceLevel.Price
}

type OrderBook struct {
	Symbol string
	Bids   *btree.BTree // sorted descending (highest first)
	Asks   *btree.BTree // sorted ascending (lowest first)
	Orders map[string]*Order
	mu     sync.RWMutex
}

func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		Symbol: symbol,
		Bids:   btree.New(32),
		Asks:   btree.New(32),
		Orders: make(map[string]*Order),
	}
}

func (ob *OrderBook) AddOrder(order *Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	ob.Orders[order.ID] = order

	var tree *btree.BTree
	var priceLevel *PriceLevel
	var item btree.Item

	if order.Side == SideBuy {
		tree = ob.Bids
		item = &PriceLevelItem{PriceLevel: &PriceLevel{Price: order.Price}}
	} else {
		tree = ob.Asks
		item = &PriceLevelItemAscending{PriceLevel: &PriceLevel{Price: order.Price}}
	}

	existing := tree.Get(item)
	if existing != nil {
		if order.Side == SideBuy {
			priceLevel = existing.(*PriceLevelItem).PriceLevel
		} else {
			priceLevel = existing.(*PriceLevelItemAscending).PriceLevel
		}
	} else {
		priceLevel = &PriceLevel{
			Price:  order.Price,
			Orders: make([]*Order, 0),
		}
		if order.Side == SideBuy {
			item = &PriceLevelItem{PriceLevel: priceLevel}
		} else {
			item = &PriceLevelItemAscending{PriceLevel: priceLevel}
		}
		tree.ReplaceOrInsert(item)
	}

	priceLevel.Orders = append(priceLevel.Orders, order)
}

func (ob *OrderBook) RemoveOrder(orderID string) bool {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	order, exists := ob.Orders[orderID]
	if !exists {
		return false
	}

	var tree *btree.BTree
	var item btree.Item

	if order.Side == SideBuy {
		tree = ob.Bids
		item = &PriceLevelItem{PriceLevel: &PriceLevel{Price: order.Price}}
	} else {
		tree = ob.Asks
		item = &PriceLevelItemAscending{PriceLevel: &PriceLevel{Price: order.Price}}
	}

	existing := tree.Get(item)
	if existing == nil {
		delete(ob.Orders, orderID)
		return false
	}

	var priceLevel *PriceLevel
	if order.Side == SideBuy {
		priceLevel = existing.(*PriceLevelItem).PriceLevel
	} else {
		priceLevel = existing.(*PriceLevelItemAscending).PriceLevel
	}

	for i, o := range priceLevel.Orders {
		if o.ID == orderID {
			priceLevel.Orders = append(priceLevel.Orders[:i], priceLevel.Orders[i+1:]...)
			break
		}
	}

	// edge case: remove empty price level
	if len(priceLevel.Orders) == 0 {
		tree.Delete(item)
	}

	delete(ob.Orders, orderID)
	return true
}

func (ob *OrderBook) GetBestBid() (price int64, quantity int64, ok bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	if ob.Bids.Len() == 0 {
		return 0, 0, false
	}

	item := ob.Bids.Min()
	if item == nil {
		return 0, 0, false
	}

	priceLevel := item.(*PriceLevelItem).PriceLevel
	if len(priceLevel.Orders) == 0 {
		return 0, 0, false
	}

	var totalQuantity int64
	for _, order := range priceLevel.Orders {
		totalQuantity += order.RemainingQuantity()
	}

	return priceLevel.Price, totalQuantity, true
}

func (ob *OrderBook) GetBestAsk() (price int64, quantity int64, ok bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	if ob.Asks.Len() == 0 {
		return 0, 0, false
	}

	item := ob.Asks.Min()
	if item == nil {
		return 0, 0, false
	}

	priceLevel := item.(*PriceLevelItemAscending).PriceLevel
	if len(priceLevel.Orders) == 0 {
		return 0, 0, false
	}

	var totalQuantity int64
	for _, order := range priceLevel.Orders {
		totalQuantity += order.RemainingQuantity()
	}

	return priceLevel.Price, totalQuantity, true
}

func (ob *OrderBook) GetOrder(orderID string) (*Order, bool) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	order, exists := ob.Orders[orderID]
	return order, exists
}

type OrderBookSnapshot struct {
	Price    int64
	Quantity int64
}

func (ob *OrderBook) GetOrderBookSnapshot(depth int) (bids []OrderBookSnapshot, asks []OrderBookSnapshot) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bids = make([]OrderBookSnapshot, 0, depth)
	asks = make([]OrderBookSnapshot, 0, depth)

	count := 0
	ob.Bids.Ascend(func(item btree.Item) bool {
		if count >= depth {
			return false
		}
		priceLevel := item.(*PriceLevelItem).PriceLevel
		var totalQuantity int64
		for _, order := range priceLevel.Orders {
			totalQuantity += order.RemainingQuantity()
		}
		bids = append(bids, OrderBookSnapshot{
			Price:    priceLevel.Price,
			Quantity: totalQuantity,
		})
		count++
		return true
	})

	count = 0
	ob.Asks.Ascend(func(item btree.Item) bool {
		if count >= depth {
			return false
		}
		priceLevel := item.(*PriceLevelItemAscending).PriceLevel
		var totalQuantity int64
		for _, order := range priceLevel.Orders {
			totalQuantity += order.RemainingQuantity()
		}
		asks = append(asks, OrderBookSnapshot{
			Price:    priceLevel.Price,
			Quantity: totalQuantity,
		})
		count++
		return true
	})

	return bids, asks
}

func (ob *OrderBook) GetPriceLevelForBid(price int64) *PriceLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	item := ob.Bids.Get(&PriceLevelItem{PriceLevel: &PriceLevel{Price: price}})
	if item == nil {
		return nil
	}
	return item.(*PriceLevelItem).PriceLevel
}

func (ob *OrderBook) GetPriceLevelForAsk(price int64) *PriceLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	item := ob.Asks.Get(&PriceLevelItemAscending{PriceLevel: &PriceLevel{Price: price}})
	if item == nil {
		return nil
	}
	return item.(*PriceLevelItemAscending).PriceLevel
}

