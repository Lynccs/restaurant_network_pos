package waiterrepo

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

var menuRepoLog = log.New(log.Writer(), "[MenuRepo] ", log.LstdFlags|log.Lshortfile)

type DishRow struct {
	ID           int
	Name         string
	Price        float64
	PortionSize  int
	CookingTime  int
	CategoryName string
	Portions     int
}

type DishLiteRow struct {
	ID           int
	Name         string
	Price        float64
	PortionSize  int
	CookingTime  int
	CategoryName string
}

type MenuRepo struct {
	db *sql.DB
}

func NewMenuRepo(db *sql.DB) *MenuRepo {
	return &MenuRepo{db: db}
}

// GetMenuWithPortions is the heavy query used only for cache warm-up.
// It calculates the maximum fully-makeable portions for each dish based on
// non-expired stock in the given restaurant.
func (r *MenuRepo) GetMenuWithPortions(restaurantID int) ([]DishRow, error) {
	const query = `
		WITH IngredientStock AS (
			-- Крок 1: Рахуємо загальний доступний залишок для КОЖНОГО інгредієнта в ресторані
			SELECT 
				ingredient_id,
				SUM(stock_ingredient_quantity) AS total_stock
			FROM stock_ingredients
			WHERE restaurant_id = @restaurantID
			AND stock_ingredient_expiration_date > GETDATE()
			GROUP BY ingredient_id
		),
		DishPortions AS (
			-- Крок 2: Рахуємо, скільки порцій кожної страви можна приготувати
			SELECT 
				di.dish_id,
				MIN(CAST(FLOOR(
					COALESCE(st.total_stock, 0) / di.dish_ingredient_quantity
				) AS INT)) AS available_portions
			FROM dish_ingredients di
			LEFT JOIN IngredientStock st ON di.ingredient_id = st.ingredient_id
			GROUP BY di.dish_id
		)
		-- Крок 3: Збираємо фінальне меню
		SELECT
			d.dish_id,
			d.dish_name,
			d.dish_price,
			d.dish_portion_size,
			d.dish_cooking_time,
			dc.dish_category_name,
			COALESCE(dp.available_portions, 0) AS available_portions
		FROM dishes d
		JOIN dish_categories dc ON dc.dish_category_id = d.dish_category_id
		LEFT JOIN DishPortions dp ON d.dish_id = dp.dish_id
		WHERE EXISTS (SELECT 1 FROM dish_ingredients di2 WHERE di2.dish_id = d.dish_id)
		ORDER BY dc.dish_category_name, d.dish_name;`

	menuRepoLog.Printf("GetMenuWithPortions: restaurantID=%d", restaurantID)

	rows, err := r.db.Query(query, sql.Named("restaurantID", restaurantID))
	if err != nil {
		menuRepoLog.Printf("GetMenuWithPortions: query error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var result []DishRow
	for rows.Next() {
		var row DishRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Price, &row.PortionSize, &row.CookingTime, &row.CategoryName, &row.Portions); err != nil {
			menuRepoLog.Printf("GetMenuWithPortions: scan error: %v", err)
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		menuRepoLog.Printf("GetMenuWithPortions: rows error: %v", err)
		return nil, err
	}

	menuRepoLog.Printf("GetMenuWithPortions: found %d dishes", len(result))
	return result, nil
}

// GetDishesLite is a lightweight query for page rendering — no stock joins.
func (r *MenuRepo) GetDishesLite() ([]DishLiteRow, error) {
	const query = `
		SELECT
			d.dish_id,
			d.dish_name,
			d.dish_price,
			d.dish_portion_size,
			d.dish_cooking_time,
			dc.dish_category_name
		FROM dishes d
		JOIN dish_categories dc ON dc.dish_category_id = d.dish_category_id
		WHERE EXISTS (SELECT 1 FROM dish_ingredients di WHERE di.dish_id = d.dish_id)
		ORDER BY dc.dish_category_name, d.dish_name`

	menuRepoLog.Printf("GetDishesLite")

	rows, err := r.db.Query(query)
	if err != nil {
		menuRepoLog.Printf("GetDishesLite: query error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var result []DishLiteRow
	for rows.Next() {
		var row DishLiteRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Price, &row.PortionSize, &row.CookingTime, &row.CategoryName); err != nil {
			menuRepoLog.Printf("GetDishesLite: scan error: %v", err)
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		menuRepoLog.Printf("GetDishesLite: rows error: %v", err)
		return nil, err
	}

	menuRepoLog.Printf("GetDishesLite: found %d dishes", len(result))
	return result, nil
}

// GetTableID resolves a table number to its primary key for a given restaurant.
func (r *MenuRepo) GetTableID(restaurantID, tableNumber int) (int, error) {
	const query = `
		SELECT table_id FROM tables
		WHERE restaurant_id = @restaurantID AND table_number = @tableNumber`

	menuRepoLog.Printf("GetTableID: restaurantID=%d tableNumber=%d", restaurantID, tableNumber)

	var id int
	err := r.db.QueryRow(query,
		sql.Named("restaurantID", restaurantID),
		sql.Named("tableNumber", tableNumber),
	).Scan(&id)
	if err != nil {
		menuRepoLog.Printf("GetTableID: query error: %v", err)
		return 0, err
	}
	return id, nil
}

// CartEntryForOrder is used exclusively by CreateOrder and SyncOrderDraft.
type CartEntryForOrder struct {
	DishID int
	Qty    int
	Price  float64
}

// ActiveOrderItemRow represents one order_item with its derived status.
type ActiveOrderItemRow struct {
	OrderItemID  int
	OrderID      int
	DishID       int
	DishName     string
	Price        float64
	Qty          int
	CancelledQty int
	Status       string // "new" | "cooking" | "done"
}

// DBItemChange describes a change to an existing order_item in a SyncOrderDraft call.
type DBItemChange struct {
	OrderItemID int
	OrigQty     int // quantity the waiter saw when the session started
	DraftQty    int // desired new quantity (0 = delete)
	CancelDelta int // additional portions to cancel (for "cooking" items)
}

// SyncDraftParams is the full set of changes to apply atomically to an existing order.
type SyncDraftParams struct {
	ExistingOrderID int
	NewItems        []CartEntryForOrder
	DBItemChanges   []DBItemChange
}

// CreateOrder inserts an order and its items inside a transaction.
// Returns the generated order_number.
func (r *MenuRepo) CreateOrder(tableID, waiterID int, items []CartEntryForOrder) (string, error) {
	menuRepoLog.Printf("CreateOrder: tableID=%d waiterID=%d items=%d", tableID, waiterID, len(items))

	tx, err := r.db.Begin()
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Get today's sequential number.
	var seq int
	err = tx.QueryRow(`
		SELECT COUNT(*) + 1 FROM orders
		WHERE CAST(order_created_at AS DATE) = CAST(GETDATE() AS DATE)`).Scan(&seq)
	if err != nil {
		return "", fmt.Errorf("get seq: %w", err)
	}

	orderNumber := fmt.Sprintf("ORD-%s-%04d", time.Now().Format("20060102"), seq)

	// Calculate total.
	var total float64
	for _, it := range items {
		total += it.Price * float64(it.Qty)
	}

	// Insert order.
	var orderID int64
	err = tx.QueryRow(`
		INSERT INTO orders
			(order_number, order_total_amount, order_created_at, order_status_id, table_id, waiter_id)
		VALUES
			(@number, @total, GETDATE(),
			 (SELECT order_status_id FROM order_statuses WHERE order_status_name = N'Нове'),
			 @tableID, @waiterID);
		SELECT SCOPE_IDENTITY()`,
		sql.Named("number", orderNumber),
		sql.Named("total", total),
		sql.Named("tableID", tableID),
		sql.Named("waiterID", waiterID),
	).Scan(&orderID)
	if err != nil {
		return "", fmt.Errorf("insert order: %w", err)
	}

	// Insert order items.
	for _, it := range items {
		_, err = tx.Exec(`
			INSERT INTO order_items (order_item_quantity, order_id, dish_id)
			VALUES (@qty, @orderID, @dishID)`,
			sql.Named("qty", it.Qty),
			sql.Named("orderID", orderID),
			sql.Named("dishID", it.DishID),
		)
		if err != nil {
			return "", fmt.Errorf("insert order_item dishID=%d: %w", it.DishID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	menuRepoLog.Printf("CreateOrder: created %s orderID=%d", orderNumber, orderID)
	return orderNumber, nil
}

// GetActiveOrderID returns the order_id of the open (non-closed, non-cancelled) order
// for a given table, or 0 if none exists.
func (r *MenuRepo) GetActiveOrderID(tableID int) (int, error) {
	const query = `
		SELECT TOP 1 order_id FROM orders
		WHERE table_id = @tableID
		  AND order_status_id NOT IN (
			  SELECT order_status_id FROM order_statuses
			  WHERE order_status_name IN ('Закрито', 'Скасовано'))`

	menuRepoLog.Printf("GetActiveOrderID: tableID=%d", tableID)

	var id int
	err := r.db.QueryRow(query, sql.Named("tableID", tableID)).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		menuRepoLog.Printf("GetActiveOrderID: error: %v", err)
		return 0, err
	}
	return id, nil
}

// GetActiveOrderItems returns all order_items for the open order of a table,
// enriched with the derived status (new/cooking/done) via LEFT JOIN cooking_tasks.
func (r *MenuRepo) GetActiveOrderItems(tableID int) ([]ActiveOrderItemRow, error) {
	const query = `
		SELECT
			oi.order_item_id,
			o.order_id,
			oi.dish_id,
			d.dish_name,
			d.dish_price,
			oi.order_item_quantity,
			oi.cancelled_quantity,
			CASE
				WHEN ct.cooking_task_id IS NULL THEN 'new'
				WHEN ct.cooking_task_end_time IS NULL THEN 'cooking'
				ELSE 'done'
			END AS item_status
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.order_id
		JOIN dishes d ON d.dish_id = oi.dish_id
		LEFT JOIN cooking_tasks ct ON ct.order_item_id = oi.order_item_id
		WHERE o.table_id = @tableID
		  AND o.order_status_id NOT IN (
			  SELECT order_status_id FROM order_statuses
			  WHERE order_status_name IN ('Закрито', 'Скасовано'))
		ORDER BY oi.order_item_id`

	menuRepoLog.Printf("GetActiveOrderItems: tableID=%d", tableID)

	rows, err := r.db.Query(query, sql.Named("tableID", tableID))
	if err != nil {
		menuRepoLog.Printf("GetActiveOrderItems: query error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var result []ActiveOrderItemRow
	for rows.Next() {
		var row ActiveOrderItemRow
		if err := rows.Scan(
			&row.OrderItemID, &row.OrderID, &row.DishID, &row.DishName,
			&row.Price, &row.Qty, &row.CancelledQty, &row.Status,
		); err != nil {
			menuRepoLog.Printf("GetActiveOrderItems: scan error: %v", err)
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		menuRepoLog.Printf("GetActiveOrderItems: rows error: %v", err)
		return nil, err
	}

	menuRepoLog.Printf("GetActiveOrderItems: found %d items for tableID=%d", len(result), tableID)
	return result, nil
}

// SyncOrderDraft applies the waiter's full draft to an existing order in a single transaction.
// For each changed DB item it re-checks the real status via LEFT JOIN cooking_tasks (security),
// then applies the appropriate SQL. New in-memory items are inserted. Total is recalculated.
func (r *MenuRepo) SyncOrderDraft(params SyncDraftParams) error {
	menuRepoLog.Printf("SyncOrderDraft: orderID=%d dbChanges=%d newItems=%d",
		params.ExistingOrderID, len(params.DBItemChanges), len(params.NewItems))

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Apply each DB item change with a server-side status re-check.
	for _, change := range params.DBItemChanges {
		var realStatus string
		err := tx.QueryRow(`
			SELECT CASE
				WHEN ct.cooking_task_id IS NULL THEN 'new'
				WHEN ct.cooking_task_end_time IS NULL THEN 'cooking'
				ELSE 'done'
			END
			FROM order_items oi
			LEFT JOIN cooking_tasks ct ON ct.order_item_id = oi.order_item_id
			WHERE oi.order_item_id = @id`,
			sql.Named("id", change.OrderItemID),
		).Scan(&realStatus)
		if err != nil {
			return fmt.Errorf("status check itemID=%d: %w", change.OrderItemID, err)
		}

		switch realStatus {
		case "new":
			if change.DraftQty == 0 {
				_, err = tx.Exec(`DELETE FROM order_items WHERE order_item_id = @id`,
					sql.Named("id", change.OrderItemID))
			} else {
				_, err = tx.Exec(`UPDATE order_items SET order_item_quantity = @qty WHERE order_item_id = @id`,
					sql.Named("qty", change.DraftQty),
					sql.Named("id", change.OrderItemID))
			}
		case "cooking":
			// Race condition fix: if status changed from 'new' to 'cooking' between
			// LoadActiveOrder and Submit, fold any qty reduction into cancel delta.
			totalCancel := change.CancelDelta
			if change.OrigQty > change.DraftQty {
				totalCancel += change.OrigQty - change.DraftQty
			}
			if totalCancel > 0 {
				_, err = tx.Exec(`
					UPDATE order_items SET cancelled_quantity = cancelled_quantity + @delta
					WHERE order_item_id = @id`,
					sql.Named("delta", totalCancel),
					sql.Named("id", change.OrderItemID))
			}
		// "done": ignore changes
		}
		if err != nil {
			return fmt.Errorf("apply change itemID=%d: %w", change.OrderItemID, err)
		}
	}

	// Insert new in-memory items.
	for _, item := range params.NewItems {
		_, err = tx.Exec(`
			INSERT INTO order_items (order_item_quantity, order_id, dish_id)
			VALUES (@qty, @orderID, @dishID)`,
			sql.Named("qty", item.Qty),
			sql.Named("orderID", params.ExistingOrderID),
			sql.Named("dishID", item.DishID),
		)
		if err != nil {
			return fmt.Errorf("insert new item dishID=%d: %w", item.DishID, err)
		}
	}

	// Recalculate order total from remaining items.
	_, err = tx.Exec(`
		UPDATE orders SET order_total_amount = (
			SELECT COALESCE(SUM((oi.order_item_quantity - oi.cancelled_quantity) * d.dish_price), 0)
			FROM order_items oi
			JOIN dishes d ON d.dish_id = oi.dish_id
			WHERE oi.order_id = @orderID)
		WHERE order_id = @orderID`,
		sql.Named("orderID", params.ExistingOrderID),
	)
	if err != nil {
		return fmt.Errorf("recalc total: %w", err)
	}

	// If new items were added and the order was already 'Готове', reset to 'Нове'.
	if len(params.NewItems) > 0 {
		_, err = tx.Exec(`
			UPDATE orders SET order_status_id =
				(SELECT order_status_id FROM order_statuses WHERE order_status_name = N'Нове')
			WHERE order_id = @orderID
			  AND order_status_id = (SELECT order_status_id FROM order_statuses WHERE order_status_name = N'Готове')`,
			sql.Named("orderID", params.ExistingOrderID),
		)
		if err != nil {
			return fmt.Errorf("reset status: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	menuRepoLog.Printf("SyncOrderDraft: completed for orderID=%d", params.ExistingOrderID)
	return nil
}
