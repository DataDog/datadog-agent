// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package gpuallocation

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

// stubBackend is a backend whose results the test controls directly, unlike
// draBackend which derives them from workloadmeta. Its fields are read fresh on
// every call, so a test can mutate them between two Run() calls.
type stubBackend struct {
	backendName string
	pendingFn   func(now time.Time) []pendingAllocation
	allocatedFn func() []allocatedDevices
}

func (s *stubBackend) name() string { return s.backendName }
func (s *stubBackend) pending(now time.Time) []pendingAllocation {
	if s.pendingFn == nil {
		return nil
	}
	return s.pendingFn(now)
}
func (s *stubBackend) allocated() []allocatedDevices {
	if s.allocatedFn == nil {
		return nil
	}
	return s.allocatedFn()
}

func newTestCheck(t *testing.T, b backend) (*Check, *mocksender.MockSender) {
	c := &Check{
		CheckBase:         core.NewCheckBase(CheckName),
		backends:          []backend{b},
		runLeaderElection: false,
		now:               func() time.Time { return now },
		isLeader:          func() (bool, error) { return true, nil },
	}
	c.BuildID(0, integration.Data("{}"), integration.Data("{}"))
	demux := mocksender.CreateDefaultDemultiplexer(t)
	sender := mocksender.NewMockSenderWithSenderManager(c.ID(), demux)
	sender.SetupAcceptAll()
	if err := c.CommonConfigure(demux, integration.Data("{}"), integration.Data("{}"), "test", "test"); err != nil {
		t.Fatalf("CommonConfigure: %v", err)
	}
	return c, sender
}

// A namespace or node that stops appearing must be reported as zero, not
// dropped silently. A Gauge that simply stops receiving samples leaves a
// monitor on its last value or in no-data, never recovering -- explicitly
// zeroing on disappearance is what lets an alert clear.
func TestRunEmitsZeroWhenPreviouslyReportedTagsDisappear(t *testing.T) {
	pendingResults := []pendingAllocation{{namespace: "team-a", waiting: 90 * time.Second}}
	allocatedResults := []allocatedDevices{{node: "node-1", count: 3}}

	b := &stubBackend{
		backendName: "dra",
		pendingFn:   func(time.Time) []pendingAllocation { return pendingResults },
		allocatedFn: func() []allocatedDevices { return allocatedResults },
	}
	c, sender := newTestCheck(t, b)

	if err := c.Run(); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	sender.AssertMetric(t, "Gauge", "gpu.allocation.pending.count", 1, "", []string{"source:dra", "kube_namespace:team-a"})
	sender.AssertMetric(t, "Gauge", "gpu.allocation.pending.seconds.max", 90, "", []string{"source:dra", "kube_namespace:team-a"})
	sender.AssertMetric(t, "Gauge", "gpu.allocation.devices.allocated", 3, "", []string{"source:dra", "kube_node:node-1"})
	sender.ResetCalls()

	// The workload finished and the node was drained: nothing to report now.
	pendingResults = nil
	allocatedResults = nil

	if err := c.Run(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	sender.AssertMetric(t, "Gauge", "gpu.allocation.pending.count", 0, "", []string{"source:dra", "kube_namespace:team-a"})
	sender.AssertMetric(t, "Gauge", "gpu.allocation.pending.seconds.max", 0, "", []string{"source:dra", "kube_namespace:team-a"})
	sender.AssertMetric(t, "Gauge", "gpu.allocation.devices.allocated", 0, "", []string{"source:dra", "kube_node:node-1"})
}

// Zero-on-disappearance is bookkeeping about what this replica has reported, so
// it must be maintained even on Runs that emit nothing. A replica that is not
// the leader still observes the cluster-wide state; if it returns before
// recording it, then after a failover the newly elected replica has no memory of
// the series it is now responsible for and can never zero one that vanished
// during the handover -- exactly the stale/no-data problem zeroing exists to fix.
func TestRunTracksStateWhileNotLeaderSoZeroSurvivesFailover(t *testing.T) {
	pendingResults := []pendingAllocation{{namespace: "team-a", waiting: time.Minute}}
	b := &stubBackend{
		backendName: "dra",
		pendingFn:   func(time.Time) []pendingAllocation { return pendingResults },
	}
	c, sender := newTestCheck(t, b)

	// First this replica is leader and publishes team-a: it now owns the series
	// and must be able to zero it later.
	c.runLeaderElection = true
	c.isLeader = func() (bool, error) { return true, nil }
	if err := c.Run(); err != nil {
		t.Fatalf("leader Run: %v", err)
	}
	sender.AssertMetric(t, "Gauge", "gpu.allocation.pending.count", 1.0, "", []string{"source:dra", "kube_namespace:team-a"})

	// Now it is a follower and team-a disappears. It must observe the state but
	// publish nothing, and -- critically -- must not advance its remembered set:
	// the disappeared namespace still owes a zero, and only a leader publishes.
	c.isLeader = func() (bool, error) { return false, nil }
	pendingResults = nil
	if err := c.Run(); err != nil {
		t.Fatalf("follower Run: %v", err)
	}
	sender.AssertNotCalled(t, "Gauge", "gpu.allocation.pending.count", 0, "", []string{"source:dra", "kube_namespace:team-a"})

	// Failover: this replica becomes leader again, and the workload is still
	// gone. Because the follower did not advance the remembered set, the zero
	// for team-a is still owed and is now emitted.
	c.isLeader = func() (bool, error) { return true, nil }
	if err := c.Run(); err != nil {
		t.Fatalf("leader Run: %v", err)
	}
	sender.AssertMetric(t, "Gauge", "gpu.allocation.pending.count", 0, "", []string{"source:dra", "kube_namespace:team-a"})
	sender.AssertMetric(t, "Gauge", "gpu.allocation.pending.seconds.max", 0, "", []string{"source:dra", "kube_namespace:team-a"})
}

// The zero must be reported exactly once, not on every subsequent Run -- an
// indefinitely departed namespace/node must not accumulate as permanent
// cardinality.
func TestRunStopsEmittingAfterOneZero(t *testing.T) {
	pendingResults := []pendingAllocation{{namespace: "team-a", waiting: time.Minute}}
	b := &stubBackend{
		backendName: "dra",
		pendingFn:   func(time.Time) []pendingAllocation { return pendingResults },
	}
	c, sender := newTestCheck(t, b)

	if err := c.Run(); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	pendingResults = nil
	if err := c.Run(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	// Confirm the zero really was emitted here -- otherwise the "not called
	// again" assertion below would pass vacuously even with no fix at all.
	sender.AssertMetric(t, "Gauge", "gpu.allocation.pending.count", 0, "", []string{"source:dra", "kube_namespace:team-a"})
	sender.ResetCalls()

	if err := c.Run(); err != nil {
		t.Fatalf("third Run: %v", err)
	}
	sender.AssertNotCalled(t, "Gauge", "gpu.allocation.pending.count", 0, "", []string{"source:dra", "kube_namespace:team-a"})
	sender.AssertNotCalled(t, "Gauge", "gpu.allocation.pending.seconds.max", 0, "", []string{"source:dra", "kube_namespace:team-a"})
}
