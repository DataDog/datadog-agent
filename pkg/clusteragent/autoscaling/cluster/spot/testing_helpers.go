// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package spot

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/clock"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// TestScheduler is an alias for the unexported scheduler type, exposed for testing.
type TestScheduler = scheduler

// NewTestScheduler creates a scheduler for testing.
func NewTestScheduler(config Config, clk clock.WithTicker, wlm workloadmeta.Component, evictPod func(namespace, name string) error, dynamicClient dynamic.Interface) *TestScheduler {
	isLeader := func() bool {
		return true
	}
	evictorFunc := podEvictorFunc(func(_ context.Context, namespace, name string) error {
		return evictPod(namespace, name)
	})
	patcherFunc := workloadPatcherFunc(func(context.Context, objectRef, time.Time) error {
		return nil
	})
	return newScheduler(config, clk, wlm, evictorFunc, patcherFunc, dynamicClient, newWLMPodLister(wlm), isLeader)
}

// TrackedCounts returns the total and spot tracked pod counts (including in-flight admissions) for the given workload.
func (s *TestScheduler) TrackedCounts(group, kind, namespace, name string) (total, spot int) {
	s.tracker.mu.RLock()
	defer s.tracker.mu.RUnlock()
	w := objectRef{Group: group, Kind: kind, Namespace: namespace, Name: name}
	owners, ok := s.tracker.podSets[w]
	if !ok {
		return 0, 0
	}
	for _, ps := range owners {
		total += ps.totalCount()
		spot += ps.spotCount()
	}
	return total, spot
}

// WaitSynced blocks until the workload config store is synced and the scheduler has subscribed to workloadmeta events.
func (s *TestScheduler) WaitSynced() {
	<-s.synced
}

// Config returns the scheduler configuration.
func (s *TestScheduler) Config() Config {
	return s.config
}

// IsSpotSchedulingDisabled returns whether spot scheduling is currently disabled for the given workload.
func (s *TestScheduler) IsSpotSchedulingDisabled(group, kind, namespace, name string) bool {
	w := objectRef{Group: group, Kind: kind, Namespace: namespace, Name: name}
	cfg, ok := s.getSpotConfig(w)
	if !ok {
		return false
	}
	return cfg.isDisabled(s.clock.Now())
}

// HasConfig reports whether the workload has an entry in the config store.
func (s *TestScheduler) HasConfig(group, kind, namespace, name string) bool {
	_, ok := s.getSpotConfig(objectRef{Group: group, Kind: kind, Namespace: namespace, Name: name})
	return ok
}

// HasTrackedPods reports whether the workload has any pods tracked in the pod tracker.
func (s *TestScheduler) HasTrackedPods(group, kind, namespace, name string) bool {
	s.tracker.mu.RLock()
	defer s.tracker.mu.RUnlock()
	_, ok := s.tracker.podSets[objectRef{Group: group, Kind: kind, Namespace: namespace, Name: name}]
	return ok
}

// Spot node selector and taint exported for tests.
const (
	SpotNodeLabelKey   = spotNodeLabelKey
	SpotNodeLabelValue = spotNodeLabelValue
	SpotNodeTaintKey   = spotNodeTaintKey
	SpotNodeTaintValue = spotNodeTaintValue
)

// IsSpotAssigned reports whether the pod is assigned to a spot instance.
func IsSpotAssigned(pod *workloadmeta.KubernetesPod) bool {
	return isSpotAssigned(pod)
}

// podEvictorFunc is a function type implementing podEvictor for testing.
type podEvictorFunc func(ctx context.Context, namespace, name string) error

func (f podEvictorFunc) evictPod(ctx context.Context, namespace, name string, _ corev1.PodPhase) error {
	return f(ctx, namespace, name)
}

// workloadPatcherFunc is a function type implementing workloadPatcher for testing.
type workloadPatcherFunc func(ctx context.Context, owner objectRef, until time.Time) error

func (f workloadPatcherFunc) setDisabledUntil(ctx context.Context, owner objectRef, until time.Time) error {
	return f(ctx, owner, until)
}

// CoreV1PodToWLM is an alias for coreV1PodToWLM, exposed for testing.
var CoreV1PodToWLM = coreV1PodToWLM
