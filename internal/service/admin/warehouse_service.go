package adminservice

import (
	"fmt"
	"time"

	adminrepo "restaurant_network_pos/internal/repository/admin"
)

type WarehouseFilters = adminrepo.WarehouseFilters

type WarehouseBatch struct {
	StockID           int
	Ingredient        string
	Brand             string
	Qty               float64
	Unit              string
	StorageZone       string
	ReceivedAt        time.Time
	ExpDate           time.Time
	StorageCondition  string
	RestaurantName    string
	RestaurantAddress string
	Supplier          string
	StatusCode        string
	StatusLabel       string
	DaysLeft          int
}

type WarehouseSummary struct {
	TotalRows         int
	UniqueIngredients int
	ExpiringSoon      int
	Expired           int
}

type WarehousePageView struct {
	RestaurantName      string
	RestaurantAddress   string
	DefaultRestaurantID int
	Items               []WarehouseBatch
	Filters             WarehouseFilters
	IngredientOptions   []string
	RestaurantOptions   []adminrepo.WarehouseRestaurantOption
	Pagination          PurchasesPagination
}

func warehouseStatus(expDate time.Time) (code, label string, daysLeft int) {
	now := time.Now()
	startToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startExp := time.Date(expDate.Year(), expDate.Month(), expDate.Day(), 0, 0, 0, 0, expDate.Location())
	daysLeft = int(startExp.Sub(startToday).Hours() / 24)

	switch {
	case expDate.Before(now):
		return "expired", "Прострочено", daysLeft
	case daysLeft <= 3:
		return "expiring", "Скоро спливає", daysLeft
	default:
		return "normal", "Норма", daysLeft
	}
}

func (s *PurchasesService) GetWarehousePage(restaurantID int, f WarehouseFilters) (*WarehousePageView, error) {
	rows, total, err := s.repo.GetWarehouseData(f)
	if err != nil {
		return nil, fmt.Errorf("GetWarehousePage data: %w", err)
	}

	selectedRestaurantID := f.RestaurantID
	if selectedRestaurantID == 0 {
		selectedRestaurantID = restaurantID
	}

	ingredientOptions, err := s.repo.GetWarehouseIngredientOptions(selectedRestaurantID)
	if err != nil {
		return nil, fmt.Errorf("GetWarehousePage ingredient options: %w", err)
	}
	restaurantOptions, err := s.repo.GetWarehouseRestaurantOptions()
	if err != nil {
		return nil, fmt.Errorf("GetWarehousePage restaurant options: %w", err)
	}

	items := make([]WarehouseBatch, 0, len(rows))
	restaurantName := ""
	restaurantAddress := ""

	for _, row := range rows {
		statusCode, statusLabel, daysLeft := warehouseStatus(row.ExpDate)
		item := WarehouseBatch{
			StockID:           row.StockID,
			Ingredient:        row.IngredientName,
			Brand:             row.IngredientBrand.String,
			Qty:               row.Qty,
			Unit:              row.UnitName,
			StorageZone:       row.StorageCondition.String,
			ReceivedAt:        row.ReceivedAt,
			ExpDate:           row.ExpDate,
			StorageCondition:  row.StorageCondition.String,
			RestaurantName:    row.RestaurantName,
			RestaurantAddress: row.RestaurantAddress.String,
			Supplier:          "—",
			StatusCode:        statusCode,
			StatusLabel:       statusLabel,
			DaysLeft:          daysLeft,
		}
		if row.SupplierName.Valid && row.SupplierName.String != "" {
			item.Supplier = row.SupplierName.String
		}

		items = append(items, item)
		if restaurantName == "" {
			restaurantName = row.RestaurantName
		}
		if restaurantAddress == "" {
			restaurantAddress = row.RestaurantAddress.String
		}
	}
	if restaurantName == "" && selectedRestaurantID > 0 {
		for _, option := range restaurantOptions {
			if option.ID == selectedRestaurantID {
				restaurantName = option.Name
				restaurantAddress = option.Address
				break
			}
		}
	}

	return &WarehousePageView{
		RestaurantName:    restaurantName,
		RestaurantAddress: restaurantAddress,
		Items:             items,
		Filters:           f,
		IngredientOptions: ingredientOptions,
		RestaurantOptions: restaurantOptions,
		Pagination:        buildPagination(total, f.Page),
	}, nil
}
