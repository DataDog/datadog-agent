// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage"
)

// newTestManagerV2WithLocalStorage builds a minimal ManagerV2 wired to a real on-disk local
// storage backend, configured to persist profiles in the security-profile format (the format the
// reload path reads back). Only the fields used by persistProfile are populated.
func newTestManagerV2WithLocalStorage(t *testing.T) (*ManagerV2, string) {
	t.Helper()

	dir := t.TempDir()
	localStorage, err := storage.NewDirectory(dir, 100)
	require.NoError(t, err)

	m := &ManagerV2{
		statsdClient: &statsd.NoOpClient{},
		localStorage: localStorage,
		configuredStorageRequests: perFormatStorageRequests([]config.StorageRequest{
			config.NewStorageRequest(config.LocalStorage, config.Profile, false, dir),
		}),
	}
	return m, dir
}

func newTestProfileWithNodes(name string, nodeCount int) *profile.Profile {
	p := profile.New(profile.WithWorkloadSelector(cgroupModel.WorkloadSelector{Image: "img", Tag: "v1"}))
	p.Metadata = mtdt.Metadata{Name: name}
	for i := 0; i < nodeCount; i++ {
		p.ActivityTree.ProcessNodes = append(p.ActivityTree.ProcessNodes, &activity_tree.ProcessNode{
			NodeBase: activity_tree.NewNodeBase(),
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/proc",
					BasenameStr: "proc",
				},
			},
		})
	}
	p.ActivityTree.ComputeActivityTreeStats()
	return p
}

// TestManagerV2_persistProfile_persistsDisabledState verifies that a profile disabled by the
// max-size safeguard is persisted to local storage carrying its disabled state, so it reloads as
// disabled after an Agent restart instead of coming back enabled and re-learning the workload.
func TestManagerV2_persistProfile_persistsDisabledState(t *testing.T) {
	selector := cgroupModel.WorkloadSelector{Image: "img", Tag: "v1"}

	// A profile disabled by the safeguard (tree dropped) must still be written to disk and must
	// reload as disabled.
	t.Run("persists a disabled profile and reloads it as disabled", func(t *testing.T) {
		m, dir := newTestManagerV2WithLocalStorage(t)
		p := newTestProfileWithNodes("disabled", 8)
		p.Disable()
		require.False(t, p.IsEnabled())
		require.True(t, p.ActivityTree.IsEmpty())

		m.persistProfile(p)

		filePath := filepath.Join(dir, "disabled."+config.Profile.String())
		_, err := os.Stat(filePath)
		require.NoError(t, err, "a disabled profile must be persisted so its state survives a restart")

		reloaded := profile.New(profile.WithWorkloadSelector(selector))
		ok, err := m.localStorage.Load(&selector, reloaded)
		require.NoError(t, err)
		require.True(t, ok, "the persisted disabled profile should be found on disk")
		assert.False(t, reloaded.IsEnabled(), "a profile persisted as disabled must reload as disabled")
	})

	// An enabled profile keeps being persisted with its tree and reloads as enabled.
	t.Run("persists an enabled profile and reloads it as enabled", func(t *testing.T) {
		m, _ := newTestManagerV2WithLocalStorage(t)
		p := newTestProfileWithNodes("enabled", 8)
		require.True(t, p.IsEnabled())

		m.persistProfile(p)

		reloaded := profile.New(profile.WithWorkloadSelector(selector))
		ok, err := m.localStorage.Load(&selector, reloaded)
		require.NoError(t, err)
		require.True(t, ok, "the persisted enabled profile should be found on disk")
		assert.True(t, reloaded.IsEnabled(), "an enabled profile must reload as enabled")
		assert.False(t, reloaded.ActivityTree.IsEmpty(), "an enabled profile keeps its tree")
	})
}

