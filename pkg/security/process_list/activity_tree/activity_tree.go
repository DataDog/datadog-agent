// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"github.com/DataDog/datadog-go/v5/statsd"

	processlist "github.com/DataDog/datadog-agent/pkg/security/process_list"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Stats represents the node counts in an activity dump
type Stats struct {
	// TODO
	Hit          int64
	Misses       int64
	ProcFallback int64
	Suppressed   int64
	Anomaly      int64
}

// ActivityTree contains a process tree and its activities. This structure has no locks.
type ActivityTree struct {
	Stats *Stats
	// pathsReducer *PathsReducer

	differentiateArgs bool
	DNSMatchMaxDepth  int

	// top level lists used to summarize the content of the tree
	DNSNames     *utils.StringKeys
	SyscallsMask map[int]int
}

// NewActivityTree returns a new ActivityTree instance
func NewActivityTree( /* pathsReducer *PathsReducer */ ) *ActivityTree {
	return &ActivityTree{
		// pathsReducer: pathsReducer,
		Stats:        &Stats{},
		DNSNames:     utils.NewStringKeys(nil),
		SyscallsMask: make(map[int]int),
	}
}

// IsValidRootNode evaluates if the provided process entry is allowed to become a root node of an Activity Dump
func IsValidRootNode(entry *model.Process) bool {
	// TODO
	return true
}

func (at *ActivityTree) Matches(p1, p2 *processlist.ExecNode) bool {
	// TODO
	return true
}

// SendStats sends the tree statistics
func (at *ActivityTree) SendStats(client statsd.ClientInterface) error {
	return nil
	// return at.Stats.SendStats(client, at.treeType)
}
