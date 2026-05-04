package adminrepo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const WarehousePageSize = 12

type WarehouseFilters struct {
	RestaurantID int
	Ingredients  []string
	Status       string
	ArrivalFrom  string
	ArrivalTo    string
	ExpFrom      string
	ExpTo        string
	MinQty       string
	MaxQty       string
	Page         int
}

type WarehouseRow struct {
	StockID           int
	IngredientName    string
	IngredientBrand   sql.NullString
	Qty               float64
	UnitName          string
	ReceivedAt        time.Time
	ExpDate           time.Time
	StorageCondition  sql.NullString
	RestaurantID      int
	RestaurantName    string
	RestaurantAddress sql.NullString
	SupplierName      sql.NullString
	TotalCount        int
	WriteOffCount     int
}

type WarehouseRestaurantOption struct {
	ID      int
	Name    string
	Address string
}

type WarehouseSummaryRow struct {
	TotalRows         int
	UniqueIngredients int
	ExpiringSoonCount int
	ExpiredCount      int
}

type WriteOffReason struct {
	ID   int
	Name string
}

type WriteOffRow struct {
	WriteOffDate time.Time
	Qty          float64
	Reason       string
	AdminName    string
}

func buildWarehouseWhere(f WarehouseFilters, args *[]any) string {
	var where strings.Builder
	if f.RestaurantID > 0 {
		where.WriteString("AND si.restaurant_id = @restaurantID\n")
		*args = append(*args, sql.Named("restaurantID", f.RestaurantID))
	}
	if len(f.Ingredients) > 0 {
		where.WriteString("AND (")
		for i, ingredient := range f.Ingredients {
			if i > 0 {
				where.WriteString(" OR ")
			}
			name := fmt.Sprintf("ingredient%d", i)
			where.WriteString("i.ingredient_name = @" + name)
			*args = append(*args, sql.Named(name, ingredient))
		}
		where.WriteString(")\n")
	}
	if f.ArrivalFrom != "" {
		where.WriteString("AND si.stock_ingredient_received_at >= @arrivalFrom\n")
		*args = append(*args, sql.Named("arrivalFrom", f.ArrivalFrom))
	}
	if f.ArrivalTo != "" {
		where.WriteString("AND si.stock_ingredient_received_at < DATEADD(day, 1, @arrivalTo)\n")
		*args = append(*args, sql.Named("arrivalTo", f.ArrivalTo))
	}
	if f.ExpFrom != "" {
		where.WriteString("AND si.stock_ingredient_expiration_date >= @expFrom\n")
		*args = append(*args, sql.Named("expFrom", f.ExpFrom))
	}
	if f.ExpTo != "" {
		where.WriteString("AND si.stock_ingredient_expiration_date < DATEADD(day, 1, @expTo)\n")
		*args = append(*args, sql.Named("expTo", f.ExpTo))
	}
	if f.MinQty != "" {
		where.WriteString("AND si.stock_ingredient_quantity >= @minQty\n")
		*args = append(*args, sql.Named("minQty", f.MinQty))
	}
	if f.MaxQty != "" {
		where.WriteString("AND si.stock_ingredient_quantity <= @maxQty\n")
		*args = append(*args, sql.Named("maxQty", f.MaxQty))
	}
	switch f.Status {
	case "not_expired":
		where.WriteString("AND si.stock_ingredient_expiration_date >= CAST(GETDATE() AS DATE)\n")
	case "normal":
		where.WriteString("AND si.stock_ingredient_expiration_date >= DATEADD(day, 4, GETDATE())\n")
	case "expiring":
		where.WriteString("AND si.stock_ingredient_expiration_date >= GETDATE() AND si.stock_ingredient_expiration_date < DATEADD(day, 4, GETDATE())\n")
	case "expired":
		where.WriteString("AND si.stock_ingredient_expiration_date < GETDATE()\n")
	}
	return where.String()
}

