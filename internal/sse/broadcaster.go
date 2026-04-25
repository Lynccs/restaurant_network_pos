package sse

import "sync"

// Broadcaster розсилає сигнали оновлення підписникам, згрупованим за restaurantID.
type Broadcaster struct {
	mu      sync.Mutex
	clients map[int][]chan struct{}
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{clients: make(map[int][]chan struct{})}
}

// Subscribe реєструє нового підписника і повертає канал, з якого він читає сигнали.
func (b *Broadcaster) Subscribe(restaurantID int) chan struct{} {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	b.clients[restaurantID] = append(b.clients[restaurantID], ch)
	b.mu.Unlock()
	return ch
}

// Unsubscribe видаляє підписника після закриття SSE-з'єднання.
func (b *Broadcaster) Unsubscribe(restaurantID int, ch chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.clients[restaurantID]
	for i, c := range list {
		if c == ch {
			b.clients[restaurantID] = append(list[:i], list[i+1:]...)
			return
		}
	}
}

// Notify надсилає сигнал усім підписникам ресторану. Не блокується.
func (b *Broadcaster) Notify(restaurantID int) {
	b.mu.Lock()
	list := make([]chan struct{}, len(b.clients[restaurantID]))
	copy(list, b.clients[restaurantID])
	b.mu.Unlock()
	for _, ch := range list {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
