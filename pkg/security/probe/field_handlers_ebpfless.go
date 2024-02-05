// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	sprocess "github.com/DataDog/datadog-agent/pkg/security/resolvers/process"

	"github.com/DataDog/datadog-agent/pkg/security/secl/args"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// EBPFLessFieldHandlers defines a field handlers
type EBPFLessFieldHandlers struct {
	// TODO(safchain) remove this when support for multiple platform with the same build tags is available
	// keeping it can be dangerous as it can hide non implemented handlers
	model.DefaultFieldHandlers

	resolvers *resolvers.EBPFLessResolvers
}

// GetProcessService returns the service tag based on the process context
func (fh *EBPFLessFieldHandlers) GetProcessService(ev *model.Event) string {
	entry, _ := fh.ResolveProcessCacheEntry(ev)
	if entry == nil {
		return ""
	}
	return getProcessService(entry)
}

// ResolveProcessCacheEntry queries the ProcessResolver to retrieve the ProcessContext of the event
func (fh *EBPFLessFieldHandlers) ResolveProcessCacheEntry(ev *model.Event) (*model.ProcessCacheEntry, bool) {
	if ev.ProcessCacheEntry == nil && ev.PIDContext.Pid != 0 {
		ev.ProcessCacheEntry = fh.resolvers.ProcessResolver.Resolve(sprocess.CacheResolverKey{
			Pid:  ev.PIDContext.Pid,
			NSID: ev.NSID,
		})
	}

	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = model.GetPlaceholderProcessCacheEntry(ev.PIDContext.Pid, ev.PIDContext.Pid, false)
		return ev.ProcessCacheEntry, false
	}

	return ev.ProcessCacheEntry, true
}

// ResolveFilePath resolves the inode to a full path
func (fh *EBPFLessFieldHandlers) ResolveFilePath(_ *model.Event, f *model.FileEvent) string {
	return f.PathnameStr
}

// ResolveFileBasename resolves the inode to a full path
func (fh *EBPFLessFieldHandlers) ResolveFileBasename(_ *model.Event, f *model.FileEvent) string {
	return f.BasenameStr
}

// ResolveProcessArgsFlags resolves the arguments flags of the event
func (fh *EBPFLessFieldHandlers) ResolveProcessArgsFlags(ev *model.Event, process *model.Process) (flags []string) {
	return args.ParseProcessFlags(fh.ResolveProcessArgv(ev, process))
}

// ResolveProcessArgsOptions resolves the arguments options of the event
func (fh *EBPFLessFieldHandlers) ResolveProcessArgsOptions(ev *model.Event, process *model.Process) (options []string) {
	return args.ParseProcessOptions(fh.ResolveProcessArgv(ev, process))
}

// ResolveContainerContext retrieve the ContainerContext of the event
func (fh *EBPFLessFieldHandlers) ResolveContainerContext(ev *model.Event) (*model.ContainerContext, bool) {
	return ev.ContainerContext, ev.ContainerContext != nil
}

// ResolveProcessArgv0 resolves the first arg of the event
func (fh *EBPFLessFieldHandlers) ResolveProcessArgv0(_ *model.Event, process *model.Process) string {
	arg0, _ := sprocess.GetProcessArgv0(process)
	return arg0
}

// ResolveProcessArgs resolves the args of the event
func (fh *EBPFLessFieldHandlers) ResolveProcessArgs(ev *model.Event, process *model.Process) string {
	return strings.Join(fh.ResolveProcessArgv(ev, process), " ")
}

// ResolveProcessArgv resolves the unscrubbed args of the process as an array. Use with caution.
func (fh *EBPFLessFieldHandlers) ResolveProcessArgv(_ *model.Event, process *model.Process) []string {
	argv, _ := sprocess.GetProcessArgv(process)
	return argv
}

// ResolveProcessArgvScrubbed resolves the args of the process as an array
func (fh *EBPFLessFieldHandlers) ResolveProcessArgvScrubbed(_ *model.Event, process *model.Process) []string {
	argv, _ := fh.resolvers.ProcessResolver.GetProcessArgvScrubbed(process)
	return argv
}

