// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package indexedset provides a generic set that assigns a stable index to each unique key.
package indexedset

// IndexedSet is a set that assigns a stable index to each unique key added to it.
type IndexedSet[K comparable] struct {
	keyMap   map[K]int32
	uniqList []K
}

// New returns a new IndexedSet instance.
func New[K comparable](size int) *IndexedSet[K] {
	return &IndexedSet[K]{
		keyMap:   make(map[K]int32, size),
		uniqList: nil,
	}
}

// Size returns the number of entries in the set.
func (s *IndexedSet[K]) Size() int {
	return len(s.uniqList)
}

// Add adds a key and returns its index in the set.
func (s *IndexedSet[K]) Add(k K) int32 {
	if v, found := s.keyMap[k]; found {
		return v
	}
	v := int32(len(s.uniqList))
	s.keyMap[k] = v
	s.uniqList = append(s.uniqList, k)
	return v
}

// AddSlice adds all the keys from the slice and returns all the corresponding indexes.
func (s *IndexedSet[K]) AddSlice(ks []K) []int32 {
	idxs := make([]int32, 0, len(ks))
	for _, k := range ks {
		idxs = append(idxs, s.Add(k))
	}
	return idxs
}

// UniqueKeys returns all the unique keys in the set, in insertion order.
// It returns a reference which should not be modifed by the caller (unless
// the caller is done with the IndexedSet completely)
func (s *IndexedSet[K]) UniqueKeys() []K {
	return s.uniqList
}

// Subset returns all the keys corresponding to the provided indexes.
func (s *IndexedSet[K]) Subset(indexes []int32) []K {
	sub := make([]K, 0, len(indexes))
	for _, index := range indexes {
		sub = append(sub, s.uniqList[index])
	}
	return sub
}
