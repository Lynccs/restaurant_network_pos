package adminrepo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const PurchasesPageSize = 10

type PurchasesFilters struct {
	Ownership   string // "all" | "mine"
	SupplierID  int    // 0 = all
	CreatedFrom string // "YYYY-MM-DD"
	CreatedTo   string // "YYYY-MM-DD"
	MinTotal    string // "" = no filter
	MaxTotal    string // "" = no filter
	StatusID    int    // 0 = all (within tab)
	Archive     bool   // true = received; false = created+sent
	Page        int
}

type PurchaseOrderRow struct {
	OrderID                int
	OrderNumber            string
	CreatedAt              time.Time
	ExpectedAt             time.Time
	TotalAmount            float64
	StatusID               int
	StatusName             string
	AdminID                int
	AdminName              string
	SupplierID             int
	SupplierName           string
	TotalCount             int
	DetailID               sql.NullInt64
	DetailQty              sql.NullFloat64
	DetailPrice            sql.NullFloat64
	IngredientID           sql.NullInt64
	IngredientName         sql.NullString
	UnitName               sql.NullString
	DetailReceivedQty      sql.NullFloat64
	DetailBatchCount       sql.NullInt64
	BatchID                sql.NullInt64
	BatchQty               sql.NullFloat64
	BatchExpDate           sql.NullTime
	BatchArrival           sql.NullTime
	BatchRestaurantName    sql.NullString
	BatchRestaurantAddress sql.NullString
	BatchAdminName         sql.NullString
	BatchAdminID           sql.NullInt64
}

type BatchEditRow struct {
	BatchID       int
	BatchQty      float64
	BatchExpDate  time.Time
	BatchAdminID  int
	DetailID      int
	IngredientID  int
	OrderQty      float64
	Ingredient    string
	Unit          string
	TotalReceived float64
}

type DetailBatchRow struct {
	DetailID               int
	UnitName               string
	OrderStatus            string
	BatchID                sql.NullInt64
	BatchQty               sql.NullFloat64
	BatchExpDate           sql.NullTime
	BatchArrival           sql.NullTime
	BatchRestaurantName    sql.NullString
	BatchRestaurantAddress sql.NullString
	BatchAdminName         sql.NullString
	BatchAdminID           sql.NullInt64
}

type SupplierRow struct {
	ID   int
	Name string
}

type StatusRow struct {
	ID   int
	Name string
}

type IngredientRow struct {
	ID       int
	Name     string
	UnitID   int
	UnitName string
}

type PurchasesRepo struct {
	db *sql.DB
}

func NewPurchasesRepo(db *sql.DB) *PurchasesRepo {
	return &PurchasesRepo{db: db}
}

