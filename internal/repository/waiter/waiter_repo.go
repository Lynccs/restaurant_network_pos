package waiterrepo

import (
	"database/sql"
	"log"
)

var repoLog = log.New(log.Writer(), "[WaiterRepo] ", log.LstdFlags|log.Lshortfile)

type TableRow struct {
	ID             int
	Number         int
	Capacity       int
	HasActiveOrder bool
	OrderCreatedAt sql.NullTime
	HasIssue       bool
}

type WaiterRepo struct {
	db *sql.DB
}

func NewWaiterRepo(db *sql.DB) *WaiterRepo {
	return &WaiterRepo{db: db}
}

func (r *WaiterRepo) GetTablesByRestaurant(restaurantID int) ([]TableRow, error) {
	const query = `
		SELECT
			t.table_id,
			t.table_number,
			t.table_capacity,
			MAX(CASE WHEN o.order_id IS NOT NULL THEN 1 ELSE 0 END),
			MAX(o.order_created_at),
			CAST(MAX(CASE
				WHEN oi.order_item_has_issue = 1
				 AND oi.order_item_quantity > oi.cancelled_quantity THEN 1
				ELSE 0
			END) AS BIT) AS has_issue
		FROM tables t
		LEFT JOIN orders o ON o.table_id = t.table_id
			AND o.order_status_id NOT IN (
				SELECT order_status_id FROM order_statuses
				WHERE order_status_name IN ('Закрито', 'Скасовано')
			)
		LEFT JOIN order_items oi ON oi.order_id = o.order_id
		WHERE t.restaurant_id = @restaurantID
		GROUP BY t.table_id, t.table_number, t.table_capacity
		ORDER BY t.table_number`

	repoLog.Printf("GetTablesByRestaurant: restaurantID=%d", restaurantID)

	rows, err := r.db.Query(query, sql.Named("restaurantID", restaurantID))
	if err != nil {
		repoLog.Printf("GetTablesByRestaurant: query error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var result []TableRow
	for rows.Next() {
		var row TableRow
		var hasActive int
		if err := rows.Scan(&row.ID, &row.Number, &row.Capacity, &hasActive, &row.OrderCreatedAt, &row.HasIssue); err != nil {
			repoLog.Printf("GetTablesByRestaurant: scan error: %v", err)
			return nil, err
		}
		row.HasActiveOrder = hasActive == 1
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		repoLog.Printf("GetTablesByRestaurant: rows error: %v", err)
		return nil, err
	}

	repoLog.Printf("GetTablesByRestaurant: found %d tables", len(result))
	return result, nil
}

func (r *WaiterRepo) GetRestaurantName(restaurantID int) (string, error) {
	const query = `SELECT restaurant_name FROM restaurants WHERE restaurant_id = @restaurantID`

	repoLog.Printf("GetRestaurantName: restaurantID=%d", restaurantID)

	var name string
	err := r.db.QueryRow(query, sql.Named("restaurantID", restaurantID)).Scan(&name)
	if err != nil {
		repoLog.Printf("GetRestaurantName: query error: %v", err)
		return "", err
	}

	repoLog.Printf("GetRestaurantName: found %s", name)
	return name, nil
}
