// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestShellScripts discovers and runs every *.sh file under test/shell/ against
// the built agent binary. The tests are skipped automatically when no agent
// binary or POSIX shell is available, so they are safe to run in any environment.
// On Windows, sh.exe is expected to be provided by Git for Windows / MSYS2.
//
// To run these tests locally:
//
//	dda inv agent.build --build-exclude=systemd
//	go test ./pkg/shell/interp -run TestShellScripts
//
// Or point at an existing binary:
//
//	AGENT_BIN=/path/to/agent go test ./pkg/shell/interp -run TestShellScripts
func TestShellScripts(t *testing.T) {
	shBin, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not found in PATH; install Git for Windows / MSYS2 to run shell tests on Windows")
	}

	agentBin := findAgentBinaryForShellTests(t)

	scripts, err := filepath.Glob(filepath.Join("test", "shell", "*", "*.sh"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(scripts) == 0 {
		t.Skip("no shell scripts found in test/shell/")
	}

	for _, script := range scripts {
		// Use the relative path from test/shell/ as the subtest name so it
		// reads like "tail/basic-seek.sh" rather than just "basic-seek.sh".
		rel, _ := filepath.Rel(filepath.Join("test", "shell"), script)
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(shBin, script)
			cmd.Env = append(os.Environ(), "AGENT_BIN="+agentBin)
			out, err := cmd.CombinedOutput()
			if err == nil {
				return
			}
			if exitErr, ok := err.(*exec.ExitError); ok {
				// Exit code 77 is the GNU test framework's "skip" convention.
				if exitErr.ExitCode() == 77 {
					t.Skipf("script skipped: %s", out)
				}
				t.Fatalf("script failed (exit %d):\n%s", exitErr.ExitCode(), out)
			}
			t.Fatalf("failed to run script: %v\n%s", err, out)
		})
	}
}

// findAgentBinaryForShellTests locates the agent binary, skipping the test
// suite if it cannot be found. Resolution order:
//  1. AGENT_BIN environment variable
//  2. bin/agent/agent relative to the repository root (3 levels up)
//  3. "agent" in PATH
func findAgentBinaryForShellTests(t *testing.T) string {
	t.Helper()

	if bin := os.Getenv("AGENT_BIN"); bin != "" {
		return bin
	}

	// pkg/shell/interp is 3 levels below the repo root.
	candidate := filepath.Join("..", "..", "..", "bin", "agent", "agent")
	if _, err := os.Stat(candidate); err == nil {
		abs, _ := filepath.Abs(candidate)
		return abs
	}

	if bin, err := exec.LookPath("agent"); err == nil {
		return bin
	}

	t.Skip("agent binary not found; build with: dda inv agent.build --build-exclude=systemd, or set AGENT_BIN=/path/to/agent")
	return ""
}
