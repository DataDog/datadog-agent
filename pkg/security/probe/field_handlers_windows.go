// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// FieldHandlers defines a field handlers
type FieldHandlers struct {
	// TODO(safchain) remove this when support for multiple platform with the same build tags is available
	// keeping it can be dangerous as it can hide non implemented handlers
	model.DefaultFieldHandlers

	resolvers *resolvers.Resolvers
}

// ResolveEventTime resolves the monolitic kernel event timestamp to an absolute time
func (fh *FieldHandlers) ResolveEventTime(ev *model.Event, _ *model.BaseEvent) time.Time {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	return ev.Timestamp
}

// ResolveContainerContext retrieve the ContainerContext of the event
func (fh *FieldHandlers) ResolveContainerContext(ev *model.Event) (*model.ContainerContext, bool) {
	return ev.ContainerContext, ev.ContainerContext != nil
}

// ResolveFilePath resolves the inode to a full path
func (fh *FieldHandlers) ResolveFilePath(_ *model.Event, f *model.FileEvent) string {
	return f.PathnameStr
}

// ResolveFileBasename resolves the inode to a full path
func (fh *FieldHandlers) ResolveFileBasename(_ *model.Event, f *model.FileEvent) string {
	return f.BasenameStr
}

// ResolveProcessEnvp resolves the envp of the event as an array
func (fh *FieldHandlers) ResolveProcessEnvp(_ *model.Event, process *model.Process) []string {
	return fh.resolvers.ProcessResolver.GetEnvp(process)
}

// ResolveProcessEnvs resolves the envs of the event
func (fh *FieldHandlers) ResolveProcessEnvs(_ *model.Event, process *model.Process) []string {
	return fh.resolvers.ProcessResolver.GetEnvs(process)
}

// ResolveProcessCacheEntry queries the ProcessResolver to retrieve the ProcessContext of the event
func (fh *FieldHandlers) ResolveProcessCacheEntry(ev *model.Event) (*model.ProcessCacheEntry, bool) {
	if ev.ProcessCacheEntry == nil && ev.PIDContext.Pid != 0 {
		ev.ProcessCacheEntry = fh.resolvers.ProcessResolver.Resolve(ev.PIDContext.Pid)
	}

	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = model.GetPlaceholderProcessCacheEntry(ev.PIDContext.Pid)
		return ev.ProcessCacheEntry, false
	}

	return ev.ProcessCacheEntry, true
}

// GetProcessService returns the service tag based on the process context
func (fh *FieldHandlers) GetProcessService(ev *model.Event) string {
	entry, _ := fh.ResolveProcessCacheEntry(ev)
	if entry == nil {
		return ""
	}
	return getProcessService(entry)
}

// ResolveProcessCmdLineScrubbed returns a scrubbed version of the cmdline
func (fh *FieldHandlers) ResolveProcessCmdLineScrubbed(_ *model.Event, e *model.Process) string {
	return fh.resolvers.ProcessResolver.GetProcessCmdLineScrubbed(e)
}
