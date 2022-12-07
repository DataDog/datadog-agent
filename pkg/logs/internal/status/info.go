// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"fmt"
	"sync"

	"go.uber.org/atomic"
)

// InfoProvider is a general interface to provide info about a log source.
// It is used in the agent status page. The expected usage is for a piece of code that
// wants to surface something on the status page register an info provider with the
// source with a unique key/name. This file contains useful base implementations, but
// InfoProvider can be extended/implemented for more complex data.
//
// When implementing InfoProvider - be aware of the 2 ways it is used by the status page:
//
//  1. when a single message is returned, the statuspage will display a single line:
//     InfoKey(): Info()[0]
//
//  2. when multiple messages are returned, the status page will display an indented list:
//     InfoKey():
//     Info()[0]
//     Info()[1]
//     Info()[n]
//
// InfoKey only needs to be unique per source, and should be human readable.
type InfoProvider interface {
	InfoKey() string
	Info() []string
}

// CountInfo records a simple count
type CountInfo struct {
	count *atomic.Int64
	key   string
}

// NewCountInfo creates a new CountInfo instance
func NewCountInfo(key string) *CountInfo {
	return &CountInfo{
		count: atomic.NewInt64(0),
		key:   key,
	}
}

// Add a new value to the count
func (c *CountInfo) Add(v int64) {
	c.count.Add(v)
}

// Get the underlying value of the count
func (c *CountInfo) Get() int64 {
	return c.count.Load()
}

// InfoKey returns the key
func (c *CountInfo) InfoKey() string {
	return c.key
}

// Info returns the info
func (c *CountInfo) Info() []string {
	return []string{fmt.Sprintf("%d", c.count.Load())}
}

// MappedInfo collects multiple info messages with a unique key
type MappedInfo struct {
	key      string
	messages map[string]string
	lock     sync.Mutex
}

// NewMappedInfo creates a new MappedInfo instance
func NewMappedInfo(key string) *MappedInfo {
	return &MappedInfo{
		key:      key,
		messages: make(map[string]string),
	}
}

// SetMessage sets a message with a unique key
func (m *MappedInfo) SetMessage(key string, message string) {
	defer m.lock.Unlock()
	m.lock.Lock()
	m.messages[key] = message
}

// RemoveMessage removes a message with a unique key
func (m *MappedInfo) RemoveMessage(key string) {
	defer m.lock.Unlock()
	m.lock.Lock()
	delete(m.messages, key)
}

// InfoKey returns the key
func (m *MappedInfo) InfoKey() string {
	return m.key
}

// Info returns the info
func (m *MappedInfo) Info() []string {
	defer m.lock.Unlock()
	m.lock.Lock()
	info := []string{}
	for _, v := range m.messages {
		info = append(info, v)
	}
	return info
}
