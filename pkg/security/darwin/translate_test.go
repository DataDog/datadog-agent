// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package darwin

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/darwin/eslogger"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/sharedconsts"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func newTestTranslator(t *testing.T) *Translator {
	t.Helper()

	// nil/nil still installs the default sensitive-word set plus *token*/*jwt*,
	// which is what the privacy constraint on argv relies on.
	scrubber, err := utils.NewScrubber(nil, nil)
	require.NoError(t, err)

	pr, err := process.NewEBPFLessResolver(nil, nil, scrubber, process.NewResolverOpts())
	require.NoError(t, err)

	fh := NewFieldHandlers(pr, "test-host")
	return NewTranslator(pr, fh)
}

// TestTranslateExecPopulatesProcess asserts the fields the PoC's rules depend on.
func TestTranslateExecPopulatesProcess(t *testing.T) {
	tr := newTestTranslator(t)

	// This package keeps its own copy of the capture rather than reading the
	// eslogger package's: a cross-package relative path is not available under
	// bazel test, and a package's tests should not reach into a sibling's
	// fixtures.
	f, err := os.Open("testdata/exec_fork_exit.jsonl")
	require.NoError(t, err)
	defer f.Close()

	var execEvents int
	d := eslogger.NewDecoder(f)
	for {
		msg, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		ev, err := tr.Translate(msg)
		require.NoError(t, err)
		if ev == nil {
			continue
		}

		if ev.GetEventType() == model.ExecEventType {
			execEvents++
			assert.NotEmpty(t, ev.Exec.Process.FileEvent.PathnameStr, "exec.file.path")
			assert.NotZero(t, ev.Exec.Process.Pid, "exec pid")
			assert.NotZero(t, ev.PIDContext.Pid, "event PIDContext.Pid")
			assert.NotNil(t, ev.ProcessCacheEntry, "process cache entry must be attached")
		}
	}

	assert.Positive(t, execEvents, "fixture must yield exec events")
}

// TestTranslateBuildsProcessTree is the load-bearing test: a fork whose parent
// was seen earlier must be linked to that parent, because every detection rule
// in this PoC keys on process.ancestors.
func TestTranslateBuildsProcessTree(t *testing.T) {
	tr := newTestTranslator(t)

	parentExec := execMessage(t, 4242, 1, "/bin/zsh", []string{"zsh"})
	_, err := tr.Translate(parentExec)
	require.NoError(t, err)

	childFork := forkMessage(t, 4243, 4242, "/bin/zsh")
	_, err = tr.Translate(childFork)
	require.NoError(t, err)

	childExec := execMessage(t, 4243, 4242, "/usr/bin/curl", []string{"curl", "https://example.com"})
	ev, err := tr.Translate(childExec)
	require.NoError(t, err)
	require.NotNil(t, ev)

	entry := tr.resolver.Resolve(process.CacheResolverKey{Pid: 4243})
	require.NotNil(t, entry, "child must be in the cache")
	require.NotNil(t, entry.Ancestor, "child must have an ancestor")
	assert.Equal(t, "/bin/zsh", entry.Ancestor.FileEvent.PathnameStr, "ancestor should be the parent shell")
}

// TestTranslateExitEvictsEntry guards the pid-reuse story: macOS recycles pids
// aggressively, and eviction on exit is what keeps a later reuse from attaching
// to a stale parent.
func TestTranslateExitEvictsEntry(t *testing.T) {
	tr := newTestTranslator(t)

	_, err := tr.Translate(execMessage(t, 5150, 1, "/bin/sh", []string{"sh"}))
	require.NoError(t, err)
	require.NotNil(t, tr.resolver.Resolve(process.CacheResolverKey{Pid: 5150}))

	_, err = tr.Translate(exitMessage(t, 5150, 0))
	require.NoError(t, err)

	assert.Nil(t, tr.resolver.Resolve(process.CacheResolverKey{Pid: 5150}),
		"exit must evict the entry so a reused pid cannot inherit a stale parent")
}

