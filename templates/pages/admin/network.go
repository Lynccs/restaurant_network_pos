package adminpages

import (
	"context"
	"html"
	"io"
	"sort"
	"strconv"
	"strings"

	adminservice "restaurant_network_pos/internal/service/admin"

	"github.com/a-h/templ"
)

type networkPageComponent struct {
	view *adminservice.NetworkPageView
}

func (c networkPageComponent) Render(_ context.Context, w io.Writer) error {
	var b strings.Builder
	b.WriteString("<div class=\"p-6 max-w-6xl mx-auto fade-in\">")
	b.WriteString("<div class=\"flex items-start justify-between mb-5\">")
	b.WriteString("<div><h1 class=\"text-xl font-semibold text-slate-900\">Управління мережею</h1>")
	b.WriteString("<p class=\"text-slate-500 text-sm mt-0.5\">Деревоподібний перегляд закладів, персоналу та плану залу</p></div>")
	b.WriteString("<button type=\"button\" hx-get=\"/admin/network/restaurants/new\" hx-target=\"#admin-modal-content\" hx-swap=\"innerHTML\" class=\"btn-primary bg-blue-600 text-white text-sm px-4 py-2 rounded-lg font-medium flex items-center gap-2 hover:bg-blue-700\">")
	b.WriteString("<svg class=\"w-4 h-4\" fill=\"none\" stroke=\"currentColor\" viewBox=\"0 0 24 24\"><path stroke-linecap=\"round\" stroke-linejoin=\"round\" stroke-width=\"2\" d=\"M12 4v16m8-8H4\"/></svg>")
	b.WriteString("Додати заклад</button>")
	b.WriteString("</div>")

	if len(c.view.Restaurants) == 0 {
		b.WriteString("<div class=\"text-center py-16 text-slate-400 text-sm\">Закладів не знайдено</div>")
		b.WriteString("</div>")
		_, err := io.WriteString(w, b.String())
		return err
	}

	restaurants := make([]adminservice.NetworkRestaurant, len(c.view.Restaurants))
	copy(restaurants, c.view.Restaurants)
	sort.Slice(restaurants, func(i, j int) bool {
		return strings.ToLower(restaurants[i].Name) < strings.ToLower(restaurants[j].Name)
	})

	staffByRestaurant := make(map[int][]adminservice.NetworkStaff)
	for _, s := range c.view.Staff {
		staffByRestaurant[s.RestaurantID] = append(staffByRestaurant[s.RestaurantID], s)
	}

	tablesByRestaurant := make(map[int][]adminservice.NetworkTable)
	for _, t := range c.view.Tables {
		tablesByRestaurant[t.RestaurantID] = append(tablesByRestaurant[t.RestaurantID], t)
	}

	b.WriteString("<div class=\"space-y-4\">")
	for _, r := range restaurants {
		isCurrent := r.ID == c.view.CurrentRestaurantID
		b.WriteString("<details class=\"bg-white border border-slate-200 rounded-2xl overflow-hidden group/res\" ")
		if isCurrent {
			b.WriteString("open")
		}
		b.WriteString(">")
		b.WriteString("<summary class=\"list-none cursor-pointer px-5 py-4 hover:bg-slate-50 transition-colors\">")
		b.WriteString("<div class=\"flex items-center justify-between\"><div>")
		b.WriteString("<p class=\"font-semibold text-slate-800\">")
		b.WriteString(html.EscapeString(r.Name))
		if isCurrent {
			b.WriteString(" <span class=\"ml-2 text-xs bg-blue-100 text-blue-600 px-2 py-0.5 rounded-full font-medium\">Ваш заклад</span>")
		}
		b.WriteString("</p>")
		b.WriteString("<p class=\"text-xs text-slate-500 mt-1\">")
		b.WriteString(html.EscapeString(r.Address))
		b.WriteString(" · <span class=\"mono\">")
		b.WriteString(html.EscapeString(r.Phone))
		b.WriteString("</span></p>")
		b.WriteString("</div>")
		b.WriteString("<svg class=\"w-4 h-4 text-slate-400 transition-transform group-open/res:rotate-180\" fill=\"none\" stroke=\"currentColor\" viewBox=\"0 0 24 24\"><path stroke-linecap=\"round\" stroke-linejoin=\"round\" stroke-width=\"2\" d=\"M19 9l-7 7-7-7\"/></svg>")
		b.WriteString("</div></summary>")
		b.WriteString("<div class=\"px-5 pb-5 space-y-3 border-t border-slate-100 bg-slate-50/40\">")

		// Staff section
		b.WriteString("<details class=\"bg-white border border-slate-200 rounded-xl overflow-hidden mt-4 group/staff\" open>")
		b.WriteString("<summary class=\"list-none cursor-pointer px-4 py-3 hover:bg-slate-50 transition-colors\">")
		b.WriteString("<div class=\"flex items-center justify-between\">")
		b.WriteString("<p class=\"text-sm font-semibold text-slate-800\">Персонал</p>")
		b.WriteString("<div class=\"flex items-center gap-2\">")
		if isCurrent {
			b.WriteString("<button type=\"button\" hx-get=\"/admin/network/staff/new?restaurant_id=")
			b.WriteString(strconv.Itoa(r.ID))
			b.WriteString("\" hx-target=\"#admin-modal-content\" hx-swap=\"innerHTML\" class=\"bg-blue-600 text-white text-xs px-3 py-1.5 rounded-lg font-medium hover:bg-blue-700\">Додати</button>")
		}
		b.WriteString("<svg class=\"w-4 h-4 text-slate-400 transition-transform group-open/staff:rotate-180\" fill=\"none\" stroke=\"currentColor\" viewBox=\"0 0 24 24\"><path stroke-linecap=\"round\" stroke-linejoin=\"round\" stroke-width=\"2\" d=\"M19 9l-7 7-7-7\"/></svg>")
		b.WriteString("</div></div></summary>")
		b.WriteString("<div class=\"px-4 pb-4 space-y-5 border-t border-slate-100 bg-slate-50/20\">")

		staff := staffByRestaurant[r.ID]
		roles := []struct {
			Key   string
			Label string
		}{
			{Key: "waiter", Label: "Офіціанти"},
			{Key: "chef", Label: "Кухарі"},
			{Key: "admin", Label: "Адміністратори"},
		}

		for _, role := range roles {
			people := make([]adminservice.NetworkStaff, 0)
			for _, s := range staff {
				if s.Role == role.Key {
					people = append(people, s)
				}
			}
			sort.Slice(people, func(i, j int) bool {
				return strings.ToLower(people[i].Name) < strings.ToLower(people[j].Name)
			})

			b.WriteString("<details class=\"bg-slate-50 border border-slate-200 rounded-lg mt-3 overflow-hidden group/role\">")
			b.WriteString("<summary class=\"list-none cursor-pointer px-3 py-2.5 hover:bg-slate-100 transition-colors flex items-center justify-between\">")
			b.WriteString("<span class=\"text-sm text-slate-700 font-medium flex items-center gap-2\">")
			b.WriteString(role.Label)
			b.WriteString("<span class=\"normal-case font-medium bg-slate-200 text-slate-600 px-1.5 py-0.5 rounded text-[10px]\">")
			b.WriteString(strconv.Itoa(len(people)))
			b.WriteString("</span></span>")
			b.WriteString("<svg class=\"w-4 h-4 text-slate-400 transition-transform group-open/role:rotate-180\" fill=\"none\" stroke=\"currentColor\" viewBox=\"0 0 24 24\"><path stroke-linecap=\"round\" stroke-linejoin=\"round\" stroke-width=\"2\" d=\"M19 9l-7 7-7-7\"/></svg>")
			b.WriteString("</summary>")
			b.WriteString("<div class=\"px-3 pb-3 border-t border-slate-200\">")

			if len(people) == 0 {
				b.WriteString("<p class=\"text-xs text-slate-400 py-4 italic text-center\">Немає працівників</p>")
			} else {
				b.WriteString("<div class=\"grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 mt-3\">")
				for _, p := range people {
					b.WriteString("<div class=\"bg-white border border-slate-200 rounded-xl p-3 flex items-center justify-between shadow-sm\">")
					b.WriteString("<div class=\"flex items-center gap-3\">")
					b.WriteString("<div class=\"w-9 h-9 bg-blue-100 text-blue-600 rounded-full flex items-center justify-center font-bold text-xs shrink-0\">")
					b.WriteString(getInitials(p.Name))
					b.WriteString("</div>")
					b.WriteString("<div class=\"min-w-0\">")
					b.WriteString("<p class=\"text-sm font-medium text-slate-800 truncate\">")
					b.WriteString(html.EscapeString(p.Name))
					b.WriteString("</p>")
					b.WriteString("<p class=\"text-xs text-slate-500 mono\">")
					b.WriteString(html.EscapeString(p.Phone))
					b.WriteString("</p></div></div>")
					if isCurrent {
						b.WriteString("<button type=\"button\" hx-get=\"/admin/network/staff/")
						b.WriteString(p.Role)
						b.WriteString("/")
						b.WriteString(strconv.Itoa(p.ID))
						b.WriteString("/edit\" hx-target=\"#admin-modal-content\" hx-swap=\"innerHTML\" class=\"p-1.5 text-slate-400 hover:text-blue-600 hover:bg-blue-50 rounded-lg transition-colors\">")
						b.WriteString("<svg class=\"w-4 h-4\" fill=\"none\" stroke=\"currentColor\" viewBox=\"0 0 24 24\"><path stroke-linecap=\"round\" stroke-linejoin=\"round\" stroke-width=\"2\" d=\"M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z\"/></svg>")
						b.WriteString("</button>")
					}
					b.WriteString("</div>")
				}
				b.WriteString("</div>")
			}
			b.WriteString("</div></details>")
		}
		b.WriteString("</div></details>")

		// Tables section
		b.WriteString("<details class=\"bg-white border border-slate-200 rounded-xl overflow-hidden group/tables\" open>")
		b.WriteString("<summary class=\"list-none cursor-pointer px-4 py-3 hover:bg-slate-50 transition-colors\">")
		b.WriteString("<div class=\"flex items-center justify-between\">")
		b.WriteString("<p class=\"text-sm font-semibold text-slate-800\">План залу</p>")
		b.WriteString("<div class=\"flex items-center gap-2\">")
		if isCurrent {
			b.WriteString("<button type=\"button\" hx-get=\"/admin/network/tables/new?restaurant_id=")
			b.WriteString(strconv.Itoa(r.ID))
			b.WriteString("\" hx-target=\"#admin-modal-content\" hx-swap=\"innerHTML\" class=\"bg-blue-600 text-white text-xs px-3 py-1.5 rounded-lg font-medium hover:bg-blue-700\">Додати столик</button>")
		}
		b.WriteString("<svg class=\"w-4 h-4 text-slate-400 transition-transform group-open/tables:rotate-180\" fill=\"none\" stroke=\"currentColor\" viewBox=\"0 0 24 24\"><path stroke-linecap=\"round\" stroke-linejoin=\"round\" stroke-width=\"2\" d=\"M19 9l-7 7-7-7\"/></svg>")
		b.WriteString("</div></div></summary>")
		b.WriteString("<div class=\"px-4 pb-4 border-t border-slate-100\">")

		tables := tablesByRestaurant[r.ID]
		sort.Slice(tables, func(i, j int) bool { return tables[i].Number < tables[j].Number })
		if len(tables) == 0 {
			b.WriteString("<p class=\"text-xs text-slate-400 py-3\">Столиків не знайдено</p>")
		} else {
			b.WriteString("<div class=\"divide-y divide-slate-100\">")
			for _, t := range tables {
				b.WriteString("<div class=\"py-3 text-sm text-slate-700 flex items-center justify-between\">")
				b.WriteString("<div class=\"flex items-center gap-3\">")
				b.WriteString("<span class=\"w-8 h-8 bg-slate-100 text-slate-600 rounded-lg flex items-center justify-center font-bold text-xs\">")
				b.WriteString(strconv.Itoa(t.Number))
				b.WriteString("</span>")
				b.WriteString("<span>Столик №")
				b.WriteString(strconv.Itoa(t.Number))
				b.WriteString("</span></div>")
				b.WriteString("<div class=\"flex items-center gap-4\">")
				b.WriteString("<span class=\"text-xs text-slate-500 bg-slate-50 px-2 py-1 rounded\">")
				b.WriteString(strconv.Itoa(t.Capacity))
				b.WriteString(" місць</span>")
				if isCurrent {
					b.WriteString("<button type=\"button\" hx-get=\"/admin/network/tables/")
					b.WriteString(strconv.Itoa(t.ID))
					b.WriteString("/edit\" hx-target=\"#admin-modal-content\" hx-swap=\"innerHTML\" class=\"p-1.5 text-slate-400 hover:text-blue-600 hover:bg-blue-50 rounded-lg transition-colors\">")
					b.WriteString("<svg class=\"w-4 h-4\" fill=\"none\" stroke=\"currentColor\" viewBox=\"0 0 24 24\"><path stroke-linecap=\"round\" stroke-linejoin=\"round\" stroke-width=\"2\" d=\"M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z\"/></svg>")
					b.WriteString("</button>")
				}
				b.WriteString("</div></div>")
			}
			b.WriteString("</div>")
		}
		b.WriteString("</div></details>")

		b.WriteString("</div></details>")
	}
	b.WriteString("</div></div>")

	_, err := io.WriteString(w, b.String())
	return err
}

func getInitials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "?"
	}
	if len(parts) == 1 {
		return strings.ToUpper(string([]rune(parts[0])[0]))
	}
	return strings.ToUpper(string([]rune(parts[0])[0]) + string([]rune(parts[1])[0]))
}

func NetworkPage(view *adminservice.NetworkPageView) templ.Component {
	return networkPageComponent{view: view}
}

