package adminhandler

import (
	"net/http"
	"strconv"

	adminservice "restaurant_network_pos/internal/service/admin"
	"restaurant_network_pos/templates/layouts"
	adminpages "restaurant_network_pos/templates/pages/admin"

	"github.com/go-chi/chi/v5"
)

// SuppliersPage — GET /admin/suppliers
func (h *Handler) SuppliersPage(w http.ResponseWriter, r *http.Request) {
	_, _, name, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	suppliers, err := h.SuppliersSvc.GetSuppliers()
	if err != nil {
		handlerLog.Printf("SuppliersPage: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	layouts.AdminLayout(name, "suppliers", adminpages.SuppliersPage(suppliers)).Render(r.Context(), w)
}

// SuppliersList — GET /admin/suppliers/list (HTMX)
func (h *Handler) SuppliersList(w http.ResponseWriter, r *http.Request) {
	suppliers, err := h.SuppliersSvc.GetSuppliers()
	if err != nil {
		handlerLog.Printf("SuppliersList: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	adminpages.SuppliersList(suppliers).Render(r.Context(), w)
}

// SupplierModal — GET /admin/suppliers/new or /admin/suppliers/{id}/edit
func (h *Handler) SupplierModal(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int
	if idStr != "" {
		id, _ = strconv.Atoi(idStr)
	}
	view, err := h.SuppliersSvc.GetSupplierFormView(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	adminpages.SupplierModal(view).Render(r.Context(), w)
}

// CreateUpdateSupplier — POST /admin/suppliers
func (h *Handler) CreateUpdateSupplier(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	id, _ := strconv.Atoi(r.FormValue("id"))
	s := adminservice.Supplier{
		ID:             id,
		CompanyName:    r.FormValue("company_name"),
		ContactPerson:  r.FormValue("contact_person"),
		Phone:          r.FormValue("phone"),
		Address:        r.FormValue("address"),
		PaymentDetails: r.FormValue("payment_details"),
	}

	if s.CompanyName == "" || s.Phone == "" {
		http.Error(w, "company name and phone are required", http.StatusBadRequest)
		return
	}

	if s.ID > 0 {
		if err := h.SuppliersSvc.UpdateSupplier(s); err != nil {
			handlerLog.Printf("UpdateSupplier: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	} else {
		newID, err := h.SuppliersSvc.CreateSupplier(s)
		if err != nil {
			handlerLog.Printf("CreateSupplier: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		s.ID = newID
	}

	rawIDs := r.Form["ingredient_ids[]"]
	ids := make([]int, 0, len(rawIDs))
	for _, rid := range rawIDs {
		vid, _ := strconv.Atoi(rid)
		if vid > 0 {
			ids = append(ids, vid)
		}
	}
	if err := h.SuppliersSvc.SetSupplierIngredients(s.ID, ids); err != nil {
		handlerLog.Printf("SetSupplierIngredients: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", `{"closeModal":null,"refreshSuppliers":null}`)
	w.WriteHeader(http.StatusOK)
}

// SupplierIngredientsModal — GET /admin/suppliers/{id}/ingredients
func (h *Handler) SupplierIngredientsModal(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	view, err := h.SuppliersSvc.GetSupplierIngredientsView(id)
	if err != nil {
		handlerLog.Printf("SupplierIngredientsModal: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	adminpages.SupplierIngredientsModal(view).Render(r.Context(), w)
}

// DeleteSupplierHandler — DELETE /admin/suppliers/{id}
func (h *Handler) DeleteSupplierHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.SuppliersSvc.DeleteSupplier(id); err != nil {
		handlerLog.Printf("DeleteSupplier: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", `{"closeModal":null,"refreshSuppliers":null}`)
	w.WriteHeader(http.StatusOK)
}

// UpdateSupplierIngredients — POST /admin/suppliers/{id}/ingredients
func (h *Handler) UpdateSupplierIngredients(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	rawIDs := r.Form["ingredient_ids[]"]
	ids := make([]int, 0, len(rawIDs))
	for _, rid := range rawIDs {
		vid, _ := strconv.Atoi(rid)
		if vid > 0 {
			ids = append(ids, vid)
		}
	}

	if err := h.SuppliersSvc.SetSupplierIngredients(id, ids); err != nil {
		handlerLog.Printf("UpdateSupplierIngredients: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", `{"closeModal":null,"refreshSuppliers":null}`)
	w.WriteHeader(http.StatusOK)
}
