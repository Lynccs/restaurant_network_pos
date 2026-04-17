package routes

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/sessions"
)

func SetupRoutes(database *sql.DB, store sessions.Store) *chi.Mux {
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	mountAuth(r, database, store)
	mountAdmin(r, store)
	mountWaiter(r, store)
	mountChef(r, store)

	return r
}
