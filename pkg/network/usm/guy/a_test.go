package guy

import (
	"fmt"
	"go.uber.org/atomic"
	"sync"
	"testing"
)

func BenchmarkMutex(b *testing.B) {
	m := sync.RWMutex{}
	list := []string{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RLock()
		for i := range list {
			fmt.Println(i)
		}
		m.RUnlock()
	}
}

func BenchmarkAtomicDisabled(b *testing.B) {
	m := sync.RWMutex{}
	boo := atomic.Bool{}

	list := []string{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if boo.Load() {
			m.RLock()
			for i := range list {
				fmt.Println(i)
			}
			m.RUnlock()
		}
	}
}

func BenchmarkAtomicEnabled(b *testing.B) {
	m := sync.RWMutex{}
	boo := atomic.Bool{}
	boo.Store(true)

	list := []string{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if boo.Load() {
			m.RLock()
			for i := range list {
				fmt.Println(i)
			}
			m.RUnlock()
		}
	}
}
