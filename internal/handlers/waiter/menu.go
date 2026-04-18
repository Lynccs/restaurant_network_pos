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
	AddItem(restaurantID, tableID, dishID int, name string, price float64) error
	RemoveItem(restaurantID, tableID, dishID int) error
	GetPortions(restaurantID, dishID int) int
	DestroyCart(restaurantID, tableID int)
	SubmitOrder(restaurantID, tableID, waiterID int) (string, error)
	BuildCartViews(restaurantID, tableID int) ([]waiterservice.CartItemView, float64)
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

	dishes, err := h.CartMgr.GetMenuForPage(restaurantID, tableID)
	if err != nil {
		menuLog.Printf("MenuPage: GetMenuForPage error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	cart, total := h.CartMgr.BuildCartViews(restaurantID, tableID)

	// Determine active category from query or first available.
	activeCat := r.URL.Query().Get("cat")
	cats := categories(dishes)
	if activeCat == "" && len(cats) > 0 {
		activeCat = cats[0]
	}

	filtered := filterByCategory(dishes, activeCat)
	menupages.MenuPage(tNum, cats, activeCat, filtered, cart, total).Render(r.Context(), w)
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
// Returns updated CartPanel HTML; appends an OOB DishCard if the dish just hit 0 portions.
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

	cart, total := h.CartMgr.BuildCartViews(restaurantID, tableID)
	menupages.CartPanel(tNum, cart, total).Render(r.Context(), w)

	portionsLeft := h.CartMgr.GetPortions(restaurantID, dishID)
	if portionsLeft == 0 {
		// Dish just hit stop-list — replace the whole card to show grey overlay.
		if dishes, err := h.CartMgr.GetMenuForPage(restaurantID, tableID); err == nil {
			for _, d := range dishes {
				if d.ID == dishID {
					menupages.DishCardOOB(tNum, d).Render(r.Context(), w)
					break
				}
			}
		}
	} else {
		// Normal add — update only the badge to avoid full card re-render (prevents flicker).
		var cartQty int
		for _, item := range cart {
			if item.DishID == dishID {
				cartQty = item.Qty
				break
			}
		}
		menupages.DishBadgeOOB(dishID, cartQty).Render(r.Context(), w)
	}
}

// CartRemove removes one unit from the cart (POST /waiter/tables/{number}/menu/cart/remove).
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

	dishID, _ := strconv.Atoi(r.FormValue("dish_id"))

	portionsBefore := h.CartMgr.GetPortions(restaurantID, dishID)
	h.CartMgr.RemoveItem(restaurantID, tableID, dishID) //nolint:errcheck
	portionsAfter := h.CartMgr.GetPortions(restaurantID, dishID)

	cart, total := h.CartMgr.BuildCartViews(restaurantID, tableID)
	menupages.CartPanel(tNum, cart, total).Render(r.Context(), w)

	if portionsBefore == 0 && portionsAfter > 0 {
		// Card was on stop-list and is now re-enabled — replace whole card.
		if dishes, err := h.CartMgr.GetMenuForPage(restaurantID, tableID); err == nil {
			for _, d := range dishes {
				if d.ID == dishID {
					menupages.DishCardOOB(tNum, d).Render(r.Context(), w)
					break
				}
			}
		}
	} else {
		// Normal remove — update only the badge.
		var cartQty int
		for _, item := range cart {
			if item.DishID == dishID {
				cartQty = item.Qty
				break
			}
		}
		menupages.DishBadgeOOB(dishID, cartQty).Render(r.Context(), w)
	}
}

// CartDestroy releases all soft reservations (DELETE /waiter/tables/{number}/menu/cart).
// The front-end JS then navigates to /waiter/tables.
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
		http.Error(w, "failed to create order", http.StatusInternalServerError)
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
