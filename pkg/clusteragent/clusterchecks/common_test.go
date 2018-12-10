// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type lockable interface {
	Lock()
	Unlock()
}

// requireNotLocked tries to lock a clusterStore to
// make sure it is not left locked after an operation
func requireNotLocked(t *testing.T, s *clusterStore) bool {
	t.Helper()
	if isLocked(s) {
		assert.FailNow(t, "clusterStore object is locked")
		return false
	}
	s.RLock()
	defer s.RUnlock()

	for node, store := range s.nodes {
		if isLocked(store) {
			assert.FailNowf(t, "nodeStore %s is locked", node)
			return false
		}
	}
	return true
}

func isLocked(l lockable) bool {
	ok := make(chan struct{}, 1)
	go func() {
		l.Lock()
		l.Unlock()
		ok <- struct{}{}
	}()
	select {
	case <-ok:
		return false
	case <-time.After(1 * time.Millisecond):
		return true
	}
}

func TestNotLocked(t *testing.T) {
	var m sync.Mutex
	assert.False(t, isLocked(&m))
}

func TestLocked(t *testing.T) {
	var m sync.Mutex
	m.Lock()

	assert.True(t, isLocked(&m))
}

func TestStoreNotLocked(t *testing.T) {
	s := newClusterStore()
	requireNotLocked(t, s)
}