func (r *PurchasesRepo) GetPurchasesData(restaurantID, adminID int, f PurchasesFilters) ([]PurchaseOrderRow, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	offset := (f.Page - 1) * PurchasesPageSize

	args := []any{
		sql.Named("offset", offset),
		sql.Named("pageSize", PurchasesPageSize),
	}

	var where strings.Builder
	if f.Archive {
		where.WriteString("AND ios.ingredient_order_status_name = 'Отримано'\n")
	} else {
		where.WriteString("AND ios.ingredient_order_status_name IN ('Створено', 'Відправлено')\n")
	}
	if f.SupplierID > 0 {
		where.WriteString("AND io.supplier_id = @supplierID\n")
		args = append(args, sql.Named("supplierID", f.SupplierID))
	}
	if f.CreatedFrom != "" {
		where.WriteString("AND CAST(io.ingredient_order_created_at AS DATE) >= @createdFrom\n")
		args = append(args, sql.Named("createdFrom", f.CreatedFrom))
	}
	if f.CreatedTo != "" {
		where.WriteString("AND CAST(io.ingredient_order_created_at AS DATE) <= @createdTo\n")
		args = append(args, sql.Named("createdTo", f.CreatedTo))
	}
	if f.MinTotal != "" {
		where.WriteString("AND io.ingredient_order_total_amount >= @minTotal\n")
		args = append(args, sql.Named("minTotal", f.MinTotal))
	}
	if f.MaxTotal != "" {
		where.WriteString("AND io.ingredient_order_total_amount <= @maxTotal\n")
		args = append(args, sql.Named("maxTotal", f.MaxTotal))
	}
	if f.StatusID > 0 {
		where.WriteString("AND io.ingredient_order_status_id = @statusID\n")
		args = append(args, sql.Named("statusID", f.StatusID))
	}
	if f.Ownership == "mine" {
		where.WriteString("AND io.administrator_id = @adminID\n")
		args = append(args, sql.Named("adminID", adminID))
	}

	query := fmt.Sprintf(`
WITH base AS (
    SELECT
        io.ingredient_order_id,
        io.ingredient_order_number,
        io.ingredient_order_created_at,
        io.ingredient_order_expected_at,
        io.ingredient_order_total_amount,
        io.ingredient_order_status_id,
        ios.ingredient_order_status_name,
        a.administrator_id,
        a.administrator_full_name,
        io.supplier_id,
        s.supplier_company_name,
        COUNT(*) OVER () AS total_count,
        ROW_NUMBER() OVER (ORDER BY io.ingredient_order_created_at DESC) AS rn
    FROM ingredient_orders io
    JOIN administrators a ON a.administrator_id = io.administrator_id
    JOIN suppliers s ON s.supplier_id = io.supplier_id
    JOIN ingredient_order_statuses ios ON ios.ingredient_order_status_id = io.ingredient_order_status_id
	WHERE 1=1
	%s
),
paged AS (
    SELECT * FROM base WHERE rn BETWEEN @offset + 1 AND @offset + @pageSize
)
SELECT
    p.ingredient_order_id,
    p.ingredient_order_number,
    p.ingredient_order_created_at,
    p.ingredient_order_expected_at,
    p.ingredient_order_total_amount,
    p.ingredient_order_status_id,
    p.ingredient_order_status_name,
    p.administrator_id,
    p.administrator_full_name,
    p.supplier_id,
    p.supplier_company_name,
    p.total_count,
    iod.ingredient_order_detail_id,
    iod.detail_quantity,
    iod.detail_purchase_price,
    i.ingredient_id,
    i.ingredient_name,
    iu.ingredient_unit_name,
	pbx.received_qty,
	pbx.batch_count,
	NULL AS product_batch_id,
	NULL AS product_batch_accepted_quantity,
	NULL AS product_batch_expiration_date,
	NULL AS product_batch_arrival_date,
	NULL AS restaurant_name,
	NULL AS restaurant_address,
	NULL AS administrator_full_name,
	NULL AS administrator_id
FROM paged p
LEFT JOIN ingredient_order_details iod ON iod.ingredient_order_id = p.ingredient_order_id
LEFT JOIN ingredients i ON i.ingredient_id = iod.ingredient_id
LEFT JOIN ingredient_units iu ON iu.ingredient_unit_id = i.ingredient_unit_id
LEFT JOIN (
	SELECT
		ingredient_order_detail_id,
		SUM(product_batch_accepted_quantity) AS received_qty,
		COUNT(*) AS batch_count
	FROM product_batches
	GROUP BY ingredient_order_detail_id
) pbx ON pbx.ingredient_order_detail_id = iod.ingredient_order_detail_id
ORDER BY p.rn, iod.ingredient_order_detail_id
OPTION (RECOMPILE)`, where.String())

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("GetPurchasesData query: %w", err)
	}
	defer rows.Close()

	var result []PurchaseOrderRow
	totalCount := 0
	for rows.Next() {
		var row PurchaseOrderRow
		if err := rows.Scan(
			&row.OrderID, &row.OrderNumber, &row.CreatedAt, &row.ExpectedAt,
			&row.TotalAmount, &row.StatusID, &row.StatusName,
			&row.AdminID, &row.AdminName,
			&row.SupplierID, &row.SupplierName,
			&row.TotalCount,
			&row.DetailID, &row.DetailQty, &row.DetailPrice,
			&row.IngredientID, &row.IngredientName, &row.UnitName,
			&row.DetailReceivedQty, &row.DetailBatchCount,
			&row.BatchID, &row.BatchQty, &row.BatchExpDate, &row.BatchArrival,
			&row.BatchRestaurantName, &row.BatchRestaurantAddress, &row.BatchAdminName, &row.BatchAdminID,
		); err != nil {
			return nil, 0, fmt.Errorf("GetPurchasesData scan: %w", err)
		}
		if row.TotalCount > totalCount {
			totalCount = row.TotalCount
		}
		result = append(result, row)
	}
	return result, totalCount, rows.Err()
}

