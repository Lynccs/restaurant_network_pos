package middleware

import (
	"net/http"

	"github.com/gorilla/sessions"
)

func RequireAuth(store sessions.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, _ := store.Get(r, "session")
			if sess.Values["user_id"] == nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

var roleHome = map[string]string{
	"admin":  "/admin",
	"waiter": "/waiter",
	"chef":   "/chef",
}

func RequireRole(store sessions.Store, role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, _ := store.Get(r, "session")
			userRole, _ := sess.Values["role"].(string)
			if userRole != role {
				if home, ok := roleHome[userRole]; ok {
					http.Redirect(w, r, home, http.StatusSeeOther)
				} else {
					http.Redirect(w, r, "/login", http.StatusSeeOther)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
