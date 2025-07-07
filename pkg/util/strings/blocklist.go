// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package strings

import (
	"slices"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// Blocklist is a strings blocklist.
// See `NewBlocklist` for details.
type Blocklist struct {
	data        []string
	matchPrefix bool
	tlmCounter  telemetry.SimpleCounter
}

// NewBlocklist creates a new strings blocklist.
// Use `matchPrefix` to  create a prefixes blocklist.
// Optional `tlmCounter` to count every time a string is matching.
func NewBlocklist(data []string, matchPrefix bool, tlmCounter telemetry.SimpleCounter) Blocklist {
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
	return Blocklist{
		data:        data,
		matchPrefix: matchPrefix,
		tlmCounter:  tlmCounter,
	}
}

// Test returns true if the given name is in the blocklist
// or matching by prefix if the string blocklist has been
// created with `matchPrefix`.
func (b *Blocklist) Test(name string) bool {
	if b == nil {
		return false
	}

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
		if b.tlmCounter != nil {
			b.tlmCounter.Inc()
		}
		return true
	}
	if i < len(b.data) {
		if name == b.data[i] {
			if b.tlmCounter != nil {
				b.tlmCounter.Inc()
			}
			return true
		}
		return false
	}

	return false
}
