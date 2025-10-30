// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sender

type indexedSet[K comparable] struct {
	keyMap   map[K]int32
	uniqList []K
}

func newIndexedSet[K comparable]() *indexedSet[K] {
	return &indexedSet[K]{
		keyMap:   make(map[K]int32),
		uniqList: make([]K, 0),
	}
}

func (s *indexedSet[K]) Size() int {
	return len(s.uniqList)
}

func (s *indexedSet[K]) Add(k K) int32 {
	if v, found := s.keyMap[k]; found {
		return v
	}
	v := int32(len(s.uniqList))
	s.keyMap[k] = v
	s.uniqList = append(s.uniqList, k)
	return v
}

func (s *indexedSet[K]) AddSlice(ks []K) []int32 {
	idxs := make([]int32, 0, len(ks))
	for _, k := range ks {
		idxs = append(idxs, s.Add(k))
	}
	return idxs
}

func (s *indexedSet[K]) UniqueKeys() []K {
	return s.uniqList
}

func (s *indexedSet[K]) Subset(indexes []int32) []K {
	sub := make([]K, 0, len(indexes))
	for _, index := range indexes {
		sub = append(sub, s.uniqList[index])
	}
	return sub
}
