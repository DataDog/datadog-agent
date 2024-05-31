// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lookaside implements a fixed-size lookaside list
package lookaside

import (
	"errors"
	"sync"
)

var (
	// ErrLookasideFull is returned when the lookaside list is full
	ErrLookasideFull = errors.New("lookaside list is full")
	// ErrLookasideEmpty is returned when the lookaside list is empty
	ErrLookasideEmpty = errors.New("lookaside list is empty")
)

// Lookaside is a fixed-length lookaside list
type Lookaside[V any] struct {
	list  []V
	index int
	size  int

	lock sync.Mutex
}

// New creates a new lookaside list
func New[V any](size int) (*Lookaside[V], error) {
	return &Lookaside[V]{
		list: make([]V, size),
		size: size,
	}, nil
}

// Put adds an entry to the list
func (l *Lookaside[V]) Put(val V) error {

	l.lock.Lock()
	defer l.lock.Unlock()

	if l.index+1 > l.size {
		return ErrLookasideFull
	}
	l.list[l.index] = val
	l.index++
	return nil
}

// Get retrieves and entry from the list
func (l *Lookaside[V]) Get() (V, error) {
	var result V
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.index == 0 {
		return result, ErrLookasideEmpty
	}
	l.index--
	return l.list[l.index], nil
}
