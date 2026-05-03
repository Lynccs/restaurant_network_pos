package adminservice

import (
	"fmt"
	"time"

	adminrepo "restaurant_network_pos/internal/repository/admin"
)

type PurchasesFilters = adminrepo.PurchasesFilters
type BatchInput = adminrepo.BatchInput

type PurchaseBatch struct {
	BatchID           int
	Qty               float64
	ExpDate           time.Time
	ReceivedAt        time.Time
	RestaurantName    string
	RestaurantAddress string
	AdminName         string
	AdminID           int
}

type BatchEditData struct {
	BatchID       int
	DetailID      int
	IngredientID  int
	Ingredient    string
	Unit          string
	BatchQty      float64
	ExpDate       time.Time
	OrderQty      float64
	TotalReceived float64
	AdminID       int
	MaxAllowed    float64
}

type PurchaseItem struct {
	DetailID     int
	IngredientID int
	Name         string
	Qty          float64
	Unit         string
	Price        float64
	ReceivedQty  float64
	Batches      []PurchaseBatch
	BatchCount   int
}

type BatchesView struct {
	DetailID    int
	Unit        string
	OrderStatus string
	Batches     []PurchaseBatch
}

type PurchaseOrder struct {
	ID         int
	Number     string
	SupplierID int
	Supplier   string
	CreatedAt  time.Time
	ExpectedAt time.Time
	Status     string
	StatusID   int
	AdminID    int
	Initiator  string
	Total      float64
	Items      []PurchaseItem
}

type PurchasesPagination struct {
	Page        int
	TotalOrders int
	TotalPages  int
}

type SupplierOption struct {
	ID   int
	Name string
}

type StatusOption struct {
	ID   int
	Name string
}

type IngredientOption struct {
	ID       int
	Name     string
	UnitName string
}

type PurchasesPageView struct {
	Orders     []PurchaseOrder
	Pagination PurchasesPagination
	Filters    PurchasesFilters
	Suppliers  []SupplierOption
	Statuses   []StatusOption
	AdminID    int
}

type ItemDraftInput struct {
	IngredientID int
	Qty          float64
	Price        float64
}

type PurchasesServicer interface {
	GetPurchasesPage(restaurantID, adminID int, f PurchasesFilters) (*PurchasesPageView, error)
	GetPurchasesList(restaurantID, adminID int, f PurchasesFilters) (*PurchasesPageView, error)
	GetOrderDetails(orderID int) (PurchaseOrder, error)
	GetIngredients() ([]IngredientOption, error)
	GetSuppliers() ([]SupplierOption, error)
	CreateOrder(adminID, supplierID int, expectedAt time.Time) (int, error)
	CreateOrderWithItems(adminID, supplierID int, expectedAt time.Time, items []ItemDraftInput) error
	AddItem(orderID, ingredientID int, qty, price float64) error
	UpdateItem(orderID, detailID int, qty, price float64) error
	RemoveItem(orderID, itemID int) error
	MarkAsSent(orderID int) error
	ReceiveBatches(restaurantID, adminID int, batches []BatchInput) error
	GetDetailBatches(detailID int) (BatchesView, error)
	GetBatchEditData(batchID int) (BatchEditData, error)
	UpdateBatch(adminID, batchID int, qty float64, expDate time.Time) error
	CompleteOrder(orderID int) error
}

type PurchasesService struct {
	repo *adminrepo.PurchasesRepo
}

func NewPurchasesService(repo *adminrepo.PurchasesRepo) *PurchasesService {
	return &PurchasesService{repo: repo}
}

