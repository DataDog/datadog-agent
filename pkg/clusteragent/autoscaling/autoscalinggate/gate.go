// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package autoscalinggate provides a gate used to coordinate the lazy startup
// of the workload autoscaling stack.
//
// The goal of this gate is to enable autoscaling by default to make onboarding
// easier, but we don't want to use any resources for users that are not using
// autoscaling. Some of the dependencies of autoscaling can use a lot of
// memory. In particular, the pod collection in workloadmeta can use a lot of
// memory in large clusters.
//
// To achieve the goal, we use this gate to only start the autoscaling stack
// when there's at least one DatadogPodAutoscaler or a workload or namespace
// with an autoscaling label.
package autoscalinggate

import (
	"context"
	"sync"
)

// Gate coordinates the lazy startup of the workload autoscaling stack.
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

// Enable marks autoscaling as enabled. To be called when there's a
// DatadogPodAutoscaler or a workload or namespace with autoscaling labels. Only
// the first call has any effect.
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
