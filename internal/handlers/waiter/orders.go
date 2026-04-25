package waiterhandler

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	waiterservice "restaurant_network_pos/internal/service/waiter"
	"restaurant_network_pos/templates/layouts"
	waiterpages "restaurant_network_pos/templates/pages/waiter"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
)

// toSQLDatetime converts datetime-local form value ("2006-01-02T15:04") to
// SQL Server-compatible format ("2006-01-02 15:04:00").
func toSQLDatetime(s string) string {
	s = strings.Replace(s, "T", " ", 1)
	if len(s) == 16 { // missing seconds
		s += ":00"
	}
	return s
}

var ordersHandlerLog = log.New(log.Writer(), "[OrdersHandler] ", log.LstdFlags|log.Lshortfile)

type OrdersServicer interface {
	GetActiveOrders(restaurantID int, search, statusName string, tableNumber int, timeFrom, timeTo string) ([]waiterservice.OrderView, error)
	GetArchiveOrders(restaurantID, waiterID int, search, statusName string, tableNumber int, dateFrom, dateTo string) ([]waiterservice.OrderView, error)
	CancelOrder(orderID, restaurantID int) error
	PayOrder(orderID, restaurantID int, paymentMethod string) error
	RejectPayment(orderID, restaurantID int, paymentMethod string) error
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

func (h *OrdersHandler) sessionData(r *http.Request) (restaurantID, waiterID int, name string, err error) {
	sess, err := h.store.Get(r, "session")
	if err != nil {
		return 0, 0, "", err
	}
	restaurantID, _ = sess.Values["restaurant_id"].(int)
	waiterID, _ = sess.Values["user_id"].(int)
	name, _ = sess.Values["full_name"].(string)
	return restaurantID, waiterID, name, nil
}

func (h *OrdersHandler) OrdersPage(w http.ResponseWriter, r *http.Request) {
	restaurantID, waiterID, name, err := h.sessionData(r)
	if err != nil {
		ordersHandlerLog.Printf("OrdersPage: session error: %v", err)
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	activeOrders, err := h.svc.GetActiveOrders(restaurantID, "", "", 0, "", "")
	if err != nil {
		ordersHandlerLog.Printf("OrdersPage: GetActiveOrders error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	today := time.Now().Format("2006-01-02")
	dateFromDisplay := today + "T00:00"
	dateToDisplay := today + "T23:59"

	archiveOrders, err := h.svc.GetArchiveOrders(restaurantID, waiterID, "", "", 0,
		toSQLDatetime(dateFromDisplay), toSQLDatetime(dateToDisplay))
	if err != nil {
		ordersHandlerLog.Printf("OrdersPage: GetArchiveOrders error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	layouts.WaiterLayout(name, "orders", waiterpages.OrdersPage(activeOrders, archiveOrders, dateFromDisplay, dateToDisplay)).Render(r.Context(), w)
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

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	paymentMethod := r.FormValue("payment_method")
	allowed := map[string]bool{"Готівка": true, "Карта": true, "Онлайн": true}
	if !allowed[paymentMethod] {
		paymentMethod = "Готівка"
	}

	if err := h.svc.PayOrder(orderID, restaurantID, paymentMethod); err != nil {
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

func (h *OrdersHandler) RejectPayment(w http.ResponseWriter, r *http.Request) {
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

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	paymentMethod := r.FormValue("payment_method")
	if !map[string]bool{"Карта": true, "Онлайн": true}[paymentMethod] {
		http.Error(w, "cash payments cannot be rejected", http.StatusBadRequest)
		return
	}

	if err := h.svc.RejectPayment(orderID, restaurantID, paymentMethod); err != nil {
		ordersHandlerLog.Printf("RejectPayment: %v", err)
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

func (h *OrdersHandler) ArchiveList(w http.ResponseWriter, r *http.Request) {
	restaurantID, waiterID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	today := time.Now().Format("2006-01-02")
	search := r.URL.Query().Get("search")
	statusName := r.URL.Query().Get("status")
	tableNumber, _ := strconv.Atoi(r.URL.Query().Get("table_number"))
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")

	if dateFrom == "" {
		dateFrom = today + "T00:00"
	}
	if dateTo == "" {
		dateTo = today + "T23:59"
	}

	orders, err := h.svc.GetArchiveOrders(restaurantID, waiterID, search, statusName, tableNumber,
		toSQLDatetime(dateFrom), toSQLDatetime(dateTo))
	if err != nil {
		ordersHandlerLog.Printf("ArchiveList: service error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	waiterpages.ArchiveList(orders).Render(r.Context(), w)
}
