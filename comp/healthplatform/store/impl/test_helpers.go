// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package storeimpl

import (
	"sync/atomic"
	"testing"
	"time"
)

// SlowSaves controls the slow saves set up by SetSaveDelay.
type SlowSaves struct {
	slow    atomic.Bool
	started atomic.Int32
}

// SpeedUp makes the saves that start after it fast again, so the test can stop the store within the stop deadline.
func (s *SlowSaves) SpeedUp() { s.slow.Store(false) }

// Started returns how many slow saves started.
func (s *SlowSaves) Started() int { return int(s.started.Load()) }

// SetSaveDelay makes every save of the stores created until the end of the test take delay, standing in for a slow
// disk or an unreachable Cluster Agent with remote persistence. It changes package state, so it can't be used from a
// parallel test.
func SetSaveDelay(t testing.TB, delay time.Duration) *SlowSaves {
	t.Setenv("DD_TEST_HEALTH_PLATFORM_SAVE_DELAY", delay.String()) // panics in a parallel test
	saves := &SlowSaves{}
	saves.slow.Store(true)
	previous := wrapPersistence
	wrapPersistence = func(p issuesPersistence) issuesPersistence {
		return &slowPersistence{issuesPersistence: previous(p), delay: delay, saves: saves}
	}
	t.Cleanup(func() {
		wrapPersistence = previous
	})
	return saves
}

type slowPersistence struct {
	issuesPersistence
	delay time.Duration
	saves *SlowSaves
}

func (p *slowPersistence) save(state *PersistedState) error {
	if p.saves.slow.Load() {
		p.saves.started.Add(1)
		time.Sleep(p.delay)
	}
	return p.issuesPersistence.save(state)
}
