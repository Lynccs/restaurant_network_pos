package waiterhandler

import (
	"log"
	"net/http"
	"strconv"

	waiterservice "restaurant_network_pos/internal/service/waiter"
	"restaurant_network_pos/templates/layouts"
	waiterpages "restaurant_network_pos/templates/pages/waiter"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
)

var ordersHandlerLog = log.New(log.Writer(), "[OrdersHandler] ", log.LstdFlags|log.Lshortfile)

type OrdersServicer interface {
	GetActiveOrders(restaurantID int, search, statusName string, tableNumber int, timeFrom, timeTo string) ([]waiterservice.OrderView, error)
	CancelOrder(orderID, restaurantID int) error
	PayOrder(orderID, restaurantID int) error
}

type OrdersHandler struct {
	svc   OrdersServicer
	store sessions.Store
}

func NewOrdersHandler(svc OrdersServicer, store sessions.Store) *OrdersHandler {
	return &OrdersHandler{svc: svc, store: store}
}

func (h *OrdersHandler) sessionRestaurant(r *http.Request) (restaurantID int, name string, err error) {
	sess, err := h.store.Get(r, "session")
	if err != nil {
		return 0, "", err
	}
	restaurantID, _ = sess.Values["restaurant_id"].(int)
	name, _ = sess.Values["full_name"].(string)
	return restaurantID, name, nil
}

func (h *OrdersHandler) OrdersPage(w http.ResponseWriter, r *http.Request) {
	restaurantID, name, err := h.sessionRestaurant(r)
	if err != nil {
		ordersHandlerLog.Printf("OrdersPage: session error: %v", err)
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	orders, err := h.svc.GetActiveOrders(restaurantID, "", "", 0, "", "")
	if err != nil {
		ordersHandlerLog.Printf("OrdersPage: service error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	layouts.WaiterLayout(name, "orders", waiterpages.OrdersPage(orders)).Render(r.Context(), w)
}

func (h *OrdersHandler) OrdersList(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, err := h.sessionRestaurant(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	search := r.URL.Query().Get("search")
	statusName := r.URL.Query().Get("status")
	tableNumber, _ := strconv.Atoi(r.URL.Query().Get("table_number"))
	timeFrom := r.URL.Query().Get("time_from")
	timeTo := r.URL.Query().Get("time_to")

	orders, err := h.svc.GetActiveOrders(restaurantID, search, statusName, tableNumber, timeFrom, timeTo)
	if err != nil {
		ordersHandlerLog.Printf("OrdersList: service error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	waiterpages.OrdersList(orders).Render(r.Context(), w)
}

func (h *OrdersHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, err := h.sessionRestaurant(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	orderID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid order id", http.StatusBadRequest)
		return
	}

	if err := h.svc.CancelOrder(orderID, restaurantID); err != nil {
		ordersHandlerLog.Printf("CancelOrder: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	orders, err := h.svc.GetActiveOrders(restaurantID, "", "", 0, "", "")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	waiterpages.OrdersList(orders).Render(r.Context(), w)
}

func (h *OrdersHandler) PayOrder(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, err := h.sessionRestaurant(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	orderID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid order id", http.StatusBadRequest)
		return
	}

	if err := h.svc.PayOrder(orderID, restaurantID); err != nil {
		ordersHandlerLog.Printf("PayOrder: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	orders, err := h.svc.GetActiveOrders(restaurantID, "", "", 0, "", "")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	waiterpages.OrdersList(orders).Render(r.Context(), w)
}
