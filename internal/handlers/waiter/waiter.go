package waiterhandler

import (
	"log"
	"net/http"

	waiterservice "restaurant_network_pos/internal/service/waiter"
	"restaurant_network_pos/templates/layouts"
	waiterpages "restaurant_network_pos/templates/pages/waiter"

	"github.com/gorilla/sessions"
)

var handlerLog = log.New(log.Writer(), "[WaiterHandler] ", log.LstdFlags|log.Lshortfile)

type WaiterServicer interface {
	GetTables(restaurantID int) ([]waiterservice.TableView, error)
	GetRestaurantName(restaurantID int) (string, error)
}

// CacheWarmer is satisfied by CartManager; used for background warm-up on the tables page.
type CacheWarmer interface {
	WarmUpCache(restaurantID int) error
}

type WaiterHandler struct {
	Svc     WaiterServicer
	Store   sessions.Store
	CartMgr CacheWarmer
}

func NewWaiterHandler(svc WaiterServicer, store sessions.Store, cartMgr CacheWarmer) *WaiterHandler {
	return &WaiterHandler{Svc: svc, Store: store, CartMgr: cartMgr}
}

func (h *WaiterHandler) TablesPage(w http.ResponseWriter, r *http.Request) {
	sess, err := h.Store.Get(r, "session")
	if err != nil {
		handlerLog.Printf("TablesPage: session error: %v", err)
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	restaurantID, _ := sess.Values["restaurant_id"].(int)
	name, _ := sess.Values["full_name"].(string)

	handlerLog.Printf("TablesPage: restaurantID=%d name=%s", restaurantID, name)

	// Background warm-up: pre-loads the ingredient stock cache so the menu
	// page renders without the heavy SQL query on the critical path.
	go h.CartMgr.WarmUpCache(restaurantID) //nolint:errcheck

	tables, err := h.Svc.GetTables(restaurantID)
	if err != nil {
		handlerLog.Printf("TablesPage: service error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	restaurantName, err := h.Svc.GetRestaurantName(restaurantID)
	if err != nil {
		handlerLog.Printf("TablesPage: get restaurant name error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	layouts.WaiterLayout(name, "tables", waiterpages.TablesPage(tables, restaurantName)).Render(r.Context(), w)
}
