package app

import (
	"database/sql"
	"net/http"
	"restaurant_network_pos/internal/handlers"
	chefhandler "restaurant_network_pos/internal/handlers/chef"
	waiterhandler "restaurant_network_pos/internal/handlers/waiter"
	"restaurant_network_pos/internal/middleware"
	"restaurant_network_pos/internal/repository"
	chefrepo "restaurant_network_pos/internal/repository/chef"
	waiterrepo "restaurant_network_pos/internal/repository/waiter"
	"restaurant_network_pos/internal/routes"
	"restaurant_network_pos/internal/service"
	chefservice "restaurant_network_pos/internal/service/chef"
	"restaurant_network_pos/internal/sse"
	waiterservice "restaurant_network_pos/internal/service/waiter"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/sessions"
)

func SetupRouter(db *sql.DB, store sessions.Store) (*chi.Mux, func()) {
	// Composition Root: збираємо весь ланцюг залежностей тут
	userRepo := repository.NewUserRepo(db)
	authSvc := service.NewAuthService(userRepo)
	authH := &handlers.AuthHandler{Auth: authSvc, Store: store}

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(middleware.NewInflightGuard(store).Middleware)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	broadcaster := sse.NewBroadcaster()

	waiterRepo := waiterrepo.NewWaiterRepo(db)
	waiterSvc := waiterservice.NewWaiterService(waiterRepo)

	menuRepo := waiterrepo.NewMenuRepo(db)
	cartMgr := waiterservice.NewCartManager(menuRepo)

	waiterH := waiterhandler.NewWaiterHandler(waiterSvc, store, cartMgr, broadcaster)
	menuH := waiterhandler.NewMenuHandler(cartMgr, menuRepo, store, broadcaster)

	ordersRepo := waiterrepo.NewOrdersRepo(db)
	ordersSvc := waiterservice.NewOrdersService(ordersRepo)
	ordersH := waiterhandler.NewOrdersHandler(ordersSvc, store, broadcaster)

	kitchenRepo := chefrepo.NewKitchenRepo(db)
	kitchenSvc := chefservice.NewKitchenService(kitchenRepo)
	kitchenH := chefhandler.NewKitchenHandler(kitchenSvc, store, broadcaster)

	routes.SetupAuthRoutes(r, authH)
	routes.SetupAdminRoutes(r, store)
	routes.SetupWaiterRoutes(r, waiterH, menuH, ordersH)
	routes.SetupChefRoutes(r, kitchenH)

	cleanup := func() {
		broadcaster.Shutdown()
	}

	return r, cleanup
}
