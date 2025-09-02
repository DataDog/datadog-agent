// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import "time"

// CapabilityNode stores capabilities usage information for a process in the activity tree.
type CapabilityNode struct {
	NodeBase
	GenerationType NodeGenerationType

	Capability uint64 // The capability number
	Capable    bool   // Whether the process was capable of using the capability
}

// NewCapabilityNode creates a new CapabilityNode
func NewCapabilityNode(capability uint64, capable bool, timestamp time.Time, imageTag string, generationType NodeGenerationType) *CapabilityNode {
	nodeBase := NewNodeBase()
	nodeBase.AppendImageTag(imageTag, timestamp)

	return &CapabilityNode{
		NodeBase:       nodeBase,
		GenerationType: generationType,

		Capability: capability,
		Capable:    capable,
	}
}
