// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import "sync"

const maxInternerStrings = 4096

type stringInterner struct {
	curr map[string]string
	prev map[string]string
}

func newStringInterner() *stringInterner {
	return &stringInterner{
		curr: make(map[string]string),
		prev: make(map[string]string),
	}
}

func (si *stringInterner) rotate() {
	clear(si.prev)
	si.prev, si.curr = si.curr, si.prev
}

func (si *stringInterner) intern(s []byte) string {
	if interned, ok := si.curr[string(s)]; ok {
		return interned
	}
	if interned, ok := si.prev[string(s)]; ok {
		if len(si.curr) >= maxInternerStrings {
			si.rotate()
		}
		si.curr[interned] = interned
		return interned
	}
	if len(si.curr) >= maxInternerStrings {
		si.rotate()
	}
	ss := string(s)
	si.curr[ss] = ss
	return ss
}

var internerPool = sync.Pool{
	New: func() any {
		return newStringInterner()
	},
}

func getInterner() *stringInterner {
	return internerPool.Get().(*stringInterner)
}

func putInterner(si *stringInterner) {
	internerPool.Put(si)
}
