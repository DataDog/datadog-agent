// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

// Package darwin implements a Workload Protection event source for macOS built
// on Apple's Endpoint Security framework, consumed through /usr/bin/eslogger.
package darwin

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/darwin/eslogger"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/sharedconsts"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Translator converts Endpoint Security messages into SECL events.
//
// Process identity is keyed on pid with NSID left at zero. pidversion is
// deliberately NOT folded into the cache key: the resolver looks parents up as
// {Pid: ppid, NSID: key.NSID}, so a per-process NSID would break every parent
// link. Eviction on exit is what makes pid reuse safe instead.
type Translator struct {
	resolver *process.EBPFLessResolver
	handlers model.FieldHandlers

	// RecycledPIDs counts genuinely reused pids, i.e. a fork arriving for a pid
	// that still has a live cache entry because we never saw its exit. Note that
	// this is not detected by comparing pidversion: macOS advances pidversion on
	// exec as well as on process creation, so a version change for a known pid
	// is normal and counting it would flag every fork/exec pair.
	RecycledPIDs uint64
	// OrphanExecs counts execs for pids we have no entry for, which means the
	// fork was dropped or predates the collector. Surfaced in collector stats as
	// a snapshot-coverage signal.
	OrphanExecs uint64
}

// NewTranslator returns a Translator writing into the given process resolver.
func NewTranslator(pr *process.EBPFLessResolver, fh model.FieldHandlers) *Translator {
	return &Translator{
		resolver: pr,
		handlers: fh,
	}
}

// key returns the process-cache key for a pid.
func key(pid uint32) process.CacheResolverKey {
	return process.CacheResolverKey{Pid: pid}
}

// newEvent returns a zeroed darwin event with field handlers attached.
func (t *Translator) newEvent() *model.Event {
	ev := model.NewFakeEvent()
	ev.FieldHandlers = t.handlers
	return ev
}

// parseTime converts an ES timestamp into a time.Time. eslogger renders time as
// an RFC3339 string with nanosecond precision; fall back to now rather than
// failing the event, since a missing timestamp is not worth dropping activity
// over.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Now()
	}
	if ts, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return ts
	}
	return time.Now()
}

// Translate converts one ES message into a SECL event. It returns (nil, nil) for
// kinds this platform does not map yet, so callers skip them.
func (t *Translator) Translate(m *eslogger.Message) (*model.Event, error) {
	kind, err := m.Kind()
	if err != nil {
		return nil, fmt.Errorf("cannot determine event kind: %w", err)
	}

	ts := parseTime(m.Time)

	switch kind {
	case "exec":
		var body eslogger.ExecEvent
		if err := m.DecodeEvent(&body); err != nil {
			return nil, fmt.Errorf("decode exec: %w", err)
		}
		if body.Target == nil || body.Target.AuditToken.PID == 0 {
			return nil, nil
		}
		return t.translateExec(&body, ts), nil

	case "fork":
		var body eslogger.ForkEvent
		if err := m.DecodeEvent(&body); err != nil {
			return nil, fmt.Errorf("decode fork: %w", err)
		}
		if body.Child == nil || body.Child.AuditToken.PID == 0 {
			return nil, nil
		}
		return t.translateFork(&body, ts), nil

	case "exit":
		var body eslogger.ExitEvent
		if err := m.DecodeEvent(&body); err != nil {
			return nil, fmt.Errorf("decode exit: %w", err)
		}
		if m.Process == nil || m.Process.AuditToken.PID == 0 {
			return nil, nil
		}
		return t.translateExit(m.Process, &body, ts), nil

	default:
		return nil, nil
	}
}

func (t *Translator) translateExec(body *eslogger.ExecEvent, ts time.Time) *model.Event {
	target := body.Target
	pid := target.AuditToken.PID

	// Endpoint Security and SECL disagree about which file an interpreted
	// execution is "about", and the disagreement is not cosmetic.
	//
	// For a shebang script ES reports the INTERPRETER as target.executable and
	// supplies the script separately, so a script named npm arrives looking like
	// "sh". SECL is the other way round: Process.FileEvent is the script and
	// Process.LinuxBinprm is the "script interpreter as identified by the
	// shebang". Following SECL is what makes exec.file.name == "npm" match a real
	// npm, which is itself a #!/usr/bin/env node script.
	execPath, interpreterPath := target.Path(), ""
	if body.Script != nil && body.Script.Path != "" {
		execPath, interpreterPath = body.Script.Path, target.Path()
	}

	if log.ShouldLog(log.TraceLvl) {
		log.Tracef("exec pid=%d file=%q interpreter=%q dyld_exec_path=%q",
			pid, execPath, interpreterPath, body.DyldExecPath)
	}

	if t.resolver.Resolve(key(pid)) == nil {
		// No entry for this pid means we never saw the fork: either it was
		// dropped, or the process predates the collector and the snapshot missed
		// it. The exec still produces an event, just with a shallower tree.
		t.OrphanExecs++
	}

	// Envs are deliberately nil: environment variables are never captured on a
	// developer laptop. eslogger does emit them, and the decoder drops them.
	entry := t.resolver.AddExecEntry(
		key(pid),
		target.PPID,
		execPath,
		body.Args,
		false, // argsTruncated
		nil,   // envs
		false, // envsTruncated
		"",    // container ID: no meaning on a laptop
		"",    // cgroup ID: likewise
		uint64(ts.UnixNano()),
		target.TTYName(),
	)
	if entry == nil {
		return nil
	}

	applyCredentials(entry, target)
	setInterpreter(entry, interpreterPath)

	ev := t.newEvent()
	ev.Type = uint32(model.ExecEventType)
	ev.Timestamp = ts
	ev.TimestampRaw = uint64(ts.UnixNano())
	ev.PIDContext.Pid = pid
	ev.ProcessCacheEntry = entry
	ev.ProcessContext = &entry.ProcessContext
	ev.Exec.Process = &entry.Process

	return ev
}

