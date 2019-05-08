// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package common

// StringSet represents a list of uniq strings
type StringSet map[string]struct{}

// NewStringSet returns as new StringSet initialized with initItems
func NewStringSet(initItems ...string) StringSet {
	newSet := StringSet{}
	for _, item := range initItems {
		newSet.Add(item)
	}
	return newSet
}

// Add adds a item to the set
func (s StringSet) Add(item string) {
	s[item] = struct{}{}
}

// GetAll returns all the strings from the set
func (s StringSet) GetAll() []string {
	res := []string{}
	for item := range s {
		res = append(res, item)
	}
	return res
}
