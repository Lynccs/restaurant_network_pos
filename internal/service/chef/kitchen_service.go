package chefservice

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	chefrepo "restaurant_network_pos/internal/repository/chef"
)

var kitchenSvcLog = log.New(log.Writer(), "[KitchenService] ", log.LstdFlags|log.Lshortfile)

const overdueThreshold = 20 * time.Minute

// TaskStatus відображає поточний стан завдання на приготування.
type TaskStatus string

const (
	TaskStatusNew     TaskStatus = "new"
	TaskStatusCooking TaskStatus = "cooking"
	TaskStatusReady   TaskStatus = "ready"
)

// KitchenTaskView — одне завдання на приготування, підготовлене для рендеру в Templ.
type KitchenTaskView struct {
	CookingTaskID int
	OrderItemID   int
	DishName      string
	DishCategory  string
	CookingTime   int // хвилин (для прогрес-таймера на фронті)
	Qty           int
	Status        TaskStatus
	StartTime     *time.Time // nil якщо статус "new"
	ChefID        int        // 0 якщо статус "new"
	ChefName      string     // порожній якщо статус "new"
}

// KitchenTicket — один тікет-замовлення (колонка KDS) зі всіма його завданнями.
type KitchenTicket struct {
	OrderID     int
	OrderNumber string // "№2", "№15" — без ведучих нулів, з префіксом №
	TableNumber int
	WaiterName  string
	CreatedAt   time.Time
	IsOverdue   bool // усі завдання "new" && очікування > 20 хв
	Tasks       []KitchenTaskView
}

type kitchenRepoIface interface {
	ProvisionCookingTasks(restaurantID int) error
	GetActiveKitchenTasks(restaurantID int) ([]chefrepo.KitchenTaskRow, error)
	StartCooking(taskID, chefID int) error
	FinishCooking(taskID int) error
}

type KitchenService struct {
	repo kitchenRepoIface
}

func NewKitchenService(repo kitchenRepoIface) *KitchenService {
	return &KitchenService{repo: repo}
}

// GetKitchenBoard повертає список тікетів для KDS-монітора.
// Спочатку провізіонує cooking_tasks для нових позицій, потім будує тікети.
func (s *KitchenService) GetKitchenBoard(restaurantID, chefID int) ([]KitchenTicket, error) {
	if err := s.repo.ProvisionCookingTasks(restaurantID); err != nil {
		kitchenSvcLog.Printf("GetKitchenBoard: ProvisionCookingTasks warning: %v", err)
	}
	rows, err := s.repo.GetActiveKitchenTasks(restaurantID)
	if err != nil {
		return nil, fmt.Errorf("GetKitchenBoard: %w", err)
	}
	tickets := buildTickets(rows, chefID)
	kitchenSvcLog.Printf("GetKitchenBoard: restaurantID=%d returned %d tickets", restaurantID, len(tickets))
	return tickets, nil
}

// GetKitchenBoardSnapshot повертає список тікетів без виклику ProvisionCookingTasks.
// Використовується після дій кухаря — провізія там не потрібна.
func (s *KitchenService) GetKitchenBoardSnapshot(restaurantID, chefID int) ([]KitchenTicket, error) {
	rows, err := s.repo.GetActiveKitchenTasks(restaurantID)
	if err != nil {
		return nil, fmt.Errorf("GetKitchenBoardSnapshot: %w", err)
	}
	return buildTickets(rows, chefID), nil
}

