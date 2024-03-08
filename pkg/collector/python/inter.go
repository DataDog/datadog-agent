package python

import "sync"

type stringInterner struct {
	m map[string]string
}

func newStringInterner() *stringInterner {
	return &stringInterner{m: make(map[string]string)}
}

func (si *stringInterner) intern(s []byte) string {
	if interned, ok := si.m[string(s)]; ok {
		return interned
	}
	ss := string(s)
	si.m[ss] = ss
	return ss
}

func newStringInternerPool(maxStrings int) *stringInternerPool {
	return &stringInternerPool{
		maxStrings: maxStrings,
		p: &sync.Pool{
			New: func() interface{} {
				return newStringInterner()
			},
		},
	}
}

type stringInternerPool struct {
	maxStrings int
	p          *sync.Pool
}

func (s stringInternerPool) Get() *stringInterner {
	return s.p.Get().(*stringInterner)
}

func (s stringInternerPool) Put(si *stringInterner) {
	if len(si.m) > s.maxStrings {
		si.m = make(map[string]string)
	}
	s.p.Put(si)
}

var sharedStringInternerPool = newStringInternerPool(1000)
