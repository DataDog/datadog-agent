// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package processresolver holds processresolver related files
package processresolver

import (
	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
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

// ProcessResolver contains a process tree and its activities. This structure has no locks.
type ProcessResolver struct {
	Stats *Stats
}

// NewProcessResolver returns a new ProcessResolver instance
func NewProcessResolver() *ProcessResolver {
	return &ProcessResolver{
		Stats: &Stats{},
	}
}

func (at *ProcessResolver) GetCacheKeyFromExec(exec *ExecNode) interface{} {
	return exec.pid
}

// IsValidRootNode evaluates if the provided process entry is allowed to become a root node of an Activity Dump
func (at *ProcessResolver) IsValidRootNode(entry *model.Process) bool {
	// TODO
	return true
}

func (at *ProcessResolver) Matches(p1, p2 *ExecNode) bool {
	return p1.pid == p2.pid
}

// SendStats sends the tree statistics
func (at *ProcessResolver) SendStats(client statsd.ClientInterface) error {
	// TODO
	return nil
}
