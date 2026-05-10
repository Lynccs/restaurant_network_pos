package waiterservice

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	waiterrepo "restaurant_network_pos/internal/repository/waiter"
)

var ordersSvcLog = log.New(log.Writer(), "[OrdersService] ", log.LstdFlags|log.Lshortfile)

type OrderItemView struct {
	DishName  string
	DishPrice float64
	Qty       int
	Subtotal  float64
	HasIssue  bool
}

type OrderView struct {
	OrderID     int
	OrderNumber string
	TableNumber int
	WaiterName  string
	TotalAmount float64
	CreatedAt   string // formatted "2006-01-02 15:04"
	StatusName  string
	HasIssue    bool // placeholder: always false — wired to kitchen notifications later
	Items       []OrderItemView
}

type ordersRepoIface interface {
	GetActiveOrdersList(restaurantID int, f waiterrepo.OrderListFilters) ([]waiterrepo.OrderListRow, error)
	GetArchiveOrdersList(restaurantID, waiterID int, f waiterrepo.ArchiveFilters) ([]waiterrepo.OrderListRow, error)
	CancelOrder(orderID, restaurantID int) error
	PayOrder(orderID, restaurantID int, paymentMethod string) error
	RejectPayment(orderID, restaurantID int, paymentMethod string) error
}

type OrdersService struct {
	repo ordersRepoIface
}

func NewOrdersService(repo ordersRepoIface) *OrdersService {
	return &OrdersService{repo: repo}
}

func (s *OrdersService) GetActiveOrders(restaurantID int, search, statusName string, tableNumber int, timeFrom, timeTo string) ([]OrderView, error) {
	rows, err := s.repo.GetActiveOrdersList(restaurantID, waiterrepo.OrderListFilters{
		Search:      search,
		StatusName:  statusName,
		TableNumber: tableNumber,
		TimeFrom:    timeFrom,
		TimeTo:      timeTo,
	})
	if err != nil {
		return nil, fmt.Errorf("GetActiveOrders: %w", err)
	}

	var orders []OrderView
	seen := make(map[int]int) // orderID → index in orders slice

	for _, row := range rows {
		idx, exists := seen[row.OrderID]
		if !exists {
			parts := strings.Split(row.OrderNumber, "-")
			n, _ := strconv.Atoi(parts[len(parts)-1])
			shortNum := strconv.Itoa(n)
			orders = append(orders, OrderView{
				OrderID:     row.OrderID,
				OrderNumber: shortNum,
				TableNumber: row.TableNumber,
				WaiterName:  row.WaiterName,
				TotalAmount: row.TotalAmount,
				CreatedAt:   row.CreatedAt.Format("2006-01-02 15:04"),
				StatusName:  row.StatusName,
				HasIssue:    row.HasIssue,
			})
			idx = len(orders) - 1
			seen[row.OrderID] = idx
		} else if row.HasIssue {
			orders[idx].HasIssue = true
		}
		orders[idx].Items = append(orders[idx].Items, OrderItemView{
			DishName:  row.DishName,
			DishPrice: row.DishPrice,
			Qty:       row.ItemQty,
			Subtotal:  row.DishPrice * float64(row.ItemQty),
			HasIssue:  row.ItemHasIssue,
		})
	}

	ordersSvcLog.Printf("GetActiveOrders: restaurantID=%d returned %d orders", restaurantID, len(orders))
	return orders, nil
}

func (s *OrdersService) GetArchiveOrders(restaurantID, waiterID int, search, statusName string, tableNumber int, dateFrom, dateTo string) ([]OrderView, error) {
	rows, err := s.repo.GetArchiveOrdersList(restaurantID, waiterID, waiterrepo.ArchiveFilters{
		Search:      search,
		StatusName:  statusName,
		TableNumber: tableNumber,
		DateFrom:    dateFrom,
		DateTo:      dateTo,
	})
	if err != nil {
		return nil, fmt.Errorf("GetArchiveOrders: %w", err)
	}

	var orders []OrderView
	seen := make(map[int]int)

	for _, row := range rows {
		idx, exists := seen[row.OrderID]
		if !exists {
			parts := strings.Split(row.OrderNumber, "-")
			n, _ := strconv.Atoi(parts[len(parts)-1])
			orders = append(orders, OrderView{
				OrderID:     row.OrderID,
				OrderNumber: "№" + strconv.Itoa(n),
				TableNumber: row.TableNumber,
				WaiterName:  row.WaiterName,
				TotalAmount: row.TotalAmount,
				CreatedAt:   row.CreatedAt.Format("2006-01-02 15:04"),
				StatusName:  row.StatusName,
				HasIssue:    false,
			})
			idx = len(orders) - 1
			seen[row.OrderID] = idx
		}
		orders[idx].Items = append(orders[idx].Items, OrderItemView{
			DishName:  row.DishName,
			DishPrice: row.DishPrice,
			Qty:       row.ItemQty,
			Subtotal:  row.DishPrice * float64(row.ItemQty),
		})
	}

	ordersSvcLog.Printf("GetArchiveOrders: restaurantID=%d waiterID=%d returned %d orders", restaurantID, waiterID, len(orders))
	return orders, nil
}

func (s *OrdersService) CancelOrder(orderID, restaurantID int) error {
	return s.repo.CancelOrder(orderID, restaurantID)
}

func (s *OrdersService) PayOrder(orderID, restaurantID int, paymentMethod string) error {
	return s.repo.PayOrder(orderID, restaurantID, paymentMethod)
}

func (s *OrdersService) RejectPayment(orderID, restaurantID int, paymentMethod string) error {
	return s.repo.RejectPayment(orderID, restaurantID, paymentMethod)
}
