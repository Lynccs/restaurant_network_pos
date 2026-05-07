package app

import (
	"database/sql"
	"net/http"
	"restaurant_network_pos/internal/handlers"
	adminhandler "restaurant_network_pos/internal/handlers/admin"
	chefhandler "restaurant_network_pos/internal/handlers/chef"
	waiterhandler "restaurant_network_pos/internal/handlers/waiter"
	"restaurant_network_pos/internal/middleware"
	"restaurant_network_pos/internal/repository"
	adminrepo "restaurant_network_pos/internal/repository/admin"
	chefrepo "restaurant_network_pos/internal/repository/chef"
	waiterrepo "restaurant_network_pos/internal/repository/waiter"
	"restaurant_network_pos/internal/routes"
	"restaurant_network_pos/internal/service"
	adminservice "restaurant_network_pos/internal/service/admin"
	chefservice "restaurant_network_pos/internal/service/chef"
	waiterservice "restaurant_network_pos/internal/service/waiter"
	"restaurant_network_pos/internal/sse"

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
	r.NotFound(handlers.NotFound)
	r.MethodNotAllowed(handlers.MethodNotAllowed)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	broadcaster := sse.NewBroadcaster()

	waiterRepo := waiterrepo.NewWaiterRepo(db)
	waiterSvc := waiterservice.NewWaiterService(waiterRepo)

	menuRepoAdmin := adminrepo.NewMenuRepo(db)
	menuSvc := adminservice.NewMenuService(menuRepoAdmin)

	menuRepo := waiterrepo.NewMenuRepo(db)
	cartMgr := waiterservice.NewCartManager(menuRepo, menuSvc)

	waiterH := waiterhandler.NewWaiterHandler(waiterSvc, store, cartMgr, broadcaster)
	menuH := waiterhandler.NewMenuHandler(cartMgr, menuRepo, store, broadcaster)

	ordersRepo := waiterrepo.NewOrdersRepo(db)
	ordersSvc := waiterservice.NewOrdersService(ordersRepo)
	ordersH := waiterhandler.NewOrdersHandler(ordersSvc, store, broadcaster)

	kitchenRepo := chefrepo.NewKitchenRepo(db)
	kitchenSvc := chefservice.NewKitchenService(kitchenRepo)
	kitchenH := chefhandler.NewKitchenHandler(kitchenSvc, store, broadcaster)

	purchasesRepo := adminrepo.NewPurchasesRepo(db)
	purchasesSvc := adminservice.NewPurchasesService(purchasesRepo)

	suppliersRepo := adminrepo.NewSuppliersRepo(db)
	suppliersSvc := adminservice.NewSuppliersService(suppliersRepo, purchasesRepo)

	networkRepo := adminrepo.NewNetworkRepo(db)
	networkSvc := adminservice.NewNetworkService(networkRepo)

	adminH := adminhandler.NewHandler(purchasesSvc, suppliersSvc, networkSvc, menuSvc, store)

	routes.SetupAuthRoutes(r, authH)
	routes.SetupAdminRoutes(r, adminH, store)
	routes.SetupWaiterRoutes(r, waiterH, menuH, ordersH)
	routes.SetupChefRoutes(r, kitchenH)

	cleanup := func() {
		broadcaster.Shutdown()
	}

	return r, cleanup
}
