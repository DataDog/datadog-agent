// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
)

func TestReachabilityTagsNotProbed(t *testing.T) {
	// Not probed must produce no tags at all. The backend has to tell "never
	// probed" from "probed, nothing reachable", and reading the former as
	// unreachable would reject hosts that work.
	assert.Empty(t, reachabilityTags(nil))
}

func TestReachabilityTagsReachable(t *testing.T) {
	tags := reachabilityTags(&oci.Reachability{
		CheckedAt:  time.Now(),
		Registries: []oci.RegistryStatus{{Registry: "install.datadoghq.com/agent-package", Reachable: true}},
	})
	assert.Equal(t, []string{"fleet_registry_reachable:true"}, tags)
}

func TestReachabilityTagsUnreachable(t *testing.T) {
	// The two causes this whole signal exists to separate: no route to the
	// registry (the customer's precondition) and an unparseable local
	// credential file (the 68L-31Y-yZy shape). Both are installer code 1 today.
	tags := reachabilityTags(&oci.Reachability{
		CheckedAt: time.Now(),
		Registries: []oci.RegistryStatus{
			{Registry: "install.datadoghq.com/agent-package", FailureKind: oci.FailureKindConnection, Err: errors.New("connection refused")},
			{Registry: "gcr.io/datadoghq/agent-package", FailureKind: oci.FailureKindAuthConfig, Err: errors.New("illegal base64 data at input byte 8")},
		},
	})
	assert.Equal(t, []string{
		"fleet_registry_reachable:false",
		"fleet_registry_failure_kind:connection",
		"fleet_registry_failure_kind:auth_config",
	}, tags)
}

func TestReachabilityTagsNoErrorText(t *testing.T) {
	// Error strings must never reach a tag: unbounded cardinality, and they
	// embed host-specific paths.
	tags := reachabilityTags(&oci.Reachability{
		CheckedAt: time.Now(),
		Registries: []oci.RegistryStatus{{
			Registry:    "install.datadoghq.com/agent-package",
			FailureKind: oci.FailureKindAuthConfig,
			Err:         errors.New("parsing /home/someone/.docker/config.json: illegal base64 data"),
		}},
	})
	for _, tag := range tags {
		assert.NotContains(t, tag, "someone")
		assert.NotContains(t, tag, "config.json")
	}
}

func TestReachabilityTagsSkipsNotAttempted(t *testing.T) {
	// Probing stops at the first reachable registry, so later entries are
	// not-reachable with a nil Err. They must not publish a spurious "unknown"
	// cause: that would invent a failure the host never had.
	tags := reachabilityTags(&oci.Reachability{
		CheckedAt: time.Now(),
		Registries: []oci.RegistryStatus{
			{Registry: "install.datadoghq.com/agent-package", Reachable: true},
			{Registry: "gcr.io/datadoghq/agent-package"},
		},
	})
	assert.Equal(t, []string{"fleet_registry_reachable:true"}, tags)
}

func TestReachabilityTagsReportsFallbackUse(t *testing.T) {
	// Primary down, fallback fine. The host can still upgrade, so it is
	// reachable — but the cause is still reported, because a fleet quietly
	// running on its fallback is worth seeing before the fallback goes too.
	tags := reachabilityTags(&oci.Reachability{
		CheckedAt: time.Now(),
		Registries: []oci.RegistryStatus{
			{Registry: "install.datadoghq.com/agent-package", FailureKind: oci.FailureKindDNS, Err: errors.New("no such host")},
			{Registry: "gcr.io/datadoghq/agent-package", Reachable: true},
		},
	})
	assert.Equal(t, []string{
		"fleet_registry_reachable:true",
		"fleet_registry_failure_kind:dns",
	}, tags)
}

