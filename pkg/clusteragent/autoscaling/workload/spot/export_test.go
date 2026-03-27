// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/clock"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// NewTestScheduler creates a Scheduler for testing.
func NewTestScheduler(config Config, clk clock.WithTicker, wlm workloadmeta.Component, evictPod func(namespace, name string) error, store workloadConfigStore) *Scheduler {
	isLeader := func() bool {
		return true
	}
	evictorFunc := podEvictorFunc(func(_ context.Context, namespace, name string) error {
		return evictPod(namespace, name)
	})
	patcherFunc := workloadPatcherFunc(func(ctx context.Context, owner workload, until time.Time) error {
		return nil
	})
	return newScheduler(config, clk, wlm, evictorFunc, patcherFunc, store, isLeader)
}

// TrackedCounts returns the total and spot tracked pod counts (including in-flight admissions) for the given owner.
func (s *Scheduler) TrackedCounts(namespace, kind, name string) (total, spot int) {
	s.tracker.mu.RLock()
	defer s.tracker.mu.RUnlock()
	owner := podOwner{Kind: kind, Namespace: namespace, Name: name}
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

// IsSpotSchedulingDisabledForOwner returns whether spot scheduling is currently disabled
// for the workload that owns the given resource.
func (s *Scheduler) IsSpotSchedulingDisabledForOwner(namespace, kind, name string) bool {
	owner := podOwner{Kind: kind, Namespace: namespace, Name: name}
	cfg, ok := s.getSpotConfig(owner)
	if !ok {
		return false
	}
	return cfg.isDisabled(s.clock.Now())
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

// workloadPatcherFunc is a function type implementing workloadPatcher for testing.
type workloadPatcherFunc func(ctx context.Context, owner workload, until time.Time) error

func (f workloadPatcherFunc) setDisabledUntil(ctx context.Context, owner workload, until time.Time) error {
	return f(ctx, owner, until)
}

// TestWorkloadConfigStore is a test-only workloadConfigStore backed by a mutable map.
type TestWorkloadConfigStore struct {
	defaultConfig spotConfig

	mu      sync.RWMutex
	configs map[workload]spotConfig
}

// NewTestWorkloadConfigStore creates a TestWorkloadConfigStore with defaults from cfg.
func NewTestWorkloadConfigStore(cfg Config) *TestWorkloadConfigStore {
	return &TestWorkloadConfigStore{
		defaultConfig: spotConfig{percentage: cfg.Percentage, minOnDemand: cfg.MinOnDemandReplicas},
		configs:       make(map[workload]spotConfig),
	}
}

func (s *TestWorkloadConfigStore) run(ctx context.Context) { <-ctx.Done() }

func (s *TestWorkloadConfigStore) getConfig(key workload) (spotConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.configs[key]
	return cfg, ok
}

// Update sets the spot config for the given workload from annotations.
func (s *TestWorkloadConfigStore) Update(namespace, kind, name string, annotations map[string]string) {
	key := workload{Kind: kind, Namespace: namespace, Name: name}
	cfg := s.defaultConfig
	overrideFromAnnotations(&cfg, annotations)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configs[key] = cfg
}

func (s *TestWorkloadConfigStore) disable(key workload, now time.Time, until time.Time) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.configs[key]
	if now.Before(cfg.disabledUntil) {
		return cfg.disabledUntil, false
	}
	cfg.disabledUntil = until
	s.configs[key] = cfg
	return until, true
}
