// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"time"

	"k8s.io/utils/clock"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// NewTestScheduler create a Scheduler for testing.
func NewTestScheduler(config Config, clk clock.WithTicker, wlm workloadmeta.Component) *Scheduler {
	rollout := rolloutFunc(func(context.Context, ownerKey, time.Time) (bool, error) {
		return true, nil
	})
	isLeader := func() bool {
		return true
	}
	return newScheduler(config, clk, wlm, rollout, isLeader)
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

// rolloutFunc is a function type implementing rollout for testing.
type rolloutFunc func(context.Context, ownerKey, time.Time) (bool, error)

func (f rolloutFunc) restart(ctx context.Context, k ownerKey, ts time.Time) (bool, error) {
	return f(ctx, k, ts)
}