// ResolveProcessArgsScrubbed resolves the args of the event
func (fh *EBPFLessFieldHandlers) ResolveProcessArgsScrubbed(ev *model.Event, process *model.Process) string {
	return strings.Join(fh.ResolveProcessArgvScrubbed(ev, process), " ")
}

// ResolveProcessEnvp resolves the envp of the event as an array
func (fh *EBPFLessFieldHandlers) ResolveProcessEnvp(_ *model.Event, process *model.Process) []string {
	envp, _ := fh.resolvers.ProcessResolver.GetProcessEnvp(process)
	return envp
}

// ResolveProcessArgsTruncated returns whether the args are truncated
func (fh *EBPFLessFieldHandlers) ResolveProcessArgsTruncated(_ *model.Event, process *model.Process) bool {
	_, truncated := sprocess.GetProcessArgv(process)
	return truncated
}

// ResolveProcessEnvsTruncated returns whether the envs are truncated
func (fh *EBPFLessFieldHandlers) ResolveProcessEnvsTruncated(_ *model.Event, process *model.Process) bool {
	_, truncated := fh.resolvers.ProcessResolver.GetProcessEnvs(process)
	return truncated
}

// ResolveProcessEnvs resolves the unscrubbed envs of the event. Use with caution.
func (fh *EBPFLessFieldHandlers) ResolveProcessEnvs(_ *model.Event, process *model.Process) []string {
	envs, _ := fh.resolvers.ProcessResolver.GetProcessEnvs(process)
	return envs
}

// GetProcessCacheEntry queries the ProcessResolver to retrieve the ProcessContext of the event
func (fh *EBPFLessFieldHandlers) GetProcessCacheEntry(ev *model.Event) (*model.ProcessCacheEntry, bool) {
	ev.ProcessCacheEntry = fh.resolvers.ProcessResolver.Resolve(sprocess.CacheResolverKey{
		Pid:  ev.PIDContext.Pid,
		NSID: ev.NSID,
	})
	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = model.GetPlaceholderProcessCacheEntry(ev.PIDContext.Pid, ev.PIDContext.Pid, false)
		return ev.ProcessCacheEntry, false
	}
	return ev.ProcessCacheEntry, true
}

// ResolveEventTime resolves the monolitic kernel event timestamp to an absolute time
func (fh *EBPFLessFieldHandlers) ResolveEventTime(ev *model.Event, _ *model.BaseEvent) time.Time {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	return ev.Timestamp
}

// ResolveContainerID resolves the container ID of the event
func (fh *EBPFLessFieldHandlers) ResolveContainerID(ev *model.Event, e *model.ContainerContext) string {
	if len(e.ID) == 0 {
		if entry, _ := fh.ResolveProcessCacheEntry(ev); entry != nil {
			e.ID = entry.ContainerID
		}
	}
	return e.ID
}

// ResolveContainerCreatedAt resolves the container creation time of the event
func (fh *EBPFLessFieldHandlers) ResolveContainerCreatedAt(ev *model.Event, e *model.ContainerContext) int {
	if e.CreatedAt == 0 {
		if containerContext, _ := fh.ResolveContainerContext(ev); containerContext != nil {
			e.CreatedAt = containerContext.CreatedAt
		}
	}
	return int(e.CreatedAt)
}

// ResolveContainerTags resolves the container tags of the event
func (fh *EBPFLessFieldHandlers) ResolveContainerTags(_ *model.Event, e *model.ContainerContext) []string {
	if len(e.Tags) == 0 && e.ID != "" {
		e.Tags = fh.resolvers.TagsResolver.Resolve(e.ID)
	}
	return e.Tags
}

// ResolveProcessCreatedAt resolves process creation time
func (fh *EBPFLessFieldHandlers) ResolveProcessCreatedAt(_ *model.Event, e *model.Process) int {
	return int(e.ExecTime.UnixNano())
}
