package adminservice

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	adminrepo "restaurant_network_pos/internal/repository/admin"
)

type MenuRecipeItem struct {
	IngredientID   int
	IngredientName string
	Qty            float64
	UnitName       string
}

type MenuDish struct {
	ID               int
	Name             string
	Price            float64
	PortionSize      int
	CookingTime      int
	CategoryID       int
	CategoryName     string
	Recipe           []MenuRecipeItem
	Cost             float64
	CostKnown        bool
	MarginPercent    float64
	Profitable       bool
	RecommendedPrice float64
}

type MenuCategory struct {
	ID   int
	Name string
}

type MenuIngredient struct {
	ID       int
	Name     string
	UnitName string
}

type MenuPageView struct {
	Categories       []MenuCategory
	SelectedCategory string
	TargetMargin     float64
	Dishes           []MenuDish
}

type MenuFormView struct {
	Dish        *MenuDish
	Categories  []MenuCategory
	Ingredients []MenuIngredient
}

type MenuDishInput struct {
	Name        string
	CategoryID  int
	PortionSize int
	CookingTime int
	Price       float64
	Recipe      []MenuRecipeItem
}

type MenuServicer interface {
	GetMenuPage(restaurantID int, targetMargin float64, category string) (*MenuPageView, error)
	GetMenuForm(dishID int) (*MenuFormView, error)
	CreateDish(input MenuDishInput) (int, error)
	UpdateDish(dishID int, input MenuDishInput) error
	UpdateDishPrice(dishID int, price float64) error
}

type MenuService struct {
	repo *adminrepo.MenuRepo
}

func NewMenuService(repo *adminrepo.MenuRepo) *MenuService {
	return &MenuService{repo: repo}
}

func (s *MenuService) GetMenuPage(restaurantID int, targetMargin float64, category string) (*MenuPageView, error) {
	dishes, err := s.repo.ListMenuDishes()
	if err != nil {
		return nil, fmt.Errorf("GetMenuPage dishes: %w", err)
	}
	recipes, err := s.repo.ListMenuRecipes()
	if err != nil {
		return nil, fmt.Errorf("GetMenuPage recipes: %w", err)
	}
	priceRows, err := s.repo.GetLatestIngredientPrices(restaurantID)
	if err != nil {
		return nil, fmt.Errorf("GetMenuPage prices: %w", err)
	}

	prices := make(map[int]float64, len(priceRows))
	for _, row := range priceRows {
		prices[row.IngredientID] = row.Price
	}

	recipesByDish := make(map[int][]MenuRecipeItem)
	for _, r := range recipes {
		recipesByDish[r.DishID] = append(recipesByDish[r.DishID], MenuRecipeItem{
			IngredientID:   r.IngredientID,
			IngredientName: r.IngredientName,
			Qty:            r.Qty,
			UnitName:       r.UnitName,
		})
	}

	categoriesMap := map[string]MenuCategory{}
	allDishes := make([]MenuDish, 0, len(dishes))
	for _, d := range dishes {
		recipe := recipesByDish[d.ID]
		if len(recipe) == 0 {
			continue
		}

		cost, costKnown := calcRecipeCost(recipe, prices)
		margin, profitable, recommended := calcMargin(d.Price, cost, costKnown, targetMargin)

		allDishes = append(allDishes, MenuDish{
			ID:               d.ID,
			Name:             d.Name,
			Price:            d.Price,
			PortionSize:      d.PortionSize,
			CookingTime:      d.CookingTime,
			CategoryID:       d.CategoryID,
			CategoryName:     d.CategoryName,
			Recipe:           recipe,
			Cost:             cost,
			CostKnown:        costKnown,
			MarginPercent:    margin,
			Profitable:       profitable,
			RecommendedPrice: recommended,
		})

		if _, ok := categoriesMap[d.CategoryName]; !ok {
			categoriesMap[d.CategoryName] = MenuCategory{ID: d.CategoryID, Name: d.CategoryName}
		}
	}

	categories := make([]MenuCategory, 0, len(categoriesMap))
	for _, c := range categoriesMap {
		categories = append(categories, c)
	}
	sort.Slice(categories, func(i, j int) bool {
		return strings.ToLower(categories[i].Name) < strings.ToLower(categories[j].Name)
	})

	selected := strings.TrimSpace(category)
	filtered := make([]MenuDish, 0, len(allDishes))
	for _, dish := range allDishes {
		if selected == "" || selected == "Усі" || dish.CategoryName == selected {
			filtered = append(filtered, dish)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		ai := profitabilityRank(filtered[i])
		aj := profitabilityRank(filtered[j])
		if ai != aj {
			return ai < aj
		}
		return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
	})

	return &MenuPageView{
		Categories:       categories,
		SelectedCategory: selected,
		TargetMargin:     targetMargin,
		Dishes:           filtered,
	}, nil
}

