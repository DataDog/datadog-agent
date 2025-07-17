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
	NodeBase
	GenerationType NodeGenerationType
	Flow           model.Flow
}

// NewFlowNode returns a new FlowNode instance
func NewFlowNode(flow model.Flow, event *model.Event, generationType NodeGenerationType, imageTag string) *FlowNode {
	node := &FlowNode{
		GenerationType: generationType,
		Flow:           flow,
	}
	node.NodeBase = NewNodeBase()
	node.AppendImageTag(imageTag, event.ResolveEventTime())
	return node
}

func (node *FlowNode) addFlow(flow model.Flow, event *model.Event, imageTag string) {
	node.AppendImageTag(imageTag, event.ResolveEventTime())

	// add metrics
	node.Flow.Egress.Add(flow.Egress)
	node.Flow.Ingress.Add(flow.Ingress)

}
