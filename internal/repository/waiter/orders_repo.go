package waiterrepo

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"
)

var ordersRepoLog = log.New(log.Writer(), "[OrdersRepo] ", log.LstdFlags|log.Lshortfile)

const payNumChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func generatePaymentNumber() string {
	b := make([]byte, 10)
	for i := range b {
		b[i] = payNumChars[rand.Intn(len(payNumChars))]
	}
	return "PAY-" + string(b)
}

type OrderListFilters struct {
	Search      string
	StatusName  string
	TableNumber int
	TimeFrom    string // "HH:MM"
	TimeTo      string // "HH:MM"
}

type OrderListRow struct {
	OrderID     int
	OrderNumber string
	TableNumber int
	WaiterName  string
	TotalAmount float64
	CreatedAt   time.Time
	StatusName  string
	ItemID      int
	DishName    string
	DishPrice   float64
	ItemQty     int
}

type OrdersRepo struct {
	db *sql.DB
}

func NewOrdersRepo(db *sql.DB) *OrdersRepo {
	return &OrdersRepo{db: db}
}

func (r *OrdersRepo) GetActiveOrdersList(restaurantID int, f OrderListFilters) ([]OrderListRow, error) {
	ordersRepoLog.Printf("GetActiveOrdersList: restaurantID=%d search=%q status=%q table=%d",
		restaurantID, f.Search, f.StatusName, f.TableNumber)

	args := []any{sql.Named("restaurantID", restaurantID)}

	var sb strings.Builder
	sb.WriteString(`
		WITH FilteredOrders AS (
			SELECT
				o.order_id, o.order_number,
				t.table_number,
				w.waiter_full_name,
				o.order_total_amount,
				o.order_created_at,
				os.order_status_name
			FROM orders o
			JOIN tables t          ON t.table_id         = o.table_id
			JOIN waiters w         ON w.waiter_id         = o.waiter_id
			JOIN order_statuses os ON os.order_status_id = o.order_status_id
			WHERE t.restaurant_id = @restaurantID
			  AND os.order_status_name NOT IN (N'Закрито', N'Скасовано')
			  AND o.order_created_at >= DATEADD(day, -2, GETDATE())`)

	if f.StatusName != "" {
		args = append(args, sql.Named("statusName", f.StatusName))
		sb.WriteString("\n\t\t  AND os.order_status_name = @statusName")
	}
	if f.TableNumber != 0 {
		args = append(args, sql.Named("tableNumber", f.TableNumber))
		sb.WriteString("\n\t\t  AND t.table_number = @tableNumber")
	}
	if f.Search != "" {
		args = append(args, sql.Named("search", f.Search))
		sb.WriteString("\n\t\t  AND RIGHT(o.order_number, CHARINDEX('-', REVERSE(o.order_number)) - 1) LIKE '%' + @search + '%'")
	}
	if f.TimeFrom != "" {
		args = append(args, sql.Named("timeFrom", f.TimeFrom))
		sb.WriteString("\n\t\t  AND CAST(o.order_created_at AS TIME) >= CAST(@timeFrom AS TIME)")
	}
	if f.TimeTo != "" {
		args = append(args, sql.Named("timeTo", f.TimeTo))
		sb.WriteString("\n\t\t  AND CAST(o.order_created_at AS TIME) <= CAST(@timeTo AS TIME)")
	}

	sb.WriteString(`
		)
		SELECT
			fo.order_id, fo.order_number,
			fo.table_number,
			fo.waiter_full_name,
			fo.order_total_amount,
			fo.order_created_at,
			fo.order_status_name,
			oi.order_item_id,
			d.dish_name,
			d.dish_price,
			oi.order_item_quantity
		FROM FilteredOrders fo
		JOIN order_items oi ON oi.order_id = fo.order_id
		                   AND oi.order_item_quantity > oi.cancelled_quantity
		JOIN dishes d       ON d.dish_id   = oi.dish_id
		ORDER BY fo.order_created_at DESC, oi.order_item_id ASC`)

	rows, err := r.db.Query(sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("GetActiveOrdersList query: %w", err)
	}
	defer rows.Close()

	var result []OrderListRow
	for rows.Next() {
		var row OrderListRow
		if err := rows.Scan(
			&row.OrderID, &row.OrderNumber,
			&row.TableNumber,
			&row.WaiterName,
			&row.TotalAmount,
			&row.CreatedAt,
			&row.StatusName,
			&row.ItemID,
			&row.DishName,
			&row.DishPrice,
			&row.ItemQty,
		); err != nil {
			return nil, fmt.Errorf("GetActiveOrdersList scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

type ArchiveFilters struct {
	Search      string
	StatusName  string // optional: "Закрито" or "Скасовано"
	TableNumber int
	DateFrom    string // "YYYY-MM-DDTHH:MM"
	DateTo      string // "YYYY-MM-DDTHH:MM"
}

func (r *OrdersRepo) GetArchiveOrdersList(restaurantID, waiterID int, f ArchiveFilters) ([]OrderListRow, error) {
	ordersRepoLog.Printf("GetArchiveOrdersList: restaurantID=%d waiterID=%d search=%q status=%q table=%d",
		restaurantID, waiterID, f.Search, f.StatusName, f.TableNumber)

	args := []any{
		sql.Named("restaurantID", restaurantID),
		sql.Named("waiterID", waiterID),
		sql.Named("dateFrom", f.DateFrom),
		sql.Named("dateTo", f.DateTo),
	}

	var sb strings.Builder
	sb.WriteString(`
		WITH FilteredOrders AS (
			SELECT
				o.order_id, o.order_number,
				t.table_number,
				w.waiter_full_name,
				o.order_total_amount,
				o.order_created_at,
				os.order_status_name
			FROM orders o
			JOIN tables t          ON t.table_id         = o.table_id
			JOIN waiters w         ON w.waiter_id         = o.waiter_id
			JOIN order_statuses os ON os.order_status_id = o.order_status_id
			WHERE t.restaurant_id = @restaurantID
			  AND o.waiter_id = @waiterID
			  AND os.order_status_name IN (N'Закрито', N'Скасовано')
			  AND o.order_created_at >= @dateFrom
			  AND o.order_created_at <= @dateTo`)

	if f.StatusName != "" {
		args = append(args, sql.Named("statusName", f.StatusName))
		sb.WriteString("\n\t\t  AND os.order_status_name = @statusName")
	}
	if f.TableNumber != 0 {
		args = append(args, sql.Named("tableNumber", f.TableNumber))
		sb.WriteString("\n\t\t  AND t.table_number = @tableNumber")
	}
	if f.Search != "" {
		args = append(args, sql.Named("search", f.Search))
		sb.WriteString("\n\t\t  AND RIGHT(o.order_number, CHARINDEX('-', REVERSE(o.order_number)) - 1) LIKE '%' + @search + '%'")
	}

	sb.WriteString(`
		)
		SELECT
			fo.order_id, fo.order_number,
			fo.table_number,
			fo.waiter_full_name,
			fo.order_total_amount,
			fo.order_created_at,
			fo.order_status_name,
			oi.order_item_id,
			d.dish_name,
			d.dish_price,
			oi.order_item_quantity
		FROM FilteredOrders fo
		JOIN order_items oi ON oi.order_id = fo.order_id
		                   AND oi.order_item_quantity > oi.cancelled_quantity
		JOIN dishes d       ON d.dish_id   = oi.dish_id
		ORDER BY fo.order_created_at DESC, oi.order_item_id ASC`)

	rows, err := r.db.Query(sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("GetArchiveOrdersList query: %w", err)
	}
	defer rows.Close()

	var result []OrderListRow
	for rows.Next() {
		var row OrderListRow
		if err := rows.Scan(
			&row.OrderID, &row.OrderNumber,
			&row.TableNumber,
			&row.WaiterName,
			&row.TotalAmount,
			&row.CreatedAt,
			&row.StatusName,
			&row.ItemID,
			&row.DishName,
			&row.DishPrice,
			&row.ItemQty,
		); err != nil {
			return nil, fmt.Errorf("GetArchiveOrdersList scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *OrdersRepo) CancelOrder(orderID, restaurantID int) error {
	ordersRepoLog.Printf("CancelOrder: orderID=%d restaurantID=%d", orderID, restaurantID)

	res, err := r.db.Exec(`
		UPDATE orders
		SET order_status_id = (
			SELECT order_status_id FROM order_statuses
			WHERE order_status_name = N'Скасовано'
		)
		WHERE order_id = @orderID
		  AND table_id IN (SELECT table_id FROM tables WHERE restaurant_id = @restaurantID)`,
		sql.Named("orderID", orderID),
		sql.Named("restaurantID", restaurantID),
	)
	if err != nil {
		return fmt.Errorf("CancelOrder exec: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("CancelOrder: order %d not found or wrong restaurant", orderID)
	}
	return nil
}

// RejectPayment inserts a rejected payment record. The DB trigger is conditioned on
// 'Оплачено' only, so the order remains open after this insert.
func (r *OrdersRepo) RejectPayment(orderID, restaurantID int, paymentMethod string) error {
	ordersRepoLog.Printf("RejectPayment: orderID=%d restaurantID=%d method=%s", orderID, restaurantID, paymentMethod)

	payNum := generatePaymentNumber()

	res, err := r.db.Exec(`
		INSERT INTO payments (payment_amount, payment_number, payment_method_id, payment_status_id, order_id)
		SELECT
			o.order_total_amount,
			@payNum,
			(SELECT payment_method_id FROM payment_methods  WHERE payment_method_name = @payMethod),
			(SELECT payment_status_id FROM payment_statuses WHERE payment_status_name = N'Відхилено'),
			@orderID
		FROM orders o
		JOIN tables t ON t.table_id = o.table_id
		WHERE o.order_id = @orderID
		  AND t.restaurant_id = @restaurantID`,
		sql.Named("payNum", payNum),
		sql.Named("orderID", orderID),
		sql.Named("restaurantID", restaurantID),
		sql.Named("payMethod", paymentMethod),
	)
	if err != nil {
		return fmt.Errorf("RejectPayment exec: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("RejectPayment: order %d not found or wrong restaurant", orderID)
	}
	return nil
}

// PayOrder inserts a cash payment record. DB triggers automatically close the order.
// The amount is taken directly from orders.order_total_amount — never from the caller.
func (r *OrdersRepo) PayOrder(orderID, restaurantID int, paymentMethod string) error {
	ordersRepoLog.Printf("PayOrder: orderID=%d restaurantID=%d method=%s", orderID, restaurantID, paymentMethod)

	payNum := generatePaymentNumber()

	res, err := r.db.Exec(`
		INSERT INTO payments (payment_amount, payment_number, payment_method_id, payment_status_id, order_id)
		SELECT
			o.order_total_amount,
			@payNum,
			(SELECT payment_method_id FROM payment_methods  WHERE payment_method_name = @payMethod),
			(SELECT payment_status_id FROM payment_statuses WHERE payment_status_name = N'Оплачено'),
			@orderID
		FROM orders o
		JOIN tables t ON t.table_id = o.table_id
		WHERE o.order_id = @orderID
		  AND t.restaurant_id = @restaurantID`,
		sql.Named("payNum", payNum),
		sql.Named("orderID", orderID),
		sql.Named("restaurantID", restaurantID),
		sql.Named("payMethod", paymentMethod),
	)
	if err != nil {
		return fmt.Errorf("PayOrder exec: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("PayOrder: order %d not found or wrong restaurant", orderID)
	}
	return nil
}
