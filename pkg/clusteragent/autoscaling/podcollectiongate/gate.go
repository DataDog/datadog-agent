// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package podcollectiongate provides a gate to enable pod collection in
// workloadmeta. Collecting all pods in a large cluster can use a lot of
// memory, so we only start once a feature needs them.
// For autoscaling, that means waiting until the cluster has at least one
// DatadogPodAutoscaler object.
package podcollectiongate

import (
	"context"
	"sync"
)

// Gate implements a gate to enable pod collection.
type Gate struct {
	ch   chan struct{}
	once sync.Once
}

// New returns a new Gate.
func New() *Gate {
	return &Gate{ch: make(chan struct{})}
}

// Enable marks the gate as enabled. Only the first call has any effect.
func (g *Gate) Enable() {
	g.once.Do(func() { close(g.ch) })
}

// WaitForEnable blocks until the gate is enabled or ctx is cancelled.
// Returns true if the gate was enabled, false if the context was cancelled
// first.
func (g *Gate) WaitForEnable(ctx context.Context) bool {
	select {
	case <-g.ch:
		return true
	case <-ctx.Done():
		return false
	}
}