// setInterpreter records the shebang interpreter, if there was one.
//
// SECL treats an interpreter as present only when LinuxBinprm's inode is
// non-zero (Process.HasInterpreter). macOS has no inode to offer here, so
// model.SetInterpreterFields is used to stamp the sentinel the SECL model
// reserves for exactly this purpose.
func setInterpreter(entry *model.ProcessCacheEntry, interpreterPath string) {
	if interpreterPath == "" {
		return
	}

	entry.Process.LinuxBinprm.FileEvent.PathnameStr = interpreterPath
	entry.Process.LinuxBinprm.FileEvent.BasenameStr = basename(interpreterPath)
	// The subField argument only has to be something other than "file.inode".
	_, _ = model.SetInterpreterFields(&entry.Process.LinuxBinprm, "file.path", nil)
}

func (t *Translator) translateFork(body *eslogger.ForkEvent, ts time.Time) *model.Event {
	child := body.Child
	pid := child.AuditToken.PID

	// A fork for a pid we still hold a live entry for is real pid reuse: macOS
	// wrapped around and handed the number out again before we saw an exit. The
	// resolver will exit the stale entry for us; we only count it here.
	if t.resolver.Resolve(key(pid)) != nil {
		t.RecycledPIDs++
	}

	entry := t.resolver.AddForkEntry(key(pid), child.PPID, uint64(ts.UnixNano()))
	if entry == nil {
		return nil
	}

	applyCredentials(entry, child)

	ev := t.newEvent()
	ev.Type = uint32(model.ForkEventType)
	ev.Timestamp = ts
	ev.TimestampRaw = uint64(ts.UnixNano())
	ev.PIDContext.Pid = pid
	ev.ProcessCacheEntry = entry
	ev.ProcessContext = &entry.ProcessContext

	return ev
}

func (t *Translator) translateExit(proc *eslogger.Process, body *eslogger.ExitEvent, ts time.Time) *model.Event {
	pid := proc.AuditToken.PID

	entry := t.resolver.Resolve(key(pid))

	ev := t.newEvent()
	ev.Type = uint32(model.ExitEventType)
	ev.Timestamp = ts
	ev.TimestampRaw = uint64(ts.UnixNano())
	ev.PIDContext.Pid = pid
	if entry != nil {
		ev.ProcessCacheEntry = entry
		ev.ProcessContext = &entry.ProcessContext
		ev.Exit.Process = &entry.Process
	}
	ev.Exit.Cause, ev.Exit.Code = exitCauseAndCode(body)

	// Evict after building the event so the exit event itself still carries
	// process context.
	t.resolver.DeleteEntry(key(pid), ts)

	return ev
}

// exitCauseAndCode maps an ES wait(2) status word onto the same cause/code pair
// the Linux probe produces in model.ExitEvent.UnmarshalBinary, so that a darwin
// exit event and a Linux one describe the same termination identically.
func exitCauseAndCode(body *eslogger.ExitEvent) (uint32, uint32) {
	switch {
	case body.Exited():
		return uint32(sharedconsts.ExitExited), body.ExitCode()
	case body.CoreDumped():
		return uint32(sharedconsts.ExitCoreDumped), body.Signal()
	default:
		return uint32(sharedconsts.ExitSignaled), body.Signal()
	}
}

// applyCredentials copies the ES audit token onto the cache entry. Only the
// identity fields macOS actually supplies are set; Linux-only notions such as
// capabilities and fs-uid are left zero rather than invented.
func applyCredentials(entry *model.ProcessCacheEntry, proc *eslogger.Process) {
	entry.Credentials.UID = proc.AuditToken.RUID
	entry.Credentials.EUID = proc.AuditToken.EUID
	entry.Credentials.GID = proc.AuditToken.RGID
	entry.Credentials.EGID = proc.AuditToken.EGID
	entry.Credentials.AUID = proc.AuditToken.AUID
}

// basename returns the last path element.
func basename(path string) string {
	return filepath.Base(path)
}
