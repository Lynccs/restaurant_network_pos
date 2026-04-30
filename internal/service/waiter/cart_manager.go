package waiterservice

import (
	"errors"
	"log"
	"sync"

	waiterrepo "restaurant_network_pos/internal/repository/waiter"
)

var cartLog = log.New(log.Writer(), "[CartManager] ", log.LstdFlags|log.Lshortfile)

var ErrInsufficientIngredients = errors.New("недостатньо інгредієнтів")

// CartEntry holds a single new (in-memory) dish line in a table's cart.
type CartEntry struct {
	DishID   int
	DishName string
	Price    float64
	Qty      int
}

// dbDraftEntry mirrors one existing order_item from the DB, tracking in-memory changes.
type dbDraftEntry struct {
	OrderItemID int
	DishID      int
	DishName    string
	Price       float64
	OrigQty     int    // quantity fetched from DB at LoadActiveOrder time
	DraftQty    int    // current in-memory qty (0 = delete at Submit)
	CancelDelta int    // additional cancellations to apply at Submit (for "cooking" items)
	Status      string // "new" | "cooking" | "done" — set from DB, never from frontend
	HasIssue    bool
}

type cart struct {
	// In-memory new dishes added this session
	items       map[int]*CartEntry
	insertOrder []int

	// DB-draft layer (loaded once from DB on MenuPage open)
	existingOrderID int
	dbDrafts        map[int]*dbDraftEntry // orderItemID → draft
	dbDraftOrder    []int                 // orderItemIDs in load order (stable render order)
	dbDraftByDish   map[int]int           // dishID → orderItemID (for AddItem lookup)
}

func newCart() *cart {
	return &cart{
		items:         make(map[int]*CartEntry),
		dbDrafts:      make(map[int]*dbDraftEntry),
		dbDraftByDish: make(map[int]int),
	}
}

// MenuRepo is the subset of waiterrepo.MenuRepo methods CartManager needs.
type MenuRepo interface {
	GetMenuWithPortions(restaurantID int) ([]waiterrepo.DishRow, error)
	GetDishesLite() ([]waiterrepo.DishLiteRow, error)
	GetTableID(restaurantID, tableNumber int) (int, error)
	CreateOrder(tableID, waiterID int, items []waiterrepo.CartEntryForOrder) (string, error)
	GetActiveOrderItems(tableID int) ([]waiterrepo.ActiveOrderItemRow, error)
	SyncOrderDraft(params waiterrepo.SyncDraftParams) error
}

// CartManager is a thread-safe in-memory store for per-table carts, per-restaurant
// soft-reservation cache, and Full-Draft tracking of changes to existing DB orders.
type CartManager struct {
	mu             sync.RWMutex
	carts          map[int]*cart       // tableID → cart
	inventoryCache map[int]map[int]int // restID → dishID → available portions
	isWarmedUp     map[int]bool        // restID → cache ready?
	menuRepo       MenuRepo

	// loadMu + loadInflight implement a per-table singleflight for LoadActiveOrder:
	// only one goroutine runs the DB query; concurrent callers wait on the channel.
	loadMu      sync.Mutex
	loadInflight map[int]chan struct{} // tableID → channel closed when load completes
}

func NewCartManager(repo MenuRepo) *CartManager {
	return &CartManager{
		carts:          make(map[int]*cart),
		inventoryCache: make(map[int]map[int]int),
		isWarmedUp:     make(map[int]bool),
		menuRepo:       repo,
		loadInflight:   make(map[int]chan struct{}),
	}
}

// WarmUpCache loads portion counts from the DB into the inventory cache.
// Safe to call concurrently; subsequent calls for an already-warmed restaurant
// are no-ops.
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

