package adminrepo

import (
	"database/sql"
	"fmt"
)

type NetworkRestaurantRow struct {
	ID      int
	Name    string
	Address string
	Phone   string
}

type NetworkTableRow struct {
	ID           int
	Number       int
	Capacity     int
	RestaurantID int
}

type NetworkStaffRow struct {
	ID           int
	Name         string
	Phone        string
	RestaurantID int
}

type ChefSpecializationRow struct {
	ID   int
	Name string
}

type NetworkRepo struct {
	db *sql.DB
}

func NewNetworkRepo(db *sql.DB) *NetworkRepo {
	return &NetworkRepo{db: db}
}

func (r *NetworkRepo) CreateRestaurant(name, address, phone string) (int, error) {
	const query = `
INSERT INTO restaurants (restaurant_name, restaurant_address, restaurant_phone)
VALUES (@name, @address, @phone);
SELECT SCOPE_IDENTITY();`

	var id int
	err := r.db.QueryRow(query,
		sql.Named("name", name),
		sql.Named("address", address),
		sql.Named("phone", phone),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("CreateRestaurant: %w", err)
	}
	return id, nil
}

func (r *NetworkRepo) GetRestaurantByID(id int) (*NetworkRestaurantRow, error) {
	const query = `
SELECT restaurant_id, restaurant_name, restaurant_address, restaurant_phone
FROM restaurants
WHERE restaurant_id = @id`

	var row NetworkRestaurantRow
	err := r.db.QueryRow(query, sql.Named("id", id)).Scan(&row.ID, &row.Name, &row.Address, &row.Phone)
	if err != nil {
		return nil, fmt.Errorf("GetRestaurantByID: %w", err)
	}
	return &row, nil
}

