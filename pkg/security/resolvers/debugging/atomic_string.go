package debugging

import (
	"sync"
)

type AtomicString struct {
	mu           sync.Mutex
	s            string
	logToConsole bool
}

func (a *AtomicString) SetLogOn() {
	a.logToConsole = true
}

func (a *AtomicString) SetLogOff() {
	a.logToConsole = false
}

func (a *AtomicString) Add(str string) {
	a.mu.Lock()
	if a.logToConsole {
		//fmt.Println("MNTP", str)
	}
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
