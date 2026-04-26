package chefrepo

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

var kitchenRepoLog = log.New(log.Writer(), "[KitchenRepo] ", log.LstdFlags|log.Lshortfile)

// KitchenTaskRow — плоский рядок, що повертається з БД.
// Один рядок = одна активна позиція замовлення (order_item),
// з опційно приєднаним cooking_task.
type KitchenTaskRow struct {
	OrderID        int
	OrderNumber    string
	TableNumber    int
	WaiterName     string
	OrderCreatedAt time.Time

	CookingTaskID sql.NullInt64
	OrderItemID   int
	DishName      string
	DishCategory  string
	CookingTime   int // dish_cooking_time у хвилинах
	EffectiveQty  int // order_item_quantity - cancelled_quantity

	StartTime sql.NullTime   // cooking_task_start_time; Not Valid → статус "new"
	EndTime   sql.NullTime   // cooking_task_end_time;   Valid     → статус "ready"
	ChefID    sql.NullInt64  // NULL поки завдання ще не взято кухарем
	ChefName  sql.NullString // NULL поки завдання ще не взято кухарем
}

type KitchenRepo struct {
	db *sql.DB
}

func NewKitchenRepo(db *sql.DB) *KitchenRepo {
	return &KitchenRepo{db: db}
}

// GetActiveKitchenTasks повертає всі активні позиції для незакритих замовлень ресторану.
// Статус позиції виводиться з cooking_tasks, якщо запис вже існує.
// Результат впорядковано: спочатку за часом створення замовлення (ASC), потім за order_item_id (ASC).
func (r *KitchenRepo) GetActiveKitchenTasks(restaurantID int) ([]KitchenTaskRow, error) {
	start := time.Now()
	kitchenRepoLog.Printf("GetActiveKitchenTasks: restaurantID=%d", restaurantID)

	const query = `
		WITH ActiveOrders AS (
			SELECT
				o.order_id,
				o.order_number,
				o.order_created_at,
				t.table_number,
				w.waiter_full_name
			FROM orders o
			JOIN tables        t  ON t.table_id        = o.table_id
			JOIN waiters       w  ON w.waiter_id        = o.waiter_id
			JOIN order_statuses os ON os.order_status_id = o.order_status_id
			WHERE t.restaurant_id = @restaurantID
			  AND os.order_status_name NOT IN (N'Закрито', N'Скасовано', N'Готове')
		)
		SELECT
			ao.order_id,
			ao.order_number,
			ao.table_number,
			ao.waiter_full_name,
			ao.order_created_at,
			ct.cooking_task_id,
			oi.order_item_id,
			d.dish_name,
			dc.dish_category_name,
			d.dish_cooking_time,
			oi.order_item_quantity - oi.cancelled_quantity AS effective_qty,
			ct.cooking_task_start_time,
			ct.cooking_task_end_time,
			ct.chef_id,
			c.chef_full_name
		FROM ActiveOrders ao
		JOIN order_items    oi ON oi.order_id          = ao.order_id
		                      AND oi.order_item_quantity > oi.cancelled_quantity
		LEFT JOIN cooking_tasks ct ON ct.order_item_id = oi.order_item_id
		JOIN dishes          d ON d.dish_id            = oi.dish_id
		JOIN dish_categories dc ON dc.dish_category_id = d.dish_category_id
		LEFT JOIN chefs      c  ON c.chef_id           = ct.chef_id
		ORDER BY ao.order_created_at ASC, oi.order_item_id ASC`

	rows, err := r.db.Query(query, sql.Named("restaurantID", restaurantID))
	if err != nil {
		return nil, fmt.Errorf("GetActiveKitchenTasks query: %w", err)
	}
	defer rows.Close()

	var result []KitchenTaskRow
	for rows.Next() {
		var row KitchenTaskRow
		if err := rows.Scan(
			&row.OrderID,
			&row.OrderNumber,
			&row.TableNumber,
			&row.WaiterName,
			&row.OrderCreatedAt,
			&row.CookingTaskID,
			&row.OrderItemID,
			&row.DishName,
			&row.DishCategory,
			&row.CookingTime,
			&row.EffectiveQty,
			&row.StartTime,
			&row.EndTime,
			&row.ChefID,
			&row.ChefName,
		); err != nil {
			return nil, fmt.Errorf("GetActiveKitchenTasks scan: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetActiveKitchenTasks rows: %w", err)
	}

	kitchenRepoLog.Printf("GetActiveKitchenTasks: done=%v rows=%d restaurantID=%d", time.Since(start), len(result), restaurantID)
	return result, nil
}

// ChefRow — рядок кухаря для фільтра KDS.
type ChefRow struct {
	ID   int
	Name string
}

// GetAllChefs повертає всіх кухарів ресторану для фільтра KDS.
func (r *KitchenRepo) GetAllChefs(restaurantID int) ([]ChefRow, error) {
	rows, err := r.db.Query(`
		SELECT chef_id, chef_full_name
		FROM chefs
		WHERE restaurant_id = @restaurantID
		ORDER BY chef_full_name ASC`,
		sql.Named("restaurantID", restaurantID),
	)
	if err != nil {
		return nil, fmt.Errorf("GetAllChefs query: %w", err)
	}
	defer rows.Close()
	var result []ChefRow
	for rows.Next() {
		var c ChefRow
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, fmt.Errorf("GetAllChefs scan: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// GetOrderItemInfo повертає назву страви та ефективну кількість для одного order_item.
func (r *KitchenRepo) GetOrderItemInfo(orderItemID int) (dishName string, qty int, err error) {
	err = r.db.QueryRow(`
		SELECT d.dish_name, oi.order_item_quantity - oi.cancelled_quantity
		FROM order_items oi
		JOIN dishes d ON d.dish_id = oi.dish_id
		WHERE oi.order_item_id = @id`,
		sql.Named("id", orderItemID),
	).Scan(&dishName, &qty)
	if err != nil {
		err = fmt.Errorf("GetOrderItemInfo: %w", err)
	}
	return
}

// StartCooking фіксує початок приготування для позиції замовлення (order_item_id).
// Якщо cooking_task відсутній, створює його одразу в статусі "cooking".
// Якщо вже існує, апдейтом переводить у "cooking" лише якщо старт ще не зафіксовано.
func (r *KitchenRepo) StartCooking(orderItemID, chefID int) error {
	kitchenRepoLog.Printf("StartCooking: orderItemID=%d chefID=%d", orderItemID, chefID)
	_, err := r.db.Exec(`
		UPDATE cooking_tasks
		SET cooking_task_start_time = GETUTCDATE(),
		    chef_id                 = @chefID
		WHERE order_item_id            = @orderItemID
		  AND cooking_task_start_time IS NULL;

		IF @@ROWCOUNT = 0
		BEGIN
			INSERT INTO cooking_tasks (order_item_id, cooking_task_start_time, chef_id)
			SELECT @orderItemID, GETUTCDATE(), @chefID
			WHERE NOT EXISTS (
				SELECT 1 FROM cooking_tasks WHERE order_item_id = @orderItemID
			)
		END`,
		sql.Named("orderItemID", orderItemID),
		sql.Named("chefID", chefID),
	)
	if err != nil {
		return fmt.Errorf("StartCooking exec: %w", err)
	}
	return nil
}

// FinishCooking фіксує завершення приготування: встановлює end_time.
// Спрацьовує лише якщо завдання вже розпочато і ще не завершено.
func (r *KitchenRepo) FinishCooking(taskID int) error {
	kitchenRepoLog.Printf("FinishCooking: taskID=%d", taskID)
	_, err := r.db.Exec(`
		UPDATE cooking_tasks
		SET cooking_task_end_time = GETUTCDATE()
		WHERE cooking_task_id            = @taskID
		  AND cooking_task_start_time IS NOT NULL
		  AND cooking_task_end_time   IS NULL`,
		sql.Named("taskID", taskID),
	)
	if err != nil {
		return fmt.Errorf("FinishCooking exec: %w", err)
	}
	return nil
}