// TestManagerV2_cleanupPendingProfiles_persistsBeforeRemoval verifies that a profile queued for
// removal after its workload stopped is written to disk before it is dropped from memory, so a
// later restart of the same workload reloads its state from disk instead of rebuilding it.
func TestManagerV2_cleanupPendingProfiles_persistsBeforeRemoval(t *testing.T) {
	selector := cgroupModel.WorkloadSelector{Image: "img", Tag: "v1"}

	newManager := func(t *testing.T) (*ManagerV2, string) {
		t.Helper()
		m, dir := newTestManagerV2WithLocalStorage(t)
		cookieMap, err := lru.New[uint32, sampleCookieEntry](sampleCookieMapSize)
		require.NoError(t, err)
		m.sampleCookieMap = cookieMap
		m.profiles = make(map[cgroupModel.WorkloadSelector]*profile.Profile)
		m.pendingProfileRemovals = make(map[cgroupModel.WorkloadSelector]time.Time)
		m.config = &config.Config{
			RuntimeSecurity: &config.RuntimeSecurityConfig{
				SecurityProfileCleanupDelay: time.Minute,
			},
		}
		return m, dir
	}

	t.Run("persists and removes a profile whose workload stopped", func(t *testing.T) {
		m, _ := newManager(t)
		p := newTestProfileWithNodes("stopped", 8)
		require.True(t, p.IsEnabled())
		m.profiles[selector] = p
		// Queued long enough ago that the cleanup delay has elapsed and no instance is active.
		m.pendingProfileRemovals[selector] = time.Now().Add(-2 * time.Minute)

		m.cleanupPendingProfiles()

		assert.NotContains(t, m.profiles, selector, "the profile must be dropped from memory after cleanup")
		assert.NotContains(t, m.pendingProfileRemovals, selector, "the pending-removal entry must be cleared")

		reloaded := profile.New(profile.WithWorkloadSelector(selector))
		ok, err := m.localStorage.Load(&selector, reloaded)
		require.NoError(t, err)
		require.True(t, ok, "the profile must be persisted to disk before removal")
		assert.False(t, reloaded.ActivityTree.IsEmpty(), "the persisted profile keeps its learned tree")
	})

	t.Run("keeps a profile that regained active instances and does not persist it", func(t *testing.T) {
		m, _ := newManager(t)
		p := newTestProfileWithNodes("regained", 8)
		p.Instances = append(p.Instances, &tags.Workload{Selector: selector})
		m.profiles[selector] = p
		m.pendingProfileRemovals[selector] = time.Now().Add(-2 * time.Minute)

		m.cleanupPendingProfiles()

		assert.Contains(t, m.profiles, selector, "a profile with active instances must not be removed")

		reloaded := profile.New(profile.WithWorkloadSelector(selector))
		ok, err := m.localStorage.Load(&selector, reloaded)
		require.NoError(t, err)
		assert.False(t, ok, "a profile that is not being removed must not be persisted by cleanup")
	})
}

// TestManagerV2_Start_persistsOnShutdown verifies that cancelling the manager's context flushes
// every in-memory profile to disk before Start returns, so state is not lost when the Agent stops
// while workloads are still running.
func TestManagerV2_Start_persistsOnShutdown(t *testing.T) {
	selector := cgroupModel.WorkloadSelector{Image: "img", Tag: "v1"}

	m, _ := newTestManagerV2WithLocalStorage(t)
	cgr, err := cgroup.NewResolver(&statsd.NoOpClient{}, nil, nil)
	require.NoError(t, err)
	m.resolvers = &resolvers.EBPFResolvers{CGroupResolver: cgr}
	m.profiles = make(map[cgroupModel.WorkloadSelector]*profile.Profile)
	// SecurityProfileEnabled is left false so no ticker fires and only the shutdown path persists.
	m.config = &config.Config{RuntimeSecurity: &config.RuntimeSecurityConfig{}}

	p := newTestProfileWithNodes("running", 8)
	m.profiles[selector] = p

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Start(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}

	reloaded := profile.New(profile.WithWorkloadSelector(selector))
	ok, err := m.localStorage.Load(&selector, reloaded)
	require.NoError(t, err)
	require.True(t, ok, "every in-memory profile must be persisted on shutdown")
	assert.False(t, reloaded.ActivityTree.IsEmpty(), "the persisted profile keeps its learned tree")
}

// TestManagerV2_evictUnusedNodes_skipsDisabledProfile verifies the eviction tick leaves disabled
// profiles untouched: Disable() already empties the activity tree, so there is nothing to evict,
// and re-enabling a disabled profile is owned by a separate path, not the eviction loop.
func TestManagerV2_evictUnusedNodes_skipsDisabledProfile(t *testing.T) {
	cgr, err := cgroup.NewResolver(&statsd.NoOpClient{}, nil, nil)
	require.NoError(t, err)

	maxSize := 1 << 20
	m := &ManagerV2{
		statsdClient: &statsd.NoOpClient{},
		resolvers:    &resolvers.EBPFResolvers{CGroupResolver: cgr},
		profiles:     make(map[cgroupModel.WorkloadSelector]*profile.Profile),
		config: &config.Config{
			RuntimeSecurity: &config.RuntimeSecurityConfig{
				SecurityProfileNodeEvictionTimeout: time.Hour,
				ActivityDumpTraceSystemdCgroups:    false,
				SecurityProfileV2MaxDumpSize:       func() int { return maxSize },
			},
		},
	}

	selector := cgroupModel.WorkloadSelector{Image: "img", Tag: "v1"}
	p := newTestProfileWithNodes("over-limit", 8)
	// The max-size safeguard disables the profile and drops its tree.
	p.Disable()
	require.False(t, p.IsEnabled())
	require.True(t, p.ActivityTree.IsEmpty())
	m.profiles[selector] = p

	m.evictUnusedNodes()

	assert.False(t, p.IsEnabled(), "eviction must not re-enable a disabled profile")
}
