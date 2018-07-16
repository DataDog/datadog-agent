package quantile

import (
	"sync"
)

const (
	defaultBinListSize = 2 * defaultBinLimit
	defaultKeyListSize = 256
)

var (
	// TODO: multiple pools, one for each size class (like github.com/oxtoacart/bpool)
	binListPool = sync.Pool{
		New: func() interface{} {
			a := make([]bin, 0, defaultBinListSize)
			return &a
		},
	}

	keyListPool = sync.Pool{
		New: func() interface{} {
			a := make([]Key, 0, defaultKeyListSize)
			return &a
		},
	}
)

func getBinList() []bin {
	a := *(binListPool.Get().(*[]bin))
	return a[:0]
}

func putBinList(a []bin) {
	binListPool.Put(&a)
}

func getKeyList() []Key {
	a := *(keyListPool.Get().(*[]Key))
	return a[:0]
}

func putKeyList(a []Key) {
	keyListPool.Put(&a)
}
