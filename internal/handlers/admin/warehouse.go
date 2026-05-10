package adminhandler

import (
	"net/http"
	"strconv"
	"strings"

	adminservice "restaurant_network_pos/internal/service/admin"
	"restaurant_network_pos/templates/layouts"
	adminpages "restaurant_network_pos/templates/pages/admin"

	"github.com/go-chi/chi/v5"
)

func parseWarehousePageFilters(r *http.Request) adminservice.WarehouseFilters {
	restaurantID, _ := strconv.Atoi(r.URL.Query().Get("restaurant_id"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	rawIngredients := r.URL.Query()["ingredient"]
	ingredients := make([]string, 0, len(rawIngredients))
	for _, ingredient := range rawIngredients {
		ingredient = strings.TrimSpace(ingredient)
		if ingredient != "" {
			ingredients = append(ingredients, ingredient)
		}
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" && r.URL.Query().Get("all") == "" {
		status = "not_expired"
	}
	return adminservice.WarehouseFilters{
		RestaurantID: restaurantID,
		Ingredients:  ingredients,
		Status:       status,
		ArrivalFrom:  strings.TrimSpace(r.URL.Query().Get("arrival_from")),
		ArrivalTo:    strings.TrimSpace(r.URL.Query().Get("arrival_to")),
		ExpFrom:      strings.TrimSpace(r.URL.Query().Get("exp_from")),
		ExpTo:        strings.TrimSpace(r.URL.Query().Get("exp_to")),
		MinQty:       strings.TrimSpace(r.URL.Query().Get("min_qty")),
		MaxQty:       strings.TrimSpace(r.URL.Query().Get("max_qty")),
		Page:         page,
	}
}

func (h *Handler) WarehousePage(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, name, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	f := parseWarehousePageFilters(r)
	if f.RestaurantID == 0 {
		f.RestaurantID = restaurantID
	}

	view, err := h.Svc.GetWarehousePage(restaurantID, f)
	if err != nil {
		handlerLog.Printf("WarehousePage: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	view.DefaultRestaurantID = restaurantID

	layouts.AdminLayout(name, "warehouse", adminpages.WarehousePage(view)).Render(r.Context(), w)
}

func (h *Handler) WarehouseCreateWriteOff(w http.ResponseWriter, r *http.Request) {
	restaurantID, adminID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	stockID, err := strconv.Atoi(r.FormValue("stock_id"))
	if err != nil || stockID <= 0 {
		http.Error(w, "invalid stock", http.StatusBadRequest)
		return
	}
	reasonID, err := strconv.Atoi(r.FormValue("reason_id"))
	if err != nil || reasonID <= 0 {
		http.Error(w, "invalid reason", http.StatusBadRequest)
		return
	}
	qty, err := strconv.ParseFloat(strings.ReplaceAll(r.FormValue("quantity"), ",", "."), 64)
	if err != nil || qty <= 0 {
		http.Error(w, "invalid quantity", http.StatusBadRequest)
		return
	}

	if err := h.Svc.CreateWriteOff(restaurantID, adminID, stockID, reasonID, qty); err != nil {
		handlerLog.Printf("WarehouseCreateWriteOff: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		item, err := h.Svc.GetWarehouseItem(restaurantID, stockID)
		if err != nil {
			handlerLog.Printf("WarehouseCreateWriteOff GetWarehouseItem: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		history, err := h.Svc.GetStockWriteOffs(restaurantID, stockID)
		if err != nil {
			handlerLog.Printf("WarehouseCreateWriteOff GetStockWriteOffs: %v", err)
			// Non-fatal, just no history
		}

		w.Header().Set("HX-Retarget", "#stock-group-"+strconv.Itoa(stockID))
		adminpages.WarehouseStockGroup(*item, true, history).Render(r.Context(), w)
		return
	}

	redirectURL := r.Referer()
	if redirectURL == "" {
		redirectURL = "/admin/warehouse"
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (h *Handler) WarehouseStockWriteOffs(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	stockID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || stockID <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	items, err := h.Svc.GetStockWriteOffs(restaurantID, stockID)
	if err != nil {
		handlerLog.Printf("WarehouseStockWriteOffs: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	unit := r.URL.Query().Get("unit")
	if unit == "" {
		unit = "г/мл"
	}

	adminpages.WarehouseWriteOffs(items, unit).Render(r.Context(), w)
}
