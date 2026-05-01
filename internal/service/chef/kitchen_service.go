package chefservice

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// IngredientView — інгредієнт для модального вікна "Почати приготування".
type IngredientView struct {
	ID        int
	Name      string
	Unit      string
	RecipeQty float64 // > 0 якщо входить до рецепту; 0 для "інших"
	StockQty  float64
}

// StartCookingView — дані для модального вікна "Почати приготування".
type StartCookingView struct {
	OrderItemID int
	DishName    string
	Qty         int
	Recipe      []IngredientView
	Others      []IngredientView
}

// ChefInfo — кухар для фільтра KDS.
type ChefInfo struct {
	ID   int
	Name string
}

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
	EndTime       *time.Time // не nil тільки для TaskStatusReady (архівний перегляд)
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
	GetActiveKitchenTasks(restaurantID int) ([]chefrepo.KitchenTaskRow, error)
	GetReadyTasksByDate(restaurantID int, date time.Time) ([]chefrepo.KitchenTaskRow, error)
	GetAllChefs(restaurantID int) ([]chefrepo.ChefRow, error)
	GetOrderItemInfo(orderItemID int) (dishName string, qty int, err error)
	GetStartCookingData(orderItemID, restaurantID int) (dishName string, qty int, recipe []chefrepo.IngredientModalRow, others []chefrepo.IngredientModalRow, err error)
	StartCooking(taskID, chefID int) error
	FinishCooking(taskID int) error
	RecordIngredientUsages(orderItemID, restaurantID int, usages map[int]float64) error
	ReportIssue(orderItemID int) (dishName string, tableNumber int, qty int, err error)
	GetWriteOffData(restaurantID int, f chefrepo.WriteOffFilters, page int) ([]chefrepo.WriteOffRow, int, error)
	GetWriteOffOptions(restaurantID int) (dishes []string, ingredients []string, err error)
}

type writeOffOptionsCache struct {
	dishes      []string
	ingredients []string
	fetchedAt   time.Time
}

type KitchenService struct {
	repo         kitchenRepoIface
	optionsCache map[int]*writeOffOptionsCache // ключ — restaurantID
	optionsMu    sync.Mutex
}

const writeOffOptionsTTL = 5 * time.Minute

func NewKitchenService(repo kitchenRepoIface) *KitchenService {
	return &KitchenService{
		repo:         repo,
		optionsCache: make(map[int]*writeOffOptionsCache),
	}
}

// GetReadyBoard повертає список тікетів з готовими стравами за вказану дату (архів).
func (s *KitchenService) GetReadyBoard(restaurantID int, date time.Time) ([]KitchenTicket, error) {
	rows, err := s.repo.GetReadyTasksByDate(restaurantID, date)
	if err != nil {
		return nil, fmt.Errorf("GetReadyBoard: %w", err)
	}
	return buildTickets(rows, 0), nil
}

// GetKitchenBoard повертає список тікетів для KDS-монітора.
// Джерелом є активні order_items; cooking_tasks лише уточнює статус.
func (s *KitchenService) GetKitchenBoard(restaurantID, chefID int) ([]KitchenTicket, error) {
	rows, err := s.repo.GetActiveKitchenTasks(restaurantID)
	if err != nil {
		return nil, fmt.Errorf("GetKitchenBoard: %w", err)
	}
	tickets := buildTickets(rows, chefID)
	kitchenSvcLog.Printf("GetKitchenBoard: restaurantID=%d returned %d tickets", restaurantID, len(tickets))
	return tickets, nil
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

	var endTime *time.Time
	if row.EndTime.Valid {
		t := row.EndTime.Time
		endTime = &t
	}

	cookingTaskID := row.OrderItemID
	if row.CookingTaskID.Valid {
		cookingTaskID = int(row.CookingTaskID.Int64)
	}

	return KitchenTaskView{
		CookingTaskID: cookingTaskID,
		OrderItemID:   row.OrderItemID,
		DishName:      row.DishName,
		DishCategory:  row.DishCategory,
		CookingTime:   row.CookingTime,
		Qty:           row.EffectiveQty,
		Status:        status,
		StartTime:     startTime,
		EndTime:       endTime,
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

// GetStartCookingData повертає дані для модального вікна "Почати приготування".
func (s *KitchenService) GetStartCookingData(orderItemID, restaurantID int) (*StartCookingView, error) {
	dishName, qty, recipe, others, err := s.repo.GetStartCookingData(orderItemID, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("GetStartCookingData: %w", err)
	}
	view := &StartCookingView{
		OrderItemID: orderItemID,
		DishName:    dishName,
		Qty:         qty,
		Recipe:      make([]IngredientView, len(recipe)),
		Others:      make([]IngredientView, len(others)),
	}
	for i, r := range recipe {
		view.Recipe[i] = IngredientView{ID: r.IngredientID, Name: r.Name, Unit: r.Unit, RecipeQty: r.RecipeQty, StockQty: r.StockQty}
	}
	for i, r := range others {
		view.Others[i] = IngredientView{ID: r.IngredientID, Name: r.Name, Unit: r.Unit, RecipeQty: 0, StockQty: r.StockQty}
	}
	return view, nil
}

// RecordIngredientUsages делегує запис використаних інгредієнтів у репозиторій.
func (s *KitchenService) RecordIngredientUsages(orderItemID, restaurantID int, usages map[int]float64) error {
	if err := s.repo.RecordIngredientUsages(orderItemID, restaurantID, usages); err != nil {
		return fmt.Errorf("RecordIngredientUsages: %w", err)
	}
	return nil
}

// GetAllChefs повертає всіх кухарів ресторану для фільтра KDS.
func (s *KitchenService) GetAllChefs(restaurantID int) ([]ChefInfo, error) {
	rows, err := s.repo.GetAllChefs(restaurantID)
	if err != nil {
		return nil, fmt.Errorf("GetAllChefs: %w", err)
	}
	chefs := make([]ChefInfo, len(rows))
	for i, r := range rows {
		chefs[i] = ChefInfo{ID: r.ID, Name: r.Name}
	}
	return chefs, nil
}

// GetOrderItemInfo повертає назву страви та ефективну кількість для одного order_item.
func (s *KitchenService) GetOrderItemInfo(orderItemID int) (string, int, error) {
	return s.repo.GetOrderItemInfo(orderItemID)
}

// StartCooking делегує старт приготування в репозиторій.
func (s *KitchenService) StartCooking(orderItemID, chefID int) error {
	if err := s.repo.StartCooking(orderItemID, chefID); err != nil {
		return fmt.Errorf("StartCooking: %w", err)
	}
	kitchenSvcLog.Printf("StartCooking: orderItemID=%d chefID=%d", orderItemID, chefID)
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

// ReportIssue фіксує нестачу інгредієнтів для позиції та повертає JSON-пейлоад для SSE.
func (s *KitchenService) ReportIssue(orderItemID int) (string, error) {
	dishName, tableNumber, qty, err := s.repo.ReportIssue(orderItemID)
	if err != nil {
		return "", fmt.Errorf("ReportIssue: %w", err)
	}
	payload := fmt.Sprintf(`{"dishName":%q,"tableNumber":%d,"qty":%d}`, dishName, tableNumber, qty)
	kitchenSvcLog.Printf("ReportIssue: orderItemID=%d table=%d dish=%s", orderItemID, tableNumber, dishName)
	return payload, nil
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
