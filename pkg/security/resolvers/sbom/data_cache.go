// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sbom

import (
	"sync"

	"github.com/hashicorp/golang-lru/v2/simplelru"
)

// dataCache holds the scan data of each workload key. The data of a key stays
// while SBOMs of the key run, and unused data waits in an LRU.
type dataCache struct {
	mu     sync.Mutex
	used   map[workloadKey]*usedData
	unused *simplelru.LRU[workloadKey, *Data]
}

// usedData holds the latest data of a key and counts the SBOMs of the key that
// hold data from the cache, whatever its generation.
type usedData struct {
	data  *Data
	users int
}

func newDataCache(size int) (*dataCache, error) {
	unused, err := simplelru.NewLRU[workloadKey, *Data](size, nil)
	if err != nil {
		return nil, err
	}
	return &dataCache{used: make(map[workloadKey]*usedData), unused: unused}, nil
}

// acquire returns the data of key for one more user, if the cache holds it.
func (c *dataCache) acquire(key workloadKey) (*Data, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if u, ok := c.used[key]; ok {
		if u.data == nil {
			return nil, false
		}
		u.users++
		return u.data, true
	}
	data, ok := c.unused.Peek(key)
	if !ok {
		return nil, false
	}
	c.unused.Remove(key)
	c.used[key] = &usedData{data: data, users: 1}
	return data, true
}

// add stores data, scanned for one more user, as the latest data of key.
func (c *dataCache) add(key workloadKey, data *Data) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.unused.Remove(key)
	u, ok := c.used[key]
	if !ok {
		u = &usedData{}
		c.used[key] = u
	}
	u.data = data
	u.users++
}

// release ends the use of the data of key by one user, and moves the latest
// data of a key with no user left to the LRU of unused data.
func (c *dataCache) release(key workloadKey) {
	c.mu.Lock()
	defer c.mu.Unlock()

	u, ok := c.used[key]
	if !ok {
		return
	}
	u.users--
	if u.users > 0 {
		return
	}
	delete(c.used, key)
	if u.data != nil {
		c.unused.Add(key, u.data)
	}
}

// remove drops the data of key, so that its next user scans again. Its users
// keep the data they hold until they release it.
func (c *dataCache) remove(key workloadKey) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if u, ok := c.used[key]; ok {
		u.data = nil
	}
	c.unused.Remove(key)
}

// lens returns the number of keys whose data SBOMs use, and of keys whose data
// waits unused.
func (c *dataCache) lens() (used, unused int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.used), c.unused.Len()
}
