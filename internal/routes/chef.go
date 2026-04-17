package routes

import (
	"fmt"
	"net/http"
	"restaurant_network_pos/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
)

func mountChef(r *chi.Mux, store sessions.Store) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(store))
		r.Use(middleware.RequireRole(store, "chef"))

		r.Get("/chef", func(w http.ResponseWriter, r *http.Request) {
			sess, _ := store.Get(r, "session")
			name, _ := sess.Values["full_name"].(string)
			fmt.Fprintf(w, "Кухар: %s <form method='POST' action='/logout'><button>Вийти</button></form>", name)
		})
	})
}