func (r *PurchasesRepo) GetWarehouseItem(restaurantID, stockID int) (WarehouseRow, error) {
	const query = `
SELECT
	si.stock_ingredient_id,
	i.ingredient_name,
	i.ingredient_brand,
	si.stock_ingredient_quantity,
	iu.ingredient_unit_name,
	si.stock_ingredient_received_at,
	si.stock_ingredient_expiration_date,
	isc.ingredient_storage_condition_name,
	r.restaurant_id,
	r.restaurant_name,
	r.restaurant_address,
	sup.supplier_company_name,
	(SELECT COUNT(*) FROM write_offs wo WHERE wo.stock_ingredient_id = si.stock_ingredient_id) AS write_off_count
FROM stock_ingredients si
JOIN ingredients i ON i.ingredient_id = si.ingredient_id
JOIN ingredient_units iu ON iu.ingredient_unit_id = i.ingredient_unit_id
LEFT JOIN ingredient_storage_conditions isc ON isc.ingredient_storage_condition_id = i.ingredient_storage_condition_id
JOIN restaurants r ON r.restaurant_id = si.restaurant_id
OUTER APPLY (
	SELECT TOP (1)
		s.supplier_company_name
	FROM product_batches pb
	JOIN ingredient_order_details iod ON iod.ingredient_order_detail_id = pb.ingredient_order_detail_id
	JOIN ingredient_orders io ON io.ingredient_order_id = iod.ingredient_order_id
	JOIN suppliers s ON s.supplier_id = io.supplier_id
	WHERE pb.stock_ingredient_id = si.stock_ingredient_id
	ORDER BY pb.product_batch_arrival_date DESC
) sup
WHERE si.stock_ingredient_id = @stockID
  AND si.restaurant_id = @restaurantID`

	var row WarehouseRow
	if err := r.db.QueryRow(query,
		sql.Named("stockID", stockID),
		sql.Named("restaurantID", restaurantID),
	).Scan(
		&row.StockID,
		&row.IngredientName,
		&row.IngredientBrand,
		&row.Qty,
		&row.UnitName,
		&row.ReceivedAt,
		&row.ExpDate,
		&row.StorageCondition,
		&row.RestaurantID,
		&row.RestaurantName,
		&row.RestaurantAddress,
		&row.SupplierName,
		&row.WriteOffCount,
	); err != nil {
		return WarehouseRow{}, fmt.Errorf("GetWarehouseItem: %w", err)
	}
	return row, nil
}

