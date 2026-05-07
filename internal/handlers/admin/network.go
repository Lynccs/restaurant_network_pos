package adminhandler

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	adminservice "restaurant_network_pos/internal/service/admin"
	"restaurant_network_pos/templates/layouts"
	adminpages "restaurant_network_pos/templates/pages/admin"

	"github.com/go-chi/chi/v5"
)

// NetworkPage — GET /admin/network
func (h *Handler) NetworkPage(w http.ResponseWriter, r *http.Request) {
	restaurantID, _, name, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	view, err := h.NetworkSvc.GetNetworkPage(restaurantID)
	if err != nil {
		handlerLog.Printf("NetworkPage: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	layouts.AdminLayout(name, "network", adminpages.NetworkPage(view)).Render(r.Context(), w)
}

func (h *Handler) NetworkRestaurantModal(w http.ResponseWriter, r *http.Request) {
	renderAdminModal(w, "Створення закладу", "", restaurantModalBody("", "", ""))
}

func (h *Handler) NetworkRestaurantEditModal(w http.ResponseWriter, r *http.Request) {
	currentRestaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if id != currentRestaurantID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	restaurant, err := h.NetworkSvc.GetRestaurant(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	renderAdminModal(w, "Редагувати заклад", "", restaurantModalBody(restaurant.Name, restaurant.Address, restaurant.Phone))
}

func (h *Handler) NetworkUpdateRestaurant(w http.ResponseWriter, r *http.Request) {
	currentRestaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if id != currentRestaurantID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	name := r.FormValue("name")
	address := r.FormValue("address")
	phone := r.FormValue("phone")

	if name == "" || address == "" || phone == "" {
		renderAdminModal(w, "Редагувати заклад", "Всі поля обов'язкові", restaurantModalBody(name, address, phone))
		return
	}

	if err := h.NetworkSvc.UpdateRestaurant(id, name, address, phone); err != nil {
		renderAdminModal(w, "Редагувати заклад", err.Error(), restaurantModalBody(name, address, phone))
		return
	}

	triggerAdminRefresh(w)
}

func (h *Handler) NetworkCreateRestaurant(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	address := r.FormValue("address")
	phone := r.FormValue("phone")

	if name == "" || address == "" || phone == "" {
		renderAdminModal(w, "Створення закладу", "Всі поля обов'язкові", restaurantModalBody(name, address, phone))
		return
	}

	// Замість збереження показуємо крок 2: Додавання адміна
	body := restaurantAdminStepModalBody(name, address, phone, "", "", "")
	renderAdminModal(w, "Крок 2: Адміністратор закладу", "", body)
}

func (h *Handler) NetworkCreateRestaurantFinal(w http.ResponseWriter, r *http.Request) {
	// Дані закладу
	resName := r.FormValue("res_name")
	resAddress := r.FormValue("res_address")
	resPhone := r.FormValue("res_phone")

	// Дані адміна
	adminName := r.FormValue("admin_name")
	adminPhone := r.FormValue("admin_phone")
	adminPin := r.FormValue("admin_pin")

	// 1. Створюємо заклад
	resID, err := h.NetworkSvc.CreateRestaurant(resName, resAddress, resPhone)
	if err != nil {
		body := restaurantAdminStepModalBody(resName, resAddress, resPhone, adminName, adminPhone, adminPin)
		renderAdminModal(w, "Крок 2: Адміністратор закладу", err.Error(), body)
		return
	}

	// 2. Створюємо адміна для цього закладу
	if err := h.NetworkSvc.CreateStaff("admin", adminName, adminPhone, adminPin, resID, 0); err != nil {
		// Якщо адмін не створився, ми вже маємо ресторан (тут краще була б транзакція, але для простоти поки так)
		body := restaurantAdminStepModalBody(resName, resAddress, resPhone, adminName, adminPhone, adminPin)
		renderAdminModal(w, "Крок 2: Адміністратор закладу", "Заклад створено, але помилка адміна: "+err.Error(), body)
		return
	}

	triggerAdminRefresh(w)
}

func restaurantAdminStepModalBody(resName, resAddress, resPhone, adminName, adminPhone, adminPin string) string {
	return fmt.Sprintf(`
<form hx-post="/admin/network/restaurants/final" hx-target="#admin-modal-content" hx-swap="innerHTML">
  <!-- Приховані дані закладу з першого кроку -->
  <input type="hidden" name="res_name" value="%s" />
  <input type="hidden" name="res_address" value="%s" />
  <input type="hidden" name="res_phone" value="%s" />

  <div class="bg-blue-50 p-3 rounded-lg mb-4 text-xs text-blue-700">
    Ви створюєте заклад <b>%s</b>. Тепер додайте першого адміністратора для нього.
  </div>

  <div class="space-y-3">
    <label class="block text-xs font-semibold text-slate-600">ПІБ Адміністратора</label>
    <input name="admin_name" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="Іван Іванов" required />
    
    <label class="block text-xs font-semibold text-slate-600">Телефон (буде логіном)</label>
    <input name="admin_phone" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="+380..." required />
    
    <label class="block text-xs font-semibold text-slate-600">PIN-код (пароль)</label>
    <input name="admin_pin" value="%s" type="password" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="1234" required />
  </div>

  <div class="mt-5 flex justify-end gap-2">
    <button type="button" hx-get="/admin/network/restaurants/new" hx-target="#admin-modal-content" class="px-4 py-2 rounded-lg text-sm border">Назад</button>
    <button type="submit" class="px-4 py-2 rounded-lg text-sm bg-blue-600 text-white font-medium">Створити</button>
  </div>
</form>`,
		template.HTMLEscapeString(resName),
		template.HTMLEscapeString(resAddress),
		template.HTMLEscapeString(resPhone),
		template.HTMLEscapeString(resName),
		template.HTMLEscapeString(adminName),
		template.HTMLEscapeString(adminPhone),
		template.HTMLEscapeString(adminPin),
	)
}

func (h *Handler) NetworkStaffModal(w http.ResponseWriter, r *http.Request) {
	currentRestaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	restaurants, err := h.NetworkSvc.ListRestaurants()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	restaurantName := findRestaurantName(restaurants, currentRestaurantID)

	specs, err := h.NetworkSvc.ListChefSpecializations()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	body := staffCreateModalBody(restaurantName, currentRestaurantID, "waiter", "", "", "", specs, 0)
	renderAdminModal(w, "Додати працівника", "", body)
}

func (h *Handler) NetworkCreateStaff(w http.ResponseWriter, r *http.Request) {
	currentRestaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	restaurants, err := h.NetworkSvc.ListRestaurants()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	restaurantName := findRestaurantName(restaurants, currentRestaurantID)

	role := r.FormValue("role")
	name := r.FormValue("name")
	phone := r.FormValue("phone")
	pin := r.FormValue("pin")
	specID, _ := strconv.Atoi(r.FormValue("chef_specialization_id"))

	specs, err := h.NetworkSvc.ListChefSpecializations()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := h.NetworkSvc.CreateStaff(role, name, phone, pin, currentRestaurantID, specID); err != nil {
		body := staffCreateModalBody(restaurantName, currentRestaurantID, role, name, phone, pin, specs, specID)
		renderAdminModal(w, "Додати працівника", err.Error(), body)
		return
	}

	triggerAdminRefresh(w)
}

func (h *Handler) NetworkTableModal(w http.ResponseWriter, r *http.Request) {
	currentRestaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	restaurants, err := h.NetworkSvc.ListRestaurants()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	restaurantName := findRestaurantName(restaurants, currentRestaurantID)

	body := tableCreateModalBody(restaurantName, currentRestaurantID, "", "")
	renderAdminModal(w, "Додати столик", "", body)
}

func (h *Handler) NetworkCreateTable(w http.ResponseWriter, r *http.Request) {
	currentRestaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	restaurants, err := h.NetworkSvc.ListRestaurants()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	restaurantName := findRestaurantName(restaurants, currentRestaurantID)

	number := r.FormValue("number")
	capacity := r.FormValue("capacity")

	numberVal, _ := strconv.Atoi(number)
	capacityVal, _ := strconv.Atoi(capacity)
	if err := h.NetworkSvc.CreateTable(currentRestaurantID, numberVal, capacityVal); err != nil {
		body := tableCreateModalBody(restaurantName, currentRestaurantID, number, capacity)
		renderAdminModal(w, "Додати столик", err.Error(), body)
		return
	}

	triggerAdminRefresh(w)
}

func (h *Handler) NetworkStaffEditModal(w http.ResponseWriter, r *http.Request) {
	currentRestaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	role := chi.URLParam(r, "role")
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	staff, err := h.NetworkSvc.GetStaff(role, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if staff.RestaurantID != currentRestaurantID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	specs, _ := h.NetworkSvc.ListChefSpecializations()
	body := staffEditModalBody(id, role, staff.Name, staff.Phone, "", specs, staff.SpecializationID)
	renderAdminModal(w, "Редагувати працівника", "", body)
}

func (h *Handler) NetworkUpdateStaff(w http.ResponseWriter, r *http.Request) {
	currentRestaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	role := chi.URLParam(r, "role")
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	name := r.FormValue("name")
	phone := r.FormValue("phone")
	pin := r.FormValue("pin")
	specID, _ := strconv.Atoi(r.FormValue("chef_specialization_id"))

	if err := h.NetworkSvc.UpdateStaff(role, name, phone, pin, currentRestaurantID, id, specID); err != nil {
		specs, _ := h.NetworkSvc.ListChefSpecializations()
		body := staffEditModalBody(id, role, name, phone, pin, specs, specID)
		renderAdminModal(w, "Редагувати працівника", err.Error(), body)
		return
	}

	triggerAdminRefresh(w)
}

func (h *Handler) NetworkTableEditModal(w http.ResponseWriter, r *http.Request) {
	currentRestaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	table, err := h.NetworkSvc.GetTable(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if table.RestaurantID != currentRestaurantID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	body := tableEditModalBody(id, strconv.Itoa(table.Number), strconv.Itoa(table.Capacity))
	renderAdminModal(w, "Редагувати столик", "", body)
}

func (h *Handler) NetworkUpdateTable(w http.ResponseWriter, r *http.Request) {
	currentRestaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	number := r.FormValue("number")
	capacity := r.FormValue("capacity")

	numberVal, _ := strconv.Atoi(number)
	capacityVal, _ := strconv.Atoi(capacity)
	if err := h.NetworkSvc.UpdateTable(id, currentRestaurantID, numberVal, capacityVal); err != nil {
		body := tableEditModalBody(id, number, capacity)
		renderAdminModal(w, "Редагувати столик", err.Error(), body)
		return
	}

	triggerAdminRefresh(w)
}

func (h *Handler) NetworkDeleteStaff(w http.ResponseWriter, r *http.Request) {
	currentRestaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	role := chi.URLParam(r, "role")
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	if err := h.NetworkSvc.DeleteStaff(role, id, currentRestaurantID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	triggerAdminRefresh(w)
}

func (h *Handler) NetworkDeleteTable(w http.ResponseWriter, r *http.Request) {
	currentRestaurantID, _, _, err := h.sessionData(r)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	if err := h.NetworkSvc.DeleteTable(id, currentRestaurantID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	triggerAdminRefresh(w)
}

func triggerAdminRefresh(w http.ResponseWriter) {
	w.Header().Set("HX-Trigger", `{"closeModal":true}`)
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func renderAdminModal(w http.ResponseWriter, title, errMsg, body string) {
	safeTitle := template.HTMLEscapeString(title)
	safeErr := template.HTMLEscapeString(errMsg)

	var sb strings.Builder
	sb.WriteString("<div class=\"bg-white rounded-xl shadow-xl w-full max-w-xl p-6 max-h-[95vh] overflow-hidden flex flex-col\">")
	sb.WriteString("<div class=\"flex items-center justify-between mb-4 flex-shrink-0\">")
	sb.WriteString("<h3 class=\"text-lg font-semibold text-slate-800\">")
	sb.WriteString(safeTitle)
	sb.WriteString("</h3>")
	sb.WriteString("<button type=\"button\" onclick=\"adminCloseModal()\" class=\"text-slate-400 hover:text-slate-600 text-xl\">×</button>")
	sb.WriteString("</div>")
	if errMsg != "" {
		sb.WriteString("<div class=\"mb-3 text-sm text-red-600 flex-shrink-0\">")
		sb.WriteString(safeErr)
		sb.WriteString("</div>")
	}
	// Body container - NOT scrollable by default here, but can be in the body itself
	sb.WriteString("<div class=\"flex-1 min-h-0\">")
	sb.WriteString(body)
	sb.WriteString("</div>")
	sb.WriteString("</div>")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(sb.String()))
}

func restaurantModalBody(name, address, phone string) string {
	return fmt.Sprintf(`
<form hx-post="/admin/network/restaurants" hx-target="#admin-modal-content" hx-swap="innerHTML">
  <div class="space-y-3">
    <label class="block text-xs font-semibold text-slate-600">Назва</label>
    <input name="name" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="Ресторан №1" required />
    <label class="block text-xs font-semibold text-slate-600">Адреса</label>
    <input name="address" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="вул. Хрещатик, 12" required />
    <label class="block text-xs font-semibold text-slate-600">Телефон</label>
    <input name="phone" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="+380..." required />
  </div>
  <div class="mt-5 flex justify-end gap-2">
    <button type="button" onclick="adminCloseModal()" class="px-4 py-2 rounded-lg text-sm border">Скасувати</button>
    <button type="submit" class="px-4 py-2 rounded-lg text-sm bg-blue-600 text-white">Зберегти</button>
  </div>
</form>`,
		template.HTMLEscapeString(name),
		template.HTMLEscapeString(address),
		template.HTMLEscapeString(phone),
	)
}

func staffCreateModalBody(restaurantName string, restaurantID int, role, name, phone, pin string, specs []adminservice.ChefSpecialization, specID int) string {
	role = strings.ToLower(role)
	if role == "" {
		role = "waiter"
	}

	specOptions := buildChefSpecOptions(specs, specID)
	specVisible := map[bool]string{true: "", false: " hidden"}[role == "chef"]

	return fmt.Sprintf(`
<form hx-post="/admin/network/staff" hx-target="#admin-modal-content" hx-swap="innerHTML">
  <input type="hidden" name="restaurant_id" value="%d" />
  <div class="space-y-3">
    <label class="block text-xs font-semibold text-slate-600">Роль</label>
    <select name="role" id="network-staff-role" class="w-full px-3 py-2 border rounded-lg text-sm">
      <option value="waiter"%s>Офіціант</option>
      <option value="chef"%s>Кухар</option>
      <option value="admin"%s>Адміністратор</option>
    </select>
		<label class="block text-xs font-semibold text-slate-600">Заклад</label>
		<div class="w-full px-3 py-2 border rounded-lg text-sm bg-slate-50 text-slate-600">%s</div>
		<div id="chef-spec-row" class="space-y-1%s">
			<label class="block text-xs font-semibold text-slate-600">Цех / спеціалізація</label>
			<select name="chef_specialization_id" id="chef-specialization" class="w-full px-3 py-2 border rounded-lg text-sm bg-white">
				<option value="">Оберіть цех...</option>
				%s
			</select>
		</div>
    <label class="block text-xs font-semibold text-slate-600">ПІБ</label>
    <input name="name" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="Ім'я Прізвище" required />
    <label class="block text-xs font-semibold text-slate-600">Телефон</label>
    <input name="phone" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="+380..." required />
		<label class="block text-xs font-semibold text-slate-600">PIN</label>
		<input name="pin" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="1234" required />
  </div>
  <div class="mt-5 flex justify-end gap-2">
    <button type="button" onclick="adminCloseModal()" class="px-4 py-2 rounded-lg text-sm border">Скасувати</button>
    <button type="submit" class="px-4 py-2 rounded-lg text-sm bg-blue-600 text-white">Зберегти</button>
  </div>
  <script>
    (function() {
      var roleEl = document.getElementById('network-staff-role');
      var row = document.getElementById('chef-spec-row');
      var sel = document.getElementById('chef-specialization');
      function sync() {
        var isChef = roleEl && roleEl.value === 'chef';
        if (row) row.classList.toggle('hidden', !isChef);
        if (sel) sel.required = isChef;
      }
      if (roleEl) roleEl.addEventListener('change', sync);
      sync();
    })();
  </script>
</form>`,
		restaurantID,
		selectedAttr(role == "waiter"),
		selectedAttr(role == "chef"),
		selectedAttr(role == "admin"),
		template.HTMLEscapeString(restaurantName),
		specVisible,
		specOptions,
		template.HTMLEscapeString(name),
		template.HTMLEscapeString(phone),
		template.HTMLEscapeString(pin),
	)
}

func buildChefSpecOptions(specs []adminservice.ChefSpecialization, selectedID int) string {
	var b strings.Builder
	for _, s := range specs {
		selected := ""
		if s.ID == selectedID {
			selected = " selected"
		}
		b.WriteString(fmt.Sprintf("<option value=\"%d\"%s>%s</option>", s.ID, selected, template.HTMLEscapeString(s.Name)))
	}
	return b.String()
}

func staffEditModalBody(id int, role, name, phone, pin string, specs []adminservice.ChefSpecialization, specID int) string {
	role = strings.ToLower(role)
	label := roleLabel(role)
	specOptions := buildChefSpecOptions(specs, specID)
	specVisible := map[bool]string{true: "", false: " hidden"}[role == "chef"]
	return fmt.Sprintf(`
<form hx-post="/admin/network/staff/%s/%d" hx-target="#admin-modal-content" hx-swap="innerHTML">
	<div class="space-y-3">
		<label class="block text-xs font-semibold text-slate-600">Роль</label>
		<div class="w-full px-3 py-2 border rounded-lg text-sm bg-slate-50 text-slate-600">%s</div>
		<div id="chef-spec-row-edit" class="space-y-1%s">
			<label class="block text-xs font-semibold text-slate-600">Цех / спеціалізація</label>
			<select name="chef_specialization_id" id="chef-specialization-edit" class="w-full px-3 py-2 border rounded-lg text-sm bg-white">
				<option value="">Оберіть цех...</option>
				%s
			</select>
		</div>
		<label class="block text-xs font-semibold text-slate-600">ПІБ</label>
		<input name="name" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="Ім'я Прізвище" required />
		<label class="block text-xs font-semibold text-slate-600">Телефон</label>
		<input name="phone" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="+380..." required />
		<label class="block text-xs font-semibold text-slate-600">Новий PIN (опційно)</label>
		<input name="pin" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="1234" />
	</div>
	<div class="mt-5 flex justify-end gap-2">
		<button type="button" onclick="adminCloseModal()" class="px-4 py-2 rounded-lg text-sm border">Скасувати</button>
		<button type="submit" class="px-4 py-2 rounded-lg text-sm bg-blue-600 text-white">Зберегти</button>
	</div>
</form>`,
		role,
		id,
		template.HTMLEscapeString(label),
		specVisible,
		specOptions,
		template.HTMLEscapeString(name),
		template.HTMLEscapeString(phone),
		template.HTMLEscapeString(pin),
	)
}

func tableCreateModalBody(restaurantName string, restaurantID int, number, capacity string) string {
	return fmt.Sprintf(`
<form hx-post="/admin/network/tables" hx-target="#admin-modal-content" hx-swap="innerHTML">
  <input type="hidden" name="restaurant_id" value="%d" />
  <div class="space-y-3">
    <label class="block text-xs font-semibold text-slate-600">Заклад</label>
    <div class="w-full px-3 py-2 border rounded-lg text-sm bg-slate-50 text-slate-600">%s</div>
    <label class="block text-xs font-semibold text-slate-600">Номер столика</label>
    <input name="number" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="12" required />
    <label class="block text-xs font-semibold text-slate-600">Місткість</label>
    <input name="capacity" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="4" required />
  </div>
  <div class="mt-5 flex justify-end gap-2">
    <button type="button" onclick="adminCloseModal()" class="px-4 py-2 rounded-lg text-sm border">Скасувати</button>
    <button type="submit" class="px-4 py-2 rounded-lg text-sm bg-blue-600 text-white">Зберегти</button>
  </div>
</form>`,
		restaurantID,
		template.HTMLEscapeString(restaurantName),
		template.HTMLEscapeString(number),
		template.HTMLEscapeString(capacity),
	)
}

func tableEditModalBody(id int, number, capacity string) string {
	return fmt.Sprintf(`
<form hx-post="/admin/network/tables/%d" hx-target="#admin-modal-content" hx-swap="innerHTML">
  <div class="space-y-3">
    <label class="block text-xs font-semibold text-slate-600">Номер столика</label>
    <input name="number" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="12" required />
    <label class="block text-xs font-semibold text-slate-600">Місткість</label>
    <input name="capacity" value="%s" class="w-full px-3 py-2 border rounded-lg text-sm" placeholder="4" required />
  </div>
  <div class="mt-5 flex justify-end gap-2">
    <button type="button" onclick="adminCloseModal()" class="px-4 py-2 rounded-lg text-sm border">Скасувати</button>
    <button type="submit" class="px-4 py-2 rounded-lg text-sm bg-blue-600 text-white">Зберегти</button>
  </div>
</form>`,
		id,
		template.HTMLEscapeString(number),
		template.HTMLEscapeString(capacity),
	)
}

func findRestaurantName(restaurants []adminservice.NetworkRestaurant, restaurantID int) string {
	for _, r := range restaurants {
		if r.ID == restaurantID {
			return r.Name
		}
	}
	return ""
}

func roleLabel(role string) string {
	switch strings.ToLower(role) {
	case "waiter":
		return "Офіціант"
	case "chef":
		return "Кухар"
	case "admin":
		return "Адміністратор"
	default:
		return "Працівник"
	}
}

func selectedAttr(selected bool) string {
	if selected {
		return " selected"
	}
	return ""
}
