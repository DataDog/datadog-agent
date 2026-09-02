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
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
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
// reload path reads back). Only the fields used by persistProfile are populated, config included:
// persistProfile reads it, so it has to be present even when every setting is left at its zero value.
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
		config: &config.Config{
			RuntimeSecurity: &config.RuntimeSecurityConfig{},
			Probe:           &pconfig.Config{},
		},
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

// newHardeningPostureTestProfile builds a profile whose tree holds a single runtime process
// node, which is the minimum a posture summary can be derived from.
func newHardeningPostureTestProfile(uid uint32) *profile.Profile {
	p := profile.New(profile.WithWorkloadSelector(cgroupModel.WorkloadSelector{Image: "img", Tag: "v1"}))
	p.Metadata = mtdt.Metadata{Name: "posture", Arch: "x86_64"}
	p.ActivityTree.ProcessNodes = []*activity_tree.ProcessNode{{
		NodeBase:       activity_tree.NewNodeBase(),
		GenerationType: activity_tree.Runtime,
		Process: activity_tree.ProcessInfo{
			FileEvent:   model.FileEvent{PathnameStr: "/usr/bin/proc", BasenameStr: "proc"},
			Credentials: model.Credentials{UID: uid, EUID: uid, FSUID: uid},
		},
	}}
	p.ActivityTree.ComputeActivityTreeStats()
	return p
}

func newHardeningPostureTestManager(t *testing.T, postureEnabled, capabilitiesEnabled bool) *ManagerV2 {
	t.Helper()

	m, _ := newTestManagerV2WithLocalStorage(t)
	m.config.RuntimeSecurity.SecurityProfileV2HardeningPostureEnabled = postureEnabled
	m.config.Probe.CapabilitiesMonitoringEnabled = capabilitiesEnabled
	return m
}

// TestManagerV2_persistProfile_attachesHardeningPosture verifies the posture summary reaches the
// dump header on the way out, and only when the feature is turned on: the block is indexed on
// every profile from every container, so it must not appear by default.
func TestManagerV2_persistProfile_attachesHardeningPosture(t *testing.T) {
	t.Run("attaches the posture when enabled", func(t *testing.T) {
		m := newHardeningPostureTestManager(t, true, true)
		p := newHardeningPostureTestProfile(1000)

		m.persistProfile(p)

		require.NotNil(t, p.Header.Hardening)
		assert.Equal(t, []uint32{1000}, p.Header.Hardening.Observed.UIDs)
		assert.True(t, p.Header.Hardening.Observed.CapabilitiesCollected)
		assert.Equal(t, "x86_64", p.Header.Hardening.Observed.Arch)
	})

	t.Run("attaches nothing when disabled", func(t *testing.T) {
		m := newHardeningPostureTestManager(t, false, true)
		p := newHardeningPostureTestProfile(1000)

		m.persistProfile(p)

		assert.Nil(t, p.Header.Hardening)
	})

	// Capability collection is a separate switch, off by default. The posture still ships, but
	// it has to say the mask means "not collected" rather than "nothing used".
	t.Run("records that capabilities were not collected", func(t *testing.T) {
		m := newHardeningPostureTestManager(t, true, false)
		p := newHardeningPostureTestProfile(1000)

		m.persistProfile(p)

		require.NotNil(t, p.Header.Hardening)
		assert.False(t, p.Header.Hardening.Observed.CapabilitiesCollected)
	})
}
