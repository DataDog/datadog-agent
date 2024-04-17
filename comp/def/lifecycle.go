// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compDef

import (
	"context"
)

type lchFunc func(context.Context) error

// Hook represents a function pair for component startup and shutdown
type Hook struct {
	OnStart lchFunc
	OnStop  lchFunc
}

// HookStorage keeps track of Hooks, should not be used directly by client code
type HookStorage struct {
	hooks []Hook
}

// Lifecycle should be added to a component's requires struct if it wants to add hooks
type Lifecycle struct {
	storage *HookStorage
}

// SetStorage assigns HookStorage to a Lifecycle
func (lc *Lifecycle) SetStorage(hs *HookStorage) {
	lc.storage = hs
}

// Append adds a Hook to the Lifecycle
func (lc *Lifecycle) Append(h Hook) {
	lc.storage.hooks = append(lc.storage.hooks, h)
}

// Hooks returns the set of assigned hooks
func (lc *Lifecycle) Hooks() []Hook {
	return lc.storage.hooks
}
