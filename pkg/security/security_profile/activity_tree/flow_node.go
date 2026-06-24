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
)

// FlowNode is used to store a flow node
type FlowNode struct {
	NodeBase
	GenerationType NodeGenerationType
	Flow           model.Flow
}

// size approximates this node's heap footprint
func (fn *FlowNode) size() int64 {
	return int64(unsafe.Sizeof(*fn)) + seenBytes(fn.NodeBase)
}

// NewFlowNode returns a new FlowNode instance
func NewFlowNode(flow model.Flow, event *model.Event, generationType NodeGenerationType, imageTagID uint64) *FlowNode {
	node := &FlowNode{
		GenerationType: generationType,
		Flow:           flow,
	}
	node.NodeBase = NewNodeBase()
	node.AppendImageTagID(imageTagID, event.ResolveEventTime())
	return node
}

func (fn *FlowNode) addFlow(flow model.Flow, event *model.Event, imageTagID uint64) {
	fn.AppendImageTagID(imageTagID, event.ResolveEventTime())

	// add metrics
	fn.Flow.Egress.Add(flow.Egress)
	fn.Flow.Ingress.Add(flow.Ingress)
}
