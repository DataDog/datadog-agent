// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package processresolver holds processresolver related files
package processresolver

import (
	"github.com/DataDog/datadog-go/v5/statsd"

	processlist "github.com/DataDog/datadog-agent/pkg/security/process_list"
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

type processKey struct {
	pid  uint32
	nsid uint64
}

type execKey struct {
	pid  uint32
	tid  uint32
	nsid uint64
}

// IsValidRootNode evaluates if the provided process entry is allowed to become a root node of an Activity Dump
func (at *ProcessResolver) IsAValidRootNode(entry *model.Process) bool {
	return entry.Pid == 1
}

func (at *ProcessResolver) ExecMatches(e1, e2 *processlist.ExecNode) bool {
	return e1.Pid == e2.Pid && e1.NSID == e2.NSID && e1.Tid == e2.Tid
}

func (at *ProcessResolver) ProcessMatches(p1, p2 *processlist.ProcessNode) bool {
	return p1.CurrentExec.Pid == p2.CurrentExec.Pid && p1.CurrentExec.NSID == p2.CurrentExec.NSID
}

// SendStats sends the tree statistics
func (at *ProcessResolver) SendStats(client statsd.ClientInterface) error {
	// TODO
	return nil
}

func (at *ProcessResolver) GetProcessCacheKey(process *model.Process) interface{} {
	return processKey{pid: process.Pid, nsid: process.NSID}
}

func (at *ProcessResolver) GetExecCacheKey(process *model.Process) interface{} {
	return execKey{pid: process.Pid, tid: process.Tid, nsid: process.NSID}
}

func (at *ProcessResolver) GetParentProcessCacheKey(event *model.Event) interface{} {
	if event.ProcessContext.Pid != 1 {
		return processKey{pid: event.ProcessContext.PPid, nsid: event.ProcessContext.NSID}
	}
	return nil
}