// TestTranslateExitCarriesProcessContext checks that the exit event itself still
// has process context, since eviction happens after the event is built.
func TestTranslateExitCarriesProcessContext(t *testing.T) {
	tr := newTestTranslator(t)

	_, err := tr.Translate(execMessage(t, 5151, 1, "/bin/sh", []string{"sh"}))
	require.NoError(t, err)

	ev, err := tr.Translate(exitMessage(t, 5151, 0))
	require.NoError(t, err)
	require.NotNil(t, ev)

	assert.Equal(t, model.ExitEventType, ev.GetEventType())
	require.NotNil(t, ev.ProcessCacheEntry, "exit event must still carry the entry it is about to evict")
	assert.Equal(t, "/bin/sh", ev.Exit.Process.FileEvent.PathnameStr)
}

// TestTranslateExitDecodesWaitStatus pins the wait(2) mapping end to end, and in
// particular that it matches what the Linux probe produces in
// model.ExitEvent.UnmarshalBinary. A raw status word in exit.code would render
// as nonsense in the UI.
func TestTranslateExitDecodesWaitStatus(t *testing.T) {
	tests := []struct {
		name  string
		stat  int32
		cause sharedconsts.ExitCause
		code  uint32
	}{
		{name: "clean exit", stat: 0, cause: sharedconsts.ExitExited, code: 0},
		{name: "exit 78 (real fixture value)", stat: 19968, cause: sharedconsts.ExitExited, code: 78},
		{name: "SIGKILL", stat: 9, cause: sharedconsts.ExitSignaled, code: 9},
		{name: "SIGSEGV with coredump", stat: 11 | 0x80, cause: sharedconsts.ExitCoreDumped, code: 11},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := newTestTranslator(t)

			_, err := tr.Translate(execMessage(t, 6000, 1, "/bin/sh", []string{"sh"}))
			require.NoError(t, err)

			ev, err := tr.Translate(exitMessage(t, 6000, tc.stat))
			require.NoError(t, err)
			require.NotNil(t, ev)

			assert.Equal(t, uint32(tc.cause), ev.Exit.Cause, "exit cause")
			assert.Equal(t, tc.code, ev.Exit.Code, "exit code")
		})
	}
}

// TestPIDVersionBumpOnExecIsNotReuse is the regression test for the subtlest bug
// in this translator. macOS advances audit_token.pidversion on exec as well as
// on process creation, so treating any version change for a known pid as pid
// reuse would flag every single fork/exec pair. The numbers here are the real
// ones captured from eslogger.
func TestPIDVersionBumpOnExecIsNotReuse(t *testing.T) {
	tr := newTestTranslator(t)

	// Exactly what the fixture contains: fork at pidversion 41075, then exec of
	// the same pid at 41076.
	_, err := tr.Translate(forkMessageVersioned(t, 20745, 574, "/usr/local/bin/example-daemon", 41075))
	require.NoError(t, err)
	_, err = tr.Translate(execMessageVersioned(t, 20745, 574, "/usr/bin/wdutil", []string{"wdutil", "info"}, 41076))
	require.NoError(t, err)

	assert.Zero(t, tr.RecycledPIDs,
		"a pidversion bump across fork->exec is normal and must not count as pid reuse")
}

// TestRecycledPIDIsDetected is the other half: a genuinely reused pid, i.e. a
// fork arriving for a pid we still hold a live entry for, must be counted.
func TestRecycledPIDIsDetected(t *testing.T) {
	tr := newTestTranslator(t)

	// A process is forked and exec'd, and we never see its exit (dropped event).
	_, err := tr.Translate(forkMessageVersioned(t, 700, 1, "/bin/zsh", 100))
	require.NoError(t, err)
	_, err = tr.Translate(execMessageVersioned(t, 700, 1, "/bin/zsh", []string{"zsh"}, 101))
	require.NoError(t, err)
	require.Zero(t, tr.RecycledPIDs)

	// The pid comes back around as a brand new process while we still think the
	// old one is alive.
	_, err = tr.Translate(forkMessageVersioned(t, 700, 1, "/usr/bin/curl", 90000))
	require.NoError(t, err)

	assert.EqualValues(t, 1, tr.RecycledPIDs,
		"a fork for a pid with a live cache entry is real pid reuse")
}

