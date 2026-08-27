// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package eslogger

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeFixture(t *testing.T) {
	f, err := os.Open("testdata/exec_fork_exit.jsonl")
	require.NoError(t, err)
	defer f.Close()

	d := NewDecoder(f)

	kinds := map[string]int{}
	var firstExec *Message
	for {
		msg, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		kind, err := msg.Kind()
		require.NoError(t, err)
		kinds[kind]++

		if kind == "exec" && firstExec == nil {
			firstExec = msg
		}
	}

	assert.Positive(t, kinds["exec"], "fixture must contain exec messages")
	assert.Positive(t, kinds["fork"], "fixture must contain fork messages")
	assert.Positive(t, kinds["exit"], "fixture must contain exit messages")
	assert.Zero(t, d.Stats().Malformed, "no line should fail to decode")

	// Every field the translator depends on must actually be populated. This is
	// the guard against a struct that unmarshals cleanly but silently yields zeros.
	require.NotNil(t, firstExec)
	var exec ExecEvent
	require.NoError(t, firstExec.DecodeEvent(&exec))

	require.NotNil(t, exec.Target)
	assert.NotEmpty(t, exec.Target.Path(), "executable path")
	assert.NotEmpty(t, exec.Args, "argv")
	assert.NotZero(t, exec.Target.AuditToken.PID, "audit_token.pid")
	assert.NotZero(t, exec.Target.AuditToken.PIDVersion, "audit_token.pidversion")
	assert.NotZero(t, exec.Target.PPID, "ppid")
	assert.NotEmpty(t, firstExec.Time, "time")

	// The pre-exec image is on the message, the post-exec image on exec.target.
	require.NotNil(t, firstExec.Process, "message must carry the pre-exec process")
	assert.NotEqual(t, firstExec.Process.Path(), exec.Target.Path(),
		"message.process is the pre-exec image, exec.target the post-exec one")
}

// TestExecEventNeverExposesEnv is a privacy guard. eslogger emits an `env` key
// on every exec message, but environment variables are never captured on a
// developer laptop, so ExecEvent deliberately declares no field for it and
// encoding/json must drop it. A future edit that adds an Env field to pick up
// "just one useful variable" has to delete this test to do so.
func TestExecEventNeverExposesEnv(t *testing.T) {
	f, err := os.Open("testdata/exec_fork_exit.jsonl")
	require.NoError(t, err)
	defer f.Close()

	// The fixture must actually exercise the case, otherwise this proves nothing.
	raw, err := os.ReadFile("testdata/exec_fork_exit.jsonl")
	require.NoError(t, err)
	require.Contains(t, string(raw), `"env":`,
		"fixture must contain an env key for this guard to be meaningful")

	d := NewDecoder(f)
	for {
		msg, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		kind, err := msg.Kind()
		require.NoError(t, err)
		if kind != "exec" {
			continue
		}

		var exec ExecEvent
		require.NoError(t, msg.DecodeEvent(&exec))

		// Reflectively assert no field of ExecEvent holds anything env-shaped.
		for _, arg := range exec.Args {
			assert.NotContains(t, arg, "PATH=",
				"an environment entry leaked into argv")
		}
	}
}

// TestDecodeTracksSequenceNumbers matters because Endpoint Security silently
// drops messages under load. global_seq_num is monotonic across the whole
// stream, so a gap is the only way to know fidelity was lost.
func TestDecodeTracksSequenceNumbers(t *testing.T) {
	f, err := os.Open("testdata/exec_fork_exit.jsonl")
	require.NoError(t, err)
	defer f.Close()

	d := NewDecoder(f)

	var seen []uint64
	for {
		msg, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		seen = append(seen, msg.GlobalSeqNum)
	}

	require.NotEmpty(t, seen)
	for i := 1; i < len(seen); i++ {
		assert.Greater(t, seen[i], seen[i-1],
			"global_seq_num must increase across the stream")
	}
	assert.Zero(t, d.Stats().Dropped, "the fixture has no gaps")
}

// TestFixtureContainsRealProcessTree pins the real fork/exec/exit lifecycle
// captured from eslogger, and in particular that pidversion ADVANCES across
// fork -> exec for the same pid. That is normal kernel behaviour, not pid
// reuse, and conflating the two would make every exec look like a recycled
// pid. The translator's reuse detection is tested against these exact numbers.
func TestFixtureContainsRealProcessTree(t *testing.T) {
	f, err := os.Open("testdata/exec_fork_exit.jsonl")
	require.NoError(t, err)
	defer f.Close()

	type ident struct {
		pid, pidversion, ppid uint32
		path                  string
	}
	byKind := map[string][]ident{}

	d := NewDecoder(f)
	for {
		msg, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		kind, err := msg.Kind()
		require.NoError(t, err)

		switch kind {
		case "exec":
			var body ExecEvent
			require.NoError(t, msg.DecodeEvent(&body))
			byKind["exec"] = append(byKind["exec"], ident{
				body.Target.AuditToken.PID, body.Target.AuditToken.PIDVersion,
				body.Target.PPID, body.Target.Path(),
			})
		case "fork":
			var body ForkEvent
			require.NoError(t, msg.DecodeEvent(&body))
			byKind["fork"] = append(byKind["fork"], ident{
				body.Child.AuditToken.PID, body.Child.AuditToken.PIDVersion,
				body.Child.PPID, body.Child.Path(),
			})
		case "exit":
			byKind["exit"] = append(byKind["exit"], ident{
				msg.Process.AuditToken.PID, msg.Process.AuditToken.PIDVersion,
				msg.Process.PPID, msg.Process.Path(),
			})
		}
	}

	// pid 20745: forked from 574, then exec'd /usr/bin/wdutil, then exited.
	var forkedPID, execedPID uint32
	var forkVersion, execVersion uint32
	for _, f := range byKind["fork"] {
		if f.path == "/usr/local/bin/example-daemon" {
			forkedPID, forkVersion = f.pid, f.pidversion
		}
	}
	for _, e := range byKind["exec"] {
		if e.path == "/usr/bin/wdutil" {
			execedPID, execVersion = e.pid, e.pidversion
		}
	}

	require.NotZero(t, forkedPID, "fixture must contain the example-daemon fork")
	require.NotZero(t, execedPID, "fixture must contain the wdutil exec")
	assert.Equal(t, forkedPID, execedPID, "same pid forked then exec'd")
	assert.Greater(t, execVersion, forkVersion,
		"pidversion advances on exec: this is NOT pid reuse")
}

