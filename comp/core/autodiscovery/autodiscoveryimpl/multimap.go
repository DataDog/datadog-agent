// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

// a multimap is a string/string map that can contain multiple values for a
// single key.  Duplicate values are allowed (but not used in this package).
type multimap struct {
	data map[string][]string
}

// newMultimap creates a new multimap
func newMultimap() multimap {
	return multimap{data: map[string][]string{}}
}

// insert adds an item into a multimap
func (m *multimap) insert(k, v string) {
	var slice []string
	if existing, found := m.data[k]; found {
		slice = existing
	} else {
		slice = []string{}
	}
	m.data[k] = append(slice, v)
}

// remove removes an item from a multimap
func (m *multimap) remove(k, v string) {
	if values, found := m.data[k]; found {
		for i, u := range values {
			if u == v {
				// remove index i from the slice
				values[i] = values[len(values)-1]
				values = values[:len(values)-1]
				break
			}
		}
		if len(values) > 0 {
			m.data[k] = values
		} else {
			delete(m.data, k)
		}
	}
}

// get gets the set of items with the given key.  The returned slice must not
// be modified, and is only valid until the next multimap operation.  This
// will always return a valid slice, not nil.
func (m *multimap) get(k string) []string {
	if values, found := m.data[k]; found {
		return values
	}
	return []string{}
}