// GetMenuForPage returns all dishes with soft-reservation-aware portion counts
// and per-table cart quantities (combining DB draft "new" items and in-memory items).
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
		// Badge qty = DB "new" draft qty + in-memory qty (cooking/done not shown on badge)
		cartQty := 0
		if c != nil {
			if oid, ok := c.dbDraftByDish[d.ID]; ok {
				if draft := c.dbDrafts[oid]; draft != nil && draft.Status == "new" && draft.DraftQty > 0 {
					cartQty += draft.DraftQty
				}
			}
			if e := c.items[d.ID]; e != nil {
				cartQty += e.Qty
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

// LoadActiveOrder fetches the open order for a table from the DB and stores it as
// an in-memory draft. No-op if already loaded or if the table has no active order.
//
// Concurrent callers for the same tableID are deduplicated: only the first goroutine
// fires the DB query; all others block until it finishes, then return immediately.
func (m *CartManager) LoadActiveOrder(restaurantID, tableID int) error {
	// Fast path: already loaded under read lock.
	m.mu.RLock()
	alreadyLoaded := m.carts[tableID] != nil && m.carts[tableID].existingOrderID > 0
	m.mu.RUnlock()
	if alreadyLoaded {
		return nil
	}

	// Inflight deduplication: if another goroutine is already loading this table,
	// wait for it to finish and return without making a second DB query.
	m.loadMu.Lock()
	if ch, ok := m.loadInflight[tableID]; ok {
		m.loadMu.Unlock()
		cartLog.Printf("LoadActiveOrder: tableID=%d waiting for inflight load", tableID)
		<-ch // block until the active load completes
		return nil
	}
	done := make(chan struct{})
	m.loadInflight[tableID] = done
	m.loadMu.Unlock()

	// Guarantee the channel is closed and inflight entry removed on return.
	defer func() {
		m.loadMu.Lock()
		delete(m.loadInflight, tableID)
		m.loadMu.Unlock()
		close(done)
	}()

	items, err := m.menuRepo.GetActiveOrderItems(tableID)
	if err != nil {
		cartLog.Printf("LoadActiveOrder: tableID=%d error: %v", tableID, err)
		return err
	}
	if len(items) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Double-check: another goroutine may have finished between our query and lock.
	if m.carts[tableID] != nil && m.carts[tableID].existingOrderID > 0 {
		return nil
	}

	if m.carts[tableID] == nil {
		m.carts[tableID] = newCart()
	}
	c := m.carts[tableID]
	c.existingOrderID = items[0].OrderID

	for _, item := range items {
		c.dbDrafts[item.OrderItemID] = &dbDraftEntry{
			OrderItemID: item.OrderItemID,
			DishID:      item.DishID,
			DishName:    item.DishName,
			Price:       item.Price,
			OrigQty:     item.Qty,
			DraftQty:    item.Qty,
			CancelDelta: 0,
			Status:      item.Status,
			HasIssue:    item.HasIssue,
		}
		c.dbDraftOrder = append(c.dbDraftOrder, item.OrderItemID)
		c.dbDraftByDish[item.DishID] = item.OrderItemID
	}
	cartLog.Printf("LoadActiveOrder: tableID=%d orderID=%d items=%d", tableID, c.existingOrderID, len(items))
	return nil
}

// AddItem adds one portion to the table's cart. For dishes that already exist
// in the DB order with "new" status, the DB draft is incremented instead of
// creating a new in-memory entry. "cooking"/"done" DB items fall through to a
// new separate in-memory entry. Returns ErrInsufficientIngredients if no stock.
func (m *CartManager) AddItem(restaurantID, tableID, dishID int, dishName string, price float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cache := m.inventoryCache[restaurantID]

	// Check if dish exists in DB draft as "new" status → increment draft.
	if m.carts[tableID] != nil {
		if oid, ok := m.carts[tableID].dbDraftByDish[dishID]; ok {
			if d := m.carts[tableID].dbDrafts[oid]; d != nil && d.Status == "new" {
				if cache == nil || cache[dishID] == 0 {
					return ErrInsufficientIngredients
				}
				cache[dishID]--
				d.DraftQty++
				return nil
			}
			// "cooking" or "done": fall through to new in-memory entry
		}
	}

	// Normal in-memory add.
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

// RemoveItem decrements an in-memory (new session) cart entry and returns the
// portion to the cache.
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

// RemoveDBItem modifies the in-memory draft for an existing DB order item:
//   - "new": decrements DraftQty and returns the portion to cache.
//   - "cooking": increments CancelDelta (no cache change — kitchen already uses it).
//   - "done": no-op (UI buttons are disabled for done items).
func (m *CartManager) RemoveDBItem(restaurantID, tableID, orderItemID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c := m.carts[tableID]
	if c == nil {
		return nil
	}
	d := c.dbDrafts[orderItemID]
	if d == nil {
		return nil
	}

	switch d.Status {
	case "new":
		if d.DraftQty <= 0 {
			return nil
		}
		d.DraftQty--
		if m.inventoryCache[restaurantID] != nil {
			m.inventoryCache[restaurantID][d.DishID]++
		}
	case "cooking":
		if d.CancelDelta < d.OrigQty {
			d.CancelDelta++
		}
	}
	return nil
}

// UnCancelDBItem decrements CancelDelta for a "cooking" item, restoring one previously
// cancelled portion back to the effective quantity (session-only, not persisted until Submit).
func (m *CartManager) UnCancelDBItem(restaurantID, tableID, orderItemID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c := m.carts[tableID]
	if c == nil {
		return nil
	}
	d := c.dbDrafts[orderItemID]
	if d == nil || d.Status != "cooking" || d.CancelDelta <= 0 {
		return nil
	}
	d.CancelDelta--
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

// HasExistingOrder reports whether the table currently has an active DB order loaded.
func (m *CartManager) HasExistingOrder(tableID int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.carts[tableID] != nil && m.carts[tableID].existingOrderID > 0
}

// DestroyCart discards the entire draft for a table (Back button).
// Returns all soft-reserved portions to the cache; does NOT touch the DB.
func (m *CartManager) DestroyCart(restaurantID, tableID int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	c := m.carts[tableID]
	if c == nil {
		return
	}
	cache := m.inventoryCache[restaurantID]

	// Return in-memory item portions.
	for dishID, e := range c.items {
		if cache != nil {
			cache[dishID] += e.Qty
		}
	}

	// Reverse net cache changes made to DB "new" drafts this session.
	if cache != nil {
		for _, d := range c.dbDrafts {
			if d.Status == "new" {
				// netConsumed = DraftQty - OrigQty
				// positive → we consumed from cache → return it (cache grows)
				// negative → we returned to cache → take it back (cache shrinks)
				netConsumed := d.DraftQty - d.OrigQty
				cache[d.DishID] += netConsumed
			}
			// "cooking"/"done": CancelDelta has no cache effect; just discard.
		}
	}

	delete(m.carts, tableID)
	cartLog.Printf("DestroyCart: tableID=%d restID=%d", tableID, restaurantID)
}

// SubmitOrder persists the full draft to the DB in one shot.
// For new tables: calls CreateOrder. For occupied tables: calls SyncOrderDraft.
// Clears the cart on success.
func (m *CartManager) SubmitOrder(restaurantID, tableID, waiterID int) (string, error) {
	m.mu.RLock()
	c := m.carts[tableID]

	var existingOrderID int
	var dbChanges []waiterrepo.DBItemChange
	var memItems []waiterrepo.CartEntryForOrder

	if c != nil {
		existingOrderID = c.existingOrderID

		for _, d := range c.dbDrafts {
			if d.DraftQty != d.OrigQty || d.CancelDelta > 0 {
				dbChanges = append(dbChanges, waiterrepo.DBItemChange{
					OrderItemID: d.OrderItemID,
					OrigQty:     d.OrigQty,
					DraftQty:    d.DraftQty,
					CancelDelta: d.CancelDelta,
				})
			}
		}
		for _, e := range c.items {
			if e.Qty > 0 {
				memItems = append(memItems, waiterrepo.CartEntryForOrder{
					DishID: e.DishID,
					Qty:    e.Qty,
					Price:  e.Price,
				})
			}
		}
	}
	m.mu.RUnlock()

	var orderNumber string
	var err error

	if existingOrderID > 0 {
		err = m.menuRepo.SyncOrderDraft(waiterrepo.SyncDraftParams{
			ExistingOrderID: existingOrderID,
			NewItems:        memItems,
			DBItemChanges:   dbChanges,
		})
	} else {
		orderNumber, err = m.menuRepo.CreateOrder(tableID, waiterID, memItems)
	}
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	delete(m.carts, tableID)
	m.mu.Unlock()

	cartLog.Printf("SubmitOrder: tableID=%d existingOrderID=%d", tableID, existingOrderID)
	return orderNumber, nil
}

// GetFullCartView builds the combined receipt view: DB draft items (in load order)
// followed by new in-memory items (LIFO). Replaces the old BuildCartViews.
func (m *CartManager) GetFullCartView(restaurantID, tableID int) ([]CartItemView, float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c := m.carts[tableID]
	if c == nil {
		return nil, 0
	}
	cache := m.inventoryCache[restaurantID]

	var views []CartItemView
	var total float64

	// DB draft items in stable (ascending) order.
	for _, oid := range c.dbDraftOrder {
		d := c.dbDrafts[oid]
		if d == nil {
			continue
		}
		switch d.Status {
		case "new":
			if d.DraftQty <= 0 {
				continue // marked for deletion; hidden until Submit
			}
			canPlus := cache != nil && cache[d.DishID] > 0
			sub := d.Price * float64(d.DraftQty)
			total += sub
			views = append(views, CartItemView{
				DishID:      d.DishID,
				OrderItemID: d.OrderItemID,
				Name:        d.DishName,
				Price:       d.Price,
				Qty:         d.DraftQty,
				Subtotal:    sub,
				CanPlus:     canPlus,
				Status:      "new",
				IsFromDB:    true,
				HasIssue:    d.HasIssue,
			})
		case "cooking":
			effectiveQty := d.OrigQty - d.CancelDelta
			if effectiveQty < 0 {
				effectiveQty = 0
			}
			sub := d.Price * float64(effectiveQty)
			total += sub
			views = append(views, CartItemView{
				DishID:       d.DishID,
				OrderItemID:  d.OrderItemID,
				Name:         d.DishName,
				Price:        d.Price,
				Qty:          effectiveQty,
				CancelledQty: d.CancelDelta,
				Subtotal:     sub,
				CanPlus:      false,
				Status:       "cooking",
				IsFromDB:     true,
				HasIssue:     d.HasIssue,
			})
		case "done":
			sub := d.Price * float64(d.OrigQty)
			total += sub
			views = append(views, CartItemView{
				DishID:      d.DishID,
				OrderItemID: d.OrderItemID,
				Name:        d.DishName,
				Price:       d.Price,
				Qty:         d.OrigQty,
				Subtotal:    sub,
				CanPlus:     false,
				Status:      "done",
				IsFromDB:    true,
				HasIssue:    d.HasIssue,
			})
		}
	}

	// In-memory new items (LIFO display).
	for i := len(c.insertOrder) - 1; i >= 0; i-- {
		dishID := c.insertOrder[i]
		e := c.items[dishID]
		if e == nil || e.Qty == 0 {
			continue
		}
		canPlus := cache != nil && cache[dishID] > 0
		sub := e.Price * float64(e.Qty)
		total += sub
		views = append(views, CartItemView{
			DishID:   dishID,
			Name:     e.DishName,
			Price:    e.Price,
			Qty:      e.Qty,
			Subtotal: sub,
			CanPlus:  canPlus,
			Status:   "",
		})
	}

	return views, total
}

// InvalidateCache resets the warm-up state for a restaurant.
func (m *CartManager) InvalidateCache(restaurantID int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.inventoryCache, restaurantID)
	m.isWarmedUp[restaurantID] = false
	cartLog.Printf("InvalidateCache: restID=%d", restaurantID)
}