func (r *PurchasesRepo) GetOrderDetails(orderID int) ([]PurchaseOrderRow, error) {
	const query = `
SELECT
    io.ingredient_order_id,
    io.ingredient_order_number,
    io.ingredient_order_created_at,
    io.ingredient_order_expected_at,
    io.ingredient_order_total_amount,
    io.ingredient_order_status_id,
    ios.ingredient_order_status_name,
    a.administrator_id,
    a.administrator_full_name,
    io.supplier_id,
    s.supplier_company_name,
    0 AS total_count,
    iod.ingredient_order_detail_id,
    iod.detail_quantity,
    iod.detail_purchase_price,
    i.ingredient_id,
    i.ingredient_name,
    iu.ingredient_unit_name,
	NULL AS received_qty,
	NULL AS batch_count,
    pb.product_batch_id,
    pb.product_batch_accepted_quantity,
    pb.product_batch_expiration_date,
	pb.product_batch_arrival_date,
	r.restaurant_name,
	r.restaurant_address,
	a2.administrator_full_name,
	a2.administrator_id
FROM ingredient_orders io
JOIN administrators a ON a.administrator_id = io.administrator_id
JOIN suppliers s ON s.supplier_id = io.supplier_id
JOIN ingredient_order_statuses ios ON ios.ingredient_order_status_id = io.ingredient_order_status_id
LEFT JOIN ingredient_order_details iod ON iod.ingredient_order_id = io.ingredient_order_id
LEFT JOIN ingredients i ON i.ingredient_id = iod.ingredient_id
LEFT JOIN ingredient_units iu ON iu.ingredient_unit_id = i.ingredient_unit_id
LEFT JOIN product_batches pb ON pb.ingredient_order_detail_id = iod.ingredient_order_detail_id
LEFT JOIN stock_ingredients si ON si.stock_ingredient_id = pb.stock_ingredient_id
LEFT JOIN restaurants r ON r.restaurant_id = si.restaurant_id
LEFT JOIN administrators a2 ON a2.administrator_id = pb.administrator_id
WHERE io.ingredient_order_id = @orderID
ORDER BY iod.ingredient_order_detail_id, pb.product_batch_arrival_date DESC, pb.product_batch_id DESC`

	rows, err := r.db.Query(query, sql.Named("orderID", orderID))
	if err != nil {
		return nil, fmt.Errorf("GetOrderDetails query: %w", err)
	}
	defer rows.Close()

	var result []PurchaseOrderRow
	for rows.Next() {
		var row PurchaseOrderRow
		if err := rows.Scan(
			&row.OrderID, &row.OrderNumber, &row.CreatedAt, &row.ExpectedAt,
			&row.TotalAmount, &row.StatusID, &row.StatusName,
			&row.AdminID, &row.AdminName,
			&row.SupplierID, &row.SupplierName,
			&row.TotalCount,
			&row.DetailID, &row.DetailQty, &row.DetailPrice,
			&row.IngredientID, &row.IngredientName, &row.UnitName,
			&row.DetailReceivedQty, &row.DetailBatchCount,
			&row.BatchID, &row.BatchQty, &row.BatchExpDate, &row.BatchArrival,
			&row.BatchRestaurantName, &row.BatchRestaurantAddress, &row.BatchAdminName, &row.BatchAdminID,
		); err != nil {
			return nil, fmt.Errorf("GetOrderDetails scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *PurchasesRepo) GetDetailBatches(detailID int) ([]DetailBatchRow, error) {
	const query = `
SELECT
    iod.ingredient_order_detail_id,
    iu.ingredient_unit_name,
    ios.ingredient_order_status_name,
    pb.product_batch_id,
    pb.product_batch_accepted_quantity,
    pb.product_batch_expiration_date,
    pb.product_batch_arrival_date,
    r.restaurant_name,
    r.restaurant_address,
    a2.administrator_full_name,
    a2.administrator_id
FROM ingredient_order_details iod
JOIN ingredient_orders io ON io.ingredient_order_id = iod.ingredient_order_id
JOIN ingredient_order_statuses ios ON ios.ingredient_order_status_id = io.ingredient_order_status_id
JOIN ingredients i ON i.ingredient_id = iod.ingredient_id
JOIN ingredient_units iu ON iu.ingredient_unit_id = i.ingredient_unit_id
LEFT JOIN product_batches pb ON pb.ingredient_order_detail_id = iod.ingredient_order_detail_id
LEFT JOIN stock_ingredients si ON si.stock_ingredient_id = pb.stock_ingredient_id
LEFT JOIN restaurants r ON r.restaurant_id = si.restaurant_id
LEFT JOIN administrators a2 ON a2.administrator_id = pb.administrator_id
WHERE iod.ingredient_order_detail_id = @detailID
ORDER BY pb.product_batch_arrival_date DESC, pb.product_batch_id DESC`

	rows, err := r.db.Query(query, sql.Named("detailID", detailID))
	if err != nil {
		return nil, fmt.Errorf("GetDetailBatches query: %w", err)
	}
	defer rows.Close()

	var result []DetailBatchRow
	for rows.Next() {
		var row DetailBatchRow
		if err := rows.Scan(
			&row.DetailID,
			&row.UnitName,
			&row.OrderStatus,
			&row.BatchID,
			&row.BatchQty,
			&row.BatchExpDate,
			&row.BatchArrival,
			&row.BatchRestaurantName,
			&row.BatchRestaurantAddress,
			&row.BatchAdminName,
			&row.BatchAdminID,
		); err != nil {
			return nil, fmt.Errorf("GetDetailBatches scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *PurchasesRepo) GetSuppliers() ([]SupplierRow, error) {
	const query = `
SELECT supplier_id, supplier_company_name
FROM suppliers
ORDER BY supplier_company_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("GetSuppliers: %w", err)
	}
	defer rows.Close()

	var result []SupplierRow
	for rows.Next() {
		var s SupplierRow
		if err := rows.Scan(&s.ID, &s.Name); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *PurchasesRepo) GetStatuses(archive bool) ([]StatusRow, error) {
	var query string
	if archive {
		query = `SELECT ingredient_order_status_id, ingredient_order_status_name FROM ingredient_order_statuses WHERE ingredient_order_status_name = 'Отримано'`
	} else {
		query = `SELECT ingredient_order_status_id, ingredient_order_status_name FROM ingredient_order_statuses WHERE ingredient_order_status_name IN ('Створено', 'Відправлено')`
	}
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("GetStatuses: %w", err)
	}
	defer rows.Close()

	var result []StatusRow
	for rows.Next() {
		var s StatusRow
		if err := rows.Scan(&s.ID, &s.Name); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *PurchasesRepo) GetIngredients() ([]IngredientRow, error) {
	const query = `
SELECT i.ingredient_id, i.ingredient_name, iu.ingredient_unit_id, iu.ingredient_unit_name
FROM ingredients i
JOIN ingredient_units iu ON iu.ingredient_unit_id = i.ingredient_unit_id
ORDER BY i.ingredient_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("GetIngredients: %w", err)
	}
	defer rows.Close()

	var result []IngredientRow
	for rows.Next() {
		var ing IngredientRow
		if err := rows.Scan(&ing.ID, &ing.Name, &ing.UnitID, &ing.UnitName); err != nil {
			return nil, err
		}
		result = append(result, ing)
	}
	return result, rows.Err()
}

func (r *PurchasesRepo) CreateOrder(adminID, supplierID int, expectedAt time.Time) (int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("CreateOrder begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Generate sequential order number for today (same pattern as waiter orders).
	var today string
	var seq int
	err = tx.QueryRow(`
DECLARE @today CHAR(8) = CONVERT(CHAR(8), GETDATE(), 112);
DECLARE @seq INT;
SELECT @seq = ISNULL(MAX(TRY_CONVERT(INT, PARSENAME(REPLACE(ingredient_order_number, '-', '.'), 1))), 0) + 1
FROM ingredient_orders WITH (UPDLOCK, HOLDLOCK)
WHERE ingredient_order_number LIKE 'SUP-' + @today + '-%';
SELECT @today AS today, @seq AS seq;`).Scan(&today, &seq)
	if err != nil {
		return 0, fmt.Errorf("CreateOrder seq: %w", err)
	}

	orderNumber := fmt.Sprintf("SUP-%s-%04d", today, seq)

	var orderID int
	err = tx.QueryRow(`
INSERT INTO ingredient_orders
    (ingredient_order_number, ingredient_order_created_at, ingredient_order_expected_at,
     ingredient_order_total_amount, ingredient_order_status_id, administrator_id, supplier_id)
OUTPUT INSERTED.ingredient_order_id
VALUES (
    @number, GETDATE(), @expectedAt, 0,
    (SELECT ingredient_order_status_id FROM ingredient_order_statuses WHERE ingredient_order_status_name = 'Створено'),
    @adminID, @supplierID
)`,
		sql.Named("number", orderNumber),
		sql.Named("expectedAt", expectedAt),
		sql.Named("adminID", adminID),
		sql.Named("supplierID", supplierID),
	).Scan(&orderID)
	if err != nil {
		return 0, fmt.Errorf("CreateOrder insert: %w", err)
	}

	return orderID, tx.Commit()
}

type OrderItemInput struct {
	IngredientID int
	Qty          float64
	Price        float64
}

func (r *PurchasesRepo) CreateOrderWithItems(adminID, supplierID int, expectedAt time.Time, items []OrderItemInput) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("CreateOrderWithItems begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var today string
	var seq int
	err = tx.QueryRow(`
DECLARE @today CHAR(8) = CONVERT(CHAR(8), GETDATE(), 112);
DECLARE @seq INT;
SELECT @seq = ISNULL(MAX(TRY_CONVERT(INT, PARSENAME(REPLACE(ingredient_order_number, '-', '.'), 1))), 0) + 1
FROM ingredient_orders WITH (UPDLOCK, HOLDLOCK)
WHERE ingredient_order_number LIKE 'SUP-' + @today + '-%';
SELECT @today AS today, @seq AS seq;`).Scan(&today, &seq)
	if err != nil {
		return fmt.Errorf("CreateOrderWithItems seq: %w", err)
	}

	orderNumber := fmt.Sprintf("SUP-%s-%04d", today, seq)

	var total float64
	for _, item := range items {
		total += item.Qty * item.Price
	}

	var orderID int
	err = tx.QueryRow(`
INSERT INTO ingredient_orders
    (ingredient_order_number, ingredient_order_created_at, ingredient_order_expected_at,
     ingredient_order_total_amount, ingredient_order_status_id, administrator_id, supplier_id)
OUTPUT INSERTED.ingredient_order_id
VALUES (
    @number, GETDATE(), @expectedAt, @total,
    (SELECT ingredient_order_status_id FROM ingredient_order_statuses WHERE ingredient_order_status_name = 'Створено'),
    @adminID, @supplierID
)`,
		sql.Named("number", orderNumber),
		sql.Named("expectedAt", expectedAt),
		sql.Named("total", total),
		sql.Named("adminID", adminID),
		sql.Named("supplierID", supplierID),
	).Scan(&orderID)
	if err != nil {
		return fmt.Errorf("CreateOrderWithItems insert: %w", err)
	}

	for _, item := range items {
		_, err = tx.Exec(`
INSERT INTO ingredient_order_details
    (ingredient_order_id, ingredient_id, detail_quantity, detail_purchase_price)
VALUES (@orderID, @ingredientID, @qty, @price)`,
			sql.Named("orderID", orderID),
			sql.Named("ingredientID", item.IngredientID),
			sql.Named("qty", item.Qty),
			sql.Named("price", item.Price),
		)
		if err != nil {
			return fmt.Errorf("CreateOrderWithItems item: %w", err)
		}
	}

	return tx.Commit()
}

func (r *PurchasesRepo) AddOrderItem(orderID, ingredientID int, qty, price float64) error {
	_, err := r.db.Exec(`
INSERT INTO ingredient_order_details
    (ingredient_order_id, ingredient_id, detail_quantity, detail_purchase_price)
VALUES (@orderID, @ingredientID, @qty, @price);
UPDATE ingredient_orders
SET ingredient_order_total_amount = (
    SELECT ISNULL(SUM(detail_quantity * detail_purchase_price), 0)
    FROM ingredient_order_details
    WHERE ingredient_order_id = @orderID
)
WHERE ingredient_order_id = @orderID;`,
		sql.Named("orderID", orderID),
		sql.Named("ingredientID", ingredientID),
		sql.Named("qty", qty),
		sql.Named("price", price),
	)
	if err != nil {
		return fmt.Errorf("AddOrderItem: %w", err)
	}
	return nil
}

func (r *PurchasesRepo) RemoveOrderItem(orderID, itemID int) error {
	_, err := r.db.Exec(`
DELETE FROM ingredient_order_details WHERE ingredient_order_detail_id = @itemID;
UPDATE ingredient_orders
SET ingredient_order_total_amount = (
    SELECT ISNULL(SUM(detail_quantity * detail_purchase_price), 0)
    FROM ingredient_order_details
    WHERE ingredient_order_id = @orderID
)
WHERE ingredient_order_id = @orderID;`,
		sql.Named("itemID", itemID),
		sql.Named("orderID", orderID),
	)
	if err != nil {
		return fmt.Errorf("RemoveOrderItem: %w", err)
	}
	return nil
}

func (r *PurchasesRepo) UpdateOrderItem(orderID, detailID int, qty, price float64) error {
	_, err := r.db.Exec(`
UPDATE ingredient_order_details
SET detail_quantity = @qty, detail_purchase_price = @price
WHERE ingredient_order_detail_id = @detailID AND ingredient_order_id = @orderID;
UPDATE ingredient_orders
SET ingredient_order_total_amount = (
    SELECT ISNULL(SUM(detail_quantity * detail_purchase_price), 0)
    FROM ingredient_order_details WHERE ingredient_order_id = @orderID
)
WHERE ingredient_order_id = @orderID;`,
		sql.Named("qty", qty),
		sql.Named("price", price),
		sql.Named("detailID", detailID),
		sql.Named("orderID", orderID),
	)
	if err != nil {
		return fmt.Errorf("UpdateOrderItem: %w", err)
	}
	return nil
}

func (r *PurchasesRepo) UpdateOrderStatus(orderID int, statusName string) error {
	_, err := r.db.Exec(`
UPDATE ingredient_orders
SET ingredient_order_status_id = (
    SELECT ingredient_order_status_id FROM ingredient_order_statuses WHERE ingredient_order_status_name = @statusName
)
WHERE ingredient_order_id = @orderID`,
		sql.Named("statusName", statusName),
		sql.Named("orderID", orderID),
	)
	if err != nil {
		return fmt.Errorf("UpdateOrderStatus: %w", err)
	}
	return nil
}

type BatchInput struct {
	DetailID     int
	IngredientID int
	Qty          float64
	ExpDate      time.Time
}

func (r *PurchasesRepo) ReceiveBatches(restaurantID, adminID int, batches []BatchInput) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("ReceiveBatches begin: %w", err)
	}
	defer tx.Rollback()

	for _, b := range batches {
		var stockID int
		err := tx.QueryRow(`
INSERT INTO stock_ingredients
    (stock_ingredient_quantity, stock_ingredient_received_at, stock_ingredient_expiration_date, restaurant_id, ingredient_id)
OUTPUT INSERTED.stock_ingredient_id
VALUES (@qty, GETDATE(), @expDate, @restaurantID, @ingredientID)`,
			sql.Named("qty", b.Qty),
			sql.Named("expDate", b.ExpDate),
			sql.Named("restaurantID", restaurantID),
			sql.Named("ingredientID", b.IngredientID),
		).Scan(&stockID)
		if err != nil {
			return fmt.Errorf("ReceiveBatches insert stock: %w", err)
		}

		_, err = tx.Exec(`
INSERT INTO product_batches
    (product_batch_arrival_date, product_batch_expiration_date, product_batch_accepted_quantity,
     ingredient_order_detail_id, administrator_id, stock_ingredient_id)
VALUES (GETDATE(), @expDate, @qty, @detailID, @adminID, @stockID)`,
			sql.Named("expDate", b.ExpDate),
			sql.Named("qty", b.Qty),
			sql.Named("detailID", b.DetailID),
			sql.Named("adminID", adminID),
			sql.Named("stockID", stockID),
		)
		if err != nil {
			return fmt.Errorf("ReceiveBatches insert batch: %w", err)
		}
	}
	return tx.Commit()
}

func (r *PurchasesRepo) GetBatchEditData(batchID int) (BatchEditRow, error) {
	const query = `
SELECT
    pb.product_batch_id,
    pb.product_batch_accepted_quantity,
    pb.product_batch_expiration_date,
    pb.administrator_id,
    iod.ingredient_order_detail_id,
    iod.ingredient_id,
    iod.detail_quantity,
    i.ingredient_name,
    iu.ingredient_unit_name,
    ISNULL(SUM(pb2.product_batch_accepted_quantity), 0) AS total_received
FROM product_batches pb
JOIN ingredient_order_details iod ON iod.ingredient_order_detail_id = pb.ingredient_order_detail_id
JOIN ingredients i ON i.ingredient_id = iod.ingredient_id
JOIN ingredient_units iu ON iu.ingredient_unit_id = i.ingredient_unit_id
LEFT JOIN product_batches pb2 ON pb2.ingredient_order_detail_id = iod.ingredient_order_detail_id
WHERE pb.product_batch_id = @batchID
GROUP BY
    pb.product_batch_id,
    pb.product_batch_accepted_quantity,
    pb.product_batch_expiration_date,
    pb.administrator_id,
    iod.ingredient_order_detail_id,
    iod.ingredient_id,
    iod.detail_quantity,
    i.ingredient_name,
    iu.ingredient_unit_name`

	var row BatchEditRow
	if err := r.db.QueryRow(query, sql.Named("batchID", batchID)).Scan(
		&row.BatchID,
		&row.BatchQty,
		&row.BatchExpDate,
		&row.BatchAdminID,
		&row.DetailID,
		&row.IngredientID,
		&row.OrderQty,
		&row.Ingredient,
		&row.Unit,
		&row.TotalReceived,
	); err != nil {
		return BatchEditRow{}, fmt.Errorf("GetBatchEditData: %w", err)
	}
	return row, nil
}

func (r *PurchasesRepo) UpdateBatch(batchID, adminID int, qty float64, expDate time.Time) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("UpdateBatch begin: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
UPDATE stock_ingredients
SET stock_ingredient_quantity = @qty,
    stock_ingredient_expiration_date = @expDate
WHERE stock_ingredient_id = (
    SELECT stock_ingredient_id
    FROM product_batches
    WHERE product_batch_id = @batchID AND administrator_id = @adminID
)`,
		sql.Named("qty", qty),
		sql.Named("expDate", expDate),
		sql.Named("batchID", batchID),
		sql.Named("adminID", adminID),
	)
	if err != nil {
		return fmt.Errorf("UpdateBatch stock: %w", err)
	}

	res, err := tx.Exec(`
UPDATE product_batches
SET product_batch_accepted_quantity = @qty,
    product_batch_expiration_date = @expDate
WHERE product_batch_id = @batchID AND administrator_id = @adminID`,
		sql.Named("qty", qty),
		sql.Named("expDate", expDate),
		sql.Named("batchID", batchID),
		sql.Named("adminID", adminID),
	)
	if err != nil {
		return fmt.Errorf("UpdateBatch batch: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return fmt.Errorf("UpdateBatch: not found or not owned")
	}

	return tx.Commit()
}