func buildOrders(rows []adminrepo.PurchaseOrderRow) []PurchaseOrder {
	type orderState struct {
		order   *PurchaseOrder
		itemIDs []int
		itemMap map[int]*PurchaseItem
	}

	states := make(map[int]*orderState)
	var orderIDs []int

	for _, row := range rows {
		st, exists := states[row.OrderID]
		if !exists {
			o := &PurchaseOrder{
				ID:         row.OrderID,
				Number:     row.OrderNumber,
				SupplierID: row.SupplierID,
				Supplier:   row.SupplierName,
				CreatedAt:  row.CreatedAt,
				ExpectedAt: row.ExpectedAt,
				Total:      row.TotalAmount,
				Status:     row.StatusName,
				StatusID:   row.StatusID,
				AdminID:    row.AdminID,
				Initiator:  row.AdminName,
			}
			st = &orderState{order: o, itemMap: make(map[int]*PurchaseItem)}
			states[row.OrderID] = st
			orderIDs = append(orderIDs, row.OrderID)
		}

		if !row.DetailID.Valid {
			continue
		}
		did := int(row.DetailID.Int64)
		item, ok := st.itemMap[did]
		if !ok {
			item = &PurchaseItem{
				DetailID:     did,
				IngredientID: int(row.IngredientID.Int64),
				Name:         row.IngredientName.String,
				Qty:          row.DetailQty.Float64,
				Unit:         row.UnitName.String,
				Price:        row.DetailPrice.Float64,
			}
			if row.DetailReceivedQty.Valid {
				item.ReceivedQty = row.DetailReceivedQty.Float64
			}
			if row.DetailBatchCount.Valid {
				item.BatchCount = int(row.DetailBatchCount.Int64)
			}
			st.itemMap[did] = item
			st.itemIDs = append(st.itemIDs, did)
		}

		if row.BatchID.Valid {
			item.Batches = append(item.Batches, PurchaseBatch{
				BatchID:           int(row.BatchID.Int64),
				Qty:               row.BatchQty.Float64,
				ExpDate:           row.BatchExpDate.Time,
				ReceivedAt:        row.BatchArrival.Time,
				RestaurantName:    row.BatchRestaurantName.String,
				RestaurantAddress: row.BatchRestaurantAddress.String,
				AdminName:         row.BatchAdminName.String,
				AdminID:           int(row.BatchAdminID.Int64),
			})
			item.ReceivedQty += row.BatchQty.Float64
			item.BatchCount++
		}
	}

	result := make([]PurchaseOrder, 0, len(orderIDs))
	for _, oid := range orderIDs {
		st := states[oid]
		items := make([]PurchaseItem, 0, len(st.itemIDs))
		for _, iid := range st.itemIDs {
			items = append(items, *st.itemMap[iid])
		}
		st.order.Items = items
		result = append(result, *st.order)
	}
	return result
}

func buildPagination(total, page int) PurchasesPagination {
	totalPages := (total + adminrepo.PurchasesPageSize - 1) / adminrepo.PurchasesPageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	return PurchasesPagination{Page: page, TotalOrders: total, TotalPages: totalPages}
}

func (s *PurchasesService) GetPurchasesPage(restaurantID, adminID int, f PurchasesFilters) (*PurchasesPageView, error) {
	rows, total, err := s.repo.GetPurchasesData(restaurantID, adminID, f)
	if err != nil {
		return nil, fmt.Errorf("GetPurchasesPage: %w", err)
	}

	suppliers, err := s.repo.GetSuppliers()
	if err != nil {
		return nil, fmt.Errorf("GetPurchasesPage suppliers: %w", err)
	}

	statuses, err := s.repo.GetStatuses(f.Archive)
	if err != nil {
		return nil, fmt.Errorf("GetPurchasesPage statuses: %w", err)
	}

	supplierOpts := make([]SupplierOption, len(suppliers))
	for i, s := range suppliers {
		supplierOpts[i] = SupplierOption{ID: s.ID, Name: s.Name}
	}
	statusOpts := make([]StatusOption, len(statuses))
	for i, s := range statuses {
		statusOpts[i] = StatusOption{ID: s.ID, Name: s.Name}
	}

	return &PurchasesPageView{
		Orders:     buildOrders(rows),
		Pagination: buildPagination(total, f.Page),
		Filters:    f,
		Suppliers:  supplierOpts,
		Statuses:   statusOpts,
		AdminID:    adminID,
	}, nil
}

func (s *PurchasesService) GetPurchasesList(restaurantID, adminID int, f PurchasesFilters) (*PurchasesPageView, error) {
	rows, total, err := s.repo.GetPurchasesData(restaurantID, adminID, f)
	if err != nil {
		return nil, fmt.Errorf("GetPurchasesList: %w", err)
	}
	return &PurchasesPageView{
		Orders:     buildOrders(rows),
		Pagination: buildPagination(total, f.Page),
		Filters:    f,
		AdminID:    adminID,
	}, nil
}

func (s *PurchasesService) GetOrderDetails(orderID int) (PurchaseOrder, error) {
	rows, err := s.repo.GetOrderDetails(orderID)
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("GetOrderDetails: %w", err)
	}
	if len(rows) == 0 {
		return PurchaseOrder{}, fmt.Errorf("order %d not found", orderID)
	}
	orders := buildOrders(rows)
	if len(orders) == 0 {
		return PurchaseOrder{}, fmt.Errorf("order %d not found after build", orderID)
	}
	return orders[0], nil
}