func (r *PurchasesRepo) GetWarehouseData(f WarehouseFilters) ([]WarehouseRow, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	offset := (f.Page - 1) * WarehousePageSize
	args := []any{
		sql.Named("offset", offset),
		sql.Named("pageSize", WarehousePageSize),
	}
	where := buildWarehouseWhere(f, &args)

	query := fmt.Sprintf(`
WITH base AS (
	SELECT
		si.stock_ingredient_id,
		i.ingredient_name,
		i.ingredient_brand,
		si.stock_ingredient_quantity,
		iu.ingredient_unit_name,
		si.stock_ingredient_received_at,
		si.stock_ingredient_expiration_date,
		isc.ingredient_storage_condition_name,
		r.restaurant_id,
		r.restaurant_name,
		r.restaurant_address,
		sup.supplier_company_name,
		(SELECT COUNT(*) FROM write_offs wo WHERE wo.stock_ingredient_id = si.stock_ingredient_id) AS write_off_count,
		COUNT(*) OVER () AS total_count,
		ROW_NUMBER() OVER (
			ORDER BY
				si.stock_ingredient_received_at DESC,
				si.stock_ingredient_expiration_date ASC,
				si.stock_ingredient_id DESC
		) AS rn
	FROM stock_ingredients si
	JOIN ingredients i ON i.ingredient_id = si.ingredient_id
	JOIN ingredient_units iu ON iu.ingredient_unit_id = i.ingredient_unit_id
	LEFT JOIN ingredient_storage_conditions isc ON isc.ingredient_storage_condition_id = i.ingredient_storage_condition_id
	JOIN restaurants r ON r.restaurant_id = si.restaurant_id
	OUTER APPLY (
		SELECT TOP (1)
			s.supplier_company_name
		FROM product_batches pb
		JOIN ingredient_order_details iod ON iod.ingredient_order_detail_id = pb.ingredient_order_detail_id
		JOIN ingredient_orders io ON io.ingredient_order_id = iod.ingredient_order_id
		JOIN suppliers s ON s.supplier_id = io.supplier_id
		WHERE pb.stock_ingredient_id = si.stock_ingredient_id
		ORDER BY pb.product_batch_arrival_date DESC
	) sup
	WHERE 1=1
	%s
)
SELECT
	stock_ingredient_id,
	ingredient_name,
	ingredient_brand,
	stock_ingredient_quantity,
	ingredient_unit_name,
	stock_ingredient_received_at,
	stock_ingredient_expiration_date,
	ingredient_storage_condition_name,
	restaurant_id,
	restaurant_name,
	restaurant_address,
	supplier_company_name,
	write_off_count,
	total_count
FROM base
WHERE rn BETWEEN @offset + 1 AND @offset + @pageSize
ORDER BY rn
OPTION (RECOMPILE)`, where)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("GetWarehouseData query: %w", err)
	}
	defer rows.Close()

	var result []WarehouseRow
	totalCount := 0
	for rows.Next() {
		var row WarehouseRow
		if err := rows.Scan(
			&row.StockID,
			&row.IngredientName,
			&row.IngredientBrand,
			&row.Qty,
			&row.UnitName,
			&row.ReceivedAt,
			&row.ExpDate,
			&row.StorageCondition,
			&row.RestaurantID,
			&row.RestaurantName,
			&row.RestaurantAddress,
			&row.SupplierName,
			&row.WriteOffCount,
			&row.TotalCount,
		); err != nil {
			return nil, 0, fmt.Errorf("GetWarehouseData scan: %w", err)
		}
		if row.TotalCount > totalCount {
			totalCount = row.TotalCount
		}
		result = append(result, row)
	}
	return result, totalCount, rows.Err()
}

func (r *PurchasesRepo) GetWarehouseSummary(f WarehouseFilters) (WarehouseSummaryRow, error) {
	args := []any{}
	where := buildWarehouseWhere(f, &args)

	query := fmt.Sprintf(`
SELECT
	COUNT(*) AS total_rows,
	COUNT(DISTINCT si.ingredient_id) AS unique_ingredients,
	SUM(CASE WHEN si.stock_ingredient_expiration_date >= GETDATE() AND si.stock_ingredient_expiration_date < DATEADD(day, 4, GETDATE()) THEN 1 ELSE 0 END) AS expiring_soon_count,
	SUM(CASE WHEN si.stock_ingredient_expiration_date < GETDATE() THEN 1 ELSE 0 END) AS expired_count
FROM stock_ingredients si
JOIN ingredients i ON i.ingredient_id = si.ingredient_id
WHERE 1=1
%s`, where)

	var row WarehouseSummaryRow
	if err := r.db.QueryRow(query, args...).Scan(
		&row.TotalRows,
		&row.UniqueIngredients,
		&row.ExpiringSoonCount,
		&row.ExpiredCount,
	); err != nil {
		return WarehouseSummaryRow{}, fmt.Errorf("GetWarehouseSummary: %w", err)
	}
	return row, nil
}

func (r *PurchasesRepo) GetWarehouseIngredientOptions(restaurantID int) ([]string, error) {
	const query = `
SELECT DISTINCT i.ingredient_name
FROM stock_ingredients si
JOIN ingredients i ON i.ingredient_id = si.ingredient_id
WHERE si.restaurant_id = @restaurantID
ORDER BY i.ingredient_name`

	rows, err := r.db.Query(query, sql.Named("restaurantID", restaurantID))
	if err != nil {
		return nil, fmt.Errorf("GetWarehouseIngredientOptions: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("GetWarehouseIngredientOptions scan: %w", err)
		}
		result = append(result, name)
	}
	return result, rows.Err()
}

