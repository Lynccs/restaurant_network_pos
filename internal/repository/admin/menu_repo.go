package adminrepo

import (
	"database/sql"
	"fmt"
)

type MenuDishRow struct {
	ID           int
	Name         string
	Price        float64
	PortionSize  int
	CookingTime  int
	CategoryID   int
	CategoryName string
}

type MenuRecipeRow struct {
	DishID         int
	IngredientID   int
	IngredientName string
	Qty            float64
	UnitName       string
}

type MenuCategoryRow struct {
	ID   int
	Name string
}

type MenuIngredientRow struct {
	ID       int
	Name     string
	UnitID   int
	UnitName string
}

type MenuIngredientCostRow struct {
	IngredientID int
	Price        float64
}

type MenuRepo struct {
	db *sql.DB
}

func NewMenuRepo(db *sql.DB) *MenuRepo {
	return &MenuRepo{db: db}
}

func (r *MenuRepo) ListMenuDishes() ([]MenuDishRow, error) {
	const query = `
SELECT
	d.dish_id,
	d.dish_name,
	d.dish_price,
	d.dish_portion_size,
	d.dish_cooking_time,
	dc.dish_category_id,
	dc.dish_category_name
FROM dishes d
JOIN dish_categories dc ON dc.dish_category_id = d.dish_category_id
ORDER BY dc.dish_category_name, d.dish_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("ListMenuDishes: %w", err)
	}
	defer rows.Close()

	var result []MenuDishRow
	for rows.Next() {
		var row MenuDishRow
		if err := rows.Scan(
			&row.ID,
			&row.Name,
			&row.Price,
			&row.PortionSize,
			&row.CookingTime,
			&row.CategoryID,
			&row.CategoryName,
		); err != nil {
			return nil, fmt.Errorf("ListMenuDishes scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *MenuRepo) ListMenuRecipes() ([]MenuRecipeRow, error) {
	const query = `
SELECT
	di.dish_id,
	i.ingredient_id,
	i.ingredient_name,
	di.dish_ingredient_quantity,
	iu.ingredient_unit_name
FROM dish_ingredients di
JOIN ingredients i ON i.ingredient_id = di.ingredient_id
JOIN ingredient_units iu ON iu.ingredient_unit_id = i.ingredient_unit_id
ORDER BY di.dish_id, i.ingredient_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("ListMenuRecipes: %w", err)
	}
	defer rows.Close()

	var result []MenuRecipeRow
	for rows.Next() {
		var row MenuRecipeRow
		if err := rows.Scan(
			&row.DishID,
			&row.IngredientID,
			&row.IngredientName,
			&row.Qty,
			&row.UnitName,
		); err != nil {
			return nil, fmt.Errorf("ListMenuRecipes scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *MenuRepo) GetMenuDish(id int) (*MenuDishRow, error) {
	const query = `
SELECT
	d.dish_id,
	d.dish_name,
	d.dish_price,
	d.dish_portion_size,
	d.dish_cooking_time,
	dc.dish_category_id,
	dc.dish_category_name
FROM dishes d
JOIN dish_categories dc ON dc.dish_category_id = d.dish_category_id
WHERE d.dish_id = @id`

	var row MenuDishRow
	if err := r.db.QueryRow(query, sql.Named("id", id)).Scan(
		&row.ID,
		&row.Name,
		&row.Price,
		&row.PortionSize,
		&row.CookingTime,
		&row.CategoryID,
		&row.CategoryName,
	); err != nil {
		return nil, fmt.Errorf("GetMenuDish: %w", err)
	}
	return &row, nil
}

func (r *MenuRepo) GetMenuRecipe(dishID int) ([]MenuRecipeRow, error) {
	const query = `
SELECT
	di.dish_id,
	i.ingredient_id,
	i.ingredient_name,
	di.dish_ingredient_quantity,
	iu.ingredient_unit_name
FROM dish_ingredients di
JOIN ingredients i ON i.ingredient_id = di.ingredient_id
JOIN ingredient_units iu ON iu.ingredient_unit_id = i.ingredient_unit_id
WHERE di.dish_id = @dishID
ORDER BY i.ingredient_name`

	rows, err := r.db.Query(query, sql.Named("dishID", dishID))
	if err != nil {
		return nil, fmt.Errorf("GetMenuRecipe: %w", err)
	}
	defer rows.Close()

	var result []MenuRecipeRow
	for rows.Next() {
		var row MenuRecipeRow
		if err := rows.Scan(
			&row.DishID,
			&row.IngredientID,
			&row.IngredientName,
			&row.Qty,
			&row.UnitName,
		); err != nil {
			return nil, fmt.Errorf("GetMenuRecipe scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *MenuRepo) ListMenuCategories() ([]MenuCategoryRow, error) {
	const query = `
SELECT dish_category_id, dish_category_name
FROM dish_categories
ORDER BY dish_category_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("ListMenuCategories: %w", err)
	}
	defer rows.Close()

	var result []MenuCategoryRow
	for rows.Next() {
		var row MenuCategoryRow
		if err := rows.Scan(&row.ID, &row.Name); err != nil {
			return nil, fmt.Errorf("ListMenuCategories scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *MenuRepo) ListMenuIngredients() ([]MenuIngredientRow, error) {
	const query = `
SELECT i.ingredient_id, i.ingredient_name, i.ingredient_unit_id, iu.ingredient_unit_name
FROM ingredients i
JOIN ingredient_units iu ON iu.ingredient_unit_id = i.ingredient_unit_id
ORDER BY i.ingredient_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("ListMenuIngredients: %w", err)
	}
	defer rows.Close()

	var result []MenuIngredientRow
	for rows.Next() {
		var row MenuIngredientRow
		if err := rows.Scan(&row.ID, &row.Name, &row.UnitID, &row.UnitName); err != nil {
			return nil, fmt.Errorf("ListMenuIngredients scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *MenuRepo) CreateMenuCategory(name string) (int, error) {
	const lookup = `SELECT dish_category_id FROM dish_categories WHERE dish_category_name = @name`
	var existing int
	if err := r.db.QueryRow(lookup, sql.Named("name", name)).Scan(&existing); err == nil {
		return existing, nil
	} else if err != sql.ErrNoRows {
		return 0, fmt.Errorf("CreateMenuCategory lookup: %w", err)
	}

	const query = `
INSERT INTO dish_categories (dish_category_name)
VALUES (@name);
SELECT SCOPE_IDENTITY();`

	var id int
	if err := r.db.QueryRow(query, sql.Named("name", name)).Scan(&id); err != nil {
		return 0, fmt.Errorf("CreateMenuCategory: %w", err)
	}
	return id, nil
}

func (r *MenuRepo) CreateMenuDish(row MenuDishRow, recipe []MenuRecipeRow) (int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("CreateMenuDish begin: %w", err)
	}
	defer tx.Rollback()

	const insertDish = `
INSERT INTO dishes (dish_name, dish_price, dish_portion_size, dish_cooking_time, dish_category_id)
VALUES (@name, @price, @portion, @time, @category);
SELECT SCOPE_IDENTITY();`

	var dishID int
	if err := tx.QueryRow(insertDish,
		sql.Named("name", row.Name),
		sql.Named("price", row.Price),
		sql.Named("portion", row.PortionSize),
		sql.Named("time", row.CookingTime),
		sql.Named("category", row.CategoryID),
	).Scan(&dishID); err != nil {
		return 0, fmt.Errorf("CreateMenuDish insert: %w", err)
	}

	if err := insertMenuRecipe(tx, dishID, recipe); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("CreateMenuDish commit: %w", err)
	}
	return dishID, nil
}

func (r *MenuRepo) UpdateMenuDish(row MenuDishRow, recipe []MenuRecipeRow) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("UpdateMenuDish begin: %w", err)
	}
	defer tx.Rollback()

	const updateDish = `
UPDATE dishes
SET dish_name = @name,
	dish_price = @price,
	dish_portion_size = @portion,
	dish_cooking_time = @time,
	dish_category_id = @category
WHERE dish_id = @id`

	if _, err := tx.Exec(updateDish,
		sql.Named("name", row.Name),
		sql.Named("price", row.Price),
		sql.Named("portion", row.PortionSize),
		sql.Named("time", row.CookingTime),
		sql.Named("category", row.CategoryID),
		sql.Named("id", row.ID),
	); err != nil {
		return fmt.Errorf("UpdateMenuDish update: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM dish_ingredients WHERE dish_id = @dishID`, sql.Named("dishID", row.ID)); err != nil {
		return fmt.Errorf("UpdateMenuDish clear recipe: %w", err)
	}

	if err := insertMenuRecipe(tx, row.ID, recipe); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("UpdateMenuDish commit: %w", err)
	}
	return nil
}

func insertMenuRecipe(tx *sql.Tx, dishID int, recipe []MenuRecipeRow) error {
	const insertRecipe = `
INSERT INTO dish_ingredients (dish_ingredient_quantity, dish_id, ingredient_id)
VALUES (@qty, @dishID, @ingredientID)`

	for _, item := range recipe {
		if _, err := tx.Exec(insertRecipe,
			sql.Named("qty", item.Qty),
			sql.Named("dishID", dishID),
			sql.Named("ingredientID", item.IngredientID),
		); err != nil {
			return fmt.Errorf("insertMenuRecipe: %w", err)
		}
	}
	return nil
}

func (r *MenuRepo) UpdateMenuDishPrice(dishID int, price float64) error {
	const query = `UPDATE dishes SET dish_price = @price WHERE dish_id = @id`
	if _, err := r.db.Exec(query, sql.Named("price", price), sql.Named("id", dishID)); err != nil {
		return fmt.Errorf("UpdateMenuDishPrice: %w", err)
	}
	return nil
}

func (r *MenuRepo) GetLatestIngredientPrices(restaurantID int) ([]MenuIngredientCostRow, error) {
	const query = `
WITH latest AS (
	SELECT
		iod.ingredient_id,
		iod.detail_purchase_price,
		ROW_NUMBER() OVER (
			PARTITION BY iod.ingredient_id
			ORDER BY pb.product_batch_arrival_date DESC, pb.product_batch_id DESC
		) AS rn
	FROM product_batches pb
	JOIN ingredient_order_details iod ON iod.ingredient_order_detail_id = pb.ingredient_order_detail_id
	JOIN ingredient_orders io ON io.ingredient_order_id = iod.ingredient_order_id
	JOIN ingredient_order_statuses ios ON ios.ingredient_order_status_id = io.ingredient_order_status_id
	JOIN administrators a ON a.administrator_id = io.administrator_id
	WHERE ios.ingredient_order_status_name <> 'Створено'
		AND a.restaurant_id = @restaurantID
)
SELECT ingredient_id, detail_purchase_price
FROM latest
WHERE rn = 1`

	rows, err := r.db.Query(query, sql.Named("restaurantID", restaurantID))
	if err != nil {
		return nil, fmt.Errorf("GetLatestIngredientPrices: %w", err)
	}
	defer rows.Close()

	var result []MenuIngredientCostRow
	for rows.Next() {
		var row MenuIngredientCostRow
		if err := rows.Scan(&row.IngredientID, &row.Price); err != nil {
			return nil, fmt.Errorf("GetLatestIngredientPrices scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
