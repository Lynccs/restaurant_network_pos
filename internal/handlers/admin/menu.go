package adminhandler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	adminservice "restaurant_network_pos/internal/service/admin"
	"restaurant_network_pos/templates/layouts"
	adminpages "restaurant_network_pos/templates/pages/admin"

	"github.com/go-chi/chi/v5"
)

func parseTargetMargin(r *http.Request) float64 {
	val := strings.TrimSpace(r.URL.Query().Get("target_margin"))
	if val == "" {
		return 30
	}
	parsed, err := strconv.ParseFloat(strings.ReplaceAll(val, ",", "."), 64)
	if err != nil || parsed <= 0 {
		return 30
	}
	return parsed
}

func (h *Handler) MenuPage(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, name, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	targetMargin := parseTargetMargin(r)
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	showArchived := r.URL.Query().Get("status") == "archive"
	showDiscounted := r.URL.Query().Get("status") == "discounted"

	view, err := h.MenuSvc.GetMenuPage(restaurantID, targetMargin, category, showArchived, showDiscounted)
	if err != nil {
		handlerLog.Printf("MenuPage: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	_, yieldCount, _ := h.MenuSvc.GetYieldAlerts(restaurantID)
	view.YieldCount = yieldCount

	layouts.AdminLayout(name, "menu", adminpages.MenuPage(view)).Render(r.Context(), w)
}

func (h *Handler) MenuDishModal(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	view, err := h.MenuSvc.GetMenuForm(id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	body := menuModalBody(view, view.Dish, id > 0)
	title := "Нова страва / Тех.картка"
	if id > 0 {
		title = "Редагувати страву"
	}
	renderAdminModal(w, title, "", body)
}

func (h *Handler) MenuSaveDish(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	categoryID, _ := strconv.Atoi(r.FormValue("category_id"))
	portionSize, _ := strconv.Atoi(r.FormValue("portion_size"))
	cookingTime, _ := strconv.Atoi(r.FormValue("cooking_time"))
	price, _ := strconv.ParseFloat(strings.ReplaceAll(r.FormValue("price"), ",", "."), 64)

	recipe := parseRecipeRows(r.Form["ingredient_id"], r.Form["quantity"])

	input := adminservice.MenuDishInput{
		Name:        name,
		CategoryID:  categoryID,
		PortionSize: portionSize,
		CookingTime: cookingTime,
		Price:       price,
		Recipe:      recipe,
	}

	var err error
	if id > 0 {
		err = h.MenuSvc.UpdateDish(id, input)
	} else {
		_, err = h.MenuSvc.CreateDish(input)
	}
	if err != nil {
		view, viewErr := h.MenuSvc.GetMenuForm(id)
		if viewErr != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		body := menuModalBody(view, buildDishFromInput(id, input), id > 0)
		renderAdminModal(w, modalTitle(id > 0), err.Error(), body)
		return
	}

	triggerAdminRefresh(w)
}

func (h *Handler) MenuArchiveDish(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := h.MenuSvc.ArchiveDish(id); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	triggerAdminRefresh(w)
}

func (h *Handler) MenuUnarchiveDish(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := h.MenuSvc.UnarchiveDish(id); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	triggerAdminRefresh(w)
}

func (h *Handler) MenuUpdatePrice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	price, _ := strconv.ParseFloat(strings.ReplaceAll(r.FormValue("price"), ",", "."), 64)

	if err := h.MenuSvc.UpdateDishPrice(id, price); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	redirectURL := r.Referer()
	if redirectURL == "" {
		redirectURL = "/admin/menu"
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (h *Handler) MenuYieldAlertsModal(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	alerts, _, err := h.MenuSvc.GetYieldAlerts(restaurantID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	body := renderYieldAlertsBody(alerts)
	renderAdminModal(w, "Yield Management — пропозиції знижок", "", body)
}

func (h *Handler) MenuApplyYieldDiscount(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	price, _ := strconv.ParseFloat(strings.ReplaceAll(r.FormValue("price"), ",", "."), 64)
	if err := h.MenuSvc.ApplyYieldDiscount(id, price); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	alerts, _, err := h.MenuSvc.GetYieldAlerts(restaurantID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	body := renderYieldAlertsBody(alerts)
	w.Header().Set("HX-Trigger", `{"adminMenuNeedsRefresh":true,"showToast":"yieldApplied"}`)
	renderAdminModal(w, "Yield Management — пропозиції знижок", "", body)
}

func (h *Handler) MenuRestorePrice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := h.MenuSvc.RestoreYieldPrice(id); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	triggerAdminRefresh(w)
}

func renderYieldAlertsBody(alerts []adminservice.YieldAlert) string {
	if len(alerts) == 0 {
		return `<div class="text-center py-8 text-slate-400 text-sm">Немає страв з інгредієнтами, термін яких спливає найближчим часом.</div>
<div class="mt-4 flex justify-end"><button type="button" onclick="adminCloseModal()" class="border border-slate-200 text-slate-700 text-sm px-4 py-2 rounded-lg font-medium">Закрити</button></div>`
	}

	var b strings.Builder
	b.WriteString(`<div class="space-y-3 max-h-[60vh] overflow-y-auto pr-1">`)

	seen := make(map[int]bool)
	for _, a := range alerts {
		if seen[a.DishID] {
			continue
		}
		seen[a.DishID] = true

		var daysLabel string
		switch a.DaysLeft {
		case 0:
			daysLabel = "сьогодні"
		case 1:
			daysLabel = "1 день"
		default:
			daysLabel = fmt.Sprintf("%d дн.", a.DaysLeft)
		}

		b.WriteString(`<div class="bg-slate-50 border border-slate-200 rounded-xl p-4">`)
		b.WriteString(`<div class="flex justify-between items-start mb-2">`)
		b.WriteString(`<div>`)
		b.WriteString(`<p class="font-bold text-slate-800 text-sm">`)
		b.WriteString(templateEscape(a.DishName))
		b.WriteString(`</p>`)
		b.WriteString(`<p class="text-xs text-amber-700 mt-0.5">`)
		b.WriteString(fmt.Sprintf("⚠ %s — %s (%g %s)",
			templateEscape(a.IngredientName), daysLabel,
			a.Qty, templateEscape(a.UnitName)))
		b.WriteString(`</p>`)
		b.WriteString(`</div>`)
		b.WriteString(`<span class="text-sm font-bold text-slate-700 mono">`)
		b.WriteString(fmt.Sprintf("%.2f ₴", a.CurrentPrice))
		b.WriteString(`</span>`)
		b.WriteString(`</div>`)

		formID := fmt.Sprintf("yield-form-%d", a.DishID)
		inputID := fmt.Sprintf("yield-price-%d", a.DishID)
		pctID := fmt.Sprintf("yield-pct-%d", a.DishID)
		b.WriteString(fmt.Sprintf(`<form id="%s" hx-post="/admin/menu/%d/yield-price" hx-target="#admin-modal-content" hx-swap="innerHTML" class="flex items-center gap-2 mt-2">`, formID, a.DishID))
		b.WriteString(`<label class="text-xs text-slate-500 font-medium whitespace-nowrap">Нова ціна:</label>`)
		b.WriteString(fmt.Sprintf(`<input id="%s" type="number" name="price" value="%.2f" step="0.01" min="0.01" data-original="%.2f" oninput="(function(el){var pct=document.getElementById('%s');var orig=parseFloat(el.dataset.original)||0;var val=parseFloat(el.value)||0;pct.textContent=orig>0&&val>0?'-'+Math.round((1-val/orig)*100)+'%%':'';})(this)" class="w-28 border border-slate-300 rounded-lg px-2 py-1.5 text-sm mono text-center" />`,
			inputID, a.SuggestedPrice, a.CurrentPrice, pctID))
		b.WriteString(`<span class="text-xs text-slate-400 font-medium">₴</span>`)
		b.WriteString(fmt.Sprintf(`<span id="%s" class="text-[10px] text-amber-600 font-bold w-8">-%.0f%%</span>`, pctID, (1-a.SuggestedPrice/a.CurrentPrice)*100))
		b.WriteString(`<button type="submit" class="ml-auto bg-amber-500 hover:bg-amber-600 text-white text-xs font-bold px-3 py-1.5 rounded-lg whitespace-nowrap">Застосувати</button>`)
		b.WriteString(`</form>`)
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)
	b.WriteString(`<div class="mt-4 flex justify-end"><button type="button" onclick="adminCloseModal()" class="border border-slate-200 text-slate-700 text-sm px-4 py-2 rounded-lg font-medium">Закрити</button></div>`)
	return b.String()
}

func menuModalBody(view *adminservice.MenuFormView, dish *adminservice.MenuDish, isEdit bool) string {
	action := "/admin/menu"
	if isEdit && dish != nil {
		action = fmt.Sprintf("/admin/menu/%d", dish.ID)
	}

	catOptions := buildCategoryOptions(view.Categories, dish)
	var recipeRows strings.Builder
	if dish != nil && len(dish.Recipe) > 0 {
		for _, r := range dish.Recipe {
			row := buildRecipeRow(view.Ingredients, r.IngredientID, r.Qty)
			recipeRows.WriteString(row)
		}
	}

	name := ""
	portion := ""
	time := ""
	price := ""
	if dish != nil {
		name = dish.Name
		portion = fmt.Sprintf("%d", dish.PortionSize)
		time = fmt.Sprintf("%d", dish.CookingTime)
		price = fmt.Sprintf("%.2f", dish.Price)
	}

	return fmt.Sprintf(`
<form hx-post="%s" hx-target="#admin-modal-content" hx-swap="innerHTML">
  <div class="space-y-4">
    <div class="grid grid-cols-2 gap-4">
      <div>
        <label class="text-[11px] font-bold text-slate-500 uppercase tracking-wider block mb-1.5">Назва страви <span class="text-red-500">*</span></label>
        <input name="name" value="%s" class="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm" placeholder="Напр. Борщ" required />
      </div>
      <div>
        <label class="text-[11px] font-bold text-slate-500 uppercase tracking-wider block mb-1.5">Категорія <span class="text-red-500">*</span></label>
		<select name="category_id" class="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm bg-white" required>
		  %s
		</select>
      </div>
    </div>
    <div class="grid grid-cols-3 gap-4">
      <div>
        <label class="text-[11px] font-bold text-slate-500 uppercase tracking-wider block mb-1.5">Вихід (г) <span class="text-red-500">*</span></label>
        <input name="portion_size" value="%s" class="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm mono text-center" placeholder="300" required />
      </div>
      <div>
        <label class="text-[11px] font-bold text-slate-500 uppercase tracking-wider block mb-1.5">Час приг. (хв) <span class="text-red-500">*</span></label>
        <input name="cooking_time" value="%s" class="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm mono text-center" placeholder="15" required />
      </div>
      <div>
        <label class="text-[11px] font-bold text-slate-500 uppercase tracking-wider block mb-1.5">Ціна (₴) <span class="text-red-500">*</span></label>
        <input name="price" value="%s" class="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm mono text-center" placeholder="100.00" required />
      </div>
    </div>

    <div class="mt-2 pt-4 border-t border-slate-200">
      <div class="flex justify-between items-center mb-3">
        <label class="text-[11px] font-bold text-slate-500 uppercase tracking-wider">Технологічна картка (Рецепт)</label>
		<button type="button" onclick="menuAddRecipeRow()" class="text-blue-600 bg-blue-50 hover:bg-blue-100 text-[11px] px-3 py-1.5 rounded-lg font-bold border border-blue-200">Додати інгредієнт</button>
      </div>
      <div class="max-h-[300px] overflow-y-auto pr-2 custom-scrollbar">
        <div id="recipe-container" class="space-y-2">
          %s
          <div id="recipe-empty" class="text-slate-400 text-[11px] font-bold uppercase tracking-wider bg-white text-center p-4 border-2 border-dashed border-slate-200 rounded-xl">Інгредієнти не додано</div>
        </div>
      </div>
    </div>
  </div>

  <style>
    .custom-scrollbar::-webkit-scrollbar { width: 4px; }
    .custom-scrollbar::-webkit-scrollbar-track { background: transparent; }
    .custom-scrollbar::-webkit-scrollbar-thumb { background: #e2e8f0; border-radius: 10px; }
    .custom-scrollbar::-webkit-scrollbar-thumb:hover { background: #cbd5e1; }
  </style>

  <div class="mt-6 flex gap-3">
    <button type="submit" class="flex-1 bg-green-600 hover:bg-green-700 text-white text-sm py-2.5 rounded-lg font-bold">%s</button>
    <button type="button" onclick="adminCloseModal()" class="flex-1 border bg-white border-slate-200 text-slate-700 text-sm py-2.5 rounded-lg font-bold">Скасувати</button>
  </div>

	<datalist id="menu-ingredient-options">
		%s
	</datalist>

	<template id="menu-recipe-template">
    %s
  </template>

  <script>
    (function() {
      function bindRow(row) {
		var input = row.querySelector('.recipe-ing-input');
		var hidden = row.querySelector('.recipe-ing-id');
        var unit = row.querySelector('.recipe-unit');
		function sync() {
			var val = (input.value || '').trim();
			var options = document.getElementById('menu-ingredient-options');
			var match = null;
			if (options) {
				options.querySelectorAll('option').forEach(function(opt) {
					if (opt.value === val) match = opt;
				});
			}
			if (match) {
				hidden.value = match.getAttribute('data-id') || '';
				unit.textContent = match.getAttribute('data-unit') || '';
				return;
			}
			hidden.value = '';
			unit.textContent = '';
		}
		input.addEventListener('input', sync);
		sync();
      }

      function toggleEmpty() {
        var container = document.getElementById('recipe-container');
        var empty = document.getElementById('recipe-empty');
        var rows = container.querySelectorAll('.recipe-row');
        empty.style.display = rows.length > 0 ? 'none' : 'block';
      }

      window.menuAddRecipeRow = function() {
        var tpl = document.getElementById('menu-recipe-template');
        var wrapper = document.createElement('div');
        wrapper.innerHTML = tpl.innerHTML.trim();
        var row = wrapper.firstElementChild;
        document.getElementById('recipe-container').appendChild(row);
        bindRow(row);
        toggleEmpty();
      };

      document.querySelectorAll('.recipe-row').forEach(bindRow);
      toggleEmpty();
    })();
  </script>
</form>`,
		action,
		templateEscape(name),
		catOptions,
		templateEscape(portion),
		templateEscape(time),
		templateEscape(price),
		recipeRows.String(),
		menuSubmitLabel(isEdit),
		buildIngredientDatalist(view.Ingredients),
		buildRecipeRow(view.Ingredients, 0, 0),
	)
}

func buildCategoryOptions(categories []adminservice.MenuCategory, dish *adminservice.MenuDish) string {
	var b strings.Builder
	selectedID := 0
	if dish != nil {
		selectedID = dish.CategoryID
	}
	for _, c := range categories {
		selected := ""
		if c.ID == selectedID {
			selected = " selected"
		}
		b.WriteString(fmt.Sprintf("<option value=\"%d\"%s>%s</option>", c.ID, selected, templateEscape(c.Name)))
	}
	return b.String()
}

func buildIngredientOptions(ingredients []adminservice.MenuIngredient, selectedID int) string {
	var b strings.Builder
	for _, ing := range ingredients {
		selected := ""
		if ing.ID == selectedID {
			selected = " selected"
		}
		b.WriteString(fmt.Sprintf("<option value=\"%d\" data-unit=\"%s\"%s>%s</option>", ing.ID, templateEscape(ing.UnitName), selected, templateEscape(ing.Name)))
	}
	return b.String()
}

func buildIngredientDatalist(ingredients []adminservice.MenuIngredient) string {
	var b strings.Builder
	for _, ing := range ingredients {
		b.WriteString(fmt.Sprintf("<option value=\"%s\" data-id=\"%d\" data-unit=\"%s\"></option>", templateEscape(ing.Name), ing.ID, templateEscape(ing.UnitName)))
	}
	return b.String()
}

func buildRecipeRow(ingredients []adminservice.MenuIngredient, selectedID int, qty float64) string {
	qtyStr := ""
	if qty > 0 {
		qtyStr = fmt.Sprintf("%g", qty)
	}
	selectedName := templateEscape(firstIngredientName(ingredients, selectedID))
	return fmt.Sprintf(`<div class="recipe-row flex gap-2 items-center bg-white p-2 rounded-xl border border-slate-200 shadow-sm relative group pr-9">
  <input class="recipe-ing-input flex-1 border border-slate-300 rounded-lg px-2 py-1.5 text-sm bg-slate-50" list="menu-ingredient-options" placeholder="Інгредієнт..." value="%s" />
  <input type="hidden" name="ingredient_id" class="recipe-ing-id" value="%d" />
  <input name="quantity" value="%s" type="number" step="0.001" min="0" class="w-24 border border-slate-300 rounded-lg px-2 py-1.5 text-sm mono text-center" placeholder="К-сть" />
	<span class="recipe-unit w-12 text-xs text-slate-500 text-center">%s</span>
  <button type="button" onclick="this.closest('.recipe-row').remove();" class="absolute right-2 text-slate-300 hover:text-red-500 hover:bg-red-50 rounded-md p-1">
    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
  </button>
</div>`,
		selectedName,
		selectedID,
		templateEscape(qtyStr),
		templateEscape(firstUnit(ingredients, selectedID)),
	)
}

func firstUnit(ingredients []adminservice.MenuIngredient, selectedID int) string {
	for _, ing := range ingredients {
		if ing.ID == selectedID {
			return ing.UnitName
		}
	}
	return ""
}

func firstIngredientName(ingredients []adminservice.MenuIngredient, selectedID int) string {
	for _, ing := range ingredients {
		if ing.ID == selectedID {
			return ing.Name
		}
	}
	return ""
}

func menuSubmitLabel(isEdit bool) string {
	if isEdit {
		return "Зберегти зміни"
	}
	return "Створити страву"
}

func modalTitle(isEdit bool) string {
	if isEdit {
		return "Редагувати страву"
	}
	return "Нова страва / Тех.картка"
}

func templateEscape(val string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	).Replace(val)
}

func parseRecipeRows(ingredientIDs, qtys []string) []adminservice.MenuRecipeItem {
	result := make([]adminservice.MenuRecipeItem, 0, len(ingredientIDs))
	for i := 0; i < len(ingredientIDs) && i < len(qtys); i++ {
		id, _ := strconv.Atoi(ingredientIDs[i])
		qty, _ := strconv.ParseFloat(strings.ReplaceAll(qtys[i], ",", "."), 64)
		if id <= 0 || qty <= 0 {
			continue
		}
		result = append(result, adminservice.MenuRecipeItem{
			IngredientID: id,
			Qty:          qty,
		})
	}
	return result
}

func buildDishFromInput(id int, input adminservice.MenuDishInput) *adminservice.MenuDish {
	return &adminservice.MenuDish{
		ID:           id,
		Name:         input.Name,
		Price:        input.Price,
		PortionSize:  input.PortionSize,
		CookingTime:  input.CookingTime,
		CategoryID:   input.CategoryID,
		CategoryName: "",
		Recipe:       input.Recipe,
	}
}
