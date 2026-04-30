package waiterhandler

import (
	"fmt"
	"log"
	"net/http"
	"time"

	waiterservice "restaurant_network_pos/internal/service/waiter"
	"restaurant_network_pos/internal/sse"
	"restaurant_network_pos/templates/layouts"
	waiterpages "restaurant_network_pos/templates/pages/waiter"

	"github.com/gorilla/sessions"
)

var handlerLog = log.New(log.Writer(), "[WaiterHandler] ", log.LstdFlags|log.Lshortfile)

type WaiterServicer interface {
	GetTables(restaurantID int) ([]waiterservice.TableView, error)
	GetRestaurantName(restaurantID int) (string, error)
}

// CacheWarmer is satisfied by CartManager; used for background warm-up on the tables page.
type CacheWarmer interface {
	WarmUpCache(restaurantID int) error
}

type WaiterHandler struct {
	Svc         WaiterServicer
	Store       sessions.Store
	CartMgr     CacheWarmer
	Broadcaster *sse.Broadcaster
}

func NewWaiterHandler(svc WaiterServicer, store sessions.Store, cartMgr CacheWarmer, bc *sse.Broadcaster) *WaiterHandler {
	return &WaiterHandler{Svc: svc, Store: store, CartMgr: cartMgr, Broadcaster: bc}
}

func (h *WaiterHandler) TablesPage(w http.ResponseWriter, r *http.Request) {
	sess, err := h.Store.Get(r, "session")
	if err != nil {
		handlerLog.Printf("TablesPage: session error: %v", err)
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	restaurantID, _ := sess.Values["restaurant_id"].(int)
	name, _ := sess.Values["full_name"].(string)

	handlerLog.Printf("TablesPage: restaurantID=%d name=%s", restaurantID, name)

	// Background warm-up: pre-loads the ingredient stock cache so the menu
	// page renders without the heavy SQL query on the critical path.
	go h.CartMgr.WarmUpCache(restaurantID) //nolint:errcheck

	tables, err := h.Svc.GetTables(restaurantID)
	if err != nil {
		handlerLog.Printf("TablesPage: service error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	restaurantName, err := h.Svc.GetRestaurantName(restaurantID)
	if err != nil {
		handlerLog.Printf("TablesPage: get restaurant name error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	hasAnyIssue := false
	for _, t := range tables {
		if t.HasIssue {
			hasAnyIssue = true
			break
		}
	}
	layouts.WaiterLayout(name, "tables", hasAnyIssue, waiterpages.TablesPage(tables, restaurantName)).Render(r.Context(), w)
}

// Events — SSE-стрім для офіціанта (GET /waiter/events).
// Порожній msg = refresh, непорожній = JSON issue-payload.
func (h *WaiterHandler) Events(w http.ResponseWriter, r *http.Request) {
	sess, err := h.Store.Get(r, "session")
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	restaurantID, _ := sess.Values["restaurant_id"].(int)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := h.Broadcaster.Subscribe(restaurantID)
	defer h.Broadcaster.Unsubscribe(restaurantID, ch)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.Broadcaster.Done():
			return
		case msg := <-ch:
			if msg == "" {
				if _, err := fmt.Fprintf(w, "event: refresh\ndata: \n\n"); err != nil {
					return
				}
			} else {
				if _, err := fmt.Fprintf(w, "event: issue\ndata: %s\n\n", msg); err != nil {
					return
				}
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// TablesGrid — HTMX-фрагмент сітки столиків (GET /waiter/tables/grid).
func (h *WaiterHandler) TablesGrid(w http.ResponseWriter, r *http.Request) {
	sess, err := h.Store.Get(r, "session")
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	restaurantID, _ := sess.Values["restaurant_id"].(int)

	tables, err := h.Svc.GetTables(restaurantID)
	if err != nil {
		handlerLog.Printf("TablesGrid: service error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	waiterpages.TableGrid(tables).Render(r.Context(), w)
}
