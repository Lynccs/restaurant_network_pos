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
// Один SQL-запит: ROW_NUMBER() для пагінації + COUNT(*) OVER() для загального числа.
// OPTION (RECOMPILE) запобігає кешуванню неоптимального плану (parameter sniffing).
func (r *KitchenRepo) GetWriteOffData(restaurantID int, f WriteOffFilters, page int) ([]WriteOffRow, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * WriteOffPageSize

	// Парсимо часові фільтри один раз.
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

	// ing_tasks CTE з часовими фільтрами — дозволяє idx_ingredient_usages_time
	// обмежити скан лише поточним діапазоном замість повної таблиці.
	var ingCTEFrag string
	var ingJoinInner string
	var ingJoinOuter string
	if f.Ingredient != "" {
		args = append(args, sql.Named("ingredient", f.Ingredient))
		var ingWhere strings.Builder
		ingWhere.WriteString("i_f.ingredient_name LIKE @ingredient + '%'")
		if hasFrom {
			ingWhere.WriteString("\n    AND iu_f.ingredient_usage_time >= @fromTime")
		}
		if hasTo {
			ingWhere.WriteString("\n    AND iu_f.ingredient_usage_time <= @toTime")
		}
		ingCTEFrag = fmt.Sprintf(`ing_tasks AS (
    SELECT DISTINCT iu_f.cooking_task_id
    FROM   ingredient_usages iu_f
    JOIN   stock_ingredients si_f ON si_f.stock_ingredient_id = iu_f.stock_ingredient_id
    JOIN   ingredients       i_f  ON i_f.ingredient_id        = si_f.ingredient_id
    WHERE  %s
)`, ingWhere.String())
		ingJoinInner = "\n\t\tJOIN ing_tasks it ON it.cooking_task_id = ct.cooking_task_id"
		ingJoinOuter = "\nJOIN ing_tasks it ON it.cooking_task_id = ct.cooking_task_id"
	}

	// WHERE-умови для внутрішнього підзапиту.
	var wb strings.Builder
	wb.WriteString("t.restaurant_id = @restaurantID")
	if f.OrderNumber != "" {
		args = append(args, sql.Named("orderNumber", f.OrderNumber))
		wb.WriteString("\n\t\tAND o.order_number LIKE '%' + @orderNumber + '%'")
	}
	if f.Dish != "" && f.Dish != "all" {
		args = append(args, sql.Named("dish", f.Dish))
		wb.WriteString("\n\t\tAND d.dish_name = @dish")
	}
	if hasFrom {
		args = append(args, sql.Named("fromTime", fromTimeVal))
		wb.WriteString("\n\t\tAND iu.ingredient_usage_time >= @fromTime")
	}
	if hasTo {
		args = append(args, sql.Named("toTime", toTimeVal))
		wb.WriteString("\n\t\tAND iu.ingredient_usage_time <= @toTime")
	}
	whereClause := wb.String()

	// innerFrom — спільний FROM для inner DISTINCT-підзапиту.
	innerFrom := fmt.Sprintf(`
FROM   ingredient_usages iu
JOIN   cooking_tasks     ct  ON ct.cooking_task_id     = iu.cooking_task_id%s
JOIN   order_items       oi  ON oi.order_item_id       = ct.order_item_id
JOIN   orders            o   ON o.order_id             = oi.order_id
JOIN   tables            t   ON t.table_id             = o.table_id
JOIN   dishes            d   ON d.dish_id              = oi.dish_id
WHERE  %s`, ingJoinInner, whereClause)

	// Аргументи з pagination-параметрами.
	allArgs := make([]any, len(args), len(args)+2)
	copy(allArgs, args)
	allArgs = append(allArgs,
		sql.Named("offset", offset),
		sql.Named("pageSize", WriteOffPageSize),
	)

	// paged_orders містить ROW_NUMBER (для пагінації) і COUNT(*) OVER () (для total).
	// Один запит замість двох — усуває зайвий round-trip до SQL Server.
	var cteBuilder strings.Builder
	cteBuilder.WriteString("WITH ")
	if ingCTEFrag != "" {
		cteBuilder.WriteString(ingCTEFrag)
		cteBuilder.WriteString(",\n")
	}
	cteBuilder.WriteString(fmt.Sprintf(`paged_orders AS (
    SELECT order_id, order_created_at,
           ROW_NUMBER() OVER (ORDER BY order_created_at DESC) AS rn,
           COUNT(*)     OVER ()                               AS total_orders
    FROM (
        SELECT DISTINCT o.order_id, o.order_created_at%s
    ) AS q
)`, innerFrom))

	// Зовнішній WHERE: відбір сторінки + re-apply часових фільтрів для outer iu.
	var owb strings.Builder
	owb.WriteString("po.rn > @offset AND po.rn <= @offset + @pageSize")
	if hasFrom {
		owb.WriteString("\n\tAND iu.ingredient_usage_time >= @fromTime")
	}
	if hasTo {
		owb.WriteString("\n\tAND iu.ingredient_usage_time <= @toTime")
	}

	// OPTION (RECOMPILE) — SQL Server будує свіжий план під поточні параметри.
	// Запобігає parameter sniffing: випадкам, коли закешований план (оптимальний
	// для великого датасету) застосовується до маленького, або навпаки.
	dataSQL := cteBuilder.String() + fmt.Sprintf(`
SELECT
	o.order_number,
	t.table_number,
	w.waiter_full_name,
	d.dish_name,
	oi.order_item_quantity - oi.cancelled_quantity AS effective_qty,
	i.ingredient_name,
	iu.ingredient_usage_quantity,
	iu.ingredient_usage_time,
	u.ingredient_unit_name,
	po.total_orders
FROM   ingredient_usages     iu
JOIN   cooking_tasks         ct  ON ct.cooking_task_id     = iu.cooking_task_id%s
JOIN   order_items           oi  ON oi.order_item_id       = ct.order_item_id
JOIN   orders                o   ON o.order_id             = oi.order_id
JOIN   paged_orders          po  ON po.order_id            = o.order_id
JOIN   tables                t   ON t.table_id             = o.table_id
JOIN   waiters               w   ON w.waiter_id            = o.waiter_id
JOIN   dishes                d   ON d.dish_id              = oi.dish_id
JOIN   stock_ingredients     si  ON si.stock_ingredient_id = iu.stock_ingredient_id
JOIN   ingredients           i   ON i.ingredient_id        = si.ingredient_id
JOIN   ingredient_units      u   ON u.ingredient_unit_id   = i.ingredient_unit_id
WHERE  %s
ORDER BY po.order_created_at DESC, oi.order_item_id ASC, i.ingredient_name ASC
OPTION (RECOMPILE)`, ingJoinOuter, owb.String())

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
