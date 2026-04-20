package waiterhandler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	waiterservice "restaurant_network_pos/internal/service/waiter"
	menupages "restaurant_network_pos/templates/pages/waiter"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
)

var menuLog = log.New(log.Writer(), "[MenuHandler] ", log.LstdFlags|log.Lshortfile)

// MenuCartManager is the CartManager interface used by MenuHandler.
type MenuCartManager interface {
	WarmUpCache(restaurantID int) error
	GetMenuForPage(restaurantID, tableID int) ([]waiterservice.DishView, error)
	LoadActiveOrder(restaurantID, tableID int) error
	AddItem(restaurantID, tableID, dishID int, name string, price float64) error
	RemoveItem(restaurantID, tableID, dishID int) error
	RemoveDBItem(restaurantID, tableID, orderItemID int) error
	UnCancelDBItem(restaurantID, tableID, orderItemID int) error
	GetPortions(restaurantID, dishID int) int
	DestroyCart(restaurantID, tableID int)
	SubmitOrder(restaurantID, tableID, waiterID int) (string, error)
	GetFullCartView(restaurantID, tableID int) ([]waiterservice.CartItemView, float64)
	HasExistingOrder(tableID int) bool
}

// MenuRepoForHandler exposes only the table-lookup needed by the handler.
type MenuRepoForHandler interface {
	GetTableID(restaurantID, tableNumber int) (int, error)
}

// MenuHandler handles all POS/menu endpoints.
type MenuHandler struct {
	CartMgr  MenuCartManager
	MenuRepo MenuRepoForHandler
	Store    sessions.Store
}

func NewMenuHandler(cartMgr MenuCartManager, repo MenuRepoForHandler, store sessions.Store) *MenuHandler {
	return &MenuHandler{CartMgr: cartMgr, MenuRepo: repo, Store: store}
}

// sessionInts extracts restaurantID and waiterID from the session.
func (h *MenuHandler) sessionInts(r *http.Request) (restaurantID, waiterID int, err error) {
	sess, err := h.Store.Get(r, "session")
	if err != nil {
		return 0, 0, err
	}
	restaurantID, _ = sess.Values["restaurant_id"].(int)
	waiterID, _ = sess.Values["user_id"].(int)
	return restaurantID, waiterID, nil
}

// tableNumber parses {number} from the URL.
func tableNumber(r *http.Request) (int, error) {
	return strconv.Atoi(chi.URLParam(r, "number"))
}

