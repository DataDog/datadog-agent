// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import "sync"

const maxInternerStrings = 1000

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

var sharedInternerMu sync.Mutex
var sharedInterner = newStringInterner()

func acquireInterner() (*stringInterner, func()) {
	sharedInternerMu.Lock()
	return sharedInterner, func() {
		if len(sharedInterner.m) > maxInternerStrings {
			sharedInterner = newStringInterner()
		}
		sharedInternerMu.Unlock()
	}
}
