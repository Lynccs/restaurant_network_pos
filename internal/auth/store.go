package auth

import (
	"database/sql"
	"errors"
	"restaurant_network_pos/internal/models"

	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid phone or pin")

func LoginByPhone(db *sql.DB, phone, pin string) (*models.User, error) {
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
		id           int
		fullName     string
		restaurantID int
		role         string
		pinHash      string
	)

	err := db.QueryRow(query, sql.Named("phone", phone)).
		Scan(&id, &fullName, &restaurantID, &role, &pinHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if err = bcrypt.CompareHashAndPassword([]byte(pinHash), []byte(pin)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return &models.User{
		ID:           id,
		FullName:     fullName,
		Phone:        phone,
		Role:         models.Role(role),
		RestaurantID: restaurantID,
	}, nil
}
