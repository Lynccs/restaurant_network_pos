package adminservice

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	adminrepo "restaurant_network_pos/internal/repository/admin"
)

type NetworkRestaurant struct {
	ID      int
	Name    string
	Address string
	Phone   string
}

type NetworkTable struct {
	ID           int
	Number       int
	Capacity     int
	RestaurantID int
}

type NetworkStaff struct {
	ID           int
	Name         string
	Phone        string
	Role         string
	RestaurantID int
}

type ChefSpecialization struct {
	ID   int
	Name string
}

type NetworkPageView struct {
	Restaurants         []NetworkRestaurant
	Tables              []NetworkTable
	Staff               []NetworkStaff
	CurrentRestaurantID int
}

type NetworkServicer interface {
	GetNetworkPage(currentRestaurantID int) (*NetworkPageView, error)
	GetRestaurant(id int) (*NetworkRestaurant, error)
	UpdateRestaurant(id int, name, address, phone string) error
	CreateRestaurant(name, address, phone string) (int, error)
	CreateTable(restaurantID, number, capacity int) error
	CreateStaff(role, name, phone, pin string, restaurantID, specializationID int) error
	ListRestaurants() ([]NetworkRestaurant, error)
	ListChefSpecializations() ([]ChefSpecialization, error)
	GetStaff(role string, id int) (*NetworkStaff, error)
	UpdateStaff(role, name, phone, pin string, restaurantID, id int) error
	GetTable(id int) (*NetworkTable, error)
	UpdateTable(id, restaurantID, number, capacity int) error
	DeleteStaff(role string, id, restaurantID int) error
	DeleteTable(id, restaurantID int) error
}

type NetworkService struct {
	repo *adminrepo.NetworkRepo
}

func NewNetworkService(repo *adminrepo.NetworkRepo) *NetworkService {
	return &NetworkService{repo: repo}
}

func (s *NetworkService) GetRestaurant(id int) (*NetworkRestaurant, error) {
	if id <= 0 {
		return nil, errors.New("invalid id")
	}
	row, err := s.repo.GetRestaurantByID(id)
	if err != nil {
		return nil, err
	}
	return &NetworkRestaurant{ID: row.ID, Name: row.Name, Address: row.Address, Phone: row.Phone}, nil
}

func (s *NetworkService) UpdateRestaurant(id int, name, address, phone string) error {
	name = strings.TrimSpace(name)
	address = strings.TrimSpace(address)
	phone = strings.TrimSpace(phone)
	if id <= 0 || name == "" || address == "" || phone == "" {
		return errors.New("id, name, address, and phone are required")
	}

	updated, err := s.repo.UpdateRestaurant(id, name, address, phone)
	if err != nil {
		return err
	}
	if !updated {
		return errors.New("restaurant not found")
	}
	return nil
}

func (s *NetworkService) GetNetworkPage(currentRestaurantID int) (*NetworkPageView, error) {
	restaurants, err := s.repo.ListRestaurants()
	if err != nil {
		return nil, fmt.Errorf("GetNetworkPage restaurants: %w", err)
	}
	tables, err := s.repo.ListTables()
	if err != nil {
		return nil, fmt.Errorf("GetNetworkPage tables: %w", err)
	}
	waiters, err := s.repo.ListWaiters()
	if err != nil {
		return nil, fmt.Errorf("GetNetworkPage waiters: %w", err)
	}
	chefs, err := s.repo.ListChefs()
	if err != nil {
		return nil, fmt.Errorf("GetNetworkPage chefs: %w", err)
	}
	admins, err := s.repo.ListAdministrators()
	if err != nil {
		return nil, fmt.Errorf("GetNetworkPage admins: %w", err)
	}

	view := &NetworkPageView{
		Restaurants:         make([]NetworkRestaurant, 0, len(restaurants)),
		Tables:              make([]NetworkTable, 0, len(tables)),
		Staff:               make([]NetworkStaff, 0, len(waiters)+len(chefs)+len(admins)),
		CurrentRestaurantID: currentRestaurantID,
	}

	for _, r := range restaurants {
		view.Restaurants = append(view.Restaurants, NetworkRestaurant{
			ID:      r.ID,
			Name:    r.Name,
			Address: r.Address,
			Phone:   r.Phone,
		})
	}
	for _, t := range tables {
		view.Tables = append(view.Tables, NetworkTable{
			ID:           t.ID,
			Number:       t.Number,
			Capacity:     t.Capacity,
			RestaurantID: t.RestaurantID,
		})
	}
	for _, w := range waiters {
		view.Staff = append(view.Staff, NetworkStaff{
			ID:           w.ID,
			Name:         w.Name,
			Phone:        w.Phone,
			Role:         "waiter",
			RestaurantID: w.RestaurantID,
		})
	}
	for _, c := range chefs {
		view.Staff = append(view.Staff, NetworkStaff{
			ID:           c.ID,
			Name:         c.Name,
			Phone:        c.Phone,
			Role:         "chef",
			RestaurantID: c.RestaurantID,
		})
	}
	for _, a := range admins {
		view.Staff = append(view.Staff, NetworkStaff{
			ID:           a.ID,
			Name:         a.Name,
			Phone:        a.Phone,
			Role:         "admin",
			RestaurantID: a.RestaurantID,
		})
	}

	return view, nil
}