// TestTranslateExecOfScriptReportsScriptAsFile pins the interpreted entry point
// mapping, which is the difference between a working macOS rule and one that
// silently never fires.
//
// Endpoint Security reports the INTERPRETER as exec.target.executable for a
// shebang script and supplies the script separately, so a script named npm
// arrives looking like "sh". SECL models it the other way round: exec.file is
// the script and exec.interpreter.file is the interpreter (model_unix.go,
// "Script interpreter as identified by the shebang"). Real npm is a
// #!/usr/bin/env node script, so getting this backwards makes
// exec.file.name == "npm" unmatchable on macOS.
//
// The values here are from a real capture: ES gave executable=/bin/sh with
// script=/private/tmp/fake-a/npm.
func TestTranslateExecOfScriptReportsScriptAsFile(t *testing.T) {
	tr := newTestTranslator(t)

	ev, err := tr.Translate(execMessageScript(t, 1259, 1,
		"/bin/sh", "/private/tmp/fake-a/npm", []string{"/bin/sh", "/private/tmp/fake-a/npm"}))
	require.NoError(t, err)
	require.NotNil(t, ev)

	p := ev.Exec.Process
	assert.Equal(t, "/private/tmp/fake-a/npm", p.FileEvent.PathnameStr,
		"exec.file.path must be the script, not the interpreter")
	assert.Equal(t, "npm", p.FileEvent.BasenameStr,
		"exec.file.name must be the script basename, so rules on npm can match")

	assert.True(t, p.HasInterpreter(), "the interpreter must be marked valid")
	assert.Equal(t, "/bin/sh", p.LinuxBinprm.FileEvent.PathnameStr,
		"exec.interpreter.file.path must be the interpreter")
	assert.Equal(t, "sh", p.LinuxBinprm.FileEvent.BasenameStr)
}

// TestTranslateExecOfBinaryHasNoInterpreter is the other half: a real Mach-O
// binary has no script, so file is the executable and there is no interpreter.
func TestTranslateExecOfBinaryHasNoInterpreter(t *testing.T) {
	tr := newTestTranslator(t)

	ev, err := tr.Translate(execMessage(t, 1269, 1, "/private/tmp/fake-c/npm", []string{"npm"}))
	require.NoError(t, err)
	require.NotNil(t, ev)

	p := ev.Exec.Process
	assert.Equal(t, "/private/tmp/fake-c/npm", p.FileEvent.PathnameStr)
	assert.Equal(t, "npm", p.FileEvent.BasenameStr)
	assert.False(t, p.HasInterpreter(),
		"a real binary must not be reported as having an interpreter")
}

func TestTranslateSkipsUnmappedKind(t *testing.T) {
	tr := newTestTranslator(t)

	msg := &eslogger.Message{
		Time:  "2026-08-12T00:00:00Z",
		Event: []byte(`{"some_future_event":{}}`),
	}

	ev, err := tr.Translate(msg)
	assert.NoError(t, err, "an unmapped kind is not an error")
	assert.Nil(t, ev, "an unmapped kind yields no event")
}

// TestTranslateNeverPopulatesEnvs is the privacy guard at the SECL layer: even
// though the decoder drops `env`, the translator must not synthesise envs from
// anywhere else either.
func TestTranslateNeverPopulatesEnvs(t *testing.T) {
	tr := newTestTranslator(t)

	ev, err := tr.Translate(execMessage(t, 7000, 1, "/bin/sh", []string{"sh", "-c", "echo hi"}))
	require.NoError(t, err)
	require.NotNil(t, ev)

	entry := tr.resolver.Resolve(process.CacheResolverKey{Pid: 7000})
	require.NotNil(t, entry)
	assert.Empty(t, entry.Process.EnvsEntry.Values, "environment variables must never be captured")
}

