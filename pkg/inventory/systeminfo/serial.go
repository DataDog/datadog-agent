// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package systeminfo

import "sync"

var cachedSerialNumber serialNumberCache

// GetSerialNumber collects the local hardware serial and caches a non-empty
// result for the process lifetime. Errors and empty results are not cached, so
// callers can retry on a later collection cycle. Collect still reads fresh data.
func GetSerialNumber() (string, error) {
	return cachedSerialNumber.get(Collect)
}

type serialNumberCache struct {
	mu    sync.Mutex
	value string
}

func (c *serialNumberCache) get(collect func() (*SystemInfo, error)) (string, error) {
	c.mu.Lock()
	serial := c.value
	c.mu.Unlock()
	if serial != "" {
		return serial, nil
	}

	// Concurrent initial misses may collect independently. Keep hardware queries
	// outside the lock so a slow query cannot block the other callers.
	info, err := collect()
	if err != nil || info == nil || info.SerialNumber == "" {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.value == "" {
		c.value = info.SerialNumber
	}
	return c.value, nil
}
