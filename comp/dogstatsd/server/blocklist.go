// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"sort"
	"strings"
)

type blocklist struct {
	data        []string
	matchPrefix bool
}

func newBlocklist(data []string, matchPrefix bool) blocklist {
	data = append([]string{}, data...)
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

	return blocklist{
		data:        data,
		matchPrefix: matchPrefix,
	}
}

func (b *blocklist) test(name string) bool {
	if len(b.data) == 0 {
		return false
	}

	i := sort.SearchStrings(b.data, name)

	// SearchStrings returns an index such that either:
	// - data[i] == name
	// - data[i-1] < name (if i > 0) && data[i] > name (if i < len(b.data))
	//
	// If for some j, data[j] is a prefix of name, then:
	//
	// - j < i, because any prefix of a string is less than string itself,
	//
	// - if j < i - 1, then strings in range [j+1, i-1] would have
	// data[j] as a prefix, which is impossible by construction of
	// data.
	//
	// Thus j must be i - 1.
	if b.matchPrefix && i > 0 && strings.HasPrefix(name, b.data[i-1]) {
		return true
	}
	if i < len(b.data) {
		return name == b.data[i]
	}

	return false
}
