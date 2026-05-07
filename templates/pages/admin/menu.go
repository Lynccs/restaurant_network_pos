package adminpages

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/url"
	"sort"
	"strings"

	adminservice "restaurant_network_pos/internal/service/admin"

	"github.com/a-h/templ"
)

type menuPageComponent struct {
	view *adminservice.MenuPageView
}

func (c menuPageComponent) Render(_ context.Context, w io.Writer) error {
	var b strings.Builder
	b.WriteString("<div class=\"p-6 max-w-6xl mx-auto fade-in\">")
	b.WriteString("<div class=\"flex items-start justify-between mb-6\">")
	b.WriteString("<div><h1 class=\"text-xl font-semibold text-slate-900\">Управління Меню</h1>")
	b.WriteString("<p class=\"text-slate-500 text-sm mt-0.5\">Технологічні картки, ціноутворення та аналіз рентабельності</p></div>")
	b.WriteString("</div>")

	categoryParam := c.view.SelectedCategory
	if categoryParam == "" {
		categoryParam = "Усі"
	}
	marginValue := fmt.Sprintf("%.1f", c.view.TargetMargin)

	if c.view.YieldCount > 0 && !c.view.ShowArchived {
		count := c.view.YieldCount
		noun := "страв"
		if count == 1 {
			noun = "страва"
		} else if count < 5 {
			noun = "страви"
		}
		b.WriteString("<div class=\"bg-amber-50 border border-amber-200 rounded-xl px-4 py-3 mb-4 flex items-center justify-between gap-4\">")
		b.WriteString("<div class=\"flex items-start gap-2\">")
		b.WriteString("<span class=\"text-amber-600 text-base leading-none mt-0.5\">⚠</span>")
		b.WriteString("<div>")
		b.WriteString(fmt.Sprintf("<p class=\"text-sm font-bold text-amber-800\">%d %s мають інгредієнти, термін яких спливає протягом 2 днів.</p>", count, noun))
		b.WriteString("<p class=\"text-xs text-amber-700 mt-0.5\">Розгляньте тимчасове зниження цін для збільшення попиту.</p>")
		b.WriteString("</div></div>")
		b.WriteString("<button type=\"button\" hx-get=\"/admin/menu/yield-alerts\" hx-target=\"#admin-modal-content\" hx-swap=\"innerHTML\" onclick=\"adminOpenModal()\" class=\"bg-amber-500 hover:bg-amber-600 text-white text-xs font-bold px-4 py-2 rounded-lg whitespace-nowrap\">Переглянути →</button>")
		b.WriteString("</div>")
	}

	b.WriteString("<div class=\"bg-white p-4 rounded-2xl border border-slate-200 mb-6 flex flex-wrap items-center justify-between gap-4 shadow-sm\">")
	b.WriteString("<form method=\"GET\" action=\"/admin/menu\" class=\"flex items-center gap-3\">")
	b.WriteString("<label class=\"text-sm font-semibold text-slate-700\">Мін. рентабельність страв (%):</label>")
	b.WriteString("<input type=\"number\" name=\"target_margin\" value=\"")
	b.WriteString(html.EscapeString(marginValue))
	b.WriteString("\" class=\"w-24 border border-slate-300 rounded-lg px-2 py-1.5 text-sm text-center mono\" />")
	b.WriteString("<input type=\"hidden\" name=\"category\" value=\"")
	b.WriteString(html.EscapeString(categoryParam))
	b.WriteString("\" />")
	b.WriteString("<button type=\"submit\" class=\"bg-slate-100 hover:bg-slate-200 text-slate-700 px-3 py-1.5 rounded-lg text-sm font-medium border border-slate-200\">Застосувати</button>")
	b.WriteString("</form>")
	b.WriteString("<button type=\"button\" hx-get=\"/admin/menu/new\" hx-target=\"#admin-modal-content\" hx-swap=\"innerHTML\" class=\"bg-blue-600 text-white text-sm px-5 py-2.5 rounded-xl font-bold shadow-sm hover:bg-blue-700\">Додати страву</button>")
	b.WriteString("</div>")

	b.WriteString("<div class=\"flex flex-wrap gap-2 mb-4\">")
	b.WriteString(statusPill("active", c.view.ShowArchived, c.view.ShowDiscounted, c.view.TargetMargin, categoryParam))
	b.WriteString(statusPill("discounted", c.view.ShowArchived, c.view.ShowDiscounted, c.view.TargetMargin, categoryParam))
	b.WriteString(statusPill("archive", c.view.ShowArchived, c.view.ShowDiscounted, c.view.TargetMargin, categoryParam))
	b.WriteString("</div>")

	if !c.view.ShowArchived && !c.view.ShowDiscounted {
		b.WriteString("<div class=\"flex flex-wrap gap-2 mb-6\">")
		b.WriteString(categoryPill("Усі", categoryParam, c.view.TargetMargin))
		cats := append([]adminservice.MenuCategory{}, c.view.Categories...)
		sort.Slice(cats, func(i, j int) bool {
			return strings.ToLower(cats[i].Name) < strings.ToLower(cats[j].Name)
		})
		for _, cat := range cats {
			b.WriteString(categoryPill(cat.Name, categoryParam, c.view.TargetMargin))
		}
		b.WriteString("</div>")
	}

	b.WriteString("<div class=\"grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-5\">")
	if len(c.view.Dishes) == 0 {
		b.WriteString("<div class=\"col-span-full text-center py-10 text-slate-400\">В цій категорії немає страв</div>")
	} else {
		for _, dish := range c.view.Dishes {
			b.WriteString(renderDishCard(dish, c.view.TargetMargin, c.view.ShowArchived))
		}
	}
	b.WriteString("</div>")

	b.WriteString("</div>")
	_, err := io.WriteString(w, b.String())
	return err
}

