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
	resolver  *process.EBPFLessResolver
	handlers  model.FieldHandlers
	userGroup nameResolver

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
	t := &Translator{
		resolver: pr,
		handlers: fh,
	}

	// process.user and process.group are plain stored fields on Credentials with
	// no field handler behind them, so names have to be filled in as entries are
	// built rather than resolved lazily at evaluation time. An unavailable
	// resolver degrades to empty names, which is exactly the symptom this fixes,
	// so it is logged rather than fatal.
	ug, err := newNameResolver()
	if err != nil {
		log.Warnf("user/group resolution unavailable, usernames will be empty: %v", err)
	} else {
		t.userGroup = ug
	}

	return t
}

// nameResolver maps uids and gids to names. It exists as an interface because
// pkg/security/resolvers/usergroup has a different constructor signature per
// platform (linux needs a cgroup resolver), and this file is //go:build unix so
// that its tests also run on Linux in CI.
type nameResolver interface {
	ResolveUser(uid int) (string, error)
	ResolveGroup(gid int) (string, error)
}

// resolveNames fills in the user and group names for an entry's credentials.
// resolveNames derives the user and group names from the ids already on the
// entry, overwriting whatever was there.
//
// Overwriting is the point rather than an optimisation detail. The process
// resolver's insertExecEntry copies the whole Credentials struct from the previous
// entry at that pid, names included, so an exec that changes uid arrives carrying
// the OLD name. Filling in only empty names left events reporting uid 502 as
// "root". A name that disagrees with the id beside it is worse than no name, so on
// failure the name is cleared rather than left stale.
func (t *Translator) resolveNames(entry *model.ProcessCacheEntry) {
	resolveCredentialNames(t.userGroup, &entry.Credentials)
}

// resolveCredentialNames derives the user and group names from the ids in creds,
// overwriting whatever was there. It is the single place that enforces the
// invariant "the names describe the ids stored beside them", and both the
// translator and the snapshot go through it.
//
// A nil resolver clears the names rather than leaving them: an inherited name
// that cannot be verified is exactly the wrong-attribution case this guards.
func resolveCredentialNames(ug nameResolver, creds *model.Credentials) {
	if ug == nil {
		creds.User = ""
		creds.Group = ""
		return
	}

	if name, err := ug.ResolveUser(int(creds.UID)); err == nil {
		creds.User = name
	} else {
		creds.User = ""
	}

	if name, err := ug.ResolveGroup(int(creds.GID)); err == nil {
		creds.Group = name
	} else {
		creds.Group = ""
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

	case "open":
		var body eslogger.OpenEvent
		if err := m.DecodeEvent(&body); err != nil {
			return nil, fmt.Errorf("decode open: %w", err)
		}
		if body.File == nil || body.File.Path == "" {
			return nil, nil
		}
		ev := t.newFileEvent(m, model.FileOpenEventType, ts)
		if ev == nil {
			return nil, nil
		}
		setFile(&ev.Open.File, body.File.Path)
		// Recorded for completeness, but see OpenEvent: macOS O_* values do not
		// match the Linux constants SECL compiles, so rules should match on path.
		ev.Open.Flags = uint32(body.FFlag)
		return ev, nil

	case "unlink":
		var body eslogger.UnlinkEvent
		if err := m.DecodeEvent(&body); err != nil {
			return nil, fmt.Errorf("decode unlink: %w", err)
		}
		if body.Target == nil || body.Target.Path == "" {
			return nil, nil
		}
		ev := t.newFileEvent(m, model.FileUnlinkEventType, ts)
		if ev == nil {
			return nil, nil
		}
		setFile(&ev.Unlink.File, body.Target.Path)
		return ev, nil

	case "rename":
		var body eslogger.RenameEvent
		if err := m.DecodeEvent(&body); err != nil {
			return nil, fmt.Errorf("decode rename: %w", err)
		}
		if body.Source == nil || body.Source.Path == "" {
			return nil, nil
		}
		ev := t.newFileEvent(m, model.FileRenameEventType, ts)
		if ev == nil {
			return nil, nil
		}
		setFile(&ev.Rename.Old, body.Source.Path)
		setFile(&ev.Rename.New, body.Destination.Path())
		return ev, nil

	case "create":
		// A creation is reported as an open of the new path. macOS has no SECL
		// create event type in the unix range -- CreateNewFileEventType is Windows
		// -- and inventing one would mislabel the event for the backend.
		var body eslogger.CreateEvent
		if err := m.DecodeEvent(&body); err != nil {
			return nil, fmt.Errorf("decode create: %w", err)
		}
		path := body.Destination.Path()
		if path == "" {
			return nil, nil
		}
		ev := t.newFileEvent(m, model.FileOpenEventType, ts)
		if ev == nil {
			return nil, nil
		}
		setFile(&ev.Open.File, path)
		return ev, nil

	default:
		return nil, nil
	}
}

// newFileEvent builds a file event attributed to the acting process.
//
// File events name the actor in the message's own process field rather than in the
// event body, and without that process context a file event is close to useless
// for a detection: every PoC rule reaches through process.ancestors.
func (t *Translator) newFileEvent(m *eslogger.Message, eventType model.EventType, ts time.Time) *model.Event {
	if m.Process == nil || m.Process.AuditToken.PID == 0 {
		return nil
	}
	pid := m.Process.AuditToken.PID

	ev := t.newEvent()
	ev.Type = uint32(eventType)
	ev.Timestamp = ts
	ev.TimestampRaw = uint64(ts.UnixNano())
	ev.PIDContext.Pid = pid

	if entry := t.resolver.Resolve(key(pid)); entry != nil {
		ev.ProcessCacheEntry = entry
		ev.ProcessContext = &entry.ProcessContext
	}

	return ev
}

// setFile fills a SECL file event from an already-resolved absolute path.
func setFile(fe *model.FileEvent, path string) {
	fe.PathnameStr = path
	fe.BasenameStr = basename(path)
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
	t.resolveNames(entry)
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
	t.resolveNames(entry)

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
