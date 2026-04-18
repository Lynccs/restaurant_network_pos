package waiterservice

import (
	"errors"
	"log"
	"sync"

	waiterrepo "restaurant_network_pos/internal/repository/waiter"
)

var cartLog = log.New(log.Writer(), "[CartManager] ", log.LstdFlags|log.Lshortfile)

var ErrInsufficientIngredients = errors.New("недостатньо інгредієнтів")

// CartEntry holds a single dish line in a table's cart.
type CartEntry struct {
	DishID   int
	DishName string
	Price    float64
	Qty      int
}

type cart struct {
	items       map[int]*CartEntry // dishID → entry
	insertOrder []int              // dishIDs in insertion order
}

func newCart() *cart {
	return &cart{items: make(map[int]*CartEntry)}
}

// MenuRepo is the subset of waiterrepo.MenuRepo methods CartManager needs.
type MenuRepo interface {
	GetMenuWithPortions(restaurantID int) ([]waiterrepo.DishRow, error)
	GetDishesLite() ([]waiterrepo.DishLiteRow, error)
	GetTableID(restaurantID, tableNumber int) (int, error)
	CreateOrder(tableID, waiterID int, items []waiterrepo.CartEntryForOrder) (string, error)
}

// CartManager is a thread-safe in-memory store for per-table carts and a
// per-restaurant soft-reservation cache of available portions.
type CartManager struct {
	mu             sync.RWMutex
	carts          map[int]*cart       // tableID → cart
	inventoryCache map[int]map[int]int // restID → dishID → available portions
	isWarmedUp     map[int]bool        // restID → cache ready?
	menuRepo       MenuRepo
}

func NewCartManager(repo MenuRepo) *CartManager {
	return &CartManager{
		carts:          make(map[int]*cart),
		inventoryCache: make(map[int]map[int]int),
		isWarmedUp:     make(map[int]bool),
		menuRepo:       repo,
	}
}