func statusPill(status string, showArchived bool, showDiscounted bool, targetMargin float64, category string) string {
	label := "Активні"
	switch status {
	case "archive":
		label = "Архів"
	case "discounted":
		label = "Акційні"
	}

	active := false
	switch status {
	case "active":
		active = !showArchived && !showDiscounted
	case "archive":
		active = showArchived
	case "discounted":
		active = showDiscounted
	}

	cls := "bg-white text-slate-600 border-slate-200 hover:bg-slate-50 shadow-sm"
	if active {
		switch status {
		case "archive":
			cls = "bg-amber-600 text-white border-amber-700"
		case "discounted":
			cls = "bg-red-600 text-white border-red-700"
		default:
			cls = "bg-slate-800 text-white border-slate-900"
		}
	}

	params := url.Values{}
	if status != "active" {
		params.Set("status", status)
	}
	params.Set("target_margin", fmt.Sprintf("%.1f", targetMargin))
	if status == "active" && category != "" && category != "Усі" {
		params.Set("category", category)
	}
	return fmt.Sprintf("<a href=\"/admin/menu?%s\" class=\"px-4 py-2 rounded-full text-sm font-medium border %s\">%s</a>", params.Encode(), cls, label)
}

func categoryPill(label, selected string, targetMargin float64) string {
	active := selected == label || (selected == "" && label == "Усі")
	cls := "bg-white text-slate-600 border-slate-200 hover:bg-slate-50 shadow-sm"
	if active {
		cls = "bg-slate-800 text-white border-slate-900"
	}
	params := url.Values{}
	params.Set("category", label)
	params.Set("target_margin", fmt.Sprintf("%.1f", targetMargin))
	return fmt.Sprintf("<a href=\"/admin/menu?%s\" class=\"px-4 py-2 rounded-full text-sm font-medium border %s\">%s</a>", params.Encode(), cls, html.EscapeString(label))
}

