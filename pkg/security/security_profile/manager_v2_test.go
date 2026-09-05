// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
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
			Process: activity_tree.ProcessInfo{
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

		assert.False(t, p.HasAlreadyBeenSent(), "persisting a disabled profile must not open the send-gated learning window")
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
		assert.True(t, p.HasAlreadyBeenSent(), "persisting an enabled profile ends the send-gated learning window")
	})
}

// TestManagerV2_evictUnusedNodes_skipsDisabledProfile verifies the eviction tick leaves disabled
// profiles untouched: Disable() already empties the activity tree, so there is nothing to evict,
// and re-enabling a disabled profile is owned by a separate path, not the eviction loop.
func TestManagerV2_evictUnusedNodes_skipsDisabledProfile(t *testing.T) {
	cgr, err := cgroup.NewResolver(&statsd.NoOpClient{}, nil, nil)
	require.NoError(t, err)

	maxSize := 1 << 20
	m := &ManagerV2{
		statsdClient:         &statsd.NoOpClient{},
		resolvers:            &resolvers.EBPFResolvers{CGroupResolver: cgr},
		profiles:             make(map[cgroupModel.WorkloadSelector]*profile.Profile),
		evictionRuns:         atomic.NewUint64(0),
		evictionNodesEvicted: atomic.NewUint64(0),
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

func TestManagerV2_shouldSendAnomalyDetection(t *testing.T) {
	start := time.Now()
	withStart := func() *profile.Profile {
		p := profile.New()
		p.Metadata = mtdt.Metadata{Start: start}
		return p
	}
	timeBased := func(period time.Duration) *ManagerV2 {
		return &ManagerV2{config: &config.Config{RuntimeSecurity: &config.RuntimeSecurityConfig{
			SecurityProfileV2UseTimeBasedAnomalyStabilization: true,
			SecurityProfileV2AnomalyStabilizationPeriod:       period,
		}}}
	}

	t.Run("default withholds until the profile has been persisted", func(t *testing.T) {
		p := withStart()
		m := &ManagerV2{config: &config.Config{RuntimeSecurity: &config.RuntimeSecurityConfig{}}}
		assert.False(t, m.shouldSendAnomalyDetection(p, start))
		p.SetHasAlreadyBeenSent()
		assert.True(t, m.shouldSendAnomalyDetection(p, start))
	})

	t.Run("time-based with a zero period sends as soon as the profile starts", func(t *testing.T) {
		p := withStart()
		assert.True(t, timeBased(0).shouldSendAnomalyDetection(p, start))
		assert.False(t, p.HasAlreadyBeenSent(), "time-based stabilization does not depend on persistence")
	})

	t.Run("time-based waits for the configured period after the profile starts", func(t *testing.T) {
		p := withStart()
		m := timeBased(time.Hour)
		assert.False(t, m.shouldSendAnomalyDetection(p, start))
		assert.False(t, m.shouldSendAnomalyDetection(p, start.Add(time.Hour-time.Nanosecond)))
		assert.True(t, m.shouldSendAnomalyDetection(p, start.Add(time.Hour)))
	})

	t.Run("time-based anchors on the persisted start, not the in-memory creation time", func(t *testing.T) {
		p := withStart()
		p.Metadata.Start = start.Add(-time.Hour)
		assert.True(t, timeBased(time.Hour).shouldSendAnomalyDetection(p, start))
	})

	t.Run("time-based clamps a backward clock jump to no elapsed time", func(t *testing.T) {
		p := withStart()
		assert.False(t, timeBased(time.Hour).shouldSendAnomalyDetection(p, start.Add(-time.Hour)))
	})

	t.Run("time-based falls back to the in-memory start when none is persisted", func(t *testing.T) {
		p := profile.New()
		require.True(t, p.Metadata.Start.IsZero())
		m := timeBased(time.Hour)
		assert.False(t, m.shouldSendAnomalyDetection(p, p.StartedAt()))
		assert.True(t, m.shouldSendAnomalyDetection(p, p.StartedAt().Add(time.Hour)))
	})
}

func TestManagerV2_withinStartupDelay(t *testing.T) {
	start := time.Now()
	newManager := func(delay time.Duration) *ManagerV2 {
		return &ManagerV2{
			startTime: start,
			config: &config.Config{RuntimeSecurity: &config.RuntimeSecurityConfig{
				SecurityProfileV2StartupDelay: delay,
			}},
		}
	}

	t.Run("disabled by default", func(t *testing.T) {
		assert.False(t, newManager(0).withinStartupDelay(start))
	})

	t.Run("ignores events within the delay and resumes after it", func(t *testing.T) {
		m := newManager(time.Minute)
		assert.True(t, m.withinStartupDelay(start))
		assert.True(t, m.withinStartupDelay(start.Add(time.Minute-time.Nanosecond)))
		assert.False(t, m.withinStartupDelay(start.Add(time.Minute)))
	})

	t.Run("a backward clock jump keeps events within the delay window", func(t *testing.T) {
		assert.True(t, newManager(time.Minute).withinStartupDelay(start.Add(-time.Hour)))
	})
}
