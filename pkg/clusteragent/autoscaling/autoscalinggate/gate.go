// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package autoscalinggate provides a gate used to defer the start of pod
// collection in workloadmeta for users that have workload autoscaling enabled
// but no autoscalers configured yet.
//
// Pod collection can use a lot of memory on large clusters, and we'd like to
// avoid that until it's needed.
//
// The gate is opened the first time the autoscaling stack observes a
// DatadogPodAutoscaler, regardless of how it was created (Kubernetes, remote
// config, profile-driven, etc.).
package autoscalinggate

import (
	"context"
	"sync"
)

// Gate coordinates the lazy startup of pod collection for workload autoscaling.
type Gate struct {
	enableCh   chan struct{}
	enableOnce sync.Once
	syncedCh   chan struct{}
	syncedOnce sync.Once
}

// New returns a new Gate.
func New() *Gate {
	return &Gate{
		enableCh: make(chan struct{}),
		syncedCh: make(chan struct{}),
	}
}

// Enable opens the gate. To be called when the autoscaling stack observes a
// DatadogPodAutoscaler. Only the first call has any effect.
func (g *Gate) Enable() {
	g.enableOnce.Do(func() { close(g.enableCh) })
}

// MarkPodCollectionSynced marks the pod collection as synced. Only the first
// call has any effect.
func (g *Gate) MarkPodCollectionSynced() {
	g.syncedOnce.Do(func() { close(g.syncedCh) })
}

// WaitForEnable blocks until Enable is called or ctx is cancelled. Returns true
// if Enable was called, false if ctx was cancelled first.
func (g *Gate) WaitForEnable(ctx context.Context) bool {
	select {
	case <-g.enableCh:
		return true
	case <-ctx.Done():
		return false
	}
}

// WaitForPodCollectionSynced blocks until MarkPodCollectionSynced is called or
// ctx is cancelled. Returns true if MarkPodCollectionSynced was called, false
// if ctx was cancelled first.
func (g *Gate) WaitForPodCollectionSynced(ctx context.Context) bool {
	select {
	case <-g.syncedCh:
		return true
	case <-ctx.Done():
		return false
	}
}
