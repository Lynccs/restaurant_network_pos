package routes

import (
	"net/http"

	adminhandler "restaurant_network_pos/internal/handlers/admin"
	"restaurant_network_pos/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
)

func SetupAdminRoutes(r chi.Router, h *adminhandler.Handler, store sessions.Store) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(store))
		r.Use(middleware.RequireRole(store, "admin"))

		r.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/purchases", http.StatusSeeOther)
		})

		r.Get("/admin/purchases", h.PurchasesPage)
		r.Get("/admin/purchases/list", h.PurchasesList)
		r.Get("/admin/purchases/new", h.CreateOrderModal)
		r.Post("/admin/purchases/draft-step2", h.CreateOrderDraftStep2)
		r.Get("/admin/purchases/ingredients", h.GetIngredientsJSON)
		r.Post("/admin/purchases", h.CreateOrder)
		r.Post("/admin/purchases/create-with-items", h.CreateOrderWithItems)
		r.Get("/admin/purchases/{id}/details", h.OrderDetails)
		r.Post("/admin/purchases/{id}/items", h.AddItem)
		r.Delete("/admin/purchases/{id}/items/{itemID}", h.RemoveItem)
		r.Post("/admin/purchases/{id}/send", h.MarkAsSent)
		r.Get("/admin/purchases/{id}/receive-modal", h.ReceiveBatchModal)
		r.Get("/admin/purchases/{id}/items/{detailID}/receive-modal", h.ReceiveItemModal)
		r.Get("/admin/purchases/items/{detailID}/batches", h.PurchaseItemBatches)
		r.Post("/admin/purchases/{id}/receive", h.ReceiveBatch)
		r.Get("/admin/purchases/batches/{batchID}/edit-modal", h.EditBatchModal)
		r.Post("/admin/purchases/batches/{batchID}/update", h.UpdateBatch)
		r.Post("/admin/purchases/{id}/complete", h.CompleteOrder)
	})
}
