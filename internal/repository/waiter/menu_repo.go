package waiterrepo

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
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
	HasIssue     bool   // order_item_has_issue = 1
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

	// Get today's sequential number atomically.
	// UPDLOCK + HOLDLOCK serializes this read in the current transaction
	// and prevents duplicate order_number under concurrent requests.
	var (
		today string
		seq   int
	)
	err = tx.QueryRow(`
		DECLARE @today CHAR(8) = CONVERT(CHAR(8), GETDATE(), 112);
		DECLARE @seq INT;

		SELECT @seq = ISNULL(MAX(TRY_CONVERT(INT, PARSENAME(REPLACE(order_number, '-', '.'), 1))), 0) + 1
		FROM orders WITH (UPDLOCK, HOLDLOCK)
		WHERE order_number LIKE 'ORD-' + @today + '-%';

		SELECT @today AS today, @seq AS seq;`).Scan(&today, &seq)
	if err != nil {
		return "", fmt.Errorf("get seq: %w", err)
	}

	orderNumber := fmt.Sprintf("ORD-%s-%04d", today, seq)

	// Calculate total.
	var total float64
	for _, it := range items {
		total += it.Price * float64(it.Qty)
	}

	// Insert order and return the generated PK via OUTPUT (works for IDENTITY and SEQUENCE).
	var orderID int64
	err = tx.QueryRow(`
		INSERT INTO orders
			(order_number, order_total_amount, order_created_at, order_status_id, table_id, waiter_id)
		OUTPUT INSERTED.order_id
		VALUES
			(@number, @total, GETDATE(),
			 (SELECT order_status_id FROM order_statuses WHERE order_status_name = N'Нове'),
			 @tableID, @waiterID)`,
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
			END AS item_status,
			oi.order_item_has_issue
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.order_id
		JOIN dishes d ON d.dish_id = oi.dish_id
		LEFT JOIN cooking_tasks ct ON ct.order_item_id = oi.order_item_id
		WHERE o.table_id = @tableID
		  AND o.order_status_id NOT IN (
			  SELECT order_status_id FROM order_statuses
			  WHERE order_status_name IN ('Закрито', 'Скасовано'))
		  AND oi.order_item_quantity > oi.cancelled_quantity
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
			&row.Price, &row.Qty, &row.CancelledQty, &row.Status, &row.HasIssue,
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
// All status checks are batched into one query; mutations are grouped by type and executed as
// batch statements, reducing round trips from 2N+M+3 to at most 8 regardless of order size.
func (r *MenuRepo) SyncOrderDraft(params SyncDraftParams) error {
	menuRepoLog.Printf("SyncOrderDraft: orderID=%d dbChanges=%d newItems=%d",
		params.ExistingOrderID, len(params.DBItemChanges), len(params.NewItems))

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if len(params.DBItemChanges) > 0 {
		// ── 1. Batch status check: one query for all changed items ──────────
		statusMap, err := syncFetchStatuses(tx, params.ExistingOrderID)
		if err != nil {
			return fmt.Errorf("fetch item statuses: %w", err)
		}

		// ── 2. Classify changes by operation type ───────────────────────────
		type qtyUpdate struct{ id, qty int }
		type cancelUpdate struct{ id, delta int }
		var (
			deleteIDs     []int
			qtyUpdates    []qtyUpdate
			cancelUpdates []cancelUpdate
		)

		for _, change := range params.DBItemChanges {
			switch statusMap[change.OrderItemID] {
			case "new":
				if change.DraftQty == 0 {
					deleteIDs = append(deleteIDs, change.OrderItemID)
				} else {
					qtyUpdates = append(qtyUpdates, qtyUpdate{change.OrderItemID, change.DraftQty})
				}
			case "cooking":
				// Race condition fix: if item moved from 'new' to 'cooking' between
				// LoadActiveOrder and Submit, fold any qty reduction into cancel delta.
				totalCancel := change.CancelDelta
				if change.OrigQty > change.DraftQty {
					totalCancel += change.OrigQty - change.DraftQty
				}
				if totalCancel > 0 {
					cancelUpdates = append(cancelUpdates, cancelUpdate{change.OrderItemID, totalCancel})
				}
				// "done": ignore changes
			}
		}

		// ── 3. Batch DELETE ─────────────────────────────────────────────────
		if len(deleteIDs) > 0 {
			if err = syncBatchDelete(tx, deleteIDs); err != nil {
				return fmt.Errorf("batch delete: %w", err)
			}
		}

		// ── 4. Batch qty UPDATE ─────────────────────────────────────────────
		if len(qtyUpdates) > 0 {
			ids := make([]int, len(qtyUpdates))
			qtys := make([]int, len(qtyUpdates))
			for i, u := range qtyUpdates {
				ids[i] = u.id
				qtys[i] = u.qty
			}
			if err = syncBatchUpdateQty(tx, ids, qtys); err != nil {
				return fmt.Errorf("batch update qty: %w", err)
			}
		}

		// ── 5. Batch cancel UPDATE ──────────────────────────────────────────
		if len(cancelUpdates) > 0 {
			ids := make([]int, len(cancelUpdates))
			deltas := make([]int, len(cancelUpdates))
			for i, u := range cancelUpdates {
				ids[i] = u.id
				deltas[i] = u.delta
			}
			if err = syncBatchUpdateCancel(tx, ids, deltas); err != nil {
				return fmt.Errorf("batch update cancel: %w", err)
			}
		}
	}

	// ── 6. Batch INSERT new items ───────────────────────────────────────────
	if len(params.NewItems) > 0 {
		if err = syncBatchInsert(tx, params.ExistingOrderID, params.NewItems); err != nil {
			return fmt.Errorf("batch insert new items: %w", err)
		}
	}

	// ── 7. Reset has_issue flag ─────────────────────────────────────────────
	if _, err = tx.Exec(`UPDATE order_items SET order_item_has_issue = 0 WHERE order_id = @orderID`,
		sql.Named("orderID", params.ExistingOrderID)); err != nil {
		return fmt.Errorf("reset has_issue: %w", err)
	}

	// ── 8. Recalculate total ────────────────────────────────────────────────
	if _, err = tx.Exec(`
		UPDATE orders SET order_total_amount = (
			SELECT COALESCE(SUM((oi.order_item_quantity - oi.cancelled_quantity) * d.dish_price), 0)
			FROM order_items oi
			JOIN dishes d ON d.dish_id = oi.dish_id
			WHERE oi.order_id = @orderID)
		WHERE order_id = @orderID`,
		sql.Named("orderID", params.ExistingOrderID)); err != nil {
		return fmt.Errorf("recalc total: %w", err)
	}

	// ── 9. Recalculate status (single pass via CTE instead of 3 × EXISTS) ──
	// Scans order_items once; priority: all cancelled → Скасовано,
	// any cooking → Готується, any new → Нове, else → Готове.
	// "Готується" takes priority over "Нове" so that editing an in-progress order
	// (adding new items while others are already cooking) keeps the cooking status.
	// Does not touch orders already in Закрито.
	if _, err = tx.Exec(`
		WITH s AS (
			SELECT
				COUNT(CASE WHEN oi.order_item_quantity > oi.cancelled_quantity                                                THEN 1 END) AS active_cnt,
				COUNT(CASE WHEN oi.order_item_quantity > oi.cancelled_quantity AND ct.cooking_task_id IS NULL                THEN 1 END) AS new_cnt,
				COUNT(CASE WHEN oi.order_item_quantity > oi.cancelled_quantity AND ct.cooking_task_id IS NOT NULL
				                                                               AND ct.cooking_task_end_time IS NULL           THEN 1 END) AS cooking_cnt
			FROM order_items oi
			LEFT JOIN cooking_tasks ct ON ct.order_item_id = oi.order_item_id
			WHERE oi.order_id = @orderID
		)
		UPDATE orders
		SET order_status_id = (
			SELECT CASE
				WHEN s.active_cnt  = 0 THEN (SELECT order_status_id FROM order_statuses WHERE order_status_name = N'Скасовано')
				WHEN s.cooking_cnt > 0 THEN (SELECT order_status_id FROM order_statuses WHERE order_status_name = N'Готується')
				WHEN s.new_cnt     > 0 THEN (SELECT order_status_id FROM order_statuses WHERE order_status_name = N'Нове')
				ELSE                        (SELECT order_status_id FROM order_statuses WHERE order_status_name = N'Готове')
			END
			FROM s
		)
		WHERE order_id = @orderID
		  AND order_status_id NOT IN (
			  SELECT order_status_id FROM order_statuses WHERE order_status_name = N'Закрито'
		  )`,
		sql.Named("orderID", params.ExistingOrderID)); err != nil {
		return fmt.Errorf("recalc status: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	menuRepoLog.Printf("SyncOrderDraft: done orderID=%d", params.ExistingOrderID)
	return nil
}

// syncFetchStatuses returns the cooking status for every order_item of the given order
// in a single query instead of one per item.
func syncFetchStatuses(tx *sql.Tx, orderID int) (map[int]string, error) {
	rows, err := tx.Query(`
		SELECT oi.order_item_id,
			CASE
				WHEN ct.cooking_task_id IS NULL       THEN 'new'
				WHEN ct.cooking_task_end_time IS NULL THEN 'cooking'
				ELSE 'done'
			END
		FROM order_items oi
		LEFT JOIN cooking_tasks ct ON ct.order_item_id = oi.order_item_id
		WHERE oi.order_id = @orderID`,
		sql.Named("orderID", orderID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[int]string)
	for rows.Next() {
		var id int
		var status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, err
		}
		m[id] = status
	}
	return m, rows.Err()
}

// syncBatchDelete removes all listed order_item rows in one DELETE … IN (…).
func syncBatchDelete(tx *sql.Tx, ids []int) error {
	args := make([]any, len(ids))
	params := make([]string, len(ids))
	for i, id := range ids {
		name := fmt.Sprintf("d%d", i)
		args[i] = sql.Named(name, id)
		params[i] = "@" + name
	}
	_, err := tx.Exec(
		"DELETE FROM order_items WHERE order_item_id IN ("+strings.Join(params, ",")+")",
		args...,
	)
	return err
}

// syncBatchUpdateQty sets order_item_quantity for multiple rows in one CASE UPDATE.
func syncBatchUpdateQty(tx *sql.Tx, ids, qtys []int) error {
	when := make([]string, len(ids))
	in := make([]string, len(ids))
	args := make([]any, 0, len(ids)*2)
	for i := range ids {
		iName := fmt.Sprintf("ui%d", i)
		qName := fmt.Sprintf("uq%d", i)
		when[i] = fmt.Sprintf("WHEN @%s THEN @%s", iName, qName)
		in[i] = "@" + iName
		args = append(args, sql.Named(iName, ids[i]), sql.Named(qName, qtys[i]))
	}
	_, err := tx.Exec(
		"UPDATE order_items SET order_item_quantity = CASE order_item_id "+
			strings.Join(when, " ")+
			" END WHERE order_item_id IN ("+strings.Join(in, ",")+")",
		args...,
	)
	return err
}

// syncBatchUpdateCancel adds each delta to cancelled_quantity in one CASE UPDATE.
func syncBatchUpdateCancel(tx *sql.Tx, ids, deltas []int) error {
	when := make([]string, len(ids))
	in := make([]string, len(ids))
	args := make([]any, 0, len(ids)*2)
	for i := range ids {
		iName := fmt.Sprintf("ci%d", i)
		dName := fmt.Sprintf("cd%d", i)
		when[i] = fmt.Sprintf("WHEN @%s THEN @%s", iName, dName)
		in[i] = "@" + iName
		args = append(args, sql.Named(iName, ids[i]), sql.Named(dName, deltas[i]))
	}
	_, err := tx.Exec(
		"UPDATE order_items SET cancelled_quantity = cancelled_quantity + CASE order_item_id "+
			strings.Join(when, " ")+
			" ELSE 0 END WHERE order_item_id IN ("+strings.Join(in, ",")+")",
		args...,
	)
	return err
}

// syncBatchInsert inserts multiple new order_items in one multi-row INSERT.
func syncBatchInsert(tx *sql.Tx, orderID int, items []CartEntryForOrder) error {
	vals := make([]string, len(items))
	args := make([]any, 0, len(items)*2+1)
	args = append(args, sql.Named("orderID", orderID))
	for i, item := range items {
		qName := fmt.Sprintf("nq%d", i)
		dName := fmt.Sprintf("nd%d", i)
		vals[i] = fmt.Sprintf("(@%s, @orderID, @%s)", qName, dName)
		args = append(args, sql.Named(qName, item.Qty), sql.Named(dName, item.DishID))
	}
	_, err := tx.Exec(
		"INSERT INTO order_items (order_item_quantity, order_id, dish_id) VALUES "+
			strings.Join(vals, ","),
		args...,
	)
	return err
}