// fakeNameResolver maps a couple of ids to names deterministically, so credential
// naming can be tested on any unix rather than only on a machine with these
// accounts.
type fakeNameResolver struct{}

func (fakeNameResolver) ResolveUser(uid int) (string, error) {
	switch uid {
	case 0:
		return "root", nil
	case 502:
		return "alice", nil
	}
	return "", errors.New("unknown uid")
}

func (fakeNameResolver) ResolveGroup(gid int) (string, error) {
	switch gid {
	case 0:
		return "wheel", nil
	case 20:
		return "staff", nil
	}
	return "", errors.New("unknown gid")
}

// TestExecDoesNotInheritStaleUserName is a regression test for a wrong-attribution
// bug seen in staging: events reported "uid": 502 alongside "user": "root".
//
// The process resolver's insertExecEntry copies the whole Credentials struct from
// the previous entry at that pid, names included. Overwriting only the numeric ids
// afterwards therefore leaves the inherited name in place, and an event ends up
// attributing a user's activity to root. Misattribution is worse than a missing
// name, so the names have to be re-derived whenever the ids they describe change.
//
// Note that the group looked correct in the wild purely by coincidence: the
// inherited gid happened to match too. The uid is what exposed it.
func TestExecDoesNotInheritStaleUserName(t *testing.T) {
	tr := newTestTranslator(t)
	tr.userGroup = fakeNameResolver{}

	// First exec at this pid runs as root, as a snapshotted or setuid parent would.
	_, err := tr.Translate(execMessageCreds(t, 5000, 1, "/usr/bin/sudo", []string{"sudo"}, 0, 0, 0, 0))
	require.NoError(t, err)

	entry := tr.resolver.Resolve(process.CacheResolverKey{Pid: 5000})
	require.NotNil(t, entry)
	require.Equal(t, "root", entry.Credentials.User)

	// A second exec at the SAME pid, now as uid 502. insertExecEntry inherits the
	// previous credentials, so this is where the stale name used to survive.
	_, err = tr.Translate(execMessageCreds(t, 5000, 1, "/bin/sh", []string{"sh"}, 502, 502, 20, 20))
	require.NoError(t, err)

	entry = tr.resolver.Resolve(process.CacheResolverKey{Pid: 5000})
	require.NotNil(t, entry)

	assert.EqualValues(t, 502, entry.Credentials.UID)
	assert.Equal(t, "alice", entry.Credentials.User,
		"the user name must match the uid set alongside it, not the inherited one")
	assert.Equal(t, "staff", entry.Credentials.Group)
}

// TestUnresolvableUIDLeavesNameEmpty checks the degraded case is empty rather than
// stale: an unknown uid must not keep a previous entry's name.
func TestUnresolvableUIDLeavesNameEmpty(t *testing.T) {
	tr := newTestTranslator(t)
	tr.userGroup = fakeNameResolver{}

	_, err := tr.Translate(execMessageCreds(t, 5001, 1, "/usr/bin/sudo", []string{"sudo"}, 0, 0, 0, 0))
	require.NoError(t, err)

	// uid 4242 is unknown to the resolver.
	_, err = tr.Translate(execMessageCreds(t, 5001, 1, "/bin/sh", []string{"sh"}, 4242, 4242, 20, 20))
	require.NoError(t, err)

	entry := tr.resolver.Resolve(process.CacheResolverKey{Pid: 5001})
	require.NotNil(t, entry)

	assert.EqualValues(t, 4242, entry.Credentials.UID)
	assert.Empty(t, entry.Credentials.User,
		"an unresolvable uid must leave the name empty, never inherit a stale one")
}

