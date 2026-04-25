package chefhandler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	chefservice "restaurant_network_pos/internal/service/chef"
	"restaurant_network_pos/internal/sse"
	"restaurant_network_pos/templates/layouts"
	chefpages "restaurant_network_pos/templates/pages/chef"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
)

var kitchenHandlerLog = log.New(log.Writer(), "[KitchenHandler] ", log.LstdFlags|log.Lshortfile)

type KitchenBoardServicer interface {
	GetKitchenBoard(restaurantID, chefID int) ([]chefservice.KitchenTicket, error)
	GetKitchenBoardSnapshot(restaurantID, chefID int) ([]chefservice.KitchenTicket, error)
	StartCooking(taskID, chefID int) error
	FinishCooking(taskID int) error
}

type KitchenHandler struct {
	Svc         KitchenBoardServicer
	Store       sessions.Store
	Broadcaster *sse.Broadcaster
}

func NewKitchenHandler(svc KitchenBoardServicer, store sessions.Store, bc *sse.Broadcaster) *KitchenHandler {
	return &KitchenHandler{Svc: svc, Store: store, Broadcaster: bc}
}

func (h *KitchenHandler) sessionData(r *http.Request) (restaurantID, chefID int, name string, err error) {
	sess, err := h.Store.Get(r, "session")
	if err != nil {
		return 0, 0, "", err
	}
	restaurantID, _ = sess.Values["restaurant_id"].(int)
	chefID, _ = sess.Values["user_id"].(int)
	name, _ = sess.Values["full_name"].(string)
	return restaurantID, chefID, name, nil
}

// KitchenBoardPage — повна сторінка (GET /chef/kitchen).
func (h *KitchenHandler) KitchenBoardPage(w http.ResponseWriter, r *http.Request) {
	restaurantID, chefID, name, err := h.sessionData(r)
	if err != nil {
		kitchenHandlerLog.Printf("KitchenBoardPage: session error: %v", err)
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	tickets, err := h.Svc.GetKitchenBoard(restaurantID, chefID)
	if err != nil {
		kitchenHandlerLog.Printf("KitchenBoardPage: service error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	kitchenHandlerLog.Printf("KitchenBoardPage: restaurantID=%d tickets=%d", restaurantID, len(tickets))
	layouts.ChefLayout(name, "kitchen", chefpages.KitchenBoard(tickets, chefID)).Render(r.Context(), w)
}

// BoardFragment — тільки дошка без layout, для HTMX-запиту після SSE-події (GET /chef/kitchen/board).
func (h *KitchenHandler) BoardFragment(w http.ResponseWriter, r *http.Request) {
	restaurantID, chefID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	tickets, err := h.Svc.GetKitchenBoard(restaurantID, chefID)
	if err != nil {
		kitchenHandlerLog.Printf("BoardFragment: service error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	chefpages.KitchenBoard(tickets, chefID).Render(r.Context(), w)
}

// Events — SSE-стрім оновлень дошки (GET /chef/kitchen/events).
func (h *KitchenHandler) Events(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Вимикаємо WriteTimeout для цього з'єднання — SSE є довготривалим стрімом,
	// тому стандартний 15-секундний таймаут вбивав би з'єднання і залишав горутини висіти.
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
		case <-ch:
			if _, err := fmt.Fprintf(w, "data: refresh\n\n"); err != nil {
				return // клієнт відключився
			}
			flusher.Flush()
		case <-ticker.C:
			// Коментар SSE-тримаєчка не тригерить onmessage, але підтримує живе з'єднання.
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// StartCooking — POST /chef/kitchen/tasks/{id}/start.
func (h *KitchenHandler) StartCooking(w http.ResponseWriter, r *http.Request) {
	restaurantID, chefID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	taskID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}

	if err := h.Svc.StartCooking(taskID, chefID); err != nil {
		kitchenHandlerLog.Printf("StartCooking: taskID=%d error: %v", taskID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.Broadcaster.Notify(restaurantID)

	tickets, err := h.Svc.GetKitchenBoardSnapshot(restaurantID, chefID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	for _, ticket := range tickets {
		for _, task := range ticket.Tasks {
			if task.CookingTaskID == taskID {
				chefpages.TicketCol(ticket, chefID).Render(r.Context(), w)
				return
			}
		}
	}
	w.Header().Set("HX-Reswap", "delete")
	w.WriteHeader(http.StatusOK)
}

// FinishCooking — POST /chef/kitchen/tasks/{id}/finish.
func (h *KitchenHandler) FinishCooking(w http.ResponseWriter, r *http.Request) {
	restaurantID, chefID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	taskID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}

	if err := h.Svc.FinishCooking(taskID); err != nil {
		kitchenHandlerLog.Printf("FinishCooking: taskID=%d error: %v", taskID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.Broadcaster.Notify(restaurantID)

	tickets, err := h.Svc.GetKitchenBoardSnapshot(restaurantID, chefID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	for _, ticket := range tickets {
		for _, task := range ticket.Tasks {
			if task.CookingTaskID == taskID {
				chefpages.TicketCol(ticket, chefID).Render(r.Context(), w)
				return
			}
		}
	}
	// Тікет не знайдено — замовлення стало "Готове" і відфільтрувалось → видалити колонку
	w.Header().Set("HX-Reswap", "delete")
	w.WriteHeader(http.StatusOK)
}

// IssueModal — GET /chef/kitchen/tasks/{id}/issue-modal.
func (h *KitchenHandler) IssueModal(w http.ResponseWriter, r *http.Request) {
	restaurantID, chefID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	taskID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}

	tickets, err := h.Svc.GetKitchenBoard(restaurantID, chefID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var dishName string
	var qty int
	for _, t := range tickets {
		for _, task := range t.Tasks {
			if task.CookingTaskID == taskID {
				dishName = task.DishName
				qty = task.Qty
			}
		}
	}

	chefpages.IssueModal(taskID, dishName, qty).Render(r.Context(), w)
}

// ReportIssue — POST /chef/kitchen/tasks/{id}/report-issue.
func (h *KitchenHandler) ReportIssue(w http.ResponseWriter, r *http.Request) {
	restaurantID, chefID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	tickets, err := h.Svc.GetKitchenBoard(restaurantID, chefID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	chefpages.KitchenBoard(tickets, chefID).Render(r.Context(), w)
}
