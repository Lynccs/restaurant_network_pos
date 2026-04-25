package routes

import (
	"net/http"
	waiterhandler "restaurant_network_pos/internal/handlers/waiter"
	"restaurant_network_pos/internal/middleware"

	"github.com/go-chi/chi/v5"
)

func SetupWaiterRoutes(r chi.Router, h *waiterhandler.WaiterHandler, menuH *waiterhandler.MenuHandler, ordersH *waiterhandler.OrdersHandler) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(h.Store))
		r.Use(middleware.RequireRole(h.Store, "waiter"))

		r.Get("/waiter", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/waiter/tables", http.StatusSeeOther)
		})
		r.Get("/waiter/tables", h.TablesPage)

		r.Get("/waiter/tables/{number}/menu", menuH.MenuPage)
		r.Get("/waiter/tables/{number}/menu/dishes", menuH.GetDishes)
		r.Post("/waiter/tables/{number}/menu/cart/add", menuH.CartAdd)
		r.Post("/waiter/tables/{number}/menu/cart/remove", menuH.CartRemove)
		r.Post("/waiter/tables/{number}/menu/cart/uncancel", menuH.CartUnCancel)
		r.Delete("/waiter/tables/{number}/menu/cart", menuH.CartDestroy)
		r.Post("/waiter/tables/{number}/menu/submit", menuH.SubmitOrder)

		r.Get("/waiter/orders",              ordersH.OrdersPage)
		r.Get("/waiter/orders/list",         ordersH.OrdersList)
		r.Post("/waiter/orders/{id}/cancel",          ordersH.CancelOrder)
		r.Post("/waiter/orders/{id}/pay",             ordersH.PayOrder)
		r.Post("/waiter/orders/{id}/reject-payment",  ordersH.RejectPayment)

		r.Get("/waiter/orders/archive/list", ordersH.ArchiveList)
	})
}
