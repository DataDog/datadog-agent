package dedup

import "sync"

// This module provides a simple string set allowing you to de-duplicate strings in memory
// A huge constraint compared to https://github.com/lkarlslund/stringdedup is that you need
// to manually decrease the number of references when you stop using the string to allow it
// to be GCed away.

type StringSet struct {
	strings map[string]*data
	lock sync.RWMutex
}

type data struct {
	ptr *string
	ref uint32
}

func NewStringSet() *StringSet {
	return &StringSet{
		strings: make(map[string]*data),
	}
}

func (ss StringSet) Get(s string) *string {
	ss.lock.Lock()
	defer ss.lock.Unlock()

	d, found := ss.strings[s]
	if !found {
		ss.strings[s] = &data{
			ptr: &s,
			ref: 1,
		}
		return &s
	}
	d.ref++
	return d.ptr
}

func (ss StringSet) Dec(s *string) {
	ss.lock.Lock()
	defer ss.lock.Unlock()
	if s == nil {
		return
	}
	vs := *s
	d, found := ss.strings[vs]
	if found {
		// TODO: do we want to compare the pointer here? If it doesn't match
		//   something is probably wrong.
		d.ref--
		if d.ref <= 0 {
			d.ptr = nil
			delete(ss.strings, vs)
		}
	}
}

func (ss StringSet) Size() int {
	ss.lock.RLock()
	defer ss.lock.RUnlock()
	return len(ss.strings)
}

func (ss StringSet) Clear() {
	ss.lock.Lock()
	defer ss.lock.Unlock()
	for k, v := range ss.strings {
		v.ptr = nil
		delete(ss.strings, k)
	}
}
