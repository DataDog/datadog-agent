// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofiletests holds securityprofiletests related files
package securityprofiletests

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	seclModel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
)

// This is a basic behavior test for FileNode: Testing
func TestNodeBase_NodeBaseBehavior_onFileNode(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	fn := &activity_tree.FileNode{
		NodeBase: activity_tree.NewNodeBase(),
		Name:     "/tmp/foo.conf",
		File:     &model.FileEvent{PathnameStr: "/tmp/foo.conf", BasenameStr: "foo.conf"},
	}

	fn.Record("v1.2.3", now)
	assert.False(t, fn.IsEmpty())
	assert.True(t, fn.HasImageTag("v1.2.3"))

	it := fn.Seen["v1.2.3"]
	assert.Equal(t, now, it.FirstSeen)
	assert.Equal(t, now, it.LastSeen)

	later := now.Add(5 * time.Minute)
	fn.Record("v1.2.3", later)
	it = fn.Seen["v1.2.3"]
	assert.Equal(t, now, it.FirstSeen, "FirstSeen should stay the same")
	assert.Equal(t, later, it.LastSeen, "LastSeen should update to later")

	removed := fn.EvictImageTag("v1.2.3")
	assert.True(t, removed, "EvictImageTag should report it removed an existing tag")
	assert.False(t, fn.HasImageTag("v1.2.3"))
	assert.True(t, fn.IsEmpty(), "After removal, NodeBase should be empty again")
}

func makeFiveTuple(srcIP string, srcPort uint16, dstIP string, dstPort uint16) seclModel.FiveTuple {
	return seclModel.FiveTuple{
		Source:      netip.AddrPortFrom(netip.MustParseAddr(srcIP), srcPort),
		Destination: netip.AddrPortFrom(netip.MustParseAddr(dstIP), dstPort),
		L4Protocol:  17,
	}
}

func makeFlow(ft seclModel.FiveTuple) seclModel.Flow {
	srcAddr := ft.Source.Addr()
	dstAddr := ft.Destination.Addr()
	
	srcIPNet := net.IPNet{IP: net.IP(srcAddr.AsSlice())}
	dstIPNet := net.IPNet{IP: net.IP(dstAddr.AsSlice())}
	
	return seclModel.Flow{
		Source:      seclModel.IPPortContext{IPNet: srcIPNet, Port: ft.Source.Port()},
		Destination: seclModel.IPPortContext{IPNet: dstIPNet, Port: ft.Destination.Port()},
		L3Protocol:  6,
		L4Protocol:  ft.L4Protocol,
		Ingress:     seclModel.NetworkStats{DataSize: 1, PacketCount: 1},
		Egress:      seclModel.NetworkStats{DataSize: 2, PacketCount: 2},
	}
}

func TestNetworkDeviceNode_TagAndEvict(t *testing.T) {
	// build two distinct flows under one device
	ft1 := makeFiveTuple("1.1.1.1", 1111, "2.2.2.2", 2222)
	ft2 := makeFiveTuple("3.3.3.3", 3333, "4.4.4.4", 4444)

	flow1 := activity_tree.NewFlowNode(makeFlow(ft1), activity_tree.Runtime, "v1.0.0")
	flow2 := activity_tree.NewFlowNode(makeFlow(ft2), activity_tree.Runtime, "v2.0.0")

	netdev := activity_tree.NewNetworkDeviceNode(&seclModel.NetworkDeviceContext{}, activity_tree.Runtime)
	netdev.FlowNodes = map[seclModel.FiveTuple]*activity_tree.FlowNode{
		ft1: flow1,
		ft2: flow2,
	}

	// attach to a ProcessNode and record both original versions
	pn := &activity_tree.ProcessNode{
		NodeBase:       activity_tree.NewNodeBase(),
		NetworkDevices: map[seclModel.NetworkDeviceContext]*activity_tree.NetworkDeviceNode{{}: netdev},
	}

	now := time.Now()
	pn.Record("v1.0.0", now)
	pn.Record("v2.0.0", now)

	// Verify initial state
	assert.Contains(t, netdev.FlowNodes, ft1, "ft1 should exist before eviction")
	assert.Contains(t, netdev.FlowNodes, ft2, "ft2 should exist before eviction")
	assert.True(t, netdev.FlowNodes[ft1].HasImageTag("v1.0.0"), "flow1 should have v1.0.0")
	assert.True(t, netdev.FlowNodes[ft2].HasImageTag("v2.0.0"), "flow2 should have v2.0.0")

	// Verify ProcessNode has the tags
	assert.True(t, pn.HasImageTag("v1.0.0"), "ProcessNode should have v1.0.0")
	assert.True(t, pn.HasImageTag("v2.0.0"), "ProcessNode should have v2.0.0")

	// Evict v1.0.0 → should drop only flow1 (which becomes empty)
	removed := pn.EvictImageTag("v1.0.0", nil, make(map[int]int))
	assert.False(t, removed, "device still has flow2 so should not be removed")
	assert.NotContains(t, netdev.FlowNodes, ft1, "ft1 should be removed after evicting v1.0.0")
	assert.Contains(t, netdev.FlowNodes, ft2, "ft2 should remain after evicting v1.0.0")
}

func TestNodeBase_DNSNode_TTL_Eviction(t *testing.T) {
	// Create a ProcessNode with DNSNames
	processNode := &activity_tree.ProcessNode{
		NodeBase: activity_tree.NewNodeBase(),
		DNSNames: make(map[string]*activity_tree.DNSNode),
	}

	now := time.Now()
	old := now.Add(-3 * time.Hour)
	recent := now.Add(-30 * time.Minute)

	// Create a DNSNode with first tag "old"
	dnsEvt := &seclModel.DNSEvent{
		Question: seclModel.DNSQuestion{Name: "example.com", Type: 1, Class: 1},
	}
	dns := activity_tree.NewDNSNode(dnsEvt, nil, activity_tree.Runtime, "old")

	// Add DNSNode to ProcessNode
	processNode.DNSNames["example.com"] = dns

	// Force its timestamp to 'old'
	dns.RecordWithTimestamps("old", old, old)
	// Append a fresh tag "new"
	dns.Record("new", recent)

	assert.True(t, dns.HasImageTag("old"))
	assert.True(t, dns.HasImageTag("new"))

	// Evict tags last‐seen before 1h ago → should remove "old"
	removedOld := dns.EvictBeforeTimestamp(now.Add(-1 * time.Hour))
	assert.Equal(t, 1, removedOld)
	assert.False(t, dns.HasImageTag("old"))
	assert.True(t, dns.HasImageTag("new"))
	assert.False(t, dns.IsEmpty())

	// Evict everything older than just after now → should remove "new"
	removedNew := dns.EvictBeforeTimestamp(now.Add(1 * time.Minute))
	assert.Equal(t, 1, removedNew)
	assert.False(t, dns.HasImageTag("new"))
	assert.True(t, dns.IsEmpty())
}