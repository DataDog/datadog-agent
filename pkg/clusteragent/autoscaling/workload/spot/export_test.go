// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/clock"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// NewTestScheduler creates a Scheduler for testing.
func NewTestScheduler(config Config, clk clock.WithTicker, wlm workloadmeta.Component, evictPod func(namespace, name string) error) *Scheduler {
	isLeader := func() bool {
		return true
	}
	evictorFunc := podEvictorFunc(func(_ context.Context, namespace, name string) error {
		return evictPod(namespace, name)
	})
	return newScheduler(config, clk, wlm, evictorFunc, noopFallbackStore{}, isLeader)
}

// TrackedCounts returns the total and spot tracked pod counts (including in-flight admissions) for the given owner.
func (s *Scheduler) TrackedCounts(namespace, kind, name string) (total, spot int) {
	s.tracker.mu.RLock()
	defer s.tracker.mu.RUnlock()
	owner := ownerKey{Namespace: namespace, Kind: kind, Name: name}
	if pods, ok := s.tracker.podsPerOwner[owner]; ok {
		return pods.totalCount(), pods.spotCount()
	}
	return 0, 0
}

// WaitSubscribed returns a channel that is closed once Run has subscribed to workloadmeta events.
func (s *Scheduler) WaitSubscribed() <-chan struct{} {
	return s.subscribed
}

// Config returns the scheduler configuration.
func (s *Scheduler) Config() Config {
	return s.config
}

// IsSpotSchedulingDisabled returns true if spot scheduling is disabled and a timestamp until it is disabled.
func (s *Scheduler) IsSpotSchedulingDisabled() (time.Time, bool) {
	return s.isSpotSchedulingDisabled()
}

// IsSpotAssigned reports whether the pod is assigned to a spot instance.
func IsSpotAssigned(pod *workloadmeta.KubernetesPod) bool {
	return isSpotAssigned(pod)
}

// podEvictorFunc is a function type implementing podEvictor for testing.
type podEvictorFunc func(ctx context.Context, namespace, name string) error

func (f podEvictorFunc) evictPod(ctx context.Context, namespace, name string, _ corev1.PodPhase) error {
	return f(ctx, namespace, name)
}

// noopFallbackStore is a test-only fallbackStore.
type noopFallbackStore struct{}

func (noopFallbackStore) store(context.Context, time.Time) error { return nil }

func (noopFallbackStore) read(context.Context) (time.Time, error) { return time.Time{}, nil }
