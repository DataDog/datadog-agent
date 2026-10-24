// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	sandboxlandlock "github.com/DataDog/datadog-agent/pkg/rshell/internal/sandbox/landlock"
	sandboxseccomp "github.com/DataDog/datadog-agent/pkg/rshell/internal/sandbox/seccomp"
	internalsystemd "github.com/DataDog/datadog-agent/pkg/rshell/internal/systemd"
	"github.com/DataDog/datadog-agent/pkg/rshell/privilegedhelper"
)

const (
	workerProtocolVersion = 1
	maxWorkerMessageBytes = 4 << 20
	maxWorkerFrameBytes   = maxWorkerMessageBytes + 4
	maxWorkerStderrBytes  = 64 << 10
)

type workerRequest struct {
	Version int                               `json:"version"`
	Command *privilegedhelper.VerifiedCommand `json:"command"`
}

type workerResponse struct {
	Version int                               `json:"version"`
	Result  *privilegedhelper.ExecuteResponse `json:"result,omitempty"`
	Error   string                            `json:"error,omitempty"`
}

func runPrivilegedWorker(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "privileged-worker does not accept arguments")
		return 2
	}
	if os.Getuid() != 0 {
		fmt.Fprintln(stderr, "privileged-worker requires real uid 0")
		return 1
	}
	unprivilegedUID := os.Geteuid()
	if unprivilegedUID == 0 {
		fmt.Fprintln(stderr, "privileged-worker requires an unprivileged effective uid")
		return 1
	}
	return servePrivilegedWorker(ctx, stdin, stdout, stderr, func(ctx context.Context, command *privilegedhelper.VerifiedCommand) (*privilegedhelper.ExecuteResponse, error) {
		return executeVerifiedCommand(ctx, command, unprivilegedUID)
	})
}

type workerExecuteFunc func(context.Context, *privilegedhelper.VerifiedCommand) (*privilegedhelper.ExecuteResponse, error)

func servePrivilegedWorker(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, execute workerExecuteFunc) int {
	var request workerRequest
	if err := readWorkerMessage(stdin, &request); err != nil {
		fmt.Fprintf(stderr, "read privileged worker request: %v\n", err)
		return 1
	}
	if request.Version != workerProtocolVersion {
		fmt.Fprintf(stderr, "unsupported privileged worker request version %d\n", request.Version)
		return 1
	}
	if request.Command == nil {
		fmt.Fprintln(stderr, "privileged worker request has no command")
		return 1
	}
	result, err := execute(ctx, request.Command)
	response := workerResponse{Version: workerProtocolVersion, Result: result}
	if err != nil {
		response.Result = nil
		response.Error = err.Error()
	}
	if err := writeWorkerMessage(stdout, response); err != nil {
		fmt.Fprintf(stderr, "write privileged worker response: %v\n", err)
		return 1
	}
	return 0
}

func writeWorkerMessage(w io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	if len(payload) == 0 || len(payload) > maxWorkerMessageBytes {
		return fmt.Errorf("message size %d is outside the allowed range", len(payload))
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(payload)))
	if _, err := w.Write(size[:]); err != nil {
		return fmt.Errorf("write message size: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

func readWorkerMessage(r io.Reader, value any) error {
	var size [4]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return fmt.Errorf("read message size: %w", err)
	}
	n := binary.BigEndian.Uint32(size[:])
	if n == 0 || n > maxWorkerMessageBytes {
		return fmt.Errorf("invalid message size %d", n)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return fmt.Errorf("read message: %w", err)
	}
	var trailing [1]byte
	if count, err := r.Read(trailing[:]); count != 0 || !errors.Is(err, io.EOF) {
		return errors.New("message contains trailing data")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode message: %w", err)
	}
	if err := ensureWorkerJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

func ensureWorkerJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("message contains trailing JSON data")
	}
	return nil
}

// applyWorkerSandbox installs irreversible per-command restrictions in the
// one-shot child after parsing and before the interpreter is constructed.
// Landlock must run first because the final seccomp policy denies prctl.
func applyWorkerSandbox(command *privilegedhelper.VerifiedCommand, systemdTarget internalsystemd.Target) error {
	trustedPaths := trustedPathsForPolicy(command, systemdTarget)
	restrict := sandboxlandlock.RestrictWithTrustedPaths
	if command.Mode == privilegedhelper.ExecutionModeReadOnly {
		restrict = sandboxlandlock.RestrictReadOnlyWithTrustedPaths
	}
	if err := restrict(command.AllowedPaths, trustedPaths); err != nil {
		return fmt.Errorf("apply Landlock policy: %w", err)
	}
	if err := sandboxseccomp.RestrictDefault(); err != nil {
		return fmt.Errorf("apply seccomp policy: %w", err)
	}
	return nil
}