func (r *PurchasesRepo) GetWarehouseRestaurantOptions() ([]WarehouseRestaurantOption, error) {
	const query = `
SELECT restaurant_id, restaurant_name, restaurant_address
FROM restaurants
ORDER BY restaurant_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("GetWarehouseRestaurantOptions: %w", err)
	}
	defer rows.Close()

	var result []WarehouseRestaurantOption
	for rows.Next() {
		var row WarehouseRestaurantOption
		if err := rows.Scan(&row.ID, &row.Name, &row.Address); err != nil {
			return nil, fmt.Errorf("GetWarehouseRestaurantOptions scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *PurchasesRepo) GetWriteOffReasons() ([]WriteOffReason, error) {
	const query = `
SELECT write_off_reason_id, write_off_reason_name
FROM write_off_reasons
ORDER BY write_off_reason_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("GetWriteOffReasons: %w", err)
	}
	defer rows.Close()

	var result []WriteOffReason
	for rows.Next() {
		var row WriteOffReason
		if err := rows.Scan(&row.ID, &row.Name); err != nil {
			return nil, fmt.Errorf("GetWriteOffReasons scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *PurchasesRepo) CreateWriteOff(restaurantID, adminID, stockID, reasonID int, qty float64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("CreateWriteOff begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
UPDATE stock_ingredients
SET stock_ingredient_quantity = stock_ingredient_quantity - @qty
WHERE stock_ingredient_id = @stockID
  AND restaurant_id = @restaurantID
  AND stock_ingredient_quantity >= @qty`,
		sql.Named("qty", qty),
		sql.Named("stockID", stockID),
		sql.Named("restaurantID", restaurantID),
	)
	if err != nil {
		return fmt.Errorf("CreateWriteOff update stock: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("CreateWriteOff: insufficient quantity or чужий ресторан")
	}

	if _, err := tx.Exec(`
INSERT INTO write_offs (write_off_date, write_off_quantity, write_off_reason_id, administrator_id, stock_ingredient_id)
VALUES (GETDATE(), @qty, @reasonID, @adminID, @stockID)`,
		sql.Named("qty", qty),
		sql.Named("reasonID", reasonID),
		sql.Named("adminID", adminID),
		sql.Named("stockID", stockID),
	); err != nil {
		return fmt.Errorf("CreateWriteOff insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("CreateWriteOff commit: %w", err)
	}
	return nil
}

func (r *PurchasesRepo) GetStockWriteOffs(restaurantID, stockID int) ([]WriteOffRow, error) {
	const query = `
SELECT wo.write_off_date,
       wo.write_off_quantity,
       wr.write_off_reason_name,
       a.administrator_full_name
FROM write_offs wo
JOIN write_off_reasons wr ON wr.write_off_reason_id = wo.write_off_reason_id
JOIN administrators a ON a.administrator_id = wo.administrator_id
JOIN stock_ingredients si ON si.stock_ingredient_id = wo.stock_ingredient_id
WHERE wo.stock_ingredient_id = @stockID
  AND si.restaurant_id = @restaurantID
ORDER BY wo.write_off_date DESC`

	rows, err := r.db.Query(query,
		sql.Named("stockID", stockID),
		sql.Named("restaurantID", restaurantID),
	)
	if err != nil {
		return nil, fmt.Errorf("GetStockWriteOffs: %w", err)
	}
	defer rows.Close()

	var result []WriteOffRow
	for rows.Next() {
		var row WriteOffRow
		if err := rows.Scan(&row.WriteOffDate, &row.Qty, &row.Reason, &row.AdminName); err != nil {
			return nil, fmt.Errorf("GetStockWriteOffs scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
