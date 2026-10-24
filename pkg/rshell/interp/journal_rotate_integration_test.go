// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package interp

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeJournalControlRequest is the subset of the io.systemd.Journal Varlink
// request payload rotate cares about.
type fakeJournalControlRequest struct {
	Method string `json:"method"`
}

// startFakeJournalControlSocket listens on a real Unix domain socket at path
// and, for each accepted connection, reads a single NUL-terminated Varlink
// request and replies with response followed by a NUL terminator. Every
// decoded request is sent to the returned channel so the test can assert the
// real internal/systemd Varlink client actually dialed this socket and sent
// the expected method, rather than merely trusting a stub. This deliberately
// re-implements the tiny NUL-terminated framing internal/systemd uses
// (see internal/systemd/journal_varlink.go's readVarlinkMessage) instead of
// importing it, since that helper is unexported and this test lives in a
// different package exercising the feature end-to-end through the real
// Runner/builtin dispatch path.
func startFakeJournalControlSocket(t *testing.T, path string, response []byte) <-chan fakeJournalControlRequest {
	t.Helper()
	listener, err := net.Listen("unix", path)
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	requests := make(chan fakeJournalControlRequest, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		var buf bytes.Buffer
		chunk := make([]byte, 4096)
		for {
			n, err := conn.Read(chunk)
			if n > 0 {
				buf.Write(chunk[:n])
				if idx := bytes.IndexByte(buf.Bytes(), 0); idx >= 0 {
					break
				}
			}
			if err != nil {
				return
			}
		}
		raw := buf.Bytes()
		terminator := bytes.IndexByte(raw, 0)
		var decoded fakeJournalControlRequest
		if jsonErr := json.Unmarshal(raw[:terminator], &decoded); jsonErr == nil {
			requests <- decoded
		}

		reply := append(append([]byte(nil), response...), 0)
		_, _ = conn.Write(reply)
	}()
	return requests
}

// TestJournalctlRotateEndToEnd exercises journalctl --rotate through a real
// Runner in ModeRemediation, dispatched to the real (non-stubbed)
// internal/systemd.Client.RotateJournal implementation: real machine-ID file
// validation, a real O_PATH-pinned open of the configured journal control
// socket path, and a real Varlink request/response exchange over an actual
// Unix domain socket connection (net.Listen("unix", ...) + net.Dial via
// /proc/self/fd, exactly as internal/systemd/journal_varlink_dial_linux.go
// does it in production).
//
// A genuine end-to-end rotation against a real systemd-journald is not
// fixtured here: that would require a real running journald process (or a
// faithful from-scratch reimplementation of journald's rotation side
// effects), which isn't safe or available in this test environment — the
// same category of limitation documented in AGENTS.md for lsof/free/ip
// route's platform-conditional coverage. Instead, everything on rshell's
// side of the wire — the sandboxed socket open, the pinned dial, and the
// Varlink framing/response handling — runs for real; only the daemon at the
// other end of the socket is a fixture that speaks the same protocol.
func TestJournalctlRotateEndToEnd(t *testing.T) {
	root := t.TempDir()
	machineID := "0123456789abcdef0123456789abcdef"
	require.NoError(t, os.MkdirAll(filepath.Join(root, "etc"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "etc", "machine-id"), []byte(machineID+"\n"), 0o600))

	socketPath := filepath.Join(root, "journal.sock")
	requests := startFakeJournalControlSocket(t, socketPath, []byte(`{"parameters":{}}`))

	var stdout, stderr bytes.Buffer
	runner, err := New(
		StdIO(nil, &stdout, &stderr),
		AllowedCommands([]string{"rshell:journalctl"}),
		AllowedSystemServices([]SystemdControlGrant{{
			Service: "systemd-journald.service",
			Actions: []SystemServiceAction{SystemServiceClean},
		}}),
		WithMode(ModeRemediation),
		WithSystemdTarget(SystemdTargetConfig{
			MachineIDPath:        filepath.Join(root, "etc", "machine-id"),
			JournalControlSocket: socketPath,
		}),
	)
	require.NoError(t, err)
	defer runner.Close()

	program, err := ParseScript("journalctl --rotate", "")
	require.NoError(t, err)
	require.NoError(t, runner.Run(context.Background(), program))
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Journal rotation completed.")

	select {
	case req := <-requests:
		assert.Equal(t, "io.systemd.Journal.Rotate", req.Method)
	case <-time.After(5 * time.Second):
		t.Fatal("fake journal control socket never received a request")
	}
}

// TestJournalctlRotateDeniedWithoutRemediationDespiteGrant strengthens the
// rotate_read_only.yaml scenario test. That scenario runs with no
// AllowedSystemServices grant and no explicit systemd target configured at
// all, so it cannot on its own prove the remediation-mode gate is what's
// blocking the request rather than, say, the grant or backend simply being
// absent. Here the request is otherwise fully authorized (grant present) and
// a real journal control socket target is configured, yet remediation mode
// is left off (the Runner default), isolating the assertion to the
// remediation gate itself: interp/system_services.go's authorizeSystemd
// checks remediation mode before consulting the grant map, so this must
// fail with the same "requires remediation mode" message and must never
// reach the fake backend.
func TestJournalctlRotateDeniedWithoutRemediationDespiteGrant(t *testing.T) {
	root := t.TempDir()
	machineID := "0123456789abcdef0123456789abcdef"
	require.NoError(t, os.MkdirAll(filepath.Join(root, "etc"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "etc", "machine-id"), []byte(machineID+"\n"), 0o600))

	socketPath := filepath.Join(root, "journal.sock")
	requests := startFakeJournalControlSocket(t, socketPath, []byte(`{"parameters":{}}`))

	var stdout, stderr bytes.Buffer
	runner, err := New(
		StdIO(nil, &stdout, &stderr),
		AllowedCommands([]string{"rshell:journalctl"}),
		AllowedSystemServices([]SystemdControlGrant{{
			Service: "systemd-journald.service",
			Actions: []SystemServiceAction{SystemServiceClean},
		}}),
		// Remediation mode intentionally left at its default (off).
		WithSystemdTarget(SystemdTargetConfig{
			MachineIDPath:        filepath.Join(root, "etc", "machine-id"),
			JournalControlSocket: socketPath,
		}),
	)
	require.NoError(t, err)
	defer runner.Close()

	program, err := ParseScript("journalctl --rotate", "")
	require.NoError(t, err)
	runErr := runner.Run(context.Background(), program)
	var exitStatus ExitStatus
	require.ErrorAs(t, runErr, &exitStatus)
	assert.Equal(t, ExitStatus(1), exitStatus)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "journalctl: systemd action \"clean\" requires remediation mode\n", stderr.String())

	select {
	case <-requests:
		t.Fatal("fake journal control socket received a request despite remediation mode being off")
	case <-time.After(200 * time.Millisecond):
	}
}
