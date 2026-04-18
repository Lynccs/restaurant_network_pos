package repository

import (
	"database/sql"
	"errors"
	"log"
	"restaurant_network_pos/internal/models"
)

var repoLog = log.New(log.Writer(), "[UserRepo] ", log.LstdFlags|log.Lshortfile)

type UserRow struct {
	User    models.User
	PinHash string
}

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) GetUserByPhone(phone string) (*UserRow, error) {
	const query = `
		SELECT id, full_name, restaurant_id, role, pin_hash FROM (
			SELECT administrator_id AS id, administrator_full_name AS full_name,
			       restaurant_id, 'admin' AS role, pin_hash
			FROM administrators WHERE administrator_phone = @phone
			UNION ALL
			SELECT waiter_id, waiter_full_name,
			       restaurant_id, 'waiter' AS role, pin_hash
			FROM waiters WHERE waiter_phone = @phone
			UNION ALL
			SELECT chef_id, chef_full_name,
			       restaurant_id, 'chef' AS role, pin_hash
			FROM chefs WHERE chef_phone = @phone
		) AS u`

	var (
		row  UserRow
		role string
	)

	repoLog.Printf("GetUserByPhone: querying for phone=%s", phone)

	err := r.db.QueryRow(query, sql.Named("phone", phone)).
		Scan(&row.User.ID, &row.User.FullName, &row.User.RestaurantID, &role, &row.PinHash)
	if errors.Is(err, sql.ErrNoRows) {
		repoLog.Printf("GetUserByPhone: no user found for phone=%s", phone)
		return nil, nil
	}
	if err != nil {
		repoLog.Printf("GetUserByPhone: db error for phone=%s: %v", phone, err)
		return nil, err
	}

	repoLog.Printf("GetUserByPhone: found user id=%d role=%s", row.User.ID, role)
	row.User.Phone = phone
	row.User.Role = models.Role(role)
	return &row, nil
}
