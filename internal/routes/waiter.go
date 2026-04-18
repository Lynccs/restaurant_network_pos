package routes

import (
	"net/http"
	waiterhandler "restaurant_network_pos/internal/handlers/waiter"
	"restaurant_network_pos/internal/middleware"

	"github.com/go-chi/chi/v5"
)

func SetupWaiterRoutes(r chi.Router, h *waiterhandler.WaiterHandler) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(h.Store))
		r.Use(middleware.RequireRole(h.Store, "waiter"))

		r.Get("/waiter", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/waiter/tables", http.StatusSeeOther)
		})
		r.Get("/waiter/tables", h.TablesPage)
	})
}