func trustedPathsForPolicy(command *privilegedhelper.VerifiedCommand, systemdTarget internalsystemd.Target) []sandboxlandlock.TrustedPath {
	trusted := trustedPathsForCommands(command.AllowedCommands)
	journalRead := slices.Contains(command.AllowedCommands, "rshell:journalctl") &&
		systemServiceActionGranted(command.AllowedSystemServices, "", "read")
	journalClean := slices.Contains(command.AllowedCommands, "rshell:journalctl") && command.Mode == privilegedhelper.ExecutionModeRemediation &&
		systemServiceActionGranted(command.AllowedSystemServices, "systemd-journald.service", "clean")
	managerAccess := slices.Contains(command.AllowedCommands, "rshell:systemctl") && command.Mode == privilegedhelper.ExecutionModeRemediation &&
		systemServiceActionGranted(command.AllowedSystemServices, "", "read", "start", "stop", "reload", "restart", "enable", "disable")

	if journalRead || journalClean || managerAccess {
		trusted = append(trusted, trustedReadOnlyFile(systemdTarget.MachineIDPath))
	}
	if journalRead || journalClean {
		access := sandboxlandlock.TrustedPathReadOnly
		if journalClean {
			access = sandboxlandlock.TrustedPathReadRemoveFiles
		}
		for _, path := range systemdTarget.JournalDirs {
			trusted = append(trusted, sandboxlandlock.TrustedPath{
				Path: path, Kind: sandboxlandlock.TrustedPathDirectory, Access: access, Optional: true,
			})
		}
	}
	return trusted
}

func systemServiceActionGranted(services map[string][]string, service string, actions ...string) bool {
	for grantedService, grantedActions := range services {
		if service != "" && grantedService != service {
			continue
		}
		if slices.Contains(grantedActions, "*") {
			return true
		}
		for _, action := range actions {
			if slices.Contains(grantedActions, action) {
				return true
			}
		}
	}
	return false
}

// trustedPathsForCommands returns fixed host paths used directly by registered
// Go builtins. These grants are derived locally from the verified effective
// command allowlist; the backend cannot supply arbitrary trusted paths.
//
// We grant the complete fixed path set a permitted builtin may use because
// shell expansion can determine its flags at runtime. This remains bounded to
// hard-coded read-only paths and avoids trying to duplicate shell evaluation
// before dispatch.
func trustedPathsForCommands(allowedCommands []string) []sandboxlandlock.TrustedPath {
	allowed := make(map[string]bool, len(allowedCommands))
	for _, command := range allowedCommands {
		allowed[command] = true
	}

	trusted := make([]sandboxlandlock.TrustedPath, 0, 8)
	if allowed["rshell:ps"] {
		trusted = append(trusted, trustedReadOnlyDirectory("/proc"))
	} else if allowed["rshell:ss"] || allowed["rshell:ip"] {
		trusted = append(trusted, trustedReadOnlyDirectory("/proc/net"))
	}
	if allowed["rshell:df"] {
		trusted = append(trusted, trustedReadOnlyFile("/proc/self/mountinfo"))
	}
	if allowed["rshell:uname"] {
		for _, path := range []string{
			"/proc/sys/kernel/ostype",
			"/proc/sys/kernel/hostname",
			"/proc/sys/kernel/osrelease",
			"/proc/sys/kernel/version",
			"/proc/sys/kernel/arch",
		} {
			trusted = append(trusted, trustedReadOnlyFile(path))
		}
	}
	return trusted
}

func trustedReadOnlyDirectory(path string) sandboxlandlock.TrustedPath {
	return sandboxlandlock.TrustedPath{
		Path:   path,
		Kind:   sandboxlandlock.TrustedPathDirectory,
		Access: sandboxlandlock.TrustedPathReadOnly,
	}
}

func trustedReadOnlyFile(path string) sandboxlandlock.TrustedPath {
	return sandboxlandlock.TrustedPath{
		Path:   path,
		Kind:   sandboxlandlock.TrustedPathFile,
		Access: sandboxlandlock.TrustedPathReadOnly,
	}
}
