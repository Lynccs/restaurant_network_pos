package chefservice

import (
	"fmt"
	"time"

	chefrepo "restaurant_network_pos/internal/repository/chef"
)

// WriteOffFilters — параметри фільтрації (дублює chefrepo.WriteOffFilters для розв'язки шарів).
type WriteOffFilters = chefrepo.WriteOffFilters

// WriteOffIngredientRow — один інгредієнт у списанні страви.
type WriteOffIngredientRow struct {
	Name string
	Qty  float64
	Unit string
}

// WriteOffDish — одна страва з усіма списаними інгредієнтами.
type WriteOffDish struct {
	DishName    string
	Qty         int
	Ingredients []WriteOffIngredientRow
}

// WriteOffOrder — одне замовлення зі стравами та їх списаними інгредієнтами.
type WriteOffOrder struct {
	OrderNumber string
	TableNumber int
	WaiterName  string
	EventTime   time.Time
	Dishes      []WriteOffDish
}

// WriteOffPagination — дані про поточну сторінку для UI-компонента пагінації.
type WriteOffPagination struct {
	Page        int
	TotalOrders int
	TotalPages  int
}

// WriteOffPageView — повна view-модель для сторінки "Списані інгредієнти".
type WriteOffPageView struct {
	Orders            []WriteOffOrder
	DishOptions       []string // всі страви ресторану для <select>
	IngredientOptions []string // всі інгредієнти ресторану для <datalist>
	Filters           WriteOffFilters
	Pagination        WriteOffPagination
}

// buildOrders перетворює плоскі рядки репозиторію в ієрархію замовлень.
func buildOrders(rows []chefrepo.WriteOffRow) []WriteOffOrder {
	var orderNums []string
	orderMap := make(map[string]*WriteOffOrder)

	// dishIngredientKeys: orderNum -> dishIndex -> ingredientName -> ingredientIndex
	type dishKey struct {
		orderNum string
		dishIdx  int
	}
	ingredientIdx := make(map[dishKey]map[string]int)

	for _, row := range rows {
		order, exists := orderMap[row.OrderNumber]
		if !exists {
			order = &WriteOffOrder{
				OrderNumber: formatOrderNumber(row.OrderNumber),
				TableNumber: row.TableNumber,
				WaiterName:  row.WaiterName,
				EventTime:   row.UsageTime,
			}
			orderMap[row.OrderNumber] = order
			orderNums = append(orderNums, row.OrderNumber)
		}
		if row.UsageTime.After(order.EventTime) {
			order.EventTime = row.UsageTime
		}

		dishIdx := -1
		for i := range order.Dishes {
			if order.Dishes[i].DishName == row.DishName && order.Dishes[i].Qty == row.EffectiveQty {
				dishIdx = i
				break
			}
		}
		if dishIdx == -1 {
			order.Dishes = append(order.Dishes, WriteOffDish{DishName: row.DishName, Qty: row.EffectiveQty})
			dishIdx = len(order.Dishes) - 1
		}

		dk := dishKey{orderNum: row.OrderNumber, dishIdx: dishIdx}
		if ingredientIdx[dk] == nil {
			ingredientIdx[dk] = make(map[string]int)
		}
		if idx, found := ingredientIdx[dk][row.IngredientName]; found {
			order.Dishes[dishIdx].Ingredients[idx].Qty += row.IngredientQty
		} else {
			ingredientIdx[dk][row.IngredientName] = len(order.Dishes[dishIdx].Ingredients)
			order.Dishes[dishIdx].Ingredients = append(order.Dishes[dishIdx].Ingredients, WriteOffIngredientRow{
				Name: row.IngredientName,
				Qty:  row.IngredientQty,
				Unit: row.IngredientUnit,
			})
		}
	}

	result := make([]WriteOffOrder, 0, len(orderNums))
	for _, num := range orderNums {
		result = append(result, *orderMap[num])
	}
	return result
}

// buildPagination обчислює WriteOffPagination за totalOrders і page.
func buildPagination(totalOrders, page int) WriteOffPagination {
	totalPages := (totalOrders + chefrepo.WriteOffPageSize - 1) / chefrepo.WriteOffPageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	return WriteOffPagination{
		Page:        page,
		TotalOrders: totalOrders,
		TotalPages:  totalPages,
	}
}

// GetWriteOffOrders повертає сторінку замовлень без опцій фільтрів.
// Використовується HTMX-фрагментом — не запускає GetWriteOffOptions взагалі.
func (s *KitchenService) GetWriteOffOrders(restaurantID int, f WriteOffFilters, page int) ([]WriteOffOrder, WriteOffPagination, error) {
	rows, total, err := s.repo.GetWriteOffData(restaurantID, f, page)
	if err != nil {
		return nil, WriteOffPagination{}, fmt.Errorf("GetWriteOffOrders: %w", err)
	}
	return buildOrders(rows), buildPagination(total, page), nil
}

// GetWriteOffPage будує WriteOffPageView з плоских рядків репозиторію.
func (s *KitchenService) GetWriteOffPage(restaurantID int, f WriteOffFilters, page int) (*WriteOffPageView, error) {
	rows, total, err := s.repo.GetWriteOffData(restaurantID, f, page)
	if err != nil {
		return nil, fmt.Errorf("GetWriteOffPage: %w", err)
	}

	dishes, ingredients, err := s.cachedWriteOffOptions(restaurantID)
	if err != nil {
		return nil, fmt.Errorf("GetWriteOffPage options: %w", err)
	}

	return &WriteOffPageView{
		Orders:            buildOrders(rows),
		DishOptions:       dishes,
		IngredientOptions: ingredients,
		Filters:           f,
		Pagination:        buildPagination(total, page),
	}, nil
}

// cachedWriteOffOptions повертає опції з кешу або запитує БД якщо кеш протух.
func (s *KitchenService) cachedWriteOffOptions(restaurantID int) ([]string, []string, error) {
	s.optionsMu.Lock()
	defer s.optionsMu.Unlock()

	if c, ok := s.optionsCache[restaurantID]; ok && time.Since(c.fetchedAt) < writeOffOptionsTTL {
		return c.dishes, c.ingredients, nil
	}

	dishes, ingredients, err := s.repo.GetWriteOffOptions(restaurantID)
	if err != nil {
		return nil, nil, err
	}

	s.optionsCache[restaurantID] = &writeOffOptionsCache{
		dishes:      dishes,
		ingredients: ingredients,
		fetchedAt:   time.Now(),
	}
	return dishes, ingredients, nil
}
