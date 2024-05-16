// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"time"
)

// MockTimeNow mocks time.Now
var MockTimeNow = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2000-01-01 00:00:00"
	t, _ := time.Parse(layout, str)
	return t
}

// MockCacher is a fake cacher used for testing purposes to avoid using the persistent cache and have conflicts between tests
type MockCacher struct {
	cache map[string]string
}

// NewMockCacher returns a new MockCacher
func NewMockCacher() *MockCacher {
	return &MockCacher{
		cache: make(map[string]string),
	}
}

func (c *MockCacher) Read(key string) (string, error) {
	return c.cache[key], nil
}

func (c *MockCacher) Write(key string, value string) error {
	c.cache[key] = value
	return nil
}