func (s *NetworkService) ListRestaurants() ([]NetworkRestaurant, error) {
	rows, err := s.repo.ListRestaurants()
	if err != nil {
		return nil, fmt.Errorf("ListRestaurants: %w", err)
	}

	result := make([]NetworkRestaurant, 0, len(rows))
	for _, r := range rows {
		result = append(result, NetworkRestaurant{
			ID:      r.ID,
			Name:    r.Name,
			Address: r.Address,
			Phone:   r.Phone,
		})
	}

	return result, nil
}

func (s *NetworkService) ListChefSpecializations() ([]ChefSpecialization, error) {
	rows, err := s.repo.ListChefSpecializations()
	if err != nil {
		return nil, fmt.Errorf("ListChefSpecializations: %w", err)
	}
	result := make([]ChefSpecialization, 0, len(rows))
	for _, r := range rows {
		result = append(result, ChefSpecialization{ID: r.ID, Name: r.Name})
	}
	return result, nil
}

func (s *NetworkService) CreateRestaurant(name, address, phone string) (int, error) {
	name = strings.TrimSpace(name)
	address = strings.TrimSpace(address)
	phone = strings.TrimSpace(phone)
	if name == "" || address == "" || phone == "" {
		return 0, errors.New("name, address, and phone are required")
	}
	return s.repo.CreateRestaurant(name, address, phone)
}

func (s *NetworkService) CreateTable(restaurantID, number, capacity int) error {
	if restaurantID <= 0 || number <= 0 || capacity <= 0 {
		return errors.New("restaurant, number, and capacity are required")
	}

	exists, err := s.repo.TableExists(restaurantID, number)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("table number already exists")
	}

	return s.repo.CreateTable(restaurantID, number, capacity)
}

func (s *NetworkService) CreateStaff(role, name, phone, pin string, restaurantID, specializationID int) error {
	role = strings.TrimSpace(strings.ToLower(role))
	name = strings.TrimSpace(name)
	phone = strings.TrimSpace(phone)
	if role == "" || name == "" || phone == "" || pin == "" || restaurantID <= 0 {
		return errors.New("role, name, phone, pin, and restaurant are required")
	}

	pinHashBytes, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash pin: %w", err)
	}
	pinHash := string(pinHashBytes)

	switch role {
	case "waiter":
		return s.repo.CreateWaiter(name, phone, pinHash, restaurantID)
	case "chef":
		if specializationID <= 0 {
			return errors.New("chef specialization is required")
		}
		return s.repo.CreateChef(name, phone, pinHash, restaurantID, specializationID)
	case "admin":
		return s.repo.CreateAdministrator(name, phone, pinHash, restaurantID)
	default:
		return errors.New("unknown role")
	}
}

