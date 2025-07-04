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

// IMDSNode is used to store a IMDS node
type IMDSNode struct {
	NodeBase
	MatchedRules   []*model.MatchedRule
	GenerationType NodeGenerationType
	Event          model.IMDSEvent
}

func NewIMDSNode(event *model.IMDSEvent, evt *model.Event, rules []*model.MatchedRule, generationType NodeGenerationType, imageTag string) *IMDSNode {
	node := &IMDSNode{
		MatchedRules:   rules,
		GenerationType: generationType,
		Event:          *event,
	}
	node.NodeBase = NewNodeBase()
	node.AppendImageTag(imageTag, evt.ResolveEventTime())
	
	return node
}

func (imds *IMDSNode) evictImageTag(imageTag string) bool {
	imds.EvictImageTag(imageTag)
	if imds.IsEmpty() {
		return true
	}
	return false
}