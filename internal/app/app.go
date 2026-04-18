package app

import (
	"database/sql"
	"net/http"
	"restaurant_network_pos/internal/handlers"
	"restaurant_network_pos/internal/repository"
	"restaurant_network_pos/internal/routes"
	"restaurant_network_pos/internal/service"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/sessions"
)

func SetupRouter(db *sql.DB, store sessions.Store) *chi.Mux {
	// Composition Root: збираємо весь ланцюг залежностей тут
	userRepo := repository.NewUserRepo(db)
	authSvc := service.NewAuthService(userRepo)
	authH := &handlers.AuthHandler{Auth: authSvc, Store: store}

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	routes.SetupAuthRoutes(r, authH)
	routes.SetupAdminRoutes(r, store)
	routes.SetupWaiterRoutes(r, store)
	routes.SetupChefRoutes(r, store)

	return r
}
