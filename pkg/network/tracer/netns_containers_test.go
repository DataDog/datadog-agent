// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package tracer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go4.org/intern"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

const testRootNetNS = 4026531840

// newTestNetNSResolver returns a resolver whose namespace lookup uses pidToNetNS instead of procfs.
func newTestNetNSResolver(pidToNetNS map[int]uint32) *netnsContainerResolver {
	return &netnsContainerResolver{
		readNetNS: func(pid int) (uint32, error) {
			if ns, ok := pidToNetNS[pid]; ok {
				return ns, nil
			}
			return 0, errors.New("no such pid")
		},
		rootNetNS:   testRootNetNS,
		ownerImages: map[string]struct{}{"ztunnel": {}},
		containers:  make(map[*intern.Value]trackedContainer),
		byNetNS:     make(map[uint32]map[*intern.Value]struct{}),
	}
}

func testContainerEvent(eventType workloadmeta.EventType, id, image string, pid int) workloadmeta.Event {
	return workloadmeta.Event{
		Type: eventType,
		Entity: &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: id},
			Image:    workloadmeta.ContainerImage{ShortName: image},
			PID:      pid,
			State:    workloadmeta.ContainerState{Running: true},
		},
	}
}

func TestNetNSContainerResolver(t *testing.T) {
	const (
		ztunnelNS  = 1001
		appNS      = 1002
		multiNS    = 1003
		unknownNS  = 1999
		ztunnelPID = 10
		appPID     = 20
		multiPID1  = 30
		multiPID2  = 31
		hostPID    = 40
	)

	r := newTestNetNSResolver(map[int]uint32{
		ztunnelPID: ztunnelNS,
		appPID:     appNS,
		multiPID1:  multiNS,
		multiPID2:  multiNS,
		hostPID:    testRootNetNS,
	})
	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "ztunnel", ztunnelPID),
		testContainerEvent(workloadmeta.EventTypeSet, "app", "router", appPID),
		testContainerEvent(workloadmeta.EventTypeSet, "multi-a", "web", multiPID1),
		testContainerEvent(workloadmeta.EventTypeSet, "multi-b", "helper", multiPID2),
		testContainerEvent(workloadmeta.EventTypeSet, "hostnet", "node-exporter", hostPID),
	})

	ztunnel := intern.GetByString("ztunnel")
	app := intern.GetByString("app")

	t.Run("owner socket in a single-container pod is reattributed", func(t *testing.T) {
		cid, ok := r.resolve(appNS, ztunnel)
		require.True(t, ok)
		assert.Equal(t, app, cid)
	})

	t.Run("owner socket in its own namespace is unchanged", func(t *testing.T) {
		_, ok := r.resolve(ztunnelNS, ztunnel)
		assert.False(t, ok)
	})

	t.Run("non-owner sockets are unchanged", func(t *testing.T) {
		_, ok := r.resolve(ztunnelNS, app)
		assert.False(t, ok)
		_, ok = r.resolve(multiNS, app)
		assert.False(t, ok)
	})

	t.Run("multi-container pods are unchanged", func(t *testing.T) {
		_, ok := r.resolve(multiNS, ztunnel)
		assert.False(t, ok)
	})

	t.Run("host and unknown namespaces are unchanged", func(t *testing.T) {
		_, ok := r.resolve(testRootNetNS, ztunnel)
		assert.False(t, ok)
		_, ok = r.resolve(unknownNS, ztunnel)
		assert.False(t, ok)
		_, ok = r.resolve(0, ztunnel)
		assert.False(t, ok)
	})

	t.Run("no owner container is unchanged", func(t *testing.T) {
		_, ok := r.resolve(appNS, nil)
		assert.False(t, ok)
	})
}

func TestNetNSContainerResolverRemovesDeletedContainers(t *testing.T) {
	const reusedNS = 2002

	r := newTestNetNSResolver(map[int]uint32{10: 2001, 20: reusedNS, 21: reusedNS})
	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "ztunnel", 10),
		testContainerEvent(workloadmeta.EventTypeSet, "old-app", "router", 20),
	})

	ztunnel := intern.GetByString("ztunnel")

	cid, ok := r.resolve(reusedNS, ztunnel)
	require.True(t, ok)
	assert.Equal(t, intern.GetByString("old-app"), cid)

	// the old pod goes away and the kernel hands its namespace inode to a new pod
	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeUnset, "old-app", "router", 20),
	})
	_, ok = r.resolve(reusedNS, ztunnel)
	assert.False(t, ok, "a deleted container must not be a reattribution target")
	assert.NotContains(t, r.byNetNS, uint32(reusedNS), "an emptied namespace must be dropped")

	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeSet, "new-app", "router", 21),
	})
	cid, ok = r.resolve(reusedNS, ztunnel)
	require.True(t, ok)
	assert.Equal(t, intern.GetByString("new-app"), cid)
}

func TestNetNSContainerResolverSkipsStoppedAndUnreadable(t *testing.T) {
	r := newTestNetNSResolver(map[int]uint32{10: 3001})

	stopped := testContainerEvent(workloadmeta.EventTypeSet, "stopped", "router", 0)
	stopped.Entity.(*workloadmeta.Container).State.Running = false

	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "ztunnel", 10),
		stopped,
		testContainerEvent(workloadmeta.EventTypeSet, "unreadable", "router", 99),
	})

	assert.Len(t, r.containers, 1)
	assert.Empty(t, r.byNetNS[0])
}

func TestNetNSContainerResolverConcurrentAccess(t *testing.T) {
	r := newTestNetNSResolver(map[int]uint32{10: 4001, 20: 4002})
	r.process([]workloadmeta.Event{testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "ztunnel", 10)})

	ztunnel := intern.GetByString("ztunnel")
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			r.process([]workloadmeta.Event{testContainerEvent(workloadmeta.EventTypeSet, "app", "router", 20)})
			r.process([]workloadmeta.Event{testContainerEvent(workloadmeta.EventTypeUnset, "app", "router", 20)})
		}
	}()
	for i := 0; i < 1000; i++ {
		r.resolve(4002, ztunnel)
	}
	<-done
}
