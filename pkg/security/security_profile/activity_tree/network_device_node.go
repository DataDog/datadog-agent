// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"time"

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

// NewNetworkDeviceNode returns a new NetworkDeviceNode instance
func NewNetworkDeviceNode(ctx *model.NetworkDeviceContext, generationType NodeGenerationType) *NetworkDeviceNode {
	node := &NetworkDeviceNode{
		GenerationType: generationType,
		Context:        *ctx,
		FlowNodes:      make(map[model.FiveTuple]*FlowNode),
	}
	return node
}

func (netdevice *NetworkDeviceNode) appendImageTag(imageTag string, timestamp time.Time) {
	for _, flow := range netdevice.FlowNodes {
		flow.AppendImageTag(imageTag, timestamp)
	}
}

func (netdevice *NetworkDeviceNode) evictImageTag(imageTag string) bool {
	for key, flow := range netdevice.FlowNodes {
		if flow.EvictImageTag(imageTag) {
			delete(netdevice.FlowNodes, key)
		}
	}

	return len(netdevice.FlowNodes) == 0
}

func (netdevice *NetworkDeviceNode) insertNetworkFlowMonitorEvent(event *model.NetworkFlowMonitorEvent, evt *model.Event, dryRun bool, rules []*model.MatchedRule, generationType NodeGenerationType, imageTag string, stats *Stats) bool {
	if len(rules) > 0 {
		netdevice.MatchedRules = model.AppendMatchedRule(netdevice.MatchedRules, rules)
	}

	var newFlow bool
	for _, flow := range event.Flows {
		existingNode, ok := netdevice.FlowNodes[flow.GetFiveTuple()]
		if ok {
			if !dryRun {
				existingNode.addFlow(flow, evt, imageTag)
			}
		} else {
			newFlow = true
			if dryRun {
				// exit early
				return newFlow
			}
			// create new entry
			netdevice.FlowNodes[flow.GetFiveTuple()] = NewFlowNode(flow, evt, generationType, imageTag)
			stats.FlowNodes++
		}
	}

	return newFlow
}
