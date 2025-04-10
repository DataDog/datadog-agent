// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
)

type blocklist struct {
	data        *atomic.Pointer[[]string]
	matchPrefix bool
}

func newBlocklist(data []string, matchPrefix bool) blocklist {
	data = slices.Clone(data)
	sort.Strings(data)

	if matchPrefix && len(data) > 0 {
		// Make sure that elements identify unique prefixes.
		i := 0
		for j := 1; j < len(data); j++ {
			if strings.HasPrefix(data[j], data[i]) {
				continue
			}
			i++
			data[i] = data[j]
		}

		data = data[:i+1]
	}

	// Invariants for data:
	// For all i, j such that i < j, data[i] < data[j].
	// for all i, j such that i != j, !HasPrefix(data[i], data[j]).

	ptr := atomic.Pointer[[]string]{}
	ptr.Store(&data)

	return blocklist{
		data:        &ptr,
		matchPrefix: matchPrefix,
	}
}

// update the blocklsit, values won't be modified.
func (b *blocklist) update(values []string) {
	data := slices.Clone(values)
	sort.Strings(data)
	b.data.Store(&data)
}

// test returns true if the given name is part of the blocklist.
func (b *blocklist) test(name string) bool {
	if b.data == nil {
		return false
	}

	// atomically access the blocklist
	blist := *(b.data.Load())

	i := sort.SearchStrings(blist, name)

	// SearchStrings returns an index such that either:
	// - blist[i] == name
	// - blist[i-1] < name (if i > 0) && blist[i] > name (if i < len(b.data))
	//
	// If for some j, blist[j] is a prefix of name, then:
	//
	// - j < i, because any prefix of a string is less than string itself,
	//
	// - if j < i - 1, then strings in range [j+1, i-1] would have
	// blist[j] as a prefix, which is impossible by construction of
	// blist.
	//
	// Thus j must be i - 1.
	if b.matchPrefix && i > 0 && strings.HasPrefix(name, blist[i-1]) {
		return true
	}
	if i < len(blist) {
		return name == blist[i]
	}

	fmt.Println("end")
	return false
}
