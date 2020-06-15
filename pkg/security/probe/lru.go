package probe

import (
	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/hashicorp/golang-lru/simplelru"
)

type LRUKTable struct {
	table eprobe.Table
	lru   *simplelru.LRU
}

func (l *LRUKTable) Get(key []byte) ([]byte, error) {
	if value, exists := l.lru.Get(string(key)); exists {
		return value.([]byte), nil
	}

	// fallback to real table in case of not handled by LRU
	return l.table.Get(key)
}

func (l *LRUKTable) Set(key []byte, value []byte) {
	l.table.Set(key, value)
	l.lru.Add(string(key), value)
}

func (l *LRUKTable) Delete(key []byte) error {
	l.lru.Remove(string(key))
	return nil
}

func NewLRUKTable(table eprobe.Table, size int) (*LRUKTable, error) {
	lru, err := simplelru.NewLRU(size, func(key interface{}, value interface{}) {
		table.Delete([]byte(key.(string)))
	})

	if err != nil {
		return nil, err
	}

	return &LRUKTable{
		table: table,
		lru:   lru,
	}, nil
}
