// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package credentials

import "sync"

type store interface {
	get(key string) (string, error)
}

type cachingStore struct {
	l     sync.Mutex
	cache map[string]string
	s     store
}

func newCachingStore(s store) store {
	return &cachingStore{
		cache: make(map[string]string),
		s:     s,
	}
}

func (s *cachingStore) get(key string) (string, error) {
	s.l.Lock()
	defer s.l.Unlock()

	value, found := s.cache[key]
	if found {
		return value, nil
	}

	var err error
	value, err = s.s.get(key)
	if err != nil {
		return "", err
	}

	s.cache[key] = value
	return value, nil
}
