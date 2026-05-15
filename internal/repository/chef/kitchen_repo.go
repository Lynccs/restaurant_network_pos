package chefrepo

import (
	"database/sql"
	"fmt"
	"log"
	"math"
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

	StartTime    sql.NullTime   // cooking_task_start_time; Not Valid → статус "new"
	EndTime      sql.NullTime   // cooking_task_end_time;   Valid     → статус "ready"
	ChefID       sql.NullInt64  // NULL поки завдання ще не взято кухарем
	ChefName     sql.NullString // NULL поки завдання ще не взято кухарем
	ChefWorkshop sql.NullString // спеціалізація кухаря (цех)
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

	// Drive from tables first (small set filtered by restaurant_id), then seek into
	// orders using idx_orders_kitchen_board(table_id, order_status_id).
	// OPTION(FORCE ORDER) locks this join strategy so the optimizer cannot revert
	// to a full orders scan regardless of index availability.
	const query = `
		WITH ActiveOrders AS (
			SELECT
				o.order_id,
				o.order_number,
				o.order_created_at,
				t.table_number,
				w.waiter_full_name
			FROM tables        t
			JOIN orders        o  ON o.table_id         = t.table_id
			JOIN waiters       w  ON w.waiter_id         = o.waiter_id
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
			c.chef_full_name,
			cs.chef_specialization_name
		FROM ActiveOrders ao
		JOIN order_items    oi ON oi.order_id           = ao.order_id
		                      AND oi.order_item_quantity > oi.cancelled_quantity
		                      AND oi.order_item_has_issue = 0
		LEFT JOIN cooking_tasks ct ON ct.order_item_id  = oi.order_item_id
		JOIN dishes          d  ON d.dish_id            = oi.dish_id
		JOIN dish_categories dc ON dc.dish_category_id  = d.dish_category_id
		LEFT JOIN chefs      c  ON c.chef_id            = ct.chef_id
		LEFT JOIN chef_specializations cs ON cs.chef_specialization_id = c.chef_specialization_id
		ORDER BY ao.order_created_at ASC, oi.order_item_id ASC
		OPTION (FORCE ORDER)`

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
			&row.ChefWorkshop,
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

// GetReadyTasksByDate повертає всі завершені позиції (cooking_task_end_time IS NOT NULL)
// за вказану дату (UTC) для ресторану. Використовується для архівного перегляду "Готових".
func (r *KitchenRepo) GetReadyTasksByDate(restaurantID int, date time.Time) ([]KitchenTaskRow, error) {
	start := time.Now()
	kitchenRepoLog.Printf("GetReadyTasksByDate: restaurantID=%d date=%s", restaurantID, date.Format("2006-01-02"))

	// Порядок колонок відповідає порядку Scan нижче (такий самий як у GetActiveKitchenTasks).
	const query = `
		SELECT
			o.order_id,
			o.order_number,
			t.table_number,
			w.waiter_full_name,
			o.order_created_at,
			ct.cooking_task_id,
			oi.order_item_id,
			d.dish_name,
			dc.dish_category_name,
			d.dish_cooking_time,
			oi.order_item_quantity - oi.cancelled_quantity AS effective_qty,
			ct.cooking_task_start_time,
			ct.cooking_task_end_time,
			ct.chef_id,
			c.chef_full_name,
			cs.chef_specialization_name
		FROM cooking_tasks    ct
		JOIN order_items    oi ON oi.order_item_id      = ct.order_item_id
		JOIN orders          o ON o.order_id             = oi.order_id
		JOIN tables          t ON t.table_id              = o.table_id
		JOIN waiters         w ON w.waiter_id              = o.waiter_id
		JOIN dishes          d ON d.dish_id               = oi.dish_id
		JOIN dish_categories dc ON dc.dish_category_id   = d.dish_category_id
		JOIN chefs           c ON c.chef_id               = ct.chef_id
		LEFT JOIN chef_specializations cs ON cs.chef_specialization_id = c.chef_specialization_id
		WHERE t.restaurant_id            = @restaurantID
		  AND ct.cooking_task_end_time  IS NOT NULL
		  AND CONVERT(DATE, ct.cooking_task_end_time) = @date
		ORDER BY ct.cooking_task_end_time ASC, ct.cooking_task_id ASC`

	rows, err := r.db.Query(query,
		sql.Named("restaurantID", restaurantID),
		sql.Named("date", date.Format("2006-01-02")),
	)
	if err != nil {
		return nil, fmt.Errorf("GetReadyTasksByDate query: %w", err)
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
			&row.ChefWorkshop,
		); err != nil {
			return nil, fmt.Errorf("GetReadyTasksByDate scan: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetReadyTasksByDate rows: %w", err)
	}

	kitchenRepoLog.Printf("GetReadyTasksByDate: done=%v rows=%d", time.Since(start), len(result))
	return result, nil
}

// ChefRow — рядок кухаря для фільтра KDS.
type ChefRow struct {
	ID       int
	Name     string
	Workshop string
}

// GetAllChefs повертає всіх кухарів ресторану для фільтра KDS.
func (r *KitchenRepo) GetAllChefs(restaurantID int) ([]ChefRow, error) {
	rows, err := r.db.Query(`
		SELECT c.chef_id, c.chef_full_name, COALESCE(cs.chef_specialization_name, '')
		FROM chefs c
		LEFT JOIN chef_specializations cs ON cs.chef_specialization_id = c.chef_specialization_id
		WHERE c.restaurant_id = @restaurantID AND c.is_deleted = 0
		ORDER BY c.chef_full_name ASC`,
		sql.Named("restaurantID", restaurantID),
	)
	if err != nil {
		return nil, fmt.Errorf("GetAllChefs query: %w", err)
	}
	defer rows.Close()
	var result []ChefRow
	for rows.Next() {
		var c ChefRow
		if err := rows.Scan(&c.ID, &c.Name, &c.Workshop); err != nil {
			return nil, fmt.Errorf("GetAllChefs scan: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// IngredientModalRow — рядок інгредієнта для модального вікна "Почати приготування".
type IngredientModalRow struct {
	IngredientID int
	Name         string
	Unit         string
	RecipeQty    float64 // > 0 якщо входить до рецепту страви
	StockQty     float64 // поточний залишок на складі (не прострочений)
}

// GetStartCookingData завантажує назву страви, к-сть порцій, рецептурні та решту інгредієнтів.
func (r *KitchenRepo) GetStartCookingData(orderItemID, restaurantID int) (dishName string, qty int, recipe []IngredientModalRow, others []IngredientModalRow, err error) {
	err = r.db.QueryRow(`
		SELECT d.dish_name, oi.order_item_quantity - oi.cancelled_quantity
		FROM order_items oi
		JOIN dishes d ON d.dish_id = oi.dish_id
		WHERE oi.order_item_id = @orderItemID`,
		sql.Named("orderItemID", orderItemID),
	).Scan(&dishName, &qty)
	if err != nil {
		err = fmt.Errorf("GetStartCookingData dish: %w", err)
		return
	}

	recipeRows, qErr := r.db.Query(`
		SELECT
			i.ingredient_id,
			i.ingredient_name,
			iu.ingredient_unit_name,
			di.dish_ingredient_quantity * CAST(@qty AS DECIMAL(10,4)) AS recipe_qty,
			COALESCE((
				SELECT SUM(si.stock_ingredient_quantity)
				FROM stock_ingredients si
				WHERE si.ingredient_id = i.ingredient_id
				  AND si.restaurant_id = @restaurantID
				  AND si.stock_ingredient_expiration_date > GETDATE()
			), 0) AS stock_qty
		FROM order_items oi
		JOIN dishes d ON d.dish_id = oi.dish_id
		JOIN dish_ingredients di ON di.dish_id = d.dish_id
		JOIN ingredients i ON i.ingredient_id = di.ingredient_id
		JOIN ingredient_units iu ON iu.ingredient_unit_id = i.ingredient_unit_id
		WHERE oi.order_item_id = @orderItemID
		ORDER BY i.ingredient_name`,
		sql.Named("orderItemID", orderItemID),
		sql.Named("qty", qty),
		sql.Named("restaurantID", restaurantID),
	)
	if qErr != nil {
		err = fmt.Errorf("GetStartCookingData recipe: %w", qErr)
		return
	}
	defer recipeRows.Close()
	for recipeRows.Next() {
		var row IngredientModalRow
		if sErr := recipeRows.Scan(&row.IngredientID, &row.Name, &row.Unit, &row.RecipeQty, &row.StockQty); sErr != nil {
			err = fmt.Errorf("GetStartCookingData recipe scan: %w", sErr)
			return
		}
		recipe = append(recipe, row)
	}
	if err = recipeRows.Err(); err != nil {
		return
	}

	othersRows, qErr := r.db.Query(`
		SELECT
			i.ingredient_id,
			i.ingredient_name,
			iu.ingredient_unit_name,
			COALESCE((
				SELECT SUM(si.stock_ingredient_quantity)
				FROM stock_ingredients si
				WHERE si.ingredient_id = i.ingredient_id
				  AND si.restaurant_id = @restaurantID
				  AND si.stock_ingredient_expiration_date > GETDATE()
			), 0) AS stock_qty
		FROM ingredients i
		JOIN ingredient_units iu ON iu.ingredient_unit_id = i.ingredient_unit_id
		WHERE i.ingredient_id NOT IN (
			SELECT di.ingredient_id
			FROM dish_ingredients di
			JOIN dishes d ON d.dish_id = di.dish_id
			JOIN order_items oi2 ON oi2.dish_id = d.dish_id
			WHERE oi2.order_item_id = @orderItemID
		)
		ORDER BY i.ingredient_name`,
		sql.Named("orderItemID", orderItemID),
		sql.Named("restaurantID", restaurantID),
	)
	if qErr != nil {
		err = fmt.Errorf("GetStartCookingData others: %w", qErr)
		return
	}
	defer othersRows.Close()
	for othersRows.Next() {
		var row IngredientModalRow
		if sErr := othersRows.Scan(&row.IngredientID, &row.Name, &row.Unit, &row.StockQty); sErr != nil {
			err = fmt.Errorf("GetStartCookingData others scan: %w", sErr)
			return
		}
		others = append(others, row)
	}
	err = othersRows.Err()
	return
}

// RecordIngredientUsages фіксує фактично використані інгредієнти для завдання приготування.
// Для кожного інгредієнта обирається найстаріша не прострочена партія (FIFO).
// Якщо запас не знайдено — запис пропускається (некритична помилка).
func (r *KitchenRepo) RecordIngredientUsages(orderItemID, restaurantID int, usages map[int]float64) error {
	var taskID int
	if err := r.db.QueryRow(`
		SELECT cooking_task_id FROM cooking_tasks WHERE order_item_id = @id`,
		sql.Named("id", orderItemID),
	).Scan(&taskID); err != nil {
		return fmt.Errorf("RecordIngredientUsages taskID: %w", err)
	}

	for ingID, totalQty := range usages {
		if totalQty <= 0 {
			continue
		}
		// FIFO по партіях: дренуємо найстаріші партії поки не спишемо потрібну кількість.
		// Після кожного INSERT тригер одразу зменшує stock_ingredient_quantity тієї партії,
		// тому наступний SELECT вже бачить оновлений залишок у межах тієї ж транзакції.
		remaining := totalQty
		for remaining > 1e-6 {
			var stockID int
			var available float64
			err := r.db.QueryRow(`
				SELECT TOP 1 stock_ingredient_id, stock_ingredient_quantity
				FROM stock_ingredients
				WHERE ingredient_id                    = @ingID
				  AND restaurant_id                    = @restID
				  AND stock_ingredient_expiration_date > GETDATE()
				  AND stock_ingredient_quantity        > 0
				ORDER BY stock_ingredient_received_at ASC`,
				sql.Named("ingID", ingID),
				sql.Named("restID", restaurantID),
			).Scan(&stockID, &available)
			if err != nil {
				// Партій більше немає — логуємо і виходимо з внутрішнього циклу
				kitchenRepoLog.Printf("RecordIngredientUsages: stock exhausted for ingID=%d, remaining=%.3f", ingID, remaining)
				break
			}
			take := math.Min(available, remaining)
			if _, err := r.db.Exec(`
				INSERT INTO ingredient_usages
					(ingredient_usage_quantity, ingredient_usage_time, stock_ingredient_id, cooking_task_id)
				VALUES (@qty, GETDATE(), @stockID, @taskID)`,
				sql.Named("qty", take),
				sql.Named("stockID", stockID),
				sql.Named("taskID", taskID),
			); err != nil {
				return fmt.Errorf("RecordIngredientUsages insert ingID=%d: %w", ingID, err)
			}
			remaining -= take
		}
	}
	return nil
}

// ReportIssue ставить флаг order_item_has_issue = 1 для позиції (не змінює cancelled_quantity).
// Повертає назву страви, номер столу та повну кількість порцій для SSE-пейлоаду.
func (r *KitchenRepo) ReportIssue(orderItemID int) (dishName string, tableNumber int, qty int, err error) {
	kitchenRepoLog.Printf("ReportIssue: orderItemID=%d", orderItemID)

	_, err = r.db.Exec(`
		UPDATE order_items
		SET order_item_has_issue = 1
		WHERE order_item_id = @orderItemID`,
		sql.Named("orderItemID", orderItemID),
	)
	if err != nil {
		err = fmt.Errorf("ReportIssue update: %w", err)
		return
	}

	err = r.db.QueryRow(`
		SELECT d.dish_name, t.table_number, oi.order_item_quantity
		FROM order_items oi
		JOIN dishes d ON d.dish_id   = oi.dish_id
		JOIN orders o ON o.order_id  = oi.order_id
		JOIN tables t ON t.table_id  = o.table_id
		WHERE oi.order_item_id = @orderItemID`,
		sql.Named("orderItemID", orderItemID),
	).Scan(&dishName, &tableNumber, &qty)
	if err != nil {
		err = fmt.Errorf("ReportIssue select: %w", err)
	}
	return
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
		SET cooking_task_start_time = GETDATE(),
		    chef_id                 = @chefID
		WHERE order_item_id            = @orderItemID
		  AND cooking_task_start_time IS NULL;

		IF @@ROWCOUNT = 0
		BEGIN
			INSERT INTO cooking_tasks (order_item_id, cooking_task_start_time, chef_id)
			SELECT @orderItemID, GETDATE(), @chefID
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
		SET cooking_task_end_time = GETDATE()
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
