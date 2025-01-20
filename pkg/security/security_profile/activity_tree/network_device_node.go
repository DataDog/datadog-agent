// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// NetworkDeviceNode is used to store a Network Device node
type NetworkDeviceNode struct {
	MatchedRules   []*model.MatchedRule
	GenerationType NodeGenerationType

	Context model.NetworkDeviceContext

	// FlowNodes are indexed by source IPPortContexts
	FlowNodes map[model.IPPortContextComparable]*FlowNode
}

// NewNetworkDeviceNode returns a new NetworkDeviceNode instance
func NewNetworkDeviceNode(ctx *model.NetworkDeviceContext, generationType NodeGenerationType) *NetworkDeviceNode {
	node := &NetworkDeviceNode{
		GenerationType: generationType,
		Context:        *ctx,
		FlowNodes:      make(map[model.IPPortContextComparable]*FlowNode),
	}
	return node
}

func (netdevice *NetworkDeviceNode) appendImageTag(imageTag string) {
	for _, flow := range netdevice.FlowNodes {
		flow.appendImageTag(imageTag)
	}
}

func (netdevice *NetworkDeviceNode) evictImageTag(imageTag string) bool {
	for key, flow := range netdevice.FlowNodes {
		if shouldRemove := flow.evictImageTag(imageTag); !shouldRemove {
			delete(netdevice.FlowNodes, key)
		}
	}

	return len(netdevice.FlowNodes) == 0
}

func (netdevice *NetworkDeviceNode) insertNetworkFlowMonitorEvent(event *model.NetworkFlowMonitorEvent, dryRun bool, rules []*model.MatchedRule, generationType NodeGenerationType, imageTag string, stats *Stats) bool {
	if len(rules) > 0 {
		netdevice.MatchedRules = model.AppendMatchedRule(netdevice.MatchedRules, rules)
	}

	var newFlow bool
	for _, flow := range event.Flows {
		existingNode, ok := netdevice.FlowNodes[flow.Source.GetComparable()]
		if ok {
			newFlow = newFlow || existingNode.insertFlow(flow, dryRun, imageTag, stats)
			if newFlow && dryRun {
				// exit early
				return newFlow
			}
		} else {
			newFlow = true
			if dryRun {
				// exit early
				return newFlow
			}
			// create new entry
			netdevice.FlowNodes[flow.Source.GetComparable()] = NewFlowNode(flow, generationType, imageTag, stats)
		}
	}

	return newFlow
}