func (s *NetworkService) GetStaff(role string, id int) (*NetworkStaff, error) {
	role = strings.TrimSpace(strings.ToLower(role))
	if id <= 0 {
		return nil, errors.New("invalid id")
	}

	switch role {
	case "waiter":
		row, err := s.repo.GetWaiterByID(id)
		if err != nil {
			return nil, err
		}
		return &NetworkStaff{ID: row.ID, Name: row.Name, Phone: row.Phone, Role: role, RestaurantID: row.RestaurantID}, nil
	case "chef":
		row, err := s.repo.GetChefByID(id)
		if err != nil {
			return nil, err
		}
		return &NetworkStaff{ID: row.ID, Name: row.Name, Phone: row.Phone, Role: role, RestaurantID: row.RestaurantID}, nil
	case "admin":
		row, err := s.repo.GetAdministratorByID(id)
		if err != nil {
			return nil, err
		}
		return &NetworkStaff{ID: row.ID, Name: row.Name, Phone: row.Phone, Role: role, RestaurantID: row.RestaurantID}, nil
	default:
		return nil, errors.New("unknown role")
	}
}

func (s *NetworkService) UpdateStaff(role, name, phone, pin string, restaurantID, id int) error {
	role = strings.TrimSpace(strings.ToLower(role))
	name = strings.TrimSpace(name)
	phone = strings.TrimSpace(phone)
	if role == "" || name == "" || phone == "" || restaurantID <= 0 || id <= 0 {
		return errors.New("role, name, phone, restaurant, and id are required")
	}

	var pinHash *string
	if strings.TrimSpace(pin) != "" {
		pinHashBytes, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash pin: %w", err)
		}
		hash := string(pinHashBytes)
		pinHash = &hash
	}

	var updated bool
	var err error
	switch role {
	case "waiter":
		updated, err = s.repo.UpdateWaiter(id, restaurantID, name, phone, pinHash)
	case "chef":
		updated, err = s.repo.UpdateChef(id, restaurantID, name, phone, pinHash)
	case "admin":
		updated, err = s.repo.UpdateAdministrator(id, restaurantID, name, phone, pinHash)
	default:
		return errors.New("unknown role")
	}
	if err != nil {
		return err
	}
	if !updated {
		return errors.New("staff not found")
	}
	return nil
}

func (s *NetworkService) GetTable(id int) (*NetworkTable, error) {
	if id <= 0 {
		return nil, errors.New("invalid id")
	}
	row, err := s.repo.GetTableByID(id)
	if err != nil {
		return nil, err
	}
	return &NetworkTable{ID: row.ID, Number: row.Number, Capacity: row.Capacity, RestaurantID: row.RestaurantID}, nil
}

func (s *NetworkService) DeleteStaff(role string, id, restaurantID int) error {
	role = strings.TrimSpace(strings.ToLower(role))
	if id <= 0 || restaurantID <= 0 {
		return errors.New("invalid id or restaurantID")
	}
	staff, err := s.GetStaff(role, id)
	if err != nil {
		return errors.New("staff not found")
	}
	if staff.RestaurantID != restaurantID {
		return errors.New("forbidden")
	}
	switch role {
	case "waiter":
		return s.repo.SoftDeleteWaiter(id)
	case "chef":
		return s.repo.SoftDeleteChef(id)
	case "admin":
		return s.repo.SoftDeleteAdministrator(id)
	default:
		return errors.New("unknown role")
	}
}

func (s *NetworkService) DeleteTable(id, restaurantID int) error {
	if id <= 0 || restaurantID <= 0 {
		return errors.New("invalid id or restaurantID")
	}
	table, err := s.repo.GetTableByID(id)
	if err != nil {
		return errors.New("table not found")
	}
	if table.RestaurantID != restaurantID {
		return errors.New("forbidden")
	}
	return s.repo.SoftDeleteTable(id)
}

func (s *NetworkService) UpdateTable(id, restaurantID, number, capacity int) error {
	if restaurantID <= 0 || id <= 0 || number <= 0 || capacity <= 0 {
		return errors.New("restaurant, id, number, and capacity are required")
	}

	exists, err := s.repo.TableNumberExistsOther(restaurantID, number, id)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("table number already exists")
	}

	updated, err := s.repo.UpdateTable(id, restaurantID, number, capacity)
	if err != nil {
		return err
	}
	if !updated {
		return errors.New("table not found")
	}
	return nil
}
