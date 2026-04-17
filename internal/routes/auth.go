package routes

import (
	"database/sql"
	"restaurant_network_pos/internal/handlers"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
)

func mountAuth(r *chi.Mux, database *sql.DB, store sessions.Store) {
	h := &handlers.AuthHandler{DB: database, Store: store}

	r.Get("/login", h.LoginPage)
	r.Post("/login", h.LoginPost)
	r.Post("/logout", h.Logout)
}
