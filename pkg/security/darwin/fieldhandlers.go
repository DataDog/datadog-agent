// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package darwin

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// FieldHandlers resolves SECL fields for darwin events.
//
// It embeds model.FakeFieldHandlers, which supplies default implementations for
// every method of the (generated, ~97 method) model.FieldHandlers interface, and
// overrides only those darwin answers differently. probe.BaseFieldHandlers is
// deliberately not reused: it is //go:build linux || windows.
//
// Endpoint Security hands us fully resolved absolute paths, so the path, dentry
// and mount resolvers that dominate the Linux implementation have no counterpart
// here.
type FieldHandlers struct {
	*model.FakeFieldHandlers

	resolver *process.EBPFLessResolver
	hostname string
}

var _ model.FieldHandlers = (*FieldHandlers)(nil)

// NewFieldHandlers returns field handlers backed by the given process resolver.
func NewFieldHandlers(pr *process.EBPFLessResolver, hostname string) *FieldHandlers {
	return &FieldHandlers{
		FakeFieldHandlers: &model.FakeFieldHandlers{
			PCEs: make(map[uint32]*model.ProcessCacheEntry),
		},
		resolver: pr,
		hostname: hostname,
	}
}

// ResolveProcessCacheEntry attaches the process cache entry for the event's pid.
// The bool reports whether the entry is real; false means the caller is looking
// at a placeholder.
func (fh *FieldHandlers) ResolveProcessCacheEntry(ev *model.Event, _ func(*model.ProcessCacheEntry, error)) (*model.ProcessCacheEntry, bool) {
	if ev.ProcessCacheEntry == nil && ev.PIDContext.Pid != 0 {
		ev.ProcessCacheEntry = fh.resolver.Resolve(process.CacheResolverKey{Pid: ev.PIDContext.Pid})
	}
	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = model.GetPlaceholderProcessCacheEntry(ev.PIDContext)
		return ev.ProcessCacheEntry, false
	}
	return ev.ProcessCacheEntry, true
}

// ResolveProcessCacheEntryFromPID looks a pid up directly in the process cache.
func (fh *FieldHandlers) ResolveProcessCacheEntryFromPID(pid uint32) *model.ProcessCacheEntry {
	if entry := fh.resolver.Resolve(process.CacheResolverKey{Pid: pid}); entry != nil {
		return entry
	}
	return model.GetPlaceholderProcessCacheEntry(model.PIDContext{Pid: pid})
}

// ResolveFilePath returns the path. Endpoint Security already resolved it.
func (fh *FieldHandlers) ResolveFilePath(_ *model.Event, f *model.FileEvent) string {
	return f.PathnameStr
}

// ResolveFileBasename returns the basename, deriving it from the path on first
// use.
func (fh *FieldHandlers) ResolveFileBasename(_ *model.Event, f *model.FileEvent) string {
	if f.BasenameStr == "" && f.PathnameStr != "" {
		f.BasenameStr = basename(f.PathnameStr)
	}
	return f.BasenameStr
}

// ResolveProcessArgv returns argv without argv0.
//
// This override is load-bearing rather than cosmetic: the process resolver
// stores arguments in Process.ArgsEntry and leaves Process.Argv empty, so the
// embedded FakeFieldHandlers (which just returns Process.Argv) would report no
// arguments at all.
func (fh *FieldHandlers) ResolveProcessArgv(_ *model.Event, p *model.Process) []string {
	argv, _ := process.GetProcessArgv(p)
	return argv
}

// ResolveProcessArgv0 returns argv0.
func (fh *FieldHandlers) ResolveProcessArgv0(_ *model.Event, p *model.Process) string {
	argv0, _ := process.GetProcessArgv0(p)
	return argv0
}

// ResolveProcessArgvScrubbed returns argv with secrets scrubbed. argv on a
// developer laptop routinely carries tokens, so rules and payloads must use the
// scrubbed form.
func (fh *FieldHandlers) ResolveProcessArgvScrubbed(_ *model.Event, p *model.Process) []string {
	argv, _ := fh.resolver.GetProcessArgvScrubbed(p)
	return argv
}

// ResolveProcessArgsScrubbed returns scrubbed argv joined into one string.
func (fh *FieldHandlers) ResolveProcessArgsScrubbed(ev *model.Event, p *model.Process) string {
	return strings.Join(fh.ResolveProcessArgvScrubbed(ev, p), " ")
}

// ResolveProcessArgsTruncated reports whether arguments were truncated.
func (fh *FieldHandlers) ResolveProcessArgsTruncated(_ *model.Event, p *model.Process) bool {
	_, truncated := process.GetProcessArgv(p)
	return truncated
}

// ResolveProcessEnvs returns nothing: environment variables are never captured
// on a developer laptop. eslogger does emit them and the decoder drops them;
// this makes sure nothing reintroduces them at the SECL layer either.
func (fh *FieldHandlers) ResolveProcessEnvs(_ *model.Event, _ *model.Process) []string {
	return nil
}

// ResolveProcessEnvp returns nothing, for the same reason.
func (fh *FieldHandlers) ResolveProcessEnvp(_ *model.Event, _ *model.Process) []string {
	return nil
}

// ResolveEventTime returns the event timestamp.
func (fh *FieldHandlers) ResolveEventTime(ev *model.Event, _ *model.BaseEvent) time.Time {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	return ev.Timestamp
}

// ResolveHostname returns the configured hostname. There is no core agent to ask
// on a laptop, so it is supplied at construction.
func (fh *FieldHandlers) ResolveHostname(_ *model.Event, _ *model.BaseEvent) string {
	return fh.hostname
}

// ResolveService returns no service: there is no container or tagger on a laptop.
func (fh *FieldHandlers) ResolveService(_ *model.Event, _ *model.BaseEvent) string {
	return ""
}
