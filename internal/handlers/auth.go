package handlers

import (
	"errors"
	"net/http"
	"restaurant_network_pos/internal/models"
	"restaurant_network_pos/internal/service"
	"restaurant_network_pos/templates/pages"

	"github.com/gorilla/sessions"
)

type AuthServicer interface {
	Login(phone, pin string) (*models.User, error)
}

type AuthHandler struct {
	Auth  AuthServicer
	Store sessions.Store
}

func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	pages.Login("").Render(r.Context(), w)
}

func (h *AuthHandler) LoginPost(w http.ResponseWriter, r *http.Request) {
	phone := r.FormValue("phone")
	pin := r.FormValue("pin")

	user, err := h.Auth.Login(phone, pin)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			w.WriteHeader(http.StatusUnauthorized)
			pages.Login("Невірний номер телефону або PIN-код.").Render(r.Context(), w)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	sess, _ := h.Store.Get(r, "session")
	sess.Values["user_id"] = user.ID
	sess.Values["role"] = string(user.Role)
	sess.Values["full_name"] = user.FullName
	sess.Values["restaurant_id"] = user.RestaurantID
	sess.Values["phone"] = user.Phone
	if err = h.Store.Save(r, w, sess); err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	switch user.Role {
	case models.RoleAdmin:
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	case models.RoleWaiter:
		http.Redirect(w, r, "/waiter", http.StatusSeeOther)
	case models.RoleChef:
		http.Redirect(w, r, "/chef", http.StatusSeeOther)
	default:
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	sess, _ := h.Store.Get(r, "session")
	sess.Options.MaxAge = -1
	h.Store.Save(r, w, sess)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
