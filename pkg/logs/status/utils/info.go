// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils provides status information utilities
package utils

import (
	"fmt"
	"sync"

	"go.uber.org/atomic"
)

// InfoProvider is a general interface to hold and render info for the status page.
//
// When implementing InfoProvider - be aware of the 2 ways it is used by the status page:
//
//  1. when a single message is returned, the status page will display a single line:
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
	lock     sync.Mutex
	key      string
	messages map[string]string
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
	m.lock.Lock()
	defer m.lock.Unlock()
	m.messages[key] = message
}

// RemoveMessage removes a message with a unique key
func (m *MappedInfo) RemoveMessage(key string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.messages, key)
}

// InfoKey returns the key
func (m *MappedInfo) InfoKey() string {
	return m.key
}

// Info returns the info
func (m *MappedInfo) Info() []string {
	m.lock.Lock()
	defer m.lock.Unlock()
	info := []string{}
	for _, v := range m.messages {
		info = append(info, v)
	}
	return info
}

// InfoRegistry keeps track of info providers
type InfoRegistry struct {
	lock sync.Mutex
	info map[string]InfoProvider
}

// NewInfoRegistry creates a new InfoRegistry instance
func NewInfoRegistry() *InfoRegistry {
	return &InfoRegistry{
		info: make(map[string]InfoProvider),
	}
}

// Register adds an info provider
func (i *InfoRegistry) Register(info InfoProvider) {
	i.lock.Lock()
	defer i.lock.Unlock()
	key := info.InfoKey()
	i.info[key] = info
}

// Get returns the provider for a given key, or nil
func (i *InfoRegistry) Get(key string) InfoProvider {
	i.lock.Lock()
	defer i.lock.Unlock()
	return i.info[key]
}

// All returns all registered info providers.
func (i *InfoRegistry) All() []InfoProvider {
	i.lock.Lock()
	defer i.lock.Unlock()
	info := make([]InfoProvider, 0, len(i.info))
	for _, v := range i.info {
		info = append(info, v)
	}

	return info
}

// Rendered renders the info for display on the status page.
func (i *InfoRegistry) Rendered() map[string][]string {
	i.lock.Lock()
	defer i.lock.Unlock()
	info := make(map[string][]string)

	for _, v := range i.info {
		if len(v.Info()) == 0 {
			continue
		}
		info[v.InfoKey()] = v.Info()
	}
	return info
}
