package routes

import (
	"restaurant_network_pos/internal/handlers"

	"github.com/go-chi/chi/v5"
)

func SetupAuthRoutes(r chi.Router, h *handlers.AuthHandler) {
	r.Get("/login", h.LoginPage)
	r.Post("/login", h.LoginPost)
	r.Post("/logout", h.Logout)
}