// WarmUpCache loads portion counts from the DB into the inventory cache.
// Safe to call concurrently; subsequent calls for an already-warmed restaurant
// are no-ops. Intended to be called as `go cartMgr.WarmUpCache(restID)` from
// the tables page so the menu page loads without the heavy SQL delay.
func (m *CartManager) WarmUpCache(restaurantID int) error {
	m.mu.RLock()
	warmed := m.isWarmedUp[restaurantID]
	m.mu.RUnlock()
	if warmed {
		return nil
	}

	cartLog.Printf("WarmUpCache: restaurantID=%d", restaurantID)
	rows, err := m.menuRepo.GetMenuWithPortions(restaurantID)
	if err != nil {
		cartLog.Printf("WarmUpCache: DB error: %v", err)
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Double-check after acquiring write lock.
	if m.isWarmedUp[restaurantID] {
		return nil
	}
	cache := make(map[int]int, len(rows))
	for _, r := range rows {
		cache[r.ID] = r.Portions
	}
	m.inventoryCache[restaurantID] = cache
	m.isWarmedUp[restaurantID] = true
	cartLog.Printf("WarmUpCache: cached %d dishes for restID=%d", len(rows), restaurantID)
	return nil
}

// GetMenuForPage returns all dishes with current soft-reservation-aware portion
// counts and per-table cart quantities. Triggers a synchronous WarmUpCache if
// the cache is not yet ready.
func (m *CartManager) GetMenuForPage(restaurantID, tableID int) ([]DishView, error) {
	if err := m.WarmUpCache(restaurantID); err != nil {
		return nil, err
	}

	lite, err := m.menuRepo.GetDishesLite()
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	cache := m.inventoryCache[restaurantID]
	c := m.carts[tableID]

	views := make([]DishView, 0, len(lite))
	for _, d := range lite {
		portions := 0
		if cache != nil {
			portions = cache[d.ID]
		}
		cartQty := 0
		if c != nil {
			if e := c.items[d.ID]; e != nil {
				cartQty = e.Qty
			}
		}
		views = append(views, DishView{
			ID:          d.ID,
			Name:        d.Name,
			Price:       d.Price,
			PortionSize: d.PortionSize,
			CookingTime: d.CookingTime,
			Category:    d.CategoryName,
			Portions:    portions,
			Stopped:     portions == 0,
			CartQty:     cartQty,
		})
	}
	return views, nil
}

// AddItem decrements one portion from the cache and increments the table's cart.
// Returns ErrInsufficientIngredients if no portions are available.
func (m *CartManager) AddItem(restaurantID, tableID, dishID int, dishName string, price float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cache := m.inventoryCache[restaurantID]
	if cache == nil || cache[dishID] == 0 {
		return ErrInsufficientIngredients
	}

	cache[dishID]--

	if m.carts[tableID] == nil {
		m.carts[tableID] = newCart()
	}
	c := m.carts[tableID]
	if c.items[dishID] == nil {
		c.items[dishID] = &CartEntry{DishID: dishID, DishName: dishName, Price: price}
		c.insertOrder = append(c.insertOrder, dishID)
	}
	c.items[dishID].Qty++
	return nil
}

// RemoveItem decrements quantity from the cart and returns the portion to the cache.
func (m *CartManager) RemoveItem(restaurantID, tableID, dishID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c := m.carts[tableID]
	if c == nil {
		return nil
	}
	e := c.items[dishID]
	if e == nil || e.Qty == 0 {
		return nil
	}
	e.Qty--
	if m.inventoryCache[restaurantID] != nil {
		m.inventoryCache[restaurantID][dishID]++
	}
	if e.Qty == 0 {
		delete(c.items, dishID)
		for i, id := range c.insertOrder {
			if id == dishID {
				c.insertOrder = append(c.insertOrder[:i], c.insertOrder[i+1:]...)
				break
			}
		}
	}
	return nil
}

// GetPortions returns the current soft-reservation-aware portion count for a dish.
func (m *CartManager) GetPortions(restaurantID, dishID int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.inventoryCache[restaurantID] == nil {
		return 0
	}
	return m.inventoryCache[restaurantID][dishID]
}

// DestroyCart clears the table's cart and returns all soft-reserved portions to
// the cache. Called when the waiter navigates away from the menu without submitting.
func (m *CartManager) DestroyCart(restaurantID, tableID int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	c := m.carts[tableID]
	if c == nil {
		return
	}
	cache := m.inventoryCache[restaurantID]
	for dishID, e := range c.items {
		if cache != nil {
			cache[dishID] += e.Qty
		}
	}
	delete(m.carts, tableID)
	cartLog.Printf("DestroyCart: tableID=%d restID=%d", tableID, restaurantID)
}

// SubmitOrder persists the cart to the DB, clears the cart, but intentionally
// does NOT return portions to the cache (the stock is consumed by the kitchen).
func (m *CartManager) SubmitOrder(restaurantID, tableID, waiterID int) (string, error) {
	m.mu.RLock()
	c := m.carts[tableID]
	var dbItems []waiterrepo.CartEntryForOrder
	if c != nil {
		for _, e := range c.items {
			dbItems = append(dbItems, waiterrepo.CartEntryForOrder{
				DishID: e.DishID,
				Qty:    e.Qty,
				Price:  e.Price,
			})
		}
	}
	m.mu.RUnlock()

	orderNumber, err := m.menuRepo.CreateOrder(tableID, waiterID, dbItems)
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	delete(m.carts, tableID)
	m.mu.Unlock()

	cartLog.Printf("SubmitOrder: %s tableID=%d", orderNumber, tableID)
	return orderNumber, nil
}

// BuildCartViews returns a slice of CartItemView for template rendering plus the
// total order amount.
func (m *CartManager) BuildCartViews(restaurantID, tableID int) ([]CartItemView, float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c := m.carts[tableID]
	if c == nil {
		return nil, 0
	}
	cache := m.inventoryCache[restaurantID]

	var views []CartItemView
	var total float64
	for i := len(c.insertOrder) - 1; i >= 0; i-- {
		dishID := c.insertOrder[i]
		e := c.items[dishID]
		if e == nil || e.Qty == 0 {
			continue
		}
		canPlus := cache != nil && cache[dishID] > 0
		subtotal := e.Price * float64(e.Qty)
		total += subtotal
		views = append(views, CartItemView{
			DishID:   dishID,
			Name:     e.DishName,
			Price:    e.Price,
			Qty:      e.Qty,
			Subtotal: subtotal,
			CanPlus:  canPlus,
		})
	}
	return views, total
}

// InvalidateCache resets the warm-up state for a restaurant so the next
// GetMenuForPage triggers a fresh DB load. Called by the Admin module when
// stock levels change.
func (m *CartManager) InvalidateCache(restaurantID int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.inventoryCache, restaurantID)
	m.isWarmedUp[restaurantID] = false
	cartLog.Printf("InvalidateCache: restID=%d", restaurantID)
}
