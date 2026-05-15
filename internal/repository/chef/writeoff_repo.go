package chefrepo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const WriteOffPageSize = 10

// WriteOffFilters — параметри фільтрації для сторінки списаних інгредієнтів.
type WriteOffFilters struct {
	OrderNumber string // часткове співпадіння по номеру замовлення
	FromTime    string // "YYYY-MM-DDTHH:MM" (datetime-local input)
	ToTime      string // "YYYY-MM-DDTHH:MM"
	Dish        string // точне ім'я страви або "all"/"" для всіх
	Ingredient  string // префіксне співпадіння по назві інгредієнта
}

// WriteOffRow — один рядок результату (один інгредієнт одного списання).
type WriteOffRow struct {
	OrderNumber    string
	TableNumber    int
	WaiterName     string
	DishName       string
	EffectiveQty   int
	IngredientName string
	IngredientQty  float64
	IngredientUnit string
	UsageTime      time.Time
}

// GetWriteOffData повертає сторінку рядків та загальну кількість замовлень.
//
// filtered_orders будується через orders→EXISTS замість ingredient_usages→DISTINCT:
//   - стартуємо з малої таблиці orders (індексована по table_id)
//   - EXISTS зупиняється на першому збігу — нема дорогого DISTINCT-сортування
//   - ingredient_usages сканується лише всередині EXISTS (по cooking_task_id)
//   - зовнішній SELECT читає дані тільки для 10 замовлень paged_orders
// OPTION (RECOMPILE) запобігає parameter sniffing.
func (r *KitchenRepo) GetWriteOffData(restaurantID int, f WriteOffFilters, page int) ([]WriteOffRow, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * WriteOffPageSize

	var fromTimeVal, toTimeVal time.Time
	var hasFrom, hasTo bool
	if f.FromTime != "" {
		if t, err := time.Parse("2006-01-02T15:04", f.FromTime); err == nil {
			fromTimeVal = t
			hasFrom = true
		}
	}
	if f.ToTime != "" {
		if t, err := time.Parse("2006-01-02T15:04", f.ToTime); err == nil {
			toTimeVal = t
			hasTo = true
		}
	}

	args := []any{sql.Named("restaurantID", restaurantID)}

	// ing_tasks — cooking_task_id де використовувався потрібний інгредієнт.
	// Будується тільки якщо активний фільтр по назві інгредієнта.
	var ingCTEFrag, ingJoinExists string
	if f.Ingredient != "" {
		args = append(args, sql.Named("ingredient", f.Ingredient))
		var ingWhere strings.Builder
		ingWhere.WriteString("i_f.ingredient_name LIKE @ingredient + '%'")
		if hasFrom {
			ingWhere.WriteString("\n        AND iu_f.ingredient_usage_time >= @fromTime")
		}
		if hasTo {
			ingWhere.WriteString("\n        AND iu_f.ingredient_usage_time <= @toTime")
		}
		ingCTEFrag = fmt.Sprintf(`ing_tasks AS (
    SELECT DISTINCT iu_f.cooking_task_id
    FROM   ingredient_usages iu_f
    JOIN   stock_ingredients si_f ON si_f.stock_ingredient_id = iu_f.stock_ingredient_id
    JOIN   ingredients       i_f  ON i_f.ingredient_id        = si_f.ingredient_id
    WHERE  %s
),
`, ingWhere.String())
		ingJoinExists = "\n        JOIN ing_tasks it ON it.cooking_task_id = ct.cooking_task_id"
	}

	// Зовнішній WHERE для orders (order_number — без переходу в ingredient_usages).
	var orderWhere strings.Builder
	orderWhere.WriteString("t.restaurant_id = @restaurantID")
	if f.OrderNumber != "" {
		args = append(args, sql.Named("orderNumber", f.OrderNumber))
		orderWhere.WriteString("\n    AND o.order_number LIKE '%' + @orderNumber + '%'")
	}

	// Фільтри всередині EXISTS (dish, час, інгредієнт).
	var existsJoins, existsWhere strings.Builder
	existsWhere.WriteString("oi.order_id = o.order_id")
	if f.Dish != "" && f.Dish != "all" {
		args = append(args, sql.Named("dish", f.Dish))
		existsJoins.WriteString("\n        JOIN dishes d_e ON d_e.dish_id = oi.dish_id")
		existsWhere.WriteString("\n        AND d_e.dish_name = @dish")
	}
	if hasFrom {
		args = append(args, sql.Named("fromTime", fromTimeVal))
		existsWhere.WriteString("\n        AND iu.ingredient_usage_time >= @fromTime")
	}
	if hasTo {
		args = append(args, sql.Named("toTime", toTimeVal))
		existsWhere.WriteString("\n        AND iu.ingredient_usage_time <= @toTime")
	}

	allArgs := make([]any, len(args), len(args)+2)
	copy(allArgs, args)
	allArgs = append(allArgs,
		sql.Named("offset", offset),
		sql.Named("pageSize", WriteOffPageSize),
	)

	dataSQL := fmt.Sprintf(`WITH %sfiltered_orders AS (
    SELECT o.order_id, o.order_created_at
    FROM   orders o
    JOIN   tables t ON t.table_id = o.table_id
    WHERE  %s
    AND    EXISTS (
        SELECT 1
        FROM   order_items       oi%s
        JOIN   cooking_tasks     ct  ON ct.order_item_id   = oi.order_item_id%s
        JOIN   ingredient_usages iu  ON iu.cooking_task_id = ct.cooking_task_id
        WHERE  %s
    )
),
total_count AS (
    SELECT COUNT(*) AS cnt FROM filtered_orders
),
paged_orders AS (
    SELECT order_id
    FROM (
        SELECT order_id, ROW_NUMBER() OVER (ORDER BY order_created_at DESC) AS rn
        FROM   filtered_orders
    ) r
    WHERE rn > @offset AND rn <= @offset + @pageSize
)
SELECT
    o.order_number,
    t.table_number,
    w.waiter_full_name,
    d.dish_name,
    oi.order_item_quantity - oi.cancelled_quantity AS effective_qty,
    i.ingredient_name,
    iu.ingredient_usage_quantity,
    iu.ingredient_usage_time,
    ISNULL(u.ingredient_unit_name, '') AS ingredient_unit_name,
    (SELECT cnt FROM total_count)      AS total_orders
FROM   paged_orders          po
JOIN   orders                o   ON o.order_id             = po.order_id
JOIN   tables                t   ON t.table_id             = o.table_id
JOIN   waiters               w   ON w.waiter_id            = o.waiter_id
JOIN   order_items           oi  ON oi.order_id            = o.order_id
JOIN   cooking_tasks         ct  ON ct.order_item_id       = oi.order_item_id
JOIN   ingredient_usages     iu  ON iu.cooking_task_id     = ct.cooking_task_id
JOIN   stock_ingredients     si  ON si.stock_ingredient_id = iu.stock_ingredient_id
JOIN   ingredients           i   ON i.ingredient_id        = si.ingredient_id
LEFT JOIN ingredient_units   u   ON u.ingredient_unit_id   = i.ingredient_unit_id
JOIN   dishes                d   ON d.dish_id              = oi.dish_id
ORDER BY o.order_created_at DESC, oi.order_item_id ASC, i.ingredient_name ASC
OPTION (RECOMPILE)`,
		ingCTEFrag,
		orderWhere.String(),
		existsJoins.String(),
		ingJoinExists,
		existsWhere.String(),
	)

	rows, err := r.db.Query(dataSQL, allArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("GetWriteOffData query: %w", err)
	}
	defer rows.Close()

	var result []WriteOffRow
	var totalOrders int
	for rows.Next() {
		var row WriteOffRow
		if err := rows.Scan(
			&row.OrderNumber,
			&row.TableNumber,
			&row.WaiterName,
			&row.DishName,
			&row.EffectiveQty,
			&row.IngredientName,
			&row.IngredientQty,
			&row.UsageTime,
			&row.IngredientUnit,
			&totalOrders,
		); err != nil {
			return nil, 0, fmt.Errorf("GetWriteOffData scan: %w", err)
		}
		result = append(result, row)
	}
	return result, totalOrders, rows.Err()
}

