package routes

import (
	"net/http"
	chefhandler "restaurant_network_pos/internal/handlers/chef"
	"restaurant_network_pos/internal/middleware"

	"github.com/go-chi/chi/v5"
)

func SetupChefRoutes(r chi.Router, h *chefhandler.KitchenHandler) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(h.Store))
		r.Use(middleware.RequireRole(h.Store, "chef"))

		r.Get("/chef", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/chef/kitchen", http.StatusSeeOther)
		})

		r.Get("/chef/kitchen", h.KitchenBoardPage)
		r.Get("/chef/kitchen/board", h.BoardFragment)
		r.Get("/chef/kitchen/board/ready", h.ReadyBoardFragment)
		r.Get("/chef/kitchen/events", h.Events)

		r.Get("/chef/kitchen/tasks/{id}/start-modal", h.StartCookingModal)
		r.Post("/chef/kitchen/tasks/{id}/start", h.StartCooking)
		r.Post("/chef/kitchen/tasks/{id}/finish", h.FinishCooking)
		r.Get("/chef/kitchen/tasks/{id}/issue-modal", h.IssueModal)
		r.Post("/chef/kitchen/tasks/{id}/report-issue", h.ReportIssue)

		r.Get("/chef/writeoff", h.WriteOffPage)
		r.Get("/chef/writeoff/list", h.WriteOffList)
	})
}
