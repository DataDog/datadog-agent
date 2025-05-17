package debugging

import "sync"

type AtomicString struct {
	mu sync.Mutex
	s  string
}

func (a *AtomicString) Add(str string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.s += str
}

func (a *AtomicString) Get() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.s
}

func (a *AtomicString) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.s = ""
}
