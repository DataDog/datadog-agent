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

// IMDSNode is used to store a IMDS node
type IMDSNode struct {
	NodeBase
	MatchedRules   []*model.MatchedRule
	GenerationType NodeGenerationType
	Event          model.IMDSEvent
}

// size returns the shallow heap size of this node.
func (in *IMDSNode) size() int64 {
	s := int64(unsafe.Sizeof(*in))
	s += int64(len(in.Event.CloudProvider))
	s += int64(len(in.Event.Type))
	s += int64(len(in.Event.URL))
	return s
}

// NewIMDSNode creates a new IMDSNode instance
func NewIMDSNode(event *model.IMDSEvent, evt *model.Event, rules []*model.MatchedRule, generationType NodeGenerationType, imageTagID uint64) *IMDSNode {
	node := &IMDSNode{
		MatchedRules:   rules,
		GenerationType: generationType,
		Event:          *event,
	}
	node.NodeBase = NewNodeBase()
	node.AppendImageTagID(imageTagID, evt.ResolveEventTime())

	return node
}
