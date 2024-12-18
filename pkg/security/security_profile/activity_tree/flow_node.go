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

// FlowNode is used to store a flow node
type FlowNode struct {
	ImageTags      []string
	GenerationType NodeGenerationType

	// Flows are indexed by destination IPPortContext
	Flows map[model.IPPortContextComparable]*model.Flow
}

// NewFlowNode returns a new FlowNode instance
func NewFlowNode(flow model.Flow, generationType NodeGenerationType, imageTag string, stats *Stats) *FlowNode {
	node := &FlowNode{
		GenerationType: generationType,
		Flows:          make(map[model.IPPortContextComparable]*model.Flow),
	}

	node.insertFlow(flow, false, imageTag, stats)

	return node
}

func (node *FlowNode) appendImageTag(imageTag string) {
	node.ImageTags, _ = AppendIfNotPresent(node.ImageTags, imageTag)
}

func (node *FlowNode) evictImageTag(imageTag string) bool {
	imageTags, removed := removeImageTagFromList(node.ImageTags, imageTag)
	if removed {
		if len(imageTags) == 0 {
			return true
		}
		node.ImageTags = imageTags
	}
	return false
}

func (node *FlowNode) insertFlow(flow model.Flow, dryRun bool, imageTag string, stats *Stats) bool {
	if imageTag != "" {
		node.appendImageTag(imageTag)
	}

	var newFlow bool
	existingFlow, ok := node.Flows[flow.Destination.GetComparable()]
	if ok {
		// add metrics
		existingFlow.Egress.Add(flow.Egress)
		existingFlow.Ingress.Add(flow.Ingress)
	} else {
		// create new entry
		newFlow = true
		if dryRun {
			// exit early
			return newFlow
		}
		node.Flows[flow.Destination.GetComparable()] = &flow
		stats.FlowNodes++
	}

	return newFlow
}