// TestTranslateSetsCredentials checks the uid/gid plumbing that process.user
// depends on.
func TestTranslateSetsCredentials(t *testing.T) {
	tr := newTestTranslator(t)

	msg := execMessageCreds(t, 8000, 1, "/bin/sh", []string{"sh"}, 501, 502, 20, 21)
	_, err := tr.Translate(msg)
	require.NoError(t, err)

	entry := tr.resolver.Resolve(process.CacheResolverKey{Pid: 8000})
	require.NotNil(t, entry)
	assert.EqualValues(t, 501, entry.Credentials.UID, "uid from audit_token.ruid")
	assert.EqualValues(t, 502, entry.Credentials.EUID, "euid from audit_token.euid")
	assert.EqualValues(t, 20, entry.Credentials.GID, "gid from audit_token.rgid")
	assert.EqualValues(t, 21, entry.Credentials.EGID, "egid from audit_token.egid")
}

//
// Fixture builders. These construct eslogger.Message values directly so the tree
// tests do not depend on the captured fixture containing a specific pid layout.
//

func execMessage(t *testing.T, pid, ppid uint32, path string, args []string) *eslogger.Message {
	t.Helper()
	return execMessageVersioned(t, pid, ppid, path, args, pid)
}

func execMessageVersioned(t *testing.T, pid, ppid uint32, path string, args []string, pidversion uint32) *eslogger.Message {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"exec": map[string]any{
			"target": map[string]any{
				"executable":  map[string]any{"path": path},
				"ppid":        ppid,
				"audit_token": map[string]any{"pid": pid, "pidversion": pidversion},
			},
			"args": args,
		},
	})
	require.NoError(t, err)
	return &eslogger.Message{Time: "2026-08-12T00:00:00Z", Event: body}
}

// execMessageScript builds an exec message shaped like a shebang execution: the
// executable is the interpreter and the script is supplied separately.
func execMessageScript(t *testing.T, pid, ppid uint32, interpreter, script string, args []string) *eslogger.Message {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"exec": map[string]any{
			"target": map[string]any{
				"executable":  map[string]any{"path": interpreter},
				"ppid":        ppid,
				"audit_token": map[string]any{"pid": pid, "pidversion": pid},
			},
			"script": map[string]any{"path": script},
			"args":   args,
		},
	})
	require.NoError(t, err)
	return &eslogger.Message{Time: "2026-08-12T00:00:00Z", Event: body}
}

func execMessageCreds(t *testing.T, pid, ppid uint32, path string, args []string, ruid, euid, rgid, egid uint32) *eslogger.Message {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"exec": map[string]any{
			"target": map[string]any{
				"executable": map[string]any{"path": path},
				"ppid":       ppid,
				"audit_token": map[string]any{
					"pid": pid, "pidversion": pid,
					"ruid": ruid, "euid": euid, "rgid": rgid, "egid": egid,
				},
			},
			"args": args,
		},
	})
	require.NoError(t, err)
	return &eslogger.Message{Time: "2026-08-12T00:00:00Z", Event: body}
}

func forkMessage(t *testing.T, childPid, parentPid uint32, path string) *eslogger.Message {
	t.Helper()
	return forkMessageVersioned(t, childPid, parentPid, path, childPid)
}

func forkMessageVersioned(t *testing.T, childPid, parentPid uint32, path string, pidversion uint32) *eslogger.Message {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"fork": map[string]any{
			"child": map[string]any{
				"executable":  map[string]any{"path": path},
				"ppid":        parentPid,
				"audit_token": map[string]any{"pid": childPid, "pidversion": pidversion},
			},
		},
	})
	require.NoError(t, err)
	return &eslogger.Message{Time: "2026-08-12T00:00:00Z", Event: body}
}

func exitMessage(t *testing.T, pid uint32, stat int32) *eslogger.Message {
	t.Helper()
	body, err := json.Marshal(map[string]any{"exit": map[string]any{"stat": stat}})
	require.NoError(t, err)
	return &eslogger.Message{
		Time:    "2026-08-12T00:00:00Z",
		Process: &eslogger.Process{AuditToken: eslogger.AuditToken{PID: pid}},
		Event:   body,
	}
}
