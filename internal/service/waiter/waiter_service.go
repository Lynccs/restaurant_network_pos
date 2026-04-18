package waiterservice

import (
	"log"

	waiterrepo "restaurant_network_pos/internal/repository/waiter"
)

var svcLog = log.New(log.Writer(), "[WaiterService] ", log.LstdFlags|log.Lshortfile)

type TableView struct {
	ID       int
	Number   int
	Capacity int
	Status   string // "free" | "occupied"
	SeatTime string // "14:20" or ""
}

type WaiterRepo interface {
	GetTablesByRestaurant(restaurantID int) ([]waiterrepo.TableRow, error)
	GetRestaurantName(restaurantID int) (string, error)
}

type WaiterService struct {
	repo WaiterRepo
}

func NewWaiterService(repo WaiterRepo) *WaiterService {
	return &WaiterService{repo: repo}
}

func (s *WaiterService) GetTables(restaurantID int) ([]TableView, error) {
	svcLog.Printf("GetTables: restaurantID=%d", restaurantID)

	rows, err := s.repo.GetTablesByRestaurant(restaurantID)
	if err != nil {
		svcLog.Printf("GetTables: repo error: %v", err)
		return nil, err
	}

	views := make([]TableView, 0, len(rows))
	for _, r := range rows {
		v := TableView{
			ID:       r.ID,
			Number:   r.Number,
			Capacity: r.Capacity,
			Status:   "free",
		}
		if r.HasActiveOrder {
			v.Status = "occupied"
			if r.OrderCreatedAt.Valid {
				v.SeatTime = r.OrderCreatedAt.Time.Format("15:04")
			}
		}
		views = append(views, v)
	}

	svcLog.Printf("GetTables: returning %d tables", len(views))
	return views, nil
}

func (s *WaiterService) GetRestaurantName(restaurantID int) (string, error) {
	svcLog.Printf("GetRestaurantName: restaurantID=%d", restaurantID)
	return s.repo.GetRestaurantName(restaurantID)
}
