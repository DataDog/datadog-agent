// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package trietest

// hashSet is a Set implementation using a Go map.
type hashSet struct {
	maxSize int
	m       map[key]struct{}
}

func newHashSet() *hashSet {
	return &hashSet{maxSize: int(maxSize), m: make(map[key]struct{})}
}

func (s *hashSet) insert(k key) (inserted, full bool) {
	if _, exists := s.m[k]; exists {
		return false, false
	}
	if len(s.m) >= s.maxSize {
		return false, true
	}
	s.m[k] = struct{}{}
	return true, false
}

func (s *hashSet) contains(k key) bool {
	_, exists := s.m[k]
	return exists
}

func (s *hashSet) clear() {
	clear(s.m)
}

func (s *hashSet) len() int {
	return len(s.m)
}
