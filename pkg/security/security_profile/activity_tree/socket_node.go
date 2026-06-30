// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// BindNode is used to store a bind node
type BindNode struct {
	NodeBase

	MatchedRules []*model.MatchedRule

	GenerationType NodeGenerationType
	Port           uint16
	IP             string
	Protocol       uint16
}

// ConnectNode is used to store a connect node
type ConnectNode struct {
	NodeBase

	MatchedRules []*model.MatchedRule

	GenerationType NodeGenerationType
	Port           uint16
	IP             string
	Protocol       uint16
}

// Matches returns true if ConnectNodes matches
func (cn *ConnectNode) Matches(toMatch *ConnectNode) bool {
	return cn.Port == toMatch.Port && cn.IP == toMatch.IP && cn.Protocol == toMatch.Protocol
}

// SocketNode is used to store a Socket node and associated events
type SocketNode struct {
	NodeBase
	Family         string
	GenerationType NodeGenerationType
	Bind           []*BindNode
	Connect        []*ConnectNode
}

// size approximates this node's heap footprint, including all owned BindNodes.
// SocketNode's children (BindNodes) are not separately walked by the activity-tree
// size accounting (recomputeSizeBytes/processNodeOwnActivitySize), so they are folded
// in here. Incremental insert/evict paths must use bindSize() for individual binds
// and only charge sn.size() at socket creation time.
func (sn *SocketNode) size() int64 {
	s := int64(unsafe.Sizeof(*sn))
	s += seenBytes(sn.NodeBase)
	s += int64(len(sn.Family))
	s += sliceBackingBytes(cap(sn.Bind), unsafe.Sizeof((*BindNode)(nil)))
	for _, bind := range sn.Bind {
		s += bindSize(bind)
	}
	s += sliceBackingBytes(cap(sn.Connect), unsafe.Sizeof((*ConnectNode)(nil)))
	for _, conn := range sn.Connect {
		s += connectSize(conn)
	}
	return s
}

// bindSize approximates the heap footprint of a single BindNode: struct overhead, the
// IP string, the MatchedRules backing slice, and the NodeBase.seen slice.
func bindSize(bn *BindNode) int64 {
	if bn == nil {
		return 0
	}
	s := int64(unsafe.Sizeof(*bn))
	s += seenBytes(bn.NodeBase)
	s += int64(len(bn.IP))
	s += sliceBackingBytes(cap(bn.MatchedRules), unsafe.Sizeof((*model.MatchedRule)(nil)))
	return s
}

func connectSize(cn *ConnectNode) int64 {
	if cn == nil {
		return 0
	}
	s := int64(unsafe.Sizeof(*cn))
	s += seenBytes(cn.NodeBase)
	s += int64(len(cn.IP))
	s += sliceBackingBytes(cap(cn.MatchedRules), unsafe.Sizeof((*model.MatchedRule)(nil)))
	return s
}

// Matches returns true if BindNodes matches
func (bn *BindNode) Matches(toMatch *BindNode) bool {
	return bn.Port == toMatch.Port && bn.IP == toMatch.IP && bn.Protocol == toMatch.Protocol
}

// Matches returns true if SocketNodes matches
func (sn *SocketNode) Matches(toMatch *SocketNode) bool {
	return sn.Family == toMatch.Family
}

// evictImageTag removes the imageTag from each owned BindNode, dropping binds that have
// no remaining tags. Returns (socketIsEmpty, bytesRemoved) where bytesRemoved is the sum
// of bindSize() for every dropped BindNode — the caller subtracts this from Stats.SizeBytes
// and additionally subtracts sn.size() if the socket itself ends up empty.
func (sn *SocketNode) evictImageTag(imageTagID uint64) (bool, int64) {
	var removed int64

	newBind := sn.Bind[:0]
	for _, bind := range sn.Bind {
		if bind.EvictImageTag(imageTagID) {
			removed += bindSize(bind)
			continue
		}
		newBind = append(newBind, bind)
	}
	clear(sn.Bind[len(newBind):])
	sn.Bind = newBind

	newConnect := sn.Connect[:0]
	for _, conn := range sn.Connect {
		if conn.EvictImageTag(imageTagID) {
			removed += connectSize(conn)
			continue
		}
		newConnect = append(newConnect, conn)
	}
	clear(sn.Connect[len(newConnect):])
	sn.Connect = newConnect

	return len(newBind) == 0 && len(newConnect) == 0, removed
}

// InsertBindEvent inserts a bind event inside a socket node. When a new BindNode is
// created the caller-provided stats is charged its size, keeping Stats.SizeBytes honest
// for bind-heavy workloads where the previous accounting only charged the socket once
// at creation time and ignored subsequent binds. stats must be non-nil — same contract as
// every other Insert*Event method.
func (sn *SocketNode) InsertBindEvent(evt *model.BindEvent, event *model.Event, imageTagID uint64, generationType NodeGenerationType, rules []*model.MatchedRule, stats *Stats, dryRun bool) bool {
	evtIP := utils.GetIPStringFromIPNet(evt.Addr.IPNet)
	for _, n := range sn.Bind {
		if evt.Addr.Port == n.Port && evtIP == n.IP && evt.Protocol == n.Protocol {
			if !dryRun {
				n.MatchedRules = model.AppendMatchedRule(n.MatchedRules, rules)
			}
			if imageTagID == 0 || n.HasImageTag(imageTagID) {
				return false
			}
			n.AppendImageTagID(imageTagID, event.ResolveEventTime())
			return false
		}
	}

	if !dryRun {
		// insert bind event now
		node := &BindNode{
			MatchedRules:   rules,
			GenerationType: generationType,
			Port:           evt.Addr.Port,
			IP:             evtIP,
			Protocol:       evt.Protocol,
		}
		node.NodeBase = NewNodeBase()

		node.AppendImageTagID(imageTagID, event.ResolveEventTime())
		sn.Bind = append(sn.Bind, node)
		stats.SizeBytes += bindSize(node)
	}
	return true
}

// InsertConnectEvent inserts a connect event inside a socket node.
func (sn *SocketNode) InsertConnectEvent(evt *model.ConnectEvent, event *model.Event, imageTagID uint64, generationType NodeGenerationType, rules []*model.MatchedRule, stats *Stats, dryRun bool) bool {
	evtIP := utils.GetIPStringFromIPNet(evt.Addr.IPNet)
	for _, n := range sn.Connect {
		if evt.Addr.Port == n.Port && evtIP == n.IP && evt.Protocol == n.Protocol {
			if !dryRun {
				n.MatchedRules = model.AppendMatchedRule(n.MatchedRules, rules)
			}
			if imageTagID == 0 || n.HasImageTag(imageTagID) {
				return false
			}
			n.AppendImageTagID(imageTagID, event.ResolveEventTime())
			return false
		}
	}

	if !dryRun {
		node := &ConnectNode{
			MatchedRules:   rules,
			GenerationType: generationType,
			Port:           evt.Addr.Port,
			IP:             evtIP,
			Protocol:       evt.Protocol,
		}
		node.NodeBase = NewNodeBase()
		node.AppendImageTagID(imageTagID, event.ResolveEventTime())
		sn.Connect = append(sn.Connect, node)
		stats.SizeBytes += connectSize(node)
	}
	return true
}

// NewSocketNode returns a new SocketNode instance
func NewSocketNode(family string, generationType NodeGenerationType) *SocketNode {
	node := &SocketNode{
		Family:         family,
		GenerationType: generationType,
	}
	node.NodeBase = NewNodeBase()
	return node
}
