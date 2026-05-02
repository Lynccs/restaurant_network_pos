package adminhandler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	adminservice "restaurant_network_pos/internal/service/admin"
	"restaurant_network_pos/templates/layouts"
	adminpages "restaurant_network_pos/templates/pages/admin"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
)

var handlerLog = log.New(log.Writer(), "[AdminHandler] ", log.LstdFlags|log.Lshortfile)

type Handler struct {
	Svc   adminservice.PurchasesServicer
	Store sessions.Store
}

func NewHandler(svc adminservice.PurchasesServicer, store sessions.Store) *Handler {
	return &Handler{Svc: svc, Store: store}
}

func (h *Handler) sessionData(r *http.Request) (restaurantID, adminID int, name string, err error) {
	sess, err := h.Store.Get(r, "session")
	if err != nil {
		return 0, 0, "", err
	}
	restaurantID, _ = sess.Values["restaurant_id"].(int)
	adminID, _ = sess.Values["user_id"].(int)
	name, _ = sess.Values["full_name"].(string)
	return restaurantID, adminID, name, nil
}

func parseFilters(r *http.Request) adminservice.PurchasesFilters {
	supplierID, _ := strconv.Atoi(r.URL.Query().Get("supplier_id"))
	statusID, _ := strconv.Atoi(r.URL.Query().Get("status_id"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	return adminservice.PurchasesFilters{
		Ownership:   r.URL.Query().Get("ownership"),
		SupplierID:  supplierID,
		CreatedFrom: r.URL.Query().Get("created_from"),
		CreatedTo:   r.URL.Query().Get("created_to"),
		MinTotal:    r.URL.Query().Get("min_total"),
		MaxTotal:    r.URL.Query().Get("max_total"),
		StatusID:    statusID,
		Archive:     r.URL.Query().Get("archive") == "1",
		Page:        page,
	}
}

// PurchasesPage — GET /admin/purchases
func (h *Handler) PurchasesPage(w http.ResponseWriter, r *http.Request) {
	restaurantID, adminID, name, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	f := parseFilters(r)
	view, err := h.Svc.GetPurchasesPage(restaurantID, adminID, f)
	if err != nil {
		handlerLog.Printf("PurchasesPage: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	layouts.AdminLayout(name, "purchases", adminpages.PurchasesPage(view)).Render(r.Context(), w)
}

// PurchasesList — GET /admin/purchases/list (HTMX fragment)
func (h *Handler) PurchasesList(w http.ResponseWriter, r *http.Request) {
	restaurantID, adminID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	f := parseFilters(r)
	view, err := h.Svc.GetPurchasesList(restaurantID, adminID, f)
	if err != nil {
		handlerLog.Printf("PurchasesList: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	adminpages.PurchasesList(view).Render(r.Context(), w)
}

// CreateOrderModal — GET /admin/purchases/new (step 1 modal: supplier + date)
func (h *Handler) CreateOrderModal(w http.ResponseWriter, r *http.Request) {
	suppliers, err := h.Svc.GetSuppliers()
	if err != nil {
		handlerLog.Printf("CreateOrderModal suppliers: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	adminpages.CreateOrderModal(suppliers).Render(r.Context(), w)
}

// CreateOrderDraftStep2 — POST /admin/purchases/draft-step2 (step 2 modal: add ingredients)
func (h *Handler) CreateOrderDraftStep2(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	supplierID, err := strconv.Atoi(r.FormValue("supplier_id"))
	if err != nil || supplierID <= 0 {
		http.Error(w, "invalid supplier", http.StatusBadRequest)
		return
	}
	expectedAt, err := time.Parse("2006-01-02", r.FormValue("expected_at"))
	if err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return
	}
	suppliers, err := h.Svc.GetSuppliers()
	if err != nil {
		handlerLog.Printf("CreateOrderDraftStep2 suppliers: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	supplierName := ""
	for _, s := range suppliers {
		if s.ID == supplierID {
			supplierName = s.Name
			break
		}
	}
	ingredients, err := h.Svc.GetIngredients()
	if err != nil {
		handlerLog.Printf("CreateOrderDraftStep2 ingredients: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	adminpages.OrderDraftItemsModal(supplierID, supplierName, expectedAt, ingredients).Render(r.Context(), w)
}

// CreateOrderWithItems — POST /admin/purchases/create-with-items
func (h *Handler) CreateOrderWithItems(w http.ResponseWriter, r *http.Request) {
	_, adminID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	supplierID, err := strconv.Atoi(r.FormValue("supplier_id"))
	if err != nil || supplierID <= 0 {
		http.Error(w, "invalid supplier", http.StatusBadRequest)
		return
	}
	expectedAt, err := time.Parse("2006-01-02", r.FormValue("expected_at"))
	if err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return
	}

	ingredientIDs := r.MultipartForm.Value["ingredient_id[]"]
	qtys := r.MultipartForm.Value["qty[]"]
	prices := r.MultipartForm.Value["price[]"]

	if len(ingredientIDs) == 0 {
		http.Error(w, "no items", http.StatusBadRequest)
		return
	}

	items := make([]adminservice.ItemDraftInput, 0, len(ingredientIDs))
	for i := range ingredientIDs {
		ingID, _ := strconv.Atoi(ingredientIDs[i])
		qty, _ := strconv.ParseFloat(safeGet(qtys, i), 64)
		price, _ := strconv.ParseFloat(safeGet(prices, i), 64)
		if ingID <= 0 || qty <= 0 || price <= 0 {
			http.Error(w, "invalid item data", http.StatusBadRequest)
			return
		}
		items = append(items, adminservice.ItemDraftInput{
			IngredientID: ingID,
			Qty:          qty,
			Price:        price,
		})
	}

	if err := h.Svc.CreateOrderWithItems(adminID, supplierID, expectedAt, items); err != nil {
		handlerLog.Printf("CreateOrderWithItems: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", `{"closeModal":null,"refreshList":null,"showToast":"orderCreated"}`)
	w.WriteHeader(http.StatusOK)
}

// CreateOrder — POST /admin/purchases
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	_, adminID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	supplierID, err := strconv.Atoi(r.FormValue("supplier_id"))
	if err != nil || supplierID <= 0 {
		http.Error(w, "invalid supplier", http.StatusBadRequest)
		return
	}

	expectedAt, err := time.Parse("2006-01-02", r.FormValue("expected_at"))
	if err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return
	}

	orderID, err := h.Svc.CreateOrder(adminID, supplierID, expectedAt)
	if err != nil {
		handlerLog.Printf("CreateOrder: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	order, err := h.Svc.GetOrderDetails(orderID)
	if err != nil {
		handlerLog.Printf("CreateOrder details: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	ingredients, err := h.Svc.GetIngredients()
	if err != nil {
		handlerLog.Printf("CreateOrder ingredients: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	adminpages.OrderDetailsModal(order, ingredients, adminID).Render(r.Context(), w)
}

// OrderDetails — GET /admin/purchases/{id}/details (modal fragment)
func (h *Handler) OrderDetails(w http.ResponseWriter, r *http.Request) {
	_, adminID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	orderID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	order, err := h.Svc.GetOrderDetails(orderID)
	if err != nil {
		handlerLog.Printf("OrderDetails: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	ingredients, err := h.Svc.GetIngredients()
	if err != nil {
		handlerLog.Printf("OrderDetails ingredients: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	adminpages.OrderDetailsModal(order, ingredients, adminID).Render(r.Context(), w)
}

// AddItem — POST /admin/purchases/{id}/items
func (h *Handler) AddItem(w http.ResponseWriter, r *http.Request) {
	_, adminID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	orderID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	detailID, _ := strconv.Atoi(r.FormValue("detail_id"))
	ingredientID, _ := strconv.Atoi(r.FormValue("ingredient_id"))
	qty, _ := strconv.ParseFloat(r.FormValue("qty"), 64)
	price, _ := strconv.ParseFloat(r.FormValue("price"), 64)

	if qty <= 0 || price <= 0 {
		http.Error(w, "invalid fields", http.StatusBadRequest)
		return
	}

	if detailID > 0 {
		if err := h.Svc.UpdateItem(orderID, detailID, qty, price); err != nil {
			handlerLog.Printf("UpdateItem: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	} else {
		if ingredientID <= 0 {
			http.Error(w, "invalid fields", http.StatusBadRequest)
			return
		}
		if err := h.Svc.AddItem(orderID, ingredientID, qty, price); err != nil {
			handlerLog.Printf("AddItem: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	order, err := h.Svc.GetOrderDetails(orderID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ingredients, err := h.Svc.GetIngredients()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	adminpages.OrderDetailsModal(order, ingredients, adminID).Render(r.Context(), w)
}

// RemoveItem — DELETE /admin/purchases/{id}/items/{itemID}
func (h *Handler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	_, adminID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	orderID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	itemID, err := strconv.Atoi(chi.URLParam(r, "itemID"))
	if err != nil {
		http.Error(w, "invalid itemID", http.StatusBadRequest)
		return
	}

	if err := h.Svc.RemoveItem(orderID, itemID); err != nil {
		handlerLog.Printf("RemoveItem: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	order, err := h.Svc.GetOrderDetails(orderID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ingredients, err := h.Svc.GetIngredients()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	adminpages.OrderDetailsModal(order, ingredients, adminID).Render(r.Context(), w)
}

// MarkAsSent — POST /admin/purchases/{id}/send
func (h *Handler) MarkAsSent(w http.ResponseWriter, r *http.Request) {
	orderID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.Svc.MarkAsSent(orderID); err != nil {
		handlerLog.Printf("MarkAsSent: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Trigger", `{"closeModal":null,"refreshList":null}`)
	w.WriteHeader(http.StatusOK)
}

// ReceiveBatchModal — GET /admin/purchases/{id}/receive-modal
func (h *Handler) ReceiveBatchModal(w http.ResponseWriter, r *http.Request) {
	orderID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	order, err := h.Svc.GetOrderDetails(orderID)
	if err != nil {
		handlerLog.Printf("ReceiveBatchModal: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	adminpages.ReceiveBatchModal(order).Render(r.Context(), w)
}

// ReceiveBatch — POST /admin/purchases/{id}/receive
func (h *Handler) ReceiveBatch(w http.ResponseWriter, r *http.Request) {
	restaurantID, adminID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	orderID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	detailIDs := r.Form["detail_id[]"]
	ingredientIDs := r.Form["ingredient_id[]"]
	qtys := r.Form["qty[]"]
	expDates := r.Form["exp_date[]"]

	if len(detailIDs) == 0 {
		http.Error(w, "no items", http.StatusBadRequest)
		return
	}

	batches := make([]adminservice.BatchInput, 0, len(detailIDs))
	for i := range detailIDs {
		detailID, _ := strconv.Atoi(detailIDs[i])
		ingredientID, _ := strconv.Atoi(safeGet(ingredientIDs, i))
		qty, _ := strconv.ParseFloat(safeGet(qtys, i), 64)
		expDate, err := time.Parse("2006-01-02", safeGet(expDates, i))
		if err != nil || detailID <= 0 || ingredientID <= 0 || qty <= 0 {
			http.Error(w, "invalid batch data", http.StatusBadRequest)
			return
		}
		batches = append(batches, adminservice.BatchInput{
			DetailID:     detailID,
			IngredientID: ingredientID,
			Qty:          qty,
			ExpDate:      expDate,
		})
	}

	if err := h.Svc.ReceiveBatches(restaurantID, adminID, batches); err != nil {
		handlerLog.Printf("ReceiveBatch orderID=%d: %v", orderID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", `{"closeModal":null,"refreshList":null,"showToast":"batchReceived"}`)
	w.WriteHeader(http.StatusOK)
}

// CompleteOrder — POST /admin/purchases/{id}/complete
func (h *Handler) CompleteOrder(w http.ResponseWriter, r *http.Request) {
	orderID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.Svc.CompleteOrder(orderID); err != nil {
		handlerLog.Printf("CompleteOrder: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Trigger", `{"closeModal":null,"refreshList":null}`)
	w.WriteHeader(http.StatusOK)
}

// ReceiveItemModal — GET /admin/purchases/{id}/items/{detailID}/receive-modal
func (h *Handler) ReceiveItemModal(w http.ResponseWriter, r *http.Request) {
	orderID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	detailID, err := strconv.Atoi(chi.URLParam(r, "detailID"))
	if err != nil {
		http.Error(w, "invalid detailID", http.StatusBadRequest)
		return
	}
	order, err := h.Svc.GetOrderDetails(orderID)
	if err != nil {
		handlerLog.Printf("ReceiveItemModal: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	for _, item := range order.Items {
		if item.DetailID == detailID {
			adminpages.ReceiveItemModal(orderID, item).Render(r.Context(), w)
			return
		}
	}
	http.Error(w, "item not found", http.StatusNotFound)
}

// GetIngredientsJSON — GET /admin/purchases/ingredients (JSON)
func (h *Handler) GetIngredientsJSON(w http.ResponseWriter, r *http.Request) {
	ingredients, err := h.Svc.GetIngredients()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ingredients)
}

func safeGet(slice []string, i int) string {
	if i < len(slice) {
		return slice[i]
	}
	return ""
}