func (s *MenuService) GetMenuForm(dishID int) (*MenuFormView, error) {
	categories, err := s.repo.ListMenuCategories()
	if err != nil {
		return nil, fmt.Errorf("GetMenuForm categories: %w", err)
	}
	ingredients, err := s.repo.ListMenuIngredients()
	if err != nil {
		return nil, fmt.Errorf("GetMenuForm ingredients: %w", err)
	}

	var dish *MenuDish
	if dishID > 0 {
		row, err := s.repo.GetMenuDish(dishID)
		if err != nil {
			return nil, fmt.Errorf("GetMenuForm dish: %w", err)
		}
		recipeRows, err := s.repo.GetMenuRecipe(dishID)
		if err != nil {
			return nil, fmt.Errorf("GetMenuForm recipe: %w", err)
		}
		recipe := make([]MenuRecipeItem, 0, len(recipeRows))
		for _, r := range recipeRows {
			recipe = append(recipe, MenuRecipeItem{
				IngredientID:   r.IngredientID,
				IngredientName: r.IngredientName,
				Qty:            r.Qty,
				UnitName:       r.UnitName,
			})
		}
		dish = &MenuDish{
			ID:           row.ID,
			Name:         row.Name,
			Price:        row.Price,
			PortionSize:  row.PortionSize,
			CookingTime:  row.CookingTime,
			CategoryID:   row.CategoryID,
			CategoryName: row.CategoryName,
			Recipe:       recipe,
		}
	}

	viewCategories := make([]MenuCategory, 0, len(categories))
	for _, c := range categories {
		viewCategories = append(viewCategories, MenuCategory{ID: c.ID, Name: c.Name})
	}
	viewIngredients := make([]MenuIngredient, 0, len(ingredients))
	for _, i := range ingredients {
		viewIngredients = append(viewIngredients, MenuIngredient{ID: i.ID, Name: i.Name, UnitName: i.UnitName})
	}

	return &MenuFormView{
		Dish:        dish,
		Categories:  viewCategories,
		Ingredients: viewIngredients,
	}, nil
}

func (s *MenuService) CreateDish(input MenuDishInput) (int, error) {
	row, recipe, err := s.normalizeDishInput(0, input)
	if err != nil {
		return 0, err
	}
	return s.repo.CreateMenuDish(row, recipe)
}

func (s *MenuService) UpdateDish(dishID int, input MenuDishInput) error {
	row, recipe, err := s.normalizeDishInput(dishID, input)
	if err != nil {
		return err
	}
	return s.repo.UpdateMenuDish(row, recipe)
}

func (s *MenuService) UpdateDishPrice(dishID int, price float64) error {
	if dishID <= 0 || price <= 0 {
		return errors.New("invalid dish price")
	}
	return s.repo.UpdateMenuDishPrice(dishID, price)
}

func (s *MenuService) normalizeDishInput(dishID int, input MenuDishInput) (adminrepo.MenuDishRow, []adminrepo.MenuRecipeRow, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || input.PortionSize <= 0 || input.CookingTime <= 0 || input.Price <= 0 {
		return adminrepo.MenuDishRow{}, nil, errors.New("name, portion size, cooking time, and price are required")
	}

	if input.CategoryID <= 0 {
		return adminrepo.MenuDishRow{}, nil, errors.New("category is required")
	}

	if len(input.Recipe) == 0 {
		return adminrepo.MenuDishRow{}, nil, errors.New("recipe is required")
	}

	row := adminrepo.MenuDishRow{
		ID:           dishID,
		Name:         name,
		Price:        input.Price,
		PortionSize:  input.PortionSize,
		CookingTime:  input.CookingTime,
		CategoryID:   input.CategoryID,
		CategoryName: "",
	}

	recipe := make([]adminrepo.MenuRecipeRow, 0, len(input.Recipe))
	for _, item := range input.Recipe {
		if item.IngredientID <= 0 || item.Qty <= 0 {
			return adminrepo.MenuDishRow{}, nil, errors.New("invalid recipe rows")
		}
		recipe = append(recipe, adminrepo.MenuRecipeRow{
			IngredientID: item.IngredientID,
			Qty:          item.Qty,
		})
	}

	return row, recipe, nil
}

func calcRecipeCost(recipe []MenuRecipeItem, prices map[int]float64) (float64, bool) {
	if len(recipe) == 0 {
		return 0, false
	}
	cost := 0.0
	for _, item := range recipe {
		price, ok := prices[item.IngredientID]
		if !ok {
			return cost, false
		}
		cost += item.Qty * price
	}
	return cost, true
}

func calcMargin(price, cost float64, costKnown bool, target float64) (float64, bool, float64) {
	if !costKnown || price <= 0 {
		return 0, false, 0
	}
	margin := ((price - cost) / price) * 100
	profitable := margin >= target
	recommended := 0.0
	if !profitable && target > 0 && target < 100 {
		recommended = math.Ceil((cost/(1-(target/100)))*100) / 100
	}
	return margin, profitable, recommended
}

func profitabilityRank(d MenuDish) int {
	if !d.CostKnown {
		return 2
	}
	if !d.Profitable {
		return 0
	}
	return 1
}
