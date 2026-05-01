package chefhandler

import (
	"log"
	"net/http"
	"strconv"
	"time"

	chefservice "restaurant_network_pos/internal/service/chef"
	"restaurant_network_pos/templates/layouts"
	chefpages "restaurant_network_pos/templates/pages/chef"
)

var writeOffHandlerLog = log.New(log.Writer(), "[WriteOffHandler] ", log.LstdFlags|log.Lshortfile)

// defaultFromTime повертає початок поточного дня у форматі datetime-local ("YYYY-MM-DDTHH:MM").
func defaultFromTime() string {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Format("2006-01-02T15:04")
}

func parseWriteOffFilters(r *http.Request) chefservice.WriteOffFilters {
	dish := r.URL.Query().Get("dish")
	if dish == "" {
		dish = "all"
	}
	fromTime := r.URL.Query().Get("from")
	if fromTime == "" {
		fromTime = defaultFromTime()
	}
	return chefservice.WriteOffFilters{
		OrderNumber: r.URL.Query().Get("order"),
		FromTime:    fromTime,
		ToTime:      r.URL.Query().Get("to"),
		Dish:        dish,
		Ingredient:  r.URL.Query().Get("ingredient"),
	}
}

func parseWriteOffPage(r *http.Request) int {
	p, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || p < 1 {
		return 1
	}
	return p
}

// WriteOffPage — повна сторінка (GET /chef/writeoff).
func (h *KitchenHandler) WriteOffPage(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, name, err := h.sessionData(r)
	if err != nil {
		writeOffHandlerLog.Printf("WriteOffPage: session error: %v", err)
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	f := parseWriteOffFilters(r)
	view, err := h.Svc.GetWriteOffPage(restaurantID, f, parseWriteOffPage(r))
	if err != nil {
		writeOffHandlerLog.Printf("WriteOffPage: service error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeOffHandlerLog.Printf("WriteOffPage: restaurantID=%d orders=%d", restaurantID, len(view.Orders))
	layouts.ChefLayout(name, "writeoff", chefpages.WriteOffPage(view)).Render(r.Context(), w)
}

// WriteOffList — HTMX-фрагмент (GET /chef/writeoff/list).
// Навмисно викликає GetWriteOffOrders, а не GetWriteOffPage —
// щоб не запускати GetWriteOffOptions при кожній зміні фільтру.
func (h *KitchenHandler) WriteOffList(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	f := parseWriteOffFilters(r)
	orders, pagination, err := h.Svc.GetWriteOffOrders(restaurantID, f, parseWriteOffPage(r))
	if err != nil {
		writeOffHandlerLog.Printf("WriteOffList: service error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	chefpages.WriteOffList(orders, pagination).Render(r.Context(), w)
}
