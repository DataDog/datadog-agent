// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sbom

import "testing"

func newTestDataCache(t *testing.T, size int) *dataCache {
	t.Helper()
	c, err := newDataCache(size)
	if err != nil {
		t.Fatalf("newDataCache: %v", err)
	}
	return c
}

// peek returns the latest data the cache holds for key, used or not.
func (c *dataCache) peek(key workloadKey) (*Data, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if u, ok := c.used[key]; ok {
		return u.data, u.data != nil
	}
	return c.unused.Peek(key)
}

// users returns the number of users of key.
func (c *dataCache) users(key workloadKey) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	if u, ok := c.used[key]; ok {
		return u.users
	}
	return 0
}

// TestDataCacheKeepsUsedData checks that used data stays in the cache while
// the LRU of unused data turns over.
func TestDataCacheKeepsUsedData(t *testing.T) {
	c := newTestDataCache(t, 1)
	x := &Data{}

	c.add("x", x)
	c.add("y", &Data{})
	c.release("y")
	c.add("z", &Data{})
	c.release("z")

	if data, ok := c.acquire("x"); !ok || data != x {
		t.Errorf("acquire(x) = %p, %v, want the used data, %p", data, ok, x)
	}
	if _, ok := c.acquire("y"); ok {
		t.Errorf("acquire(y) found the unused data the LRU evicted")
	}
}

// TestDataCacheReleasesToLRU checks that data stays used until its last user
// releases it, and then waits in the LRU for the next container of its key.
func TestDataCacheReleasesToLRU(t *testing.T) {
	c := newTestDataCache(t, 1)
	x := &Data{}

	c.add("x", x)
	if data, ok := c.acquire("x"); !ok || data != x {
		t.Fatalf("acquire(x) = %p, %v, want %p", data, ok, x)
	}

	c.release("x")
	if used, unused := c.lens(); c.users("x") != 1 || used != 1 || unused != 0 {
		t.Fatalf("after one release: %d users, %d used and %d unused keys, want x used once", c.users("x"), used, unused)
	}

	c.release("x")
	if used, unused := c.lens(); used != 0 || unused != 1 {
		t.Fatalf("after the last release: %d used and %d unused keys, want x unused", used, unused)
	}
	if data, ok := c.acquire("x"); !ok || data != x {
		t.Errorf("acquire(x) = %p, %v, want the unused data, %p", data, ok, x)
	}
}

// TestDataCacheCountsUsersAcrossRefresh checks that containers still on the data
// from before a refresh keep its key used once the refreshing container leaves.
func TestDataCacheCountsUsersAcrossRefresh(t *testing.T) {
	c := newTestDataCache(t, 1)
	old, refreshed := &Data{}, &Data{}

	c.add("x", old) // a scans
	c.acquire("x")  // b shares the scan of a
	c.remove("x")   // a refreshes
	c.add("x", refreshed)
	c.release("x") // a gives up its old data

	c.release("x") // a leaves, b runs on
	if data, ok := c.peek("x"); !ok || data != refreshed || c.users("x") != 1 {
		t.Errorf("x holds %p with %d users, want the refreshed data, %p, used by b", data, c.users("x"), refreshed)
	}
}

// TestDataCacheRemoveMakesScanAgain checks that once a key is removed its next
// user scans again, rather than taking the data from before the removal.
func TestDataCacheRemoveMakesScanAgain(t *testing.T) {
	c := newTestDataCache(t, 1)
	c.add("x", &Data{})
	c.remove("x")

	if _, ok := c.acquire("x"); ok {
		t.Errorf("acquire(x) found data after its removal")
	}
}
