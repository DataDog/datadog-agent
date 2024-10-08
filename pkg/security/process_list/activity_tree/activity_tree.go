// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"strings"

	"github.com/DataDog/datadog-go/v5/statsd"
	"golang.org/x/exp/slices"

	processlist "github.com/DataDog/datadog-agent/pkg/security/process_list"
	processresolver "github.com/DataDog/datadog-agent/pkg/security/process_list/process_resolver"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activitytree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
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
	Stats        *Stats
	pathsReducer *activitytree.PathsReducer

	differentiateArgs bool
	DNSMatchMaxDepth  int

	// top level lists used to summarize the content of the tree
	DNSNames     *utils.StringKeys
	SyscallsMask map[int]int
}

// NewActivityTree returns a new ActivityTree instance
func NewActivityTree(pathsReducer *activitytree.PathsReducer, differentiateArgs bool, DNSMatchMaxDepth int) *ActivityTree {
	return &ActivityTree{
		pathsReducer:      pathsReducer,
		Stats:             &Stats{},
		DNSNames:          utils.NewStringKeys(nil),
		SyscallsMask:      make(map[int]int),
		differentiateArgs: differentiateArgs,
		DNSMatchMaxDepth:  DNSMatchMaxDepth,
	}
}

// /!\ for now, everything related to cache funcs are equal to the process resolver
// TODO: if it's still the case when starting to replace parts of original code, remove
// it from the interface and make it generic

type processKey struct {
	pid  uint32
	nsid uint64
}

type execKey struct {
	pid         uint32
	nsid        uint64
	execTime    int64
	pathnameStr string
}

// GetProcessCacheKey returns the process unique identifier
func (at *ActivityTree) GetProcessCacheKey(process *model.Process) interface{} {
	if process.Pid != 0 {
		return processKey{pid: process.Pid, nsid: process.NSID}
	}
	return nil
}

// GetExecCacheKey returns the exec unique identifier
func (at *ActivityTree) GetExecCacheKey(process *model.Process) interface{} {
	if process.Pid != 0 {
		path := process.FileEvent.PathnameStr
		if processresolver.IsBusybox(process.FileEvent.PathnameStr) {
			path = process.Argv[0]
		}
		return execKey{
			pid:         process.Pid,
			nsid:        process.NSID,
			execTime:    process.ExecTime.UnixMicro(),
			pathnameStr: path,
		}
	}
	return nil
}

// GetParentProcessCacheKey returns the parent process unique identifier
func (at *ActivityTree) GetParentProcessCacheKey(process *model.Process) interface{} {
	if process.Pid != 1 && process.PPid > 0 {
		// will be ok on most cases (where the parent PID namespace ID is the same), but
		// sometime this will fail and triggers a procfs fallback
		return processKey{pid: process.PPid, nsid: process.NSID}
	}
	return nil
}

// IsAValidRootNode evaluates if the provided process entry is allowed to become a root node of an Activity Dump
// nolint: all
func (at *ActivityTree) IsAValidRootNode(entry *model.ProcessContext) bool {
	// an ancestor is required
	ancestor := GetNextAncestorBinaryOrArgv0(entry)
	if ancestor == nil {
		return false
	}

	if entry.FileEvent.IsFileless() {
		// a fileless node is a valid root node only if not having runc as parent
		// ex: runc -> exec(fileless) -> init.sh; exec(fileless) is not a valid root node
		return !(isContainerRuntimePrefix(ancestor.FileEvent.BasenameStr) || isContainerRuntimePrefix(entry.FileEvent.BasenameStr))
	}

	// container runtime prefixes are not valid root nodes
	return !isContainerRuntimePrefix(entry.FileEvent.BasenameStr)
}

func isContainerRuntimePrefix(basename string) bool {
	return strings.HasPrefix(basename, "runc") || strings.HasPrefix(basename, "containerd-shim")
}

// GetNextAncestorBinaryOrArgv0 returns the first ancestor with a different binary, or a different argv0 in the case of busybox processes
func GetNextAncestorBinaryOrArgv0(entry *model.ProcessContext) *model.ProcessContext {
	if entry == nil {
		return nil
	}
	current := entry
	ancestor := entry.Ancestor
	for ancestor != nil {
		if current.FileEvent.PathnameStr != ancestor.FileEvent.PathnameStr {
			return &ancestor.ProcessContext
		}
		if process.IsBusybox(current.FileEvent.PathnameStr) && process.IsBusybox(ancestor.FileEvent.PathnameStr) {
			currentArgv0, _ := process.GetProcessArgv0(&current.Process)
			if len(currentArgv0) == 0 {
				return nil
			}
			ancestorArgv0, _ := process.GetProcessArgv0(&ancestor.Process)
			if len(ancestorArgv0) == 0 {
				return nil
			}
			if currentArgv0 != ancestorArgv0 {
				return &ancestor.ProcessContext
			}
		}
		current = &ancestor.ProcessContext
		ancestor = ancestor.Ancestor
	}
	return nil
}

// ExecMatches returns true if both exec matches
func (at *ActivityTree) ExecMatches(e1, e2 *processlist.ExecNode) bool {
	if e1.FileEvent.PathnameStr == e2.FileEvent.PathnameStr {
		if at.differentiateArgs {
			return slices.Equal(e1.Process.Argv, e2.Process.Argv)
		}
		return true
	}
	return false
}

// ProcessMatches returns true if both process nodes matches
func (at *ActivityTree) ProcessMatches(p1, p2 *processlist.ProcessNode) bool {
	if p1.CurrentExec == nil || p2.CurrentExec == nil {
		return false
	}
	return at.ExecMatches(p1.CurrentExec, p2.CurrentExec)
}

// SendStats sends the tree statistics
// nolint: all
func (at *ActivityTree) SendStats(client statsd.ClientInterface) error {
	// TODO
	return nil
}
