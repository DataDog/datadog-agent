// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tools

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/mcp/types"
)

// Registry manages MCP tool handlers
type Registry struct {
	handlers map[string]types.Handler
	mu       sync.RWMutex
}

// NewRegistry creates a new tool registry
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]types.Handler),
	}
}

// Register adds a new tool handler to the registry
func (r *Registry) Register(name string, handler types.Handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.handlers[name]; exists {
		return fmt.Errorf("tool %s already registered", name)
	}

	r.handlers[name] = handler
	return nil
}

// GetHandler retrieves a handler by name
func (r *Registry) GetHandler(name string) (types.Handler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handler, exists := r.handlers[name]
	if !exists {
		return nil, fmt.Errorf("tool %s not found", name)
	}

	return handler, nil
}

// List returns all registered tool names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	return names
}

// Unregister removes a tool handler from the registry
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.handlers[name]; !exists {
		return fmt.Errorf("tool %s not found", name)
	}

	delete(r.handlers, name)
	return nil
}
