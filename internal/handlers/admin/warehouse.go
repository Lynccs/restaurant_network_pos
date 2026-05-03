package adminhandler

import (
	"net/http"
	"strconv"
	"strings"

	adminservice "restaurant_network_pos/internal/service/admin"
	"restaurant_network_pos/templates/layouts"
	adminpages "restaurant_network_pos/templates/pages/admin"
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
	return adminservice.WarehouseFilters{
		RestaurantID: restaurantID,
		Ingredients:  ingredients,
		Status:       strings.TrimSpace(r.URL.Query().Get("status")),
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

	view, err := h.Svc.GetWarehousePage(f.RestaurantID, f)
	if err != nil {
		handlerLog.Printf("WarehousePage: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	view.DefaultRestaurantID = restaurantID

	layouts.AdminLayout(name, "warehouse", adminpages.WarehousePage(view)).Render(r.Context(), w)
}
