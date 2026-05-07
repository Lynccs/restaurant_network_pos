package waiterservice

type DishView struct {
	ID               int
	Name             string
	Price            float64
	OriginalPrice    float64
	HasYieldDiscount bool
	PortionSize      int
	CookingTime      int
	Category         string
	Portions         int
	Stopped          bool
	CartQty          int
}

type CartItemView struct {
	DishID       int
	OrderItemID  int     // 0 = in-memory only
	Name         string
	Price        float64
	Qty          int     // effective qty (qty - cancelDelta for cooking items)
	CancelledQty int     // > 0 only for "cooking" items being cancelled this session
	Subtotal     float64
	CanPlus      bool
	Status       string  // "new" | "cooking" | "done" | "" (in-memory, treated as new)
	IsFromDB     bool
	HasIssue     bool
}
