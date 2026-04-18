package waiterservice

type DishView struct {
	ID          int
	Name        string
	Price       float64
	PortionSize int
	CookingTime int
	Category    string
	Portions    int
	Stopped     bool
	CartQty     int
}

type CartItemView struct {
	DishID   int
	Name     string
	Price    float64
	Qty      int
	Subtotal float64
	CanPlus  bool
}
