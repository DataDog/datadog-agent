// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package compdef defines basic types used for components
package compdef

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLifecycle is a testing spy for fx.Lifecycle
type TestLifecycle struct {
	hooks []Hook
	t     *testing.T
}

// NewTestLifecycle returns a lifecycle for testing
func NewTestLifecycle(t *testing.T) *TestLifecycle {
	return &TestLifecycle{t: t}
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

// AssertHooksNumber asserts that the TestLifecycle contains the given number of hooks
func (t *TestLifecycle) AssertHooksNumber(expectedNumber int) {
	assert.Len(t.t, t.hooks, expectedNumber, "Wrong number of expected hooks")
}