// GetWriteOffOptions повертає назви страв та інгредієнтів для фільтрів.
func (r *KitchenRepo) GetWriteOffOptions(restaurantID int) (dishes []string, ingredients []string, err error) {
	dishRows, err := r.db.Query(`
		SELECT DISTINCT d.dish_name
		FROM   dishes      d
		JOIN   order_items oi ON oi.dish_id  = d.dish_id
		JOIN   orders      o  ON o.order_id  = oi.order_id
		JOIN   tables      t  ON t.table_id  = o.table_id
		WHERE  t.restaurant_id = @restaurantID
		ORDER BY d.dish_name`,
		sql.Named("restaurantID", restaurantID),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("GetWriteOffOptions dishes: %w", err)
	}
	defer dishRows.Close()
	for dishRows.Next() {
		var name string
		if err := dishRows.Scan(&name); err != nil {
			return nil, nil, fmt.Errorf("GetWriteOffOptions dish scan: %w", err)
		}
		dishes = append(dishes, name)
	}
	if err := dishRows.Err(); err != nil {
		return nil, nil, err
	}

	ingRows, err := r.db.Query(`SELECT ingredient_name FROM ingredients ORDER BY ingredient_name`)
	if err != nil {
		return nil, nil, fmt.Errorf("GetWriteOffOptions ingredients: %w", err)
	}
	defer ingRows.Close()
	for ingRows.Next() {
		var name string
		if err := ingRows.Scan(&name); err != nil {
			return nil, nil, fmt.Errorf("GetWriteOffOptions ingredient scan: %w", err)
		}
		ingredients = append(ingredients, name)
	}
	return dishes, ingredients, ingRows.Err()
}
