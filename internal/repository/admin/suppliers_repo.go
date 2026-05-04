package adminrepo

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type Supplier struct {
	ID             int
	CompanyName    string
	ContactPerson  string
	Phone          string
	Address        string
	PaymentDetails string
}

type SuppliersRepo struct {
	db       *sql.DB
	jsonPath string
}

func NewSuppliersRepo(db *sql.DB) *SuppliersRepo {
	return &SuppliersRepo{
		db:       db,
		jsonPath: "data/supplier_ingredients.json",
	}
}

func (r *SuppliersRepo) ListSuppliers() ([]Supplier, error) {
	const query = `
SELECT 
	supplier_id, 
	supplier_company_name, 
	supplier_contact_person, 
	supplier_phone, 
	supplier_address, 
	supplier_payment_details
FROM suppliers
ORDER BY supplier_company_name`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("ListSuppliers query: %w", err)
	}
	defer rows.Close()

	var result []Supplier
	for rows.Next() {
		var s Supplier
		if err := rows.Scan(
			&s.ID,
			&s.CompanyName,
			&s.ContactPerson,
			&s.Phone,
			&s.Address,
			&s.PaymentDetails,
		); err != nil {
			return nil, fmt.Errorf("ListSuppliers scan: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *SuppliersRepo) GetSupplier(id int) (Supplier, error) {
	const query = `
SELECT 
	supplier_id, 
	supplier_company_name, 
	supplier_contact_person, 
	supplier_phone, 
	supplier_address, 
	supplier_payment_details
FROM suppliers
WHERE supplier_id = @id`

	var s Supplier
	err := r.db.QueryRow(query, sql.Named("id", id)).Scan(
		&s.ID,
		&s.CompanyName,
		&s.ContactPerson,
		&s.Phone,
		&s.Address,
		&s.PaymentDetails,
	)
	if err != nil {
		return Supplier{}, fmt.Errorf("GetSupplier: %w", err)
	}
	return s, nil
}

func (r *SuppliersRepo) CreateSupplier(s Supplier) (int, error) {
	const query = `
INSERT INTO suppliers (
	supplier_company_name, 
	supplier_contact_person, 
	supplier_phone, 
	supplier_address, 
	supplier_payment_details
)
VALUES (@name, @contact, @phone, @address, @payment);
SELECT SCOPE_IDENTITY();`

	var id int
	err := r.db.QueryRow(query,
		sql.Named("name", s.CompanyName),
		sql.Named("contact", s.ContactPerson),
		sql.Named("phone", s.Phone),
		sql.Named("address", s.Address),
		sql.Named("payment", s.PaymentDetails),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("CreateSupplier: %w", err)
	}
	return id, nil
}

func (r *SuppliersRepo) UpdateSupplier(s Supplier) error {
	const query = `
UPDATE suppliers
SET 
	supplier_company_name = @name,
	supplier_contact_person = @contact,
	supplier_phone = @phone,
	supplier_address = @address,
	supplier_payment_details = @payment
WHERE supplier_id = @id`

	_, err := r.db.Exec(query,
		sql.Named("name", s.CompanyName),
		sql.Named("contact", s.ContactPerson),
		sql.Named("phone", s.Phone),
		sql.Named("address", s.Address),
		sql.Named("payment", s.PaymentDetails),
		sql.Named("id", s.ID),
	)
	if err != nil {
		return fmt.Errorf("UpdateSupplier: %w", err)
	}
	return nil
}

func (r *SuppliersRepo) DeleteSupplier(id int) error {
	const query = `DELETE FROM suppliers WHERE supplier_id = @id`
	_, err := r.db.Exec(query, sql.Named("id", id))
	if err != nil {
		return fmt.Errorf("DeleteSupplier: %w", err)
	}
	return nil
}

// Ingredients Mapping logic

func (r *SuppliersRepo) loadIngredientsMap() (map[string][]int, error) {
	data, err := os.ReadFile(r.jsonPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string][]int), nil
		}
		return nil, err
	}
	var m map[string][]int
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (r *SuppliersRepo) saveIngredientsMap(m map[string][]int) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.jsonPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(r.jsonPath, data, 0644)
}

func (r *SuppliersRepo) GetSupplierIngredients(supplierID int) ([]int, error) {
	m, err := r.loadIngredientsMap()
	if err != nil {
		return nil, err
	}
	return m[strconv.Itoa(supplierID)], nil
}

func (r *SuppliersRepo) SetSupplierIngredients(supplierID int, ingredientIDs []int) error {
	m, err := r.loadIngredientsMap()
	if err != nil {
		return err
	}
	seen := make(map[int]bool, len(ingredientIDs))
	unique := make([]int, 0, len(ingredientIDs))
	for _, id := range ingredientIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	m[strconv.Itoa(supplierID)] = unique
	return r.saveIngredientsMap(m)
}

func (r *SuppliersRepo) DeleteSupplierIngredients(supplierID int) error {
	m, err := r.loadIngredientsMap()
	if err != nil {
		return err
	}
	key := strconv.Itoa(supplierID)
	if _, ok := m[key]; !ok {
		return nil
	}
	delete(m, key)
	return r.saveIngredientsMap(m)
}