func (s *PurchasesService) GetIngredients() ([]IngredientOption, error) {
	rows, err := s.repo.GetIngredients()
	if err != nil {
		return nil, fmt.Errorf("GetIngredients: %w", err)
	}
	result := make([]IngredientOption, len(rows))
	for i, r := range rows {
		result[i] = IngredientOption{ID: r.ID, Name: r.Name, UnitName: r.UnitName}
	}
	return result, nil
}

func (s *PurchasesService) GetSuppliers() ([]SupplierOption, error) {
	rows, err := s.repo.GetSuppliers()
	if err != nil {
		return nil, err
	}
	result := make([]SupplierOption, len(rows))
	for i, r := range rows {
		result[i] = SupplierOption{ID: r.ID, Name: r.Name}
	}
	return result, nil
}

func (s *PurchasesService) CreateOrder(adminID, supplierID int, expectedAt time.Time) (int, error) {
	return s.repo.CreateOrder(adminID, supplierID, expectedAt)
}

func (s *PurchasesService) CreateOrderWithItems(adminID, supplierID int, expectedAt time.Time, items []ItemDraftInput) error {
	repoItems := make([]adminrepo.OrderItemInput, len(items))
	for i, item := range items {
		repoItems[i] = adminrepo.OrderItemInput{
			IngredientID: item.IngredientID,
			Qty:          item.Qty,
			Price:        item.Price,
		}
	}
	return s.repo.CreateOrderWithItems(adminID, supplierID, expectedAt, repoItems)
}

func (s *PurchasesService) AddItem(orderID, ingredientID int, qty, price float64) error {
	return s.repo.AddOrderItem(orderID, ingredientID, qty, price)
}

func (s *PurchasesService) UpdateItem(orderID, detailID int, qty, price float64) error {
	return s.repo.UpdateOrderItem(orderID, detailID, qty, price)
}

func (s *PurchasesService) RemoveItem(orderID, itemID int) error {
	return s.repo.RemoveOrderItem(orderID, itemID)
}

func (s *PurchasesService) MarkAsSent(orderID int) error {
	return s.repo.UpdateOrderStatus(orderID, "Відправлено")
}

func (s *PurchasesService) ReceiveBatches(restaurantID, adminID int, batches []BatchInput) error {
	return s.repo.ReceiveBatches(restaurantID, adminID, batches)
}

func (s *PurchasesService) GetDetailBatches(detailID int) (BatchesView, error) {
	rows, err := s.repo.GetDetailBatches(detailID)
	if err != nil {
		return BatchesView{}, err
	}
	if len(rows) == 0 {
		return BatchesView{DetailID: detailID}, nil
	}
	view := BatchesView{
		DetailID:    rows[0].DetailID,
		Unit:        rows[0].UnitName,
		OrderStatus: rows[0].OrderStatus,
	}
	for _, row := range rows {
		if !row.BatchID.Valid {
			continue
		}
		view.Batches = append(view.Batches, PurchaseBatch{
			BatchID:           int(row.BatchID.Int64),
			Qty:               row.BatchQty.Float64,
			ExpDate:           row.BatchExpDate.Time,
			ReceivedAt:        row.BatchArrival.Time,
			RestaurantName:    row.BatchRestaurantName.String,
			RestaurantAddress: row.BatchRestaurantAddress.String,
			AdminName:         row.BatchAdminName.String,
			AdminID:           int(row.BatchAdminID.Int64),
		})
	}
	return view, nil
}

func (s *PurchasesService) GetBatchEditData(batchID int) (BatchEditData, error) {
	row, err := s.repo.GetBatchEditData(batchID)
	if err != nil {
		return BatchEditData{}, err
	}
	return BatchEditData{
		BatchID:       row.BatchID,
		DetailID:      row.DetailID,
		IngredientID:  row.IngredientID,
		Ingredient:    row.Ingredient,
		Unit:          row.Unit,
		BatchQty:      row.BatchQty,
		ExpDate:       row.BatchExpDate,
		OrderQty:      row.OrderQty,
		TotalReceived: row.TotalReceived,
		AdminID:       row.BatchAdminID,
	}, nil
}

func (s *PurchasesService) UpdateBatch(adminID, batchID int, qty float64, expDate time.Time) error {
	return s.repo.UpdateBatch(batchID, adminID, qty, expDate)
}

func (s *PurchasesService) CompleteOrder(orderID int) error {
	return s.repo.UpdateOrderStatus(orderID, "Отримано")
}