// buildTickets перетворює плоский список рядків репо на зрізи тікетів зі збереженням порядку.
func buildTickets(rows []chefrepo.KitchenTaskRow, chefID int) []KitchenTicket {
	var orderIDs []int
	ticketsMap := make(map[int]*KitchenTicket)
	for _, row := range rows {
		ticket, exists := ticketsMap[row.OrderID]
		if !exists {
			ticket = &KitchenTicket{
				OrderID:     row.OrderID,
				OrderNumber: formatOrderNumber(row.OrderNumber),
				TableNumber: row.TableNumber,
				WaiterName:  row.WaiterName,
				CreatedAt:   row.OrderCreatedAt,
			}
			ticketsMap[row.OrderID] = ticket
			orderIDs = append(orderIDs, row.OrderID)
		}
		ticket.Tasks = append(ticket.Tasks, mapTaskView(row))
	}
	result := make([]KitchenTicket, 0, len(orderIDs))
	for _, id := range orderIDs {
		t := ticketsMap[id]
		t.IsOverdue = isOverdue(t)
		sortTasks(t.Tasks, chefID)
		result = append(result, *t)
	}
	return result
}

// sortTasks сортує позиції тікета за пріоритетом для поточного кухаря:
// 0 — поточний кухар готує, 1 — інший кухар готує, 2 — нові, 3 — готові.
func sortTasks(tasks []KitchenTaskView, chefID int) {
	sort.SliceStable(tasks, func(i, j int) bool {
		return taskPriority(tasks[i], chefID) < taskPriority(tasks[j], chefID)
	})
}

func taskPriority(t KitchenTaskView, chefID int) int {
	switch {
	case t.Status == TaskStatusCooking && t.ChefID == chefID:
		return 0
	case t.Status == TaskStatusCooking:
		return 1
	case t.Status == TaskStatusNew:
		return 2
	default:
		return 3
	}
}

// mapTaskView конвертує рядок репозиторію у view-структуру для шаблону.
func mapTaskView(row chefrepo.KitchenTaskRow) KitchenTaskView {
	status := deriveStatus(row)

	var startTime *time.Time
	if row.StartTime.Valid {
		t := row.StartTime.Time
		startTime = &t
	}

	return KitchenTaskView{
		CookingTaskID: row.CookingTaskID,
		OrderItemID:   row.OrderItemID,
		DishName:      row.DishName,
		DishCategory:  row.DishCategory,
		CookingTime:   row.CookingTime,
		Qty:           row.EffectiveQty,
		Status:        status,
		StartTime:     startTime,
		ChefID:        int(row.ChefID.Int64),
		ChefName:      row.ChefName.String,
	}
}

// deriveStatus визначає статус завдання за наявністю start/end часу.
func deriveStatus(row chefrepo.KitchenTaskRow) TaskStatus {
	if !row.StartTime.Valid {
		return TaskStatusNew
	}
	if !row.EndTime.Valid {
		return TaskStatusCooking
	}
	return TaskStatusReady
}

// isOverdue повертає true, якщо всі завдання тікета ще не почато
// і замовлення очікує довше ніж overdueThreshold.
func isOverdue(t *KitchenTicket) bool {
	for _, task := range t.Tasks {
		if task.Status != TaskStatusNew {
			return false
		}
	}
	return time.Since(t.CreatedAt) > overdueThreshold
}

// StartCooking делегує старт приготування в репозиторій.
func (s *KitchenService) StartCooking(taskID, chefID int) error {
	if err := s.repo.StartCooking(taskID, chefID); err != nil {
		return fmt.Errorf("StartCooking: %w", err)
	}
	kitchenSvcLog.Printf("StartCooking: taskID=%d chefID=%d", taskID, chefID)
	return nil
}

// FinishCooking делегує завершення приготування в репозиторій.
func (s *KitchenService) FinishCooking(taskID int) error {
	if err := s.repo.FinishCooking(taskID); err != nil {
		return fmt.Errorf("FinishCooking: %w", err)
	}
	kitchenSvcLog.Printf("FinishCooking: taskID=%d", taskID)
	return nil
}

// formatOrderNumber перетворює "RES1-ORD-0002" → "№2".
// Беремо останній сегмент після дефісу, конвертуємо в int (знімає ведучі нулі),
// додаємо префікс №.
func formatOrderNumber(orderNumber string) string {
	parts := strings.Split(orderNumber, "-")
	n, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return "№" + parts[len(parts)-1]
	}
	return "№" + strconv.Itoa(n)
}
