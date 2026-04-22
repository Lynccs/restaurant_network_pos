package middleware

import (
	"net/http"
	"sync"

	"github.com/gorilla/sessions"
)

type inflightKey struct {
	userID   int
	endpoint string
}

type InflightGuard struct {
	mu    sync.Mutex
	keys  map[inflightKey]struct{}
	store sessions.Store
}

func NewInflightGuard(store sessions.Store) *InflightGuard {
	return &InflightGuard{keys: make(map[inflightKey]struct{}), store: store}
}

func (g *InflightGuard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		sess, _ := g.store.Get(r, "session")
		userID, _ := sess.Values["user_id"].(int)
		if userID == 0 {
			next.ServeHTTP(w, r)
			return
		}

		key := inflightKey{userID: userID, endpoint: r.URL.Path}

		g.mu.Lock()
		if _, busy := g.keys[key]; busy {
			g.mu.Unlock()
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		g.keys[key] = struct{}{}
		g.mu.Unlock()

		defer func() {
			g.mu.Lock()
			delete(g.keys, key)
			g.mu.Unlock()
		}()

		next.ServeHTTP(w, r)
	})
}
