// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"time"
)

// SyscallNode is used to store a syscall node
type SyscallNode struct {
	NodeBase
	GenerationType NodeGenerationType
	Syscall        int
}

// NewSyscallNode returns a new SyscallNode instance
func NewSyscallNode(syscall int, timestamp time.Time, imageTag string, generationType NodeGenerationType) *SyscallNode {
	node := &SyscallNode{
		Syscall:        syscall,
		GenerationType: generationType,
	}
	node.NodeBase = NewNodeBase()
	node.AppendImageTag(imageTag, timestamp)
	return node
}
