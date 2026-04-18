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

type WaiterHandler struct {
	Svc   WaiterServicer
	Store sessions.Store
}

func NewWaiterHandler(svc WaiterServicer, store sessions.Store) *WaiterHandler {
	return &WaiterHandler{Svc: svc, Store: store}
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