func renderDishCard(dish adminservice.MenuDish, targetMargin float64, showArchived bool) string {
	var b strings.Builder
	b.WriteString("<div class=\"bg-white rounded-2xl border border-slate-200 shadow-sm hover:shadow-md transition-shadow p-5 flex flex-col\">")
	b.WriteString("<div class=\"flex justify-between items-start mb-2 pr-2\">")
	b.WriteString("<div><h3 class=\"font-bold text-slate-800 text-base leading-tight\">")
	b.WriteString(html.EscapeString(dish.Name))
	b.WriteString("</h3>")
	b.WriteString("<p class=\"text-[11px] text-slate-400 font-medium mt-0.5 mb-1\">")
	b.WriteString(html.EscapeString(dish.CategoryName))
	b.WriteString("</p>")
	b.WriteString("<div class=\"text-xs text-slate-500 font-medium bg-slate-50 inline-block px-1.5 py-0.5 rounded border border-slate-100\">")
	b.WriteString(fmt.Sprintf("%d г · %d хв", dish.PortionSize, dish.CookingTime))
	b.WriteString("</div></div>")
	if dish.HasYieldDiscount && !showArchived {
		b.WriteString("<div class=\"flex flex-col items-end gap-0.5\">")
		b.WriteString(fmt.Sprintf("<span class=\"text-xs text-slate-400 mono line-through\">%.2f ₴</span>", dish.OriginalPrice))
		b.WriteString(fmt.Sprintf("<span class=\"text-lg font-bold text-red-600 mono bg-red-50 px-2.5 py-1 rounded-lg border border-red-200 shadow-inner\">%.2f ₴</span>", dish.Price))
		b.WriteString("</div>")
	} else {
		b.WriteString("<div class=\"text-lg font-bold text-slate-900 mono bg-slate-50 px-2.5 py-1 rounded-lg border border-slate-100 shadow-inner\">")
		b.WriteString(fmt.Sprintf("%.2f ₴", dish.Price))
		b.WriteString("</div>")
	}
	b.WriteString("</div>")

	if dish.HasYieldDiscount && !showArchived {
		discount := (1 - dish.Price/dish.OriginalPrice) * 100
		b.WriteString("<div class=\"flex items-center justify-between mt-1.5 mb-1\">")
		b.WriteString(fmt.Sprintf("<span class=\"text-[11px] text-red-600 font-bold\">Знижка: %.0f%% · діє до: %s</span>", discount, dish.YieldExpiresAt.Format("02.01 15:04")))
		b.WriteString(fmt.Sprintf("<button type=\"button\" hx-post=\"/admin/menu/%d/restore-price\" hx-swap=\"none\" hx-on::after-request=\"location.reload()\" class=\"text-[10px] bg-white hover:bg-red-50 text-red-600 border border-red-200 font-bold px-2 py-1 rounded-md whitespace-nowrap\">Відновити ціну</button>", dish.ID))
		b.WriteString("</div>")
	}

	if len(dish.Recipe) > 0 {
		b.WriteString("<div class=\"flex flex-wrap gap-1.5 mt-3\">")
		for _, r := range dish.Recipe {
			b.WriteString("<span class=\"bg-slate-100 text-slate-600 border border-slate-200 text-[10px] px-2 py-0.5 rounded font-medium\">")
			b.WriteString(html.EscapeString(r.IngredientName))
			b.WriteString(": ")
			b.WriteString(fmt.Sprintf("%g%s", r.Qty, html.EscapeString(r.UnitName)))
			b.WriteString("</span>")
		}
		b.WriteString("</div>")
		b.WriteString("<div class=\"flex-1\"></div>")
	}

	b.WriteString(renderProfitabilityBlock(dish, targetMargin))

	b.WriteString("<div class=\"mt-4 flex gap-2\">")
	if showArchived {
		b.WriteString(fmt.Sprintf("<button type=\"button\" hx-post=\"/admin/menu/%d/unarchive\" hx-swap=\"none\" class=\"flex-1 bg-green-50 hover:bg-green-100 text-green-700 border border-green-200 text-[11px] uppercase tracking-wider font-bold py-2 rounded-lg\">Розархівувати</button>", dish.ID))
	} else {
		b.WriteString(fmt.Sprintf("<button type=\"button\" hx-get=\"/admin/menu/%d/edit\" hx-target=\"#admin-modal-content\" hx-swap=\"innerHTML\" class=\"flex-1 bg-slate-50 hover:bg-slate-100 text-slate-700 border border-slate-200 text-[11px] uppercase tracking-wider font-bold py-2 rounded-lg\">Редагувати</button>", dish.ID))
		b.WriteString(fmt.Sprintf("<button type=\"button\" data-archive-url=\"/admin/menu/%d\" data-archive-label=\"%s\" onclick=\"adminConfirmArchive(this.dataset.archiveUrl,this.dataset.archiveLabel)\" class=\"flex-1 bg-amber-50 hover:bg-amber-100 text-amber-700 border border-amber-200 text-[11px] uppercase tracking-wider font-bold py-2 rounded-lg\">В архів</button>", dish.ID, html.EscapeString(dish.Name)))
	}
	b.WriteString("</div>")

	b.WriteString("</div>")
	return b.String()
}

