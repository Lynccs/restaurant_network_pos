package adminservice

import (
	"fmt"
	adminrepo "restaurant_network_pos/internal/repository/admin"
)

type Supplier = adminrepo.Supplier

type SupplierIngredientsView struct {
	SupplierID   int
	SupplierName string
	Ingredients  []IngredientOption
	SelectedIDs  map[int]bool
}

type SupplierView struct {
	Supplier
	Ingredients []string
}

type SuppliersServicer interface {
	GetSuppliers() ([]SupplierView, error)
	GetSupplier(id int) (Supplier, error)
	CreateSupplier(s Supplier) (int, error)
	UpdateSupplier(s Supplier) error
	DeleteSupplier(id int) error
	GetSupplierFormView(supplierID int) (*SupplierFormView, error)
	GetSupplierIngredientsView(supplierID int) (*SupplierIngredientsView, error)
	SetSupplierIngredients(supplierID int, ingredientIDs []int) error
}

type SupplierFormView struct {
	Supplier    Supplier
	Ingredients []IngredientOption
	SelectedIDs map[int]bool
}

type SuppliersService struct {
	repo          *adminrepo.SuppliersRepo
	purchasesRepo *adminrepo.PurchasesRepo
}

func NewSuppliersService(repo *adminrepo.SuppliersRepo, purchasesRepo *adminrepo.PurchasesRepo) *SuppliersService {
	return &SuppliersService{
		repo:          repo,
		purchasesRepo: purchasesRepo,
	}
}

func (s *SuppliersService) GetSuppliers() ([]SupplierView, error) {
	sups, err := s.repo.ListSuppliers()
	if err != nil {
		return nil, err
	}

	allIngs, err := s.purchasesRepo.GetIngredients()
	if err != nil {
		return nil, err
	}
	ingMap := make(map[int]string)
	for _, ing := range allIngs {
		ingMap[ing.ID] = ing.Name
	}

	result := make([]SupplierView, len(sups))
	for i, sup := range sups {
		ids, err := s.repo.GetSupplierIngredients(sup.ID)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(ids))
		for _, id := range ids {
			if name, ok := ingMap[id]; ok {
				names = append(names, name)
			}
		}
		result[i] = SupplierView{
			Supplier:    sup,
			Ingredients: names,
		}
	}
	return result, nil
}

func (s *SuppliersService) GetSupplier(id int) (Supplier, error) {
	return s.repo.GetSupplier(id)
}

func (s *SuppliersService) CreateSupplier(sup Supplier) (int, error) {
	return s.repo.CreateSupplier(sup)
}

func (s *SuppliersService) UpdateSupplier(sup Supplier) error {
	return s.repo.UpdateSupplier(sup)
}

func (s *SuppliersService) DeleteSupplier(id int) error {
	if err := s.repo.DeleteSupplier(id); err != nil {
		return err
	}
	if err := s.repo.DeleteSupplierIngredients(id); err != nil {
		return fmt.Errorf("delete supplier ingredients: %w", err)
	}
	return nil
}

func (s *SuppliersService) GetSupplierFormView(supplierID int) (*SupplierFormView, error) {
	var sup Supplier
	selectedMap := make(map[int]bool)

	if supplierID > 0 {
		var err error
		sup, err = s.repo.GetSupplier(supplierID)
		if err != nil {
			return nil, err
		}

		selected, err := s.repo.GetSupplierIngredients(supplierID)
		if err != nil {
			return nil, err
		}
		for _, id := range selected {
			selectedMap[id] = true
		}
	}

	allIngs, err := s.purchasesRepo.GetIngredients()
	if err != nil {
		return nil, err
	}

	ingOptions := make([]IngredientOption, len(allIngs))
	for i, ing := range allIngs {
		ingOptions[i] = IngredientOption{
			ID:       ing.ID,
			Name:     ing.Name,
			UnitName: ing.UnitName,
		}
	}

	return &SupplierFormView{
		Supplier:    sup,
		Ingredients: ingOptions,
		SelectedIDs: selectedMap,
	}, nil
}

func (s *SuppliersService) GetSupplierIngredientsView(supplierID int) (*SupplierIngredientsView, error) {
	sup, err := s.repo.GetSupplier(supplierID)
	if err != nil {
		return nil, err
	}

	selected, err := s.repo.GetSupplierIngredients(supplierID)
	if err != nil {
		return nil, err
	}

	selectedMap := make(map[int]bool)
	for _, id := range selected {
		selectedMap[id] = true
	}

	allIngs, err := s.purchasesRepo.GetIngredients()
	if err != nil {
		return nil, err
	}

	ingOptions := make([]IngredientOption, len(allIngs))
	for i, ing := range allIngs {
		ingOptions[i] = IngredientOption{
			ID:       ing.ID,
			Name:     ing.Name,
			UnitName: ing.UnitName,
		}
	}

	return &SupplierIngredientsView{
		SupplierID:   supplierID,
		SupplierName: sup.CompanyName,
		Ingredients:  ingOptions,
		SelectedIDs:  selectedMap,
	}, nil
}

func (s *SuppliersService) SetSupplierIngredients(supplierID int, ingredientIDs []int) error {
	return s.repo.SetSupplierIngredients(supplierID, ingredientIDs)
}
