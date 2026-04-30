package chefhandler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
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
	GetReadyBoard(restaurantID int, date time.Time) ([]chefservice.KitchenTicket, error)
	GetAllChefs(restaurantID int) ([]chefservice.ChefInfo, error)
	GetOrderItemInfo(orderItemID int) (string, int, error)
	GetStartCookingData(orderItemID, restaurantID int) (*chefservice.StartCookingView, error)
	StartCooking(orderItemID, chefID int) error
	FinishCooking(taskID int) error
	RecordIngredientUsages(orderItemID, restaurantID int, usages map[int]float64) error
	ReportIssue(orderItemID int) (string, error)
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

	allChefs, err := h.Svc.GetAllChefs(restaurantID)
	if err != nil {
		kitchenHandlerLog.Printf("KitchenBoardPage: GetAllChefs error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	kitchenHandlerLog.Printf("KitchenBoardPage: restaurantID=%d tickets=%d", restaurantID, len(tickets))
	layouts.ChefLayout(name, "kitchen", chefpages.KitchenBoard(tickets, chefID, allChefs)).Render(r.Context(), w)
}

// BoardFragment — тільки дошка без layout, для HTMX-запиту після SSE-події (GET /chef/kitchen/board).
func (h *KitchenHandler) BoardFragment(w http.ResponseWriter, r *http.Request) {
	restaurantID, chefID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	start := time.Now()
	tickets, err := h.Svc.GetKitchenBoard(restaurantID, chefID)
	kitchenHandlerLog.Printf("BoardFragment: restaurantID=%d took=%v", restaurantID, time.Since(start))
	if err != nil {
		kitchenHandlerLog.Printf("BoardFragment: service error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	allChefs, err := h.Svc.GetAllChefs(restaurantID)
	if err != nil {
		kitchenHandlerLog.Printf("BoardFragment: GetAllChefs error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	chefpages.KitchenBoard(tickets, chefID, allChefs).Render(r.Context(), w)
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
		case <-h.Broadcaster.Done():
			return
		case msg := <-ch:
			_ = msg // кухар не потребує payload
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

// ReadyBoardFragment — архівний перегляд готових страв за дату (GET /chef/kitchen/board/ready?date=YYYY-MM-DD).
func (h *KitchenHandler) ReadyBoardFragment(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	dateStr := r.URL.Query().Get("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.Error(w, "invalid date: expected YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	tickets, err := h.Svc.GetReadyBoard(restaurantID, date)
	if err != nil {
		kitchenHandlerLog.Printf("ReadyBoardFragment: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	allChefs, err := h.Svc.GetAllChefs(restaurantID)
	if err != nil {
		kitchenHandlerLog.Printf("ReadyBoardFragment: GetAllChefs: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	chefpages.ReadyHistoryBoard(tickets, allChefs, dateStr).Render(r.Context(), w)
}

// StartCookingModal — GET /chef/kitchen/tasks/{id}/start-modal.
// Повертає HTML модального вікна з рецептом страви та всіма інгредієнтами.
func (h *KitchenHandler) StartCookingModal(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	orderItemID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	data, err := h.Svc.GetStartCookingData(orderItemID, restaurantID)
	if err != nil {
		kitchenHandlerLog.Printf("StartCookingModal: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	chefpages.StartCookingModal(*data).Render(r.Context(), w)
}

// StartCooking — POST /chef/kitchen/tasks/{id}/start.
// Виконує DB-операцію, записує використані інгредієнти та сповіщає кухарів через SSE.
func (h *KitchenHandler) StartCooking(w http.ResponseWriter, r *http.Request) {
	restaurantID, chefID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	actionID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}

	// Зчитуємо вибрані інгредієнти: поля виду ing_{ingredientID} = кількість
	usages := make(map[int]float64)
	if parseErr := r.ParseForm(); parseErr == nil {
		for key, vals := range r.Form {
			if !strings.HasPrefix(key, "ing_") || len(vals) == 0 {
				continue
			}
			ingID, idErr := strconv.Atoi(key[4:])
			qty, qtyErr := strconv.ParseFloat(vals[0], 64)
			if idErr == nil && qtyErr == nil && ingID > 0 && qty > 0 {
				usages[ingID] = qty
			}
		}
	}

	if err := h.Svc.StartCooking(actionID, chefID); err != nil {
		kitchenHandlerLog.Printf("StartCooking: id=%d error: %v", actionID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if len(usages) > 0 {
		if err := h.Svc.RecordIngredientUsages(actionID, restaurantID, usages); err != nil {
			kitchenHandlerLog.Printf("StartCooking: RecordIngredientUsages (non-fatal): %v", err)
		}
	}

	h.Broadcaster.Notify(restaurantID)
	w.Header().Set("HX-Reswap", "none")
	w.WriteHeader(http.StatusOK)
}

// FinishCooking — POST /chef/kitchen/tasks/{id}/finish.
// Виконує DB-операцію та сповіщає всіх підключених кухарів через SSE.
// Не повертає HTML тікета — оновлення board відбувається через SSE-refresh.
func (h *KitchenHandler) FinishCooking(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, _, err := h.sessionData(r)
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
	w.Header().Set("HX-Reswap", "none")
	w.WriteHeader(http.StatusOK)
}

// IssueModal — GET /chef/kitchen/tasks/{id}/issue-modal.
func (h *KitchenHandler) IssueModal(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	actionID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}

	dishName, qty, err := h.Svc.GetOrderItemInfo(actionID)
	if err != nil {
		kitchenHandlerLog.Printf("IssueModal: GetOrderItemInfo id=%d error: %v", actionID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	chefpages.IssueModal(actionID, dishName, qty).Render(r.Context(), w)
}

// ReportIssue — POST /chef/kitchen/tasks/{id}/report-issue.
func (h *KitchenHandler) ReportIssue(w http.ResponseWriter, r *http.Request) {
	restaurantID, chefID, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	actionID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}

	payload, err := h.Svc.ReportIssue(actionID)
	if err != nil {
		kitchenHandlerLog.Printf("ReportIssue: id=%d error: %v", actionID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.Broadcaster.NotifyIssue(restaurantID, payload) // сповіщає офіціантів
	h.Broadcaster.Notify(restaurantID)               // оновлює KDS (позиція зникла)

	tickets, err := h.Svc.GetKitchenBoard(restaurantID, chefID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	allChefs, err := h.Svc.GetAllChefs(restaurantID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	chefpages.KitchenBoard(tickets, chefID, allChefs).Render(r.Context(), w)
}
