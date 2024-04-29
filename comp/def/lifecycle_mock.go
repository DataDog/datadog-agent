// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package compdef defines basic types used for components
package compdef

import (
	"context"
)

// TestLifecycle is a testing spy for fx.Lifecycle
type TestLifecycle struct {
	hooks []Hook
}

// NewTestLifecycle returns a lifecycle for testing
func NewTestLifecycle() *TestLifecycle {
	return &TestLifecycle{}
}

// Append adds a hook to the lifecycle
func (t *TestLifecycle) Append(h Hook) {
	t.hooks = append(t.hooks, h)
}

// Start executes all registered OnStart hooks in order, halting at the first
func (t *TestLifecycle) Start(ctx context.Context) error {
	for _, h := range t.hooks {
		err := h.OnStart(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

// Stop executes all registered OnStop hooks in reverse order, halting at the first
func (t *TestLifecycle) Stop(ctx context.Context) error {
	for _, h := range t.hooks {
		err := h.OnStop(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}
