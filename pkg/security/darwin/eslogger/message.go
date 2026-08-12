// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package eslogger decodes the JSON-Lines output of macOS /usr/bin/eslogger.
//
// eslogger is explicitly not a stable API: Apple documents that its output may
// change between releases. Decoding here is therefore deliberately tolerant —
// unknown event kinds and unparseable lines are counted and skipped rather than
// treated as fatal.
//
// The struct tags below were reconciled against a real capture from macOS
// 26.5.1 (schema_version 1, version 10).
package eslogger

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Message is one line of eslogger output: an Endpoint Security NOTIFY message.
type Message struct {
	EventType     uint32 `json:"event_type"`
	Version       uint32 `json:"version"`
	SchemaVersion uint32 `json:"schema_version"`
	Time          string `json:"time"`
	MachTime      uint64 `json:"mach_time"`

	// SeqNum is per-event-type; GlobalSeqNum is monotonic across the whole
	// stream. Endpoint Security drops messages silently under load, so a gap in
	// GlobalSeqNum is the only evidence that fidelity was lost.
	SeqNum       uint64 `json:"seq_num"`
	GlobalSeqNum uint64 `json:"global_seq_num"`

	// Process is the process that caused the event. For exec it is the
	// pre-exec image; the post-exec process is Event's exec.target.
	Process *Process `json:"process"`

	// Event holds exactly one key, named for the event kind.
	Event json.RawMessage `json:"event"`
}

// Kind returns the event kind, e.g. "exec", "fork", "exit".
func (m *Message) Kind() (string, error) {
	var keyed map[string]json.RawMessage
	if err := json.Unmarshal(m.Event, &keyed); err != nil {
		return "", fmt.Errorf("event is not an object: %w", err)
	}
	if len(keyed) != 1 {
		return "", fmt.Errorf("expected exactly one event key, got %d", len(keyed))
	}
	for kind := range keyed {
		return kind, nil
	}
	return "", errors.New("unreachable")
}

// DecodeEvent unmarshals the inner event body into out.
func (m *Message) DecodeEvent(out any) error {
	var keyed map[string]json.RawMessage
	if err := json.Unmarshal(m.Event, &keyed); err != nil {
		return err
	}
	for _, body := range keyed {
		return json.Unmarshal(body, out)
	}
	return errors.New("empty event")
}

// AuditToken identifies a process.
//
// PIDVersion is Apple's pid-reuse uniquifier: pid alone is ambiguous because
// macOS recycles pids and wraps at 99999. Note that it advances on exec as
// well as on process creation, so a change in PIDVersion for a known pid does
// not by itself indicate pid reuse.
type AuditToken struct {
	PID        uint32 `json:"pid"`
	PIDVersion uint32 `json:"pidversion"`
	RUID       uint32 `json:"ruid"`
	EUID       uint32 `json:"euid"`
	RGID       uint32 `json:"rgid"`
	EGID       uint32 `json:"egid"`
	// ASID is the audit session ID, AUID the audit user ID.
	ASID uint32 `json:"asid"`
	AUID uint32 `json:"auid"`
}

// File is a file referenced by an ES message. ES supplies fully resolved
// absolute paths, which is why darwin needs no dentry, mount or path resolver.
//
// The `stat` member ES also provides is deliberately not decoded: nothing in
// the PoC consumes it and it is the bulk of each message.
type File struct {
	Path          string `json:"path"`
	PathTruncated bool   `json:"path_truncated"`
}

// Process is an ES process description.
type Process struct {
	Executable   *File  `json:"executable"`
	PPID         uint32 `json:"ppid"`
	OriginalPPID uint32 `json:"original_ppid"`
	GroupID      uint32 `json:"group_id"`
	SessionID    uint32 `json:"session_id"`

	AuditToken AuditToken `json:"audit_token"`
	// ParentAuditToken and ResponsibleAuditToken let us distinguish the
	// immediate parent from the process macOS holds responsible, which differ
	// whenever launchd or an XPC broker sits in between.
	ParentAuditToken      AuditToken `json:"parent_audit_token"`
	ResponsibleAuditToken AuditToken `json:"responsible_audit_token"`

	// SigningID, TeamID, CDHash and the codesigning flags are the macOS
	// identity signals that have no Linux equivalent. TeamID is null for
	// platform binaries, which decodes to "".
	SigningID            string `json:"signing_id"`
	TeamID               string `json:"team_id"`
	CDHash               string `json:"cdhash"`
	IsPlatformBinary     bool   `json:"is_platform_binary"`
	IsESClient           bool   `json:"is_es_client"`
	CodesigningFlags     uint32 `json:"codesigning_flags"`
	CSValidationCategory uint32 `json:"cs_validation_category"`

	StartTime string `json:"start_time"`
	TTY       *File  `json:"tty"`
}

// Path returns the executable path, or "" when absent.
func (p *Process) Path() string {
	if p == nil || p.Executable == nil {
		return ""
	}
	return p.Executable.Path
}

// TTYName returns the controlling tty path, or "" when absent.
func (p *Process) TTYName() string {
	if p == nil || p.TTY == nil {
		return ""
	}
	return p.TTY.Path
}

// ExecEvent is es_event_exec_t.
//
// There is deliberately no Env field. eslogger emits an `env` key on every
// exec message, but environment variables are never captured on a developer
// laptop, so encoding/json is left to drop it. Do not add one.
type ExecEvent struct {
	Target       *Process `json:"target"`
	Args         []string `json:"args"`
	CWD          *File    `json:"cwd"`
	DyldExecPath string   `json:"dyld_exec_path"`
	Script       *File    `json:"script"`
}

// ForkEvent is es_event_fork_t.
type ForkEvent struct {
	Child *Process `json:"child"`
}

// ExitEvent is es_event_exit_t.
//
// Stat is a wait(2)-style status word, not an exit code: a process that exits
// 78 reports Stat 19968 (0x4E00). Use ExitCode and Signal rather than reading
// Stat directly.
type ExitEvent struct {
	Stat int32 `json:"stat"`
}

// Exited reports whether the process terminated normally rather than by signal.
func (e *ExitEvent) Exited() bool {
	return e.Stat&0x7f == 0
}

// ExitCode returns the process exit status, i.e. WEXITSTATUS. It is only
// meaningful when Exited reports true.
func (e *ExitEvent) ExitCode() uint32 {
	return uint32(e.Stat>>8) & 0xff
}

// Signal returns the signal that terminated the process, i.e. WTERMSIG, or 0
// when it exited normally.
func (e *ExitEvent) Signal() uint32 {
	if e.Exited() {
		return 0
	}
	return uint32(e.Stat) & 0x7f
}