// MenuPage renders the full POS page (GET /waiter/tables/{number}/menu).
func (h *MenuHandler) MenuPage(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, err := h.sessionInts(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	tNum, err := tableNumber(r)
	if err != nil {
		http.Error(w, "invalid table", http.StatusBadRequest)
		return
	}

	tableID, err := h.MenuRepo.GetTableID(restaurantID, tNum)
	if err != nil {
		menuLog.Printf("MenuPage: GetTableID error: %v", err)
		http.Error(w, "table not found", http.StatusNotFound)
		return
	}

	// Load existing DB order into draft (no-op for free tables).
	if err := h.CartMgr.LoadActiveOrder(restaurantID, tableID); err != nil {
		menuLog.Printf("MenuPage: LoadActiveOrder error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	dishes, err := h.CartMgr.GetMenuForPage(restaurantID, tableID)
	if err != nil {
		menuLog.Printf("MenuPage: GetMenuForPage error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	cart, total := h.CartMgr.GetFullCartView(restaurantID, tableID)
	hasExistingOrder := h.CartMgr.HasExistingOrder(tableID)

	activeCat := r.URL.Query().Get("cat")
	cats := categories(dishes)
	if activeCat == "" && len(cats) > 0 {
		activeCat = cats[0]
	}

	filtered := filterByCategory(dishes, activeCat)
	menupages.MenuPage(tNum, cats, activeCat, filtered, cart, total, hasExistingOrder).Render(r.Context(), w)
}

// GetDishes returns the dish grid fragment for HTMX category switching
// (GET /waiter/tables/{number}/menu/dishes?cat=X).
func (h *MenuHandler) GetDishes(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, err := h.sessionInts(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	tNum, err := tableNumber(r)
	if err != nil {
		http.Error(w, "invalid table", http.StatusBadRequest)
		return
	}

	tableID, err := h.MenuRepo.GetTableID(restaurantID, tNum)
	if err != nil {
		http.Error(w, "table not found", http.StatusNotFound)
		return
	}

	dishes, err := h.CartMgr.GetMenuForPage(restaurantID, tableID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	cat := r.URL.Query().Get("cat")
	filtered := filterByCategory(dishes, cat)
	menupages.DishGrid(tNum, cat, filtered).Render(r.Context(), w)
}

// CartAdd adds a dish to the cart (POST /waiter/tables/{number}/menu/cart/add).
func (h *MenuHandler) CartAdd(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, err := h.sessionInts(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	tNum, err := tableNumber(r)
	if err != nil {
		http.Error(w, "invalid table", http.StatusBadRequest)
		return
	}

	tableID, err := h.MenuRepo.GetTableID(restaurantID, tNum)
	if err != nil {
		http.Error(w, "table not found", http.StatusNotFound)
		return
	}

	dishID, _ := strconv.Atoi(r.FormValue("dish_id"))
	dishName := r.FormValue("dish_name")
	price, _ := strconv.ParseFloat(r.FormValue("price"), 64)

	addErr := h.CartMgr.AddItem(restaurantID, tableID, dishID, dishName, price)
	if errors.Is(addErr, waiterservice.ErrInsufficientIngredients) {
		w.WriteHeader(http.StatusConflict)
		menupages.CartError("Недостатньо інгредієнтів на складі").Render(r.Context(), w)
		return
	}
	if addErr != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	cart, total := h.CartMgr.GetFullCartView(restaurantID, tableID)
	hasExistingOrder := h.CartMgr.HasExistingOrder(tableID)
	menupages.CartPanel(tNum, cart, total, hasExistingOrder).Render(r.Context(), w)

	portionsLeft := h.CartMgr.GetPortions(restaurantID, dishID)
	if portionsLeft == 0 {
		if dishes, err := h.CartMgr.GetMenuForPage(restaurantID, tableID); err == nil {
			for _, d := range dishes {
				if d.ID == dishID {
					menupages.DishCardOOB(tNum, d).Render(r.Context(), w)
					break
				}
			}
		}
	} else {
		menupages.DishBadgeOOB(dishID, activeBadgeQty(cart, dishID)).Render(r.Context(), w)
	}
}

// CartRemove removes one unit from the cart (POST /waiter/tables/{number}/menu/cart/remove).
// Accepts optional order_item_id for DB draft items; falls back to in-memory removal.
func (h *MenuHandler) CartRemove(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, err := h.sessionInts(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	tNum, err := tableNumber(r)
	if err != nil {
		http.Error(w, "invalid table", http.StatusBadRequest)
		return
	}

	tableID, err := h.MenuRepo.GetTableID(restaurantID, tNum)
	if err != nil {
		http.Error(w, "table not found", http.StatusNotFound)
		return
	}

	orderItemID, _ := strconv.Atoi(r.FormValue("order_item_id"))
	dishID, _ := strconv.Atoi(r.FormValue("dish_id"))

	portionsBefore := h.CartMgr.GetPortions(restaurantID, dishID)

	if orderItemID > 0 {
		h.CartMgr.RemoveDBItem(restaurantID, tableID, orderItemID) //nolint:errcheck
	} else {
		h.CartMgr.RemoveItem(restaurantID, tableID, dishID) //nolint:errcheck
	}

	portionsAfter := h.CartMgr.GetPortions(restaurantID, dishID)

	cart, total := h.CartMgr.GetFullCartView(restaurantID, tableID)
	hasExistingOrder := h.CartMgr.HasExistingOrder(tableID)
	menupages.CartPanel(tNum, cart, total, hasExistingOrder).Render(r.Context(), w)

	if portionsBefore == 0 && portionsAfter > 0 {
		if dishes, err := h.CartMgr.GetMenuForPage(restaurantID, tableID); err == nil {
			for _, d := range dishes {
				if d.ID == dishID {
					menupages.DishCardOOB(tNum, d).Render(r.Context(), w)
					break
				}
			}
		}
	} else {
		menupages.DishBadgeOOB(dishID, activeBadgeQty(cart, dishID)).Render(r.Context(), w)
	}
}

// CartUnCancel restores one cancelled portion for a "cooking" item (POST /waiter/tables/{number}/menu/cart/uncancel).
func (h *MenuHandler) CartUnCancel(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, err := h.sessionInts(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	tNum, err := tableNumber(r)
	if err != nil {
		http.Error(w, "invalid table", http.StatusBadRequest)
		return
	}

	tableID, err := h.MenuRepo.GetTableID(restaurantID, tNum)
	if err != nil {
		http.Error(w, "table not found", http.StatusNotFound)
		return
	}

	orderItemID, _ := strconv.Atoi(r.FormValue("order_item_id"))
	if orderItemID > 0 {
		h.CartMgr.UnCancelDBItem(restaurantID, tableID, orderItemID) //nolint:errcheck
	}

	cart, total := h.CartMgr.GetFullCartView(restaurantID, tableID)
	hasExistingOrder := h.CartMgr.HasExistingOrder(tableID)
	menupages.CartPanel(tNum, cart, total, hasExistingOrder).Render(r.Context(), w)
}

// CartDestroy releases all soft reservations (DELETE /waiter/tables/{number}/menu/cart).
func (h *MenuHandler) CartDestroy(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, err := h.sessionInts(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	tNum, err := tableNumber(r)
	if err != nil {
		http.Error(w, "invalid table", http.StatusBadRequest)
		return
	}

	tableID, err := h.MenuRepo.GetTableID(restaurantID, tNum)
	if err != nil {
		http.Error(w, "table not found", http.StatusNotFound)
		return
	}

	h.CartMgr.DestroyCart(restaurantID, tableID)
	w.WriteHeader(http.StatusOK)
}

// SubmitOrder commits the cart to the database (POST /waiter/tables/{number}/menu/submit).
// For occupied tables uses SyncOrderDraft; for free tables creates a new order.
func (h *MenuHandler) SubmitOrder(w http.ResponseWriter, r *http.Request) {
	restaurantID, waiterID, err := h.sessionInts(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	tNum, err := tableNumber(r)
	if err != nil {
		http.Error(w, "invalid table", http.StatusBadRequest)
		return
	}

	tableID, err := h.MenuRepo.GetTableID(restaurantID, tNum)
	if err != nil {
		http.Error(w, "table not found", http.StatusNotFound)
		return
	}

	_, err = h.CartMgr.SubmitOrder(restaurantID, tableID, waiterID)
	if err != nil {
		menuLog.Printf("SubmitOrder: error: %v", err)
		http.Error(w, "failed to submit order", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/waiter/tables")
	w.WriteHeader(http.StatusOK)
}

// helpers

func categories(dishes []waiterservice.DishView) []string {
	seen := make(map[string]bool)
	var cats []string
	for _, d := range dishes {
		if !seen[d.Category] {
			seen[d.Category] = true
			cats = append(cats, d.Category)
		}
	}
	return cats
}

func filterByCategory(dishes []waiterservice.DishView, cat string) []waiterservice.DishView {
	if cat == "" {
		return dishes
	}
	var out []waiterservice.DishView
	for _, d := range dishes {
		if d.Category == cat {
			out = append(out, d)
		}
	}
	return out
}

// activeBadgeQty sums the Qty of all "new" or in-memory items for a dish.
// "cooking" and "done" items are excluded from the badge count.
func activeBadgeQty(cart []waiterservice.CartItemView, dishID int) int {
	qty := 0
	for _, item := range cart {
		if item.DishID == dishID && (item.Status == "new" || item.Status == "") {
			qty += item.Qty
		}
	}
	return qty
}
