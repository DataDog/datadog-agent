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

// SocketNode is used to store a Socket node and associated events
type SocketNode struct {
	NodeBase
	Family         string
	GenerationType NodeGenerationType
	Bind           []*BindNode
}

// size approximates this node's heap footprint, including all owned BindNodes.
// Unlike the *Node siblings, SocketNode's children (BindNodes) are not separately walked
// by the activity-tree size accounting, so they are folded in here. We count the Bind
// slice's backing array plus each BindNode's struct, IP string, MatchedRules backing
// array, and NodeBase.seen slice.
func (sn *SocketNode) size() int64 {
	s := int64(unsafe.Sizeof(*sn))
	s += seenBytes(sn.NodeBase)
	s += int64(len(sn.Family))
	s += sliceBackingBytes(cap(sn.Bind), unsafe.Sizeof((*BindNode)(nil)))
	for _, bind := range sn.Bind {
		if bind == nil {
			continue
		}
		s += int64(unsafe.Sizeof(*bind))
		s += seenBytes(bind.NodeBase)
		s += int64(len(bind.IP))
		s += sliceBackingBytes(cap(bind.MatchedRules), unsafe.Sizeof((*model.MatchedRule)(nil)))
	}
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

func (sn *SocketNode) evictImageTag(imageTagID uint64) bool {
	newBind := []*BindNode{}
	for _, bind := range sn.Bind {
		if shouldRemoveNode := bind.EvictImageTag(imageTagID); !shouldRemoveNode {
			newBind = append(newBind, bind)
		}
	}
	if len(newBind) == 0 {
		return true
	}
	sn.Bind = newBind
	return false
}

// InsertBindEvent inserts a bind even inside a socket node
func (sn *SocketNode) InsertBindEvent(evt *model.BindEvent, event *model.Event, imageTagID uint64, generationType NodeGenerationType, rules []*model.MatchedRule, dryRun bool) bool {
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
