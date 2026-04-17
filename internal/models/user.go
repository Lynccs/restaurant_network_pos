package models

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleWaiter Role = "waiter"
	RoleChef   Role = "chef"
)

type User struct {
	ID           int
	FullName     string
	Phone        string
	Role         Role
	RestaurantID int
}
