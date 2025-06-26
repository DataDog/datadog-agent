// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"time"

	processlist "github.com/DataDog/datadog-agent/pkg/security/process_list"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// FlowNode is used to store a flow node
type FlowNode struct {
	processlist.NodeBase
	ImageTags      []string
	GenerationType NodeGenerationType
	Flow           model.Flow
}

// NewFlowNode returns a new FlowNode instance
func NewFlowNode(flow model.Flow, generationType NodeGenerationType, imageTag string) *FlowNode {
	now := time.Now()
	node := &FlowNode{
		GenerationType: generationType,
		Flow:           flow,
	}
	node.NodeBase = processlist.NewNodeBase()
	node.Record(imageTag, now)
	node.appendImageTag(imageTag)
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

func (node *FlowNode) addFlow(flow model.Flow, imageTag string) {
	if imageTag != "" {
		node.appendImageTag(imageTag)
	}

	// add metrics
	node.Flow.Egress.Add(flow.Egress)
	node.Flow.Ingress.Add(flow.Ingress)
	// update timestamps
	node.updateTimes(imageTag)
}

func (node *FlowNode) updateTimes(imageTag string) {
	now := time.Now()
	node.Record(imageTag, now)
}