func TestReachabilityTagsDeduplicatesCauses(t *testing.T) {
	// Both registries behind the same firewall produce the same cause once.
	tags := reachabilityTags(&oci.Reachability{
		CheckedAt: time.Now(),
		Registries: []oci.RegistryStatus{
			{Registry: "install.datadoghq.com/agent-package", FailureKind: oci.FailureKindConnection, Err: errors.New("i/o timeout")},
			{Registry: "gcr.io/datadoghq/agent-package", FailureKind: oci.FailureKindConnection, Err: errors.New("i/o timeout")},
		},
	})
	assert.Equal(t, []string{
		"fleet_registry_reachable:false",
		"fleet_registry_failure_kind:connection",
	}, tags)
}

func TestReachabilityTagsEveryFailureKindIsDistinct(t *testing.T) {
	// Every cause must produce its own tag value. Two kinds collapsing into one
	// string would silently merge a customer precondition with a defect of ours,
	// which is the exact confusion this signal exists to remove.
	seen := map[string]oci.FailureKind{}
	for k := oci.FailureKindUnknown; k <= oci.FailureKindInvalidReference; k++ {
		tags := reachabilityTags(&oci.Reachability{
			CheckedAt:  time.Now(),
			Registries: []oci.RegistryStatus{{Registry: "r", FailureKind: k, Err: errors.New("boom")}},
		})
		require.Len(t, tags, 2)
		assert.Equal(t, "fleet_registry_reachable:false", tags[0])

		tag := tags[1]
		assert.Equal(t, fmt.Sprintf("fleet_registry_failure_kind:%s", k), tag)
		if prev, dup := seen[tag]; dup {
			t.Fatalf("kinds %d and %d both produce %q", prev, k, tag)
		}
		seen[tag] = k
	}
	assert.Len(t, seen, int(oci.FailureKindInvalidReference)+1)
}

func TestReachabilityResultDisabled(t *testing.T) {
	// Probing off must report nothing rather than a fabricated result.
	d := &daemonImpl{}
	assert.Nil(t, d.reachabilityResult())
	assert.Empty(t, reachabilityTags(d.reachabilityResult()))
}

func TestReachabilityResultPeeksOnly(t *testing.T) {
	// reachabilityResult runs on the state-refresh path, which must never block
	// on a network probe.
	probed := false
	cache := oci.NewReachabilityCache(proberFunc(func() *oci.Reachability {
		probed = true
		return &oci.Reachability{}
	}), time.Hour, "")
	d := &daemonImpl{reachability: cache}

	assert.Nil(t, d.reachabilityResult())
	assert.False(t, probed)

	seeded := &oci.Reachability{
		Registries: []oci.RegistryStatus{{Registry: "gcr.io/datadoghq/agent-package", Reachable: true}},
		CheckedAt:  time.Now(),
	}
	cache.Seed(seeded)
	assert.Same(t, seeded, d.reachabilityResult())
	assert.False(t, probed)
}

func TestTriggerReachabilityProbeDoesNotBlock(t *testing.T) {
	// The trigger is called from the task-completion path while the daemon
	// goroutine may be busy. It must never block there.
	d := &daemonImpl{probeReachability: make(chan struct{}, 1)}
	for i := 0; i < 10; i++ {
		d.triggerReachabilityProbe()
	}
	assert.Len(t, d.probeReachability, 1)
}

func TestNewOptionalTicker(t *testing.T) {
	// A zero or negative interval must yield a ticker that never fires, so the
	// daemon's select can hold the case unconditionally.
	stopped := newOptionalTicker(0)
	defer stopped.Stop()
	select {
	case <-stopped.C:
		t.Fatal("ticker with a zero interval fired")
	case <-time.After(50 * time.Millisecond):
	}

	running := newOptionalTicker(time.Millisecond)
	defer running.Stop()
	select {
	case <-running.C:
	case <-time.After(5 * time.Second):
		t.Fatal("ticker did not fire")
	}
}

// proberFunc adapts a function to oci.Prober.
type proberFunc func() *oci.Reachability

func (f proberFunc) CheckReachability(_ context.Context, _ string) *oci.Reachability { return f() }
