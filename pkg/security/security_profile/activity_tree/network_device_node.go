// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// NetworkDeviceNode is used to store a Network Device node
type NetworkDeviceNode struct {
	MatchedRules   []*model.MatchedRule
	GenerationType NodeGenerationType
	Context        model.NetworkDeviceContext
	// FlowNodes are indexed by source IPPortContexts
	FlowNodes map[model.FiveTuple]*FlowNode
}

// size approximates this node's own heap footprint
func (netdevice *NetworkDeviceNode) size() int64 {
	s := int64(unsafe.Sizeof(*netdevice))
	s += fixedKeyMapBytes(netdevice.FlowNodes)
	s += sliceBackingBytes(cap(netdevice.MatchedRules), unsafe.Sizeof((*model.MatchedRule)(nil)))
	return s
}

// NewNetworkDeviceNode returns a new NetworkDeviceNode instance
func NewNetworkDeviceNode(ctx *model.NetworkDeviceContext, generationType NodeGenerationType) *NetworkDeviceNode {
	node := &NetworkDeviceNode{
		GenerationType: generationType,
		Context:        *ctx,
		FlowNodes:      make(map[model.FiveTuple]*FlowNode),
	}
	return node
}

func (netdevice *NetworkDeviceNode) appendImageTag(imageTagID uint64, timestamp time.Time) {
	for _, flow := range netdevice.FlowNodes {
		flow.AppendImageTagID(imageTagID, timestamp)
	}
}

func (netdevice *NetworkDeviceNode) evictImageTag(imageTagID uint64) (bool, int64) {
	var removed int64
	for key, flow := range netdevice.FlowNodes {
		if flow.EvictImageTag(imageTagID) {
			removed += flow.size()
			delete(netdevice.FlowNodes, key)
		}
	}
	return len(netdevice.FlowNodes) == 0, removed
}

func (netdevice *NetworkDeviceNode) insertNetworkFlowMonitorEvent(event *model.NetworkFlowMonitorEvent, evt *model.Event, dryRun bool, rules []*model.MatchedRule, generationType NodeGenerationType, imageTagID uint64, stats *Stats) bool {
	if len(rules) > 0 {
		netdevice.MatchedRules = model.AppendMatchedRule(netdevice.MatchedRules, rules)
	}

	var newFlow bool
	for _, flow := range event.Flows {
		existingNode, ok := netdevice.FlowNodes[flow.GetFiveTuple()]
		if ok {
			if !dryRun {
				existingNode.addFlow(flow, evt, imageTagID)
			}
		} else {
			newFlow = true
			if dryRun {
				// exit early
				return newFlow
			}
			// create new entry
			flowNode := NewFlowNode(flow, evt, generationType, imageTagID)
			netdevice.FlowNodes[flow.GetFiveTuple()] = flowNode
			stats.FlowNodes++
			stats.SizeBytes += flowNode.size()
		}
	}

	return newFlow
}