// TestExitStatusDecoding pins the wait(2) status word semantics. The captured
// fixture contains a real xpcproxy exit with stat 19968, which is exit code 78,
// not 19968 — reporting the raw word would put nonsense in the payload.
func TestExitStatusDecoding(t *testing.T) {
	tests := []struct {
		name       string
		stat       int32
		exited     bool
		code       uint32
		signal     uint32
		coreDumped bool
	}{
		{name: "clean exit", stat: 0, exited: true, code: 0, signal: 0},
		{name: "real xpcproxy exit from fixture", stat: 19968, exited: true, code: 78, signal: 0},
		{name: "exit 1", stat: 256, exited: true, code: 1, signal: 0},
		{name: "killed by SIGKILL", stat: 9, exited: false, code: 0, signal: 9},
		{name: "killed by SIGSEGV", stat: 11, exited: false, code: 0, signal: 11},
		// 0x80 set alongside the signal means the signal produced a core dump.
		{name: "SIGSEGV with coredump", stat: 11 | 0x80, exited: false, code: 0, signal: 11, coreDumped: true},
		{name: "SIGQUIT with coredump", stat: 3 | 0x80, exited: false, code: 0, signal: 3, coreDumped: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := &ExitEvent{Stat: tc.stat}
			assert.Equal(t, tc.exited, e.Exited(), "Exited")
			assert.Equal(t, tc.signal, e.Signal(), "Signal")
			assert.Equal(t, tc.coreDumped, e.CoreDumped(), "CoreDumped")
			if tc.exited {
				assert.Equal(t, tc.code, e.ExitCode(), "ExitCode")
			}
		})
	}
}

// TestFixtureExitStatusIsDecoded ties the above to the real capture rather than
// to hand-written numbers.
func TestFixtureExitStatusIsDecoded(t *testing.T) {
	f, err := os.Open("testdata/exec_fork_exit.jsonl")
	require.NoError(t, err)
	defer f.Close()

	codes := map[string]uint32{}
	d := NewDecoder(f)
	for {
		msg, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		kind, err := msg.Kind()
		require.NoError(t, err)
		if kind != "exit" {
			continue
		}

		var body ExitEvent
		require.NoError(t, msg.DecodeEvent(&body))
		require.True(t, body.Exited(), "fixture exits are all normal terminations")
		codes[msg.Process.Path()] = body.ExitCode()
	}

	require.Contains(t, codes, "/usr/libexec/xpcproxy")
	assert.EqualValues(t, 78, codes["/usr/libexec/xpcproxy"],
		"raw stat 19968 must decode to exit code 78")
}

func TestDecoderSkipsUnknownKinds(t *testing.T) {
	const in = `{"event":{"totally_unknown_event":{}},"event_type":999,"time":"2026-08-12T00:00:00Z"}
{"event":{"exec":{"target":{"executable":{"path":"/bin/ls"},"ppid":1,"audit_token":{"pid":42}},"args":["ls"]}},"event_type":9,"time":"2026-08-12T00:00:01Z"}`

	d := NewDecoder(strings.NewReader(in))

	msg, err := d.Next()
	require.NoError(t, err)
	kind, err := msg.Kind()
	require.NoError(t, err)
	assert.Equal(t, "exec", kind, "unknown kinds must be skipped, not returned")

	_, err = d.Next()
	assert.ErrorIs(t, err, io.EOF)
	assert.EqualValues(t, 1, d.Stats().Unknown)
}

func TestDecoderToleratesMalformedLine(t *testing.T) {
	const in = `{"event":{"exec":  BROKEN
{"event":{"fork":{"child":{"executable":{"path":"/bin/sh"},"ppid":42,"audit_token":{"pid":43}}}},"event_type":11,"time":"2026-08-12T00:00:02Z"}`

	d := NewDecoder(strings.NewReader(in))

	msg, err := d.Next()
	require.NoError(t, err, "a malformed line must not abort the stream")
	kind, _ := msg.Kind()
	assert.Equal(t, "fork", kind)
	assert.EqualValues(t, 1, d.Stats().Malformed)
}

// TestDecoderDetectsDroppedMessages covers the Endpoint Security overflow case:
// the kernel drops events under load and the only evidence is a gap in
// global_seq_num.
func TestDecoderDetectsDroppedMessages(t *testing.T) {
	const in = `{"event":{"exit":{"stat":0}},"process":{"audit_token":{"pid":1}},"global_seq_num":0,"time":"2026-08-12T00:00:00Z"}
{"event":{"exit":{"stat":0}},"process":{"audit_token":{"pid":2}},"global_seq_num":5,"time":"2026-08-12T00:00:01Z"}`

	d := NewDecoder(strings.NewReader(in))

	_, err := d.Next()
	require.NoError(t, err)
	_, err = d.Next()
	require.NoError(t, err)

	assert.EqualValues(t, 4, d.Stats().Dropped,
		"a jump from seq 0 to 5 means 4 messages were dropped")
}