func (r *NetworkRepo) UpdateRestaurant(id int, name, address, phone string) (bool, error) {
	const query = `
UPDATE restaurants
SET restaurant_name = @name, restaurant_address = @address, restaurant_phone = @phone
WHERE restaurant_id = @id`

	res, err := r.db.Exec(query,
		sql.Named("name", name),
		sql.Named("address", address),
		sql.Named("phone", phone),
		sql.Named("id", id),
	)
	if err != nil {
		return false, fmt.Errorf("UpdateRestaurant: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("UpdateRestaurant rows: %w", err)
	}
	return rows > 0, nil
}

func (r *NetworkRepo) CreateTable(restaurantID, number, capacity int) error {
	const query = `
INSERT INTO tables (table_number, table_capacity, restaurant_id)
VALUES (@number, @capacity, @restaurantID)`

	_, err := r.db.Exec(query,
		sql.Named("number", number),
		sql.Named("capacity", capacity),
		sql.Named("restaurantID", restaurantID),
	)
	if err != nil {
		return fmt.Errorf("CreateTable: %w", err)
	}
	return nil
}

func (r *NetworkRepo) TableExists(restaurantID, number int) (bool, error) {
	const query = `
SELECT 1
FROM tables
WHERE restaurant_id = @restaurantID AND table_number = @number`

	var exists int
	err := r.db.QueryRow(query,
		sql.Named("restaurantID", restaurantID),
		sql.Named("number", number),
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("TableExists: %w", err)
	}
	return true, nil
}

func (r *NetworkRepo) CreateWaiter(name, phone, pinHash string, restaurantID int) error {
	const query = `
INSERT INTO waiters (waiter_full_name, waiter_phone, restaurant_id, pin_hash)
VALUES (@name, @phone, @restaurantID, @pinHash)`

	_, err := r.db.Exec(query,
		sql.Named("name", name),
		sql.Named("phone", phone),
		sql.Named("restaurantID", restaurantID),
		sql.Named("pinHash", pinHash),
	)
	if err != nil {
		return fmt.Errorf("CreateWaiter: %w", err)
	}
	return nil
}

func (r *NetworkRepo) CreateChef(name, phone, pinHash string, restaurantID, specializationID int) error {
	const query = `
INSERT INTO chefs (chef_full_name, chef_phone, chef_specialization_id, restaurant_id, pin_hash)
VALUES (@name, @phone, @specID, @restaurantID, @pinHash)`

	_, err := r.db.Exec(query,
		sql.Named("name", name),
		sql.Named("phone", phone),
		sql.Named("specID", specializationID),
		sql.Named("restaurantID", restaurantID),
		sql.Named("pinHash", pinHash),
	)
	if err != nil {
		return fmt.Errorf("CreateChef: %w", err)
	}
	return nil
}

func (r *NetworkRepo) CreateAdministrator(name, phone, pinHash string, restaurantID int) error {
	const query = `
INSERT INTO administrators (administrator_full_name, administrator_phone, restaurant_id, pin_hash)
VALUES (@name, @phone, @restaurantID, @pinHash)`

	_, err := r.db.Exec(query,
		sql.Named("name", name),
		sql.Named("phone", phone),
		sql.Named("restaurantID", restaurantID),
		sql.Named("pinHash", pinHash),
	)
	if err != nil {
		return fmt.Errorf("CreateAdministrator: %w", err)
	}
	return nil
}

func (r *NetworkRepo) GetDefaultChefSpecializationID() (int, error) {
	const query = `
SELECT TOP 1 chef_specialization_id
FROM chef_specializations
ORDER BY chef_specialization_id`

	var id int
	if err := r.db.QueryRow(query).Scan(&id); err != nil {
		return 0, fmt.Errorf("GetDefaultChefSpecializationID: %w", err)
	}
	return id, nil
}

func (r *NetworkRepo) ListChefSpecializations() ([]ChefSpecializationRow, error) {
	const query = `
SELECT chef_specialization_id, chef_specialization_name
FROM chef_specializations
ORDER BY chef_specialization_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("ListChefSpecializations query: %w", err)
	}
	defer rows.Close()

	var result []ChefSpecializationRow
	for rows.Next() {
		var row ChefSpecializationRow
		if err := rows.Scan(&row.ID, &row.Name); err != nil {
			return nil, fmt.Errorf("ListChefSpecializations scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *NetworkRepo) ListRestaurants() ([]NetworkRestaurantRow, error) {
	const query = `
SELECT restaurant_id, restaurant_name, restaurant_address, restaurant_phone
FROM restaurants
ORDER BY restaurant_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("ListRestaurants query: %w", err)
	}
	defer rows.Close()

	var result []NetworkRestaurantRow
	for rows.Next() {
		var row NetworkRestaurantRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Address, &row.Phone); err != nil {
			return nil, fmt.Errorf("ListRestaurants scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *NetworkRepo) SoftDeleteWaiter(id int) error {
	const query = `UPDATE waiters SET is_deleted = 1 WHERE waiter_id = @id`
	_, err := r.db.Exec(query, sql.Named("id", id))
	if err != nil {
		return fmt.Errorf("SoftDeleteWaiter: %w", err)
	}
	return nil
}

func (r *NetworkRepo) SoftDeleteChef(id int) error {
	const query = `UPDATE chefs SET is_deleted = 1 WHERE chef_id = @id`
	_, err := r.db.Exec(query, sql.Named("id", id))
	if err != nil {
		return fmt.Errorf("SoftDeleteChef: %w", err)
	}
	return nil
}

func (r *NetworkRepo) SoftDeleteAdministrator(id int) error {
	const query = `UPDATE administrators SET is_deleted = 1 WHERE administrator_id = @id`
	_, err := r.db.Exec(query, sql.Named("id", id))
	if err != nil {
		return fmt.Errorf("SoftDeleteAdministrator: %w", err)
	}
	return nil
}

func (r *NetworkRepo) SoftDeleteTable(id int) error {
	const query = `UPDATE tables SET is_deleted = 1 WHERE table_id = @id`
	_, err := r.db.Exec(query, sql.Named("id", id))
	if err != nil {
		return fmt.Errorf("SoftDeleteTable: %w", err)
	}
	return nil
}

func (r *NetworkRepo) ListTables() ([]NetworkTableRow, error) {
	const query = `
SELECT table_id, table_number, table_capacity, restaurant_id
FROM tables
WHERE is_deleted = 0
ORDER BY table_number`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("ListTables query: %w", err)
	}
	defer rows.Close()

	var result []NetworkTableRow
	for rows.Next() {
		var row NetworkTableRow
		if err := rows.Scan(&row.ID, &row.Number, &row.Capacity, &row.RestaurantID); err != nil {
			return nil, fmt.Errorf("ListTables scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *NetworkRepo) ListWaiters() ([]NetworkStaffRow, error) {
	const query = `
SELECT waiter_id, waiter_full_name, waiter_phone, restaurant_id
FROM waiters
WHERE is_deleted = 0
ORDER BY waiter_full_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("ListWaiters query: %w", err)
	}
	defer rows.Close()

	var result []NetworkStaffRow
	for rows.Next() {
		var row NetworkStaffRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Phone, &row.RestaurantID); err != nil {
			return nil, fmt.Errorf("ListWaiters scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *NetworkRepo) ListChefs() ([]NetworkStaffRow, error) {
	const query = `
SELECT chef_id, chef_full_name, chef_phone, restaurant_id
FROM chefs
WHERE is_deleted = 0
ORDER BY chef_full_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("ListChefs query: %w", err)
	}
	defer rows.Close()

	var result []NetworkStaffRow
	for rows.Next() {
		var row NetworkStaffRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Phone, &row.RestaurantID); err != nil {
			return nil, fmt.Errorf("ListChefs scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *NetworkRepo) ListAdministrators() ([]NetworkStaffRow, error) {
	const query = `
SELECT administrator_id, administrator_full_name, administrator_phone, restaurant_id
FROM administrators
WHERE is_deleted = 0
ORDER BY administrator_full_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("ListAdministrators query: %w", err)
	}
	defer rows.Close()

	var result []NetworkStaffRow
	for rows.Next() {
		var row NetworkStaffRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Phone, &row.RestaurantID); err != nil {
			return nil, fmt.Errorf("ListAdministrators scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *NetworkRepo) GetWaiterByID(id int) (*NetworkStaffRow, error) {
	const query = `
SELECT waiter_id, waiter_full_name, waiter_phone, restaurant_id
FROM waiters
WHERE waiter_id = @id`

	var row NetworkStaffRow
	err := r.db.QueryRow(query, sql.Named("id", id)).Scan(&row.ID, &row.Name, &row.Phone, &row.RestaurantID)
	if err != nil {
		return nil, fmt.Errorf("GetWaiterByID: %w", err)
	}
	return &row, nil
}

func (r *NetworkRepo) GetChefByID(id int) (*NetworkStaffRow, error) {
	const query = `
SELECT chef_id, chef_full_name, chef_phone, restaurant_id
FROM chefs
WHERE chef_id = @id`

	var row NetworkStaffRow
	err := r.db.QueryRow(query, sql.Named("id", id)).Scan(&row.ID, &row.Name, &row.Phone, &row.RestaurantID)
	if err != nil {
		return nil, fmt.Errorf("GetChefByID: %w", err)
	}
	return &row, nil
}

func (r *NetworkRepo) GetAdministratorByID(id int) (*NetworkStaffRow, error) {
	const query = `
SELECT administrator_id, administrator_full_name, administrator_phone, restaurant_id
FROM administrators
WHERE administrator_id = @id`

	var row NetworkStaffRow
	err := r.db.QueryRow(query, sql.Named("id", id)).Scan(&row.ID, &row.Name, &row.Phone, &row.RestaurantID)
	if err != nil {
		return nil, fmt.Errorf("GetAdministratorByID: %w", err)
	}
	return &row, nil
}

func (r *NetworkRepo) UpdateWaiter(id, restaurantID int, name, phone string, pinHash *string) (bool, error) {
	query := `
UPDATE waiters
SET waiter_full_name = @name, waiter_phone = @phone`
	if pinHash != nil {
		query += ", pin_hash = @pinHash"
	}
	query += " WHERE waiter_id = @id AND restaurant_id = @restaurantID"

	args := []any{
		sql.Named("name", name),
		sql.Named("phone", phone),
		sql.Named("id", id),
		sql.Named("restaurantID", restaurantID),
	}
	if pinHash != nil {
		args = append(args, sql.Named("pinHash", *pinHash))
	}

	res, err := r.db.Exec(query, args...)
	if err != nil {
		return false, fmt.Errorf("UpdateWaiter: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("UpdateWaiter rows: %w", err)
	}
	return rows > 0, nil
}

func (r *NetworkRepo) UpdateChef(id, restaurantID int, name, phone string, pinHash *string) (bool, error) {
	query := `
UPDATE chefs
SET chef_full_name = @name, chef_phone = @phone`
	if pinHash != nil {
		query += ", pin_hash = @pinHash"
	}
	query += " WHERE chef_id = @id AND restaurant_id = @restaurantID"

	args := []any{
		sql.Named("name", name),
		sql.Named("phone", phone),
		sql.Named("id", id),
		sql.Named("restaurantID", restaurantID),
	}
	if pinHash != nil {
		args = append(args, sql.Named("pinHash", *pinHash))
	}

	res, err := r.db.Exec(query, args...)
	if err != nil {
		return false, fmt.Errorf("UpdateChef: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("UpdateChef rows: %w", err)
	}
	return rows > 0, nil
}

func (r *NetworkRepo) UpdateAdministrator(id, restaurantID int, name, phone string, pinHash *string) (bool, error) {
	query := `
UPDATE administrators
SET administrator_full_name = @name, administrator_phone = @phone`
	if pinHash != nil {
		query += ", pin_hash = @pinHash"
	}
	query += " WHERE administrator_id = @id AND restaurant_id = @restaurantID"

	args := []any{
		sql.Named("name", name),
		sql.Named("phone", phone),
		sql.Named("id", id),
		sql.Named("restaurantID", restaurantID),
	}
	if pinHash != nil {
		args = append(args, sql.Named("pinHash", *pinHash))
	}

	res, err := r.db.Exec(query, args...)
	if err != nil {
		return false, fmt.Errorf("UpdateAdministrator: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("UpdateAdministrator rows: %w", err)
	}
	return rows > 0, nil
}

func (r *NetworkRepo) GetTableByID(id int) (*NetworkTableRow, error) {
	const query = `
SELECT table_id, table_number, table_capacity, restaurant_id
FROM tables
WHERE table_id = @id`

	var row NetworkTableRow
	err := r.db.QueryRow(query, sql.Named("id", id)).Scan(&row.ID, &row.Number, &row.Capacity, &row.RestaurantID)
	if err != nil {
		return nil, fmt.Errorf("GetTableByID: %w", err)
	}
	return &row, nil
}

func (r *NetworkRepo) TableNumberExistsOther(restaurantID, number, tableID int) (bool, error) {
	const query = `
SELECT 1
FROM tables
WHERE restaurant_id = @restaurantID AND table_number = @number AND table_id <> @tableID`

	var exists int
	err := r.db.QueryRow(query,
		sql.Named("restaurantID", restaurantID),
		sql.Named("number", number),
		sql.Named("tableID", tableID),
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("TableNumberExistsOther: %w", err)
	}
	return true, nil
}

func (r *NetworkRepo) UpdateTable(id, restaurantID, number, capacity int) (bool, error) {
	const query = `
UPDATE tables
SET table_number = @number, table_capacity = @capacity
WHERE table_id = @id AND restaurant_id = @restaurantID`

	res, err := r.db.Exec(query,
		sql.Named("number", number),
		sql.Named("capacity", capacity),
		sql.Named("id", id),
		sql.Named("restaurantID", restaurantID),
	)
	if err != nil {
		return false, fmt.Errorf("UpdateTable: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("UpdateTable rows: %w", err)
	}
	return rows > 0, nil
}