func renderProfitabilityBlock(dish adminservice.MenuDish, targetMargin float64) string {
	var b strings.Builder
	b.WriteString("<div class=\"mt-4 pt-3 border-t border-slate-100\">")
	if !dish.CostKnown {
		b.WriteString("<div class=\"text-xs text-slate-500 bg-slate-50 border border-slate-200 rounded-lg px-2 py-1.5\">Немає цін закупівлі для розрахунку рентабельності</div>")
		b.WriteString("</div>")
		return b.String()
	}

	b.WriteString("<div class=\"flex items-center justify-between mb-2\">")
	b.WriteString("<span class=\"text-xs text-slate-500 font-medium\">Собівартість: <span class=\"mono\">")
	b.WriteString(fmt.Sprintf("%.2f ₴", dish.Cost))
	b.WriteString("</span></span>")

	if dish.Profitable {
		b.WriteString("<span class=\"text-xs font-bold text-green-600 bg-green-50 px-2 py-1 rounded-md border border-green-200 shadow-sm\">Рент.: ")
		b.WriteString(fmt.Sprintf("%.1f%%", dish.MarginPercent))
		b.WriteString("</span>")
		b.WriteString("</div></div>")
		return b.String()
	}

	b.WriteString("<span class=\"text-[11px] font-bold text-red-600 bg-red-50 px-2 py-1 rounded-md border border-red-200 shadow-sm\">Рент.: ")
	b.WriteString(fmt.Sprintf("%.1f%%", dish.MarginPercent))
	b.WriteString(fmt.Sprintf(" (Мін: %.1f%%)", targetMargin))
	b.WriteString("</span></div>")

	if dish.RecommendedPrice > 0 {
		b.WriteString("<div class=\"bg-amber-50 border border-amber-200 rounded-lg p-2.5 flex items-center justify-between\">")
		b.WriteString("<span class=\"text-[10px] text-amber-800 font-bold uppercase tracking-wide\">Низька рентабельність</span>")
		b.WriteString("<form method=\"POST\" action=\"/admin/menu/")
		b.WriteString(fmt.Sprintf("%d/price\" class=\"m-0\">", dish.ID))
		b.WriteString("<input type=\"hidden\" name=\"price\" value=\"")
		b.WriteString(fmt.Sprintf("%.2f", dish.RecommendedPrice))
		b.WriteString("\" />")
		b.WriteString("<button type=\"submit\" class=\"text-xs bg-amber-500 hover:bg-amber-600 text-white font-bold px-2 py-1 rounded-md\">Встановити: ")
		b.WriteString(fmt.Sprintf("%.2f ₴", dish.RecommendedPrice))
		b.WriteString("</button></form>")
		b.WriteString("</div>")
	}

	b.WriteString("</div>")
	return b.String()
}

func MenuPage(view *adminservice.MenuPageView) templ.Component {
	return menuPageComponent{view: view}
}
