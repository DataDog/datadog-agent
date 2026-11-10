// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && !cgo

package allowedpaths

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/sandbox/landlock"
	"github.com/DataDog/datadog-agent/pkg/rshell/internal/sandbox/seccomp"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestUsermodUnderExistingSandbox(t *testing.T) {
	if dir := os.Getenv("RSHELL_USERMOD_SANDBOX_FIXTURE"); dir != "" {
		if !strings.HasPrefix(dir, os.TempDir()+string(os.PathSeparator)) {
			os.Exit(2)
		}
		sandbox, _, err := New([]string{dir + ":rw"})
		if err != nil {
			t.Fatal(err)
		}
		sandbox.SetWritable()
		if err := landlock.Restrict([]string{dir + ":rw"}); err != nil {
			t.Fatal(err)
		}
		if err := seccomp.RestrictDefault(); err != nil {
			t.Fatal(err)
		}
		if err := sandbox.addDDAgentToDocker(context.Background(), dir); err != nil {
			t.Fatal(err)
		}
		if err := sandbox.addDDAgentToDocker(context.Background(), dir); err != nil {
			t.Fatal(err)
		}
		if err := unix.Chmod(filepath.Join(dir, "group"), 0o600); !errors.Is(err, unix.EPERM) {
			t.Fatalf("chmod: %v", err)
		}
		os.Exit(0)
	}
	dir, _ := usermodFixture(t)
	binary, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(binary, "-test.run=^TestUsermodUnderExistingSandbox$")
	cmd.Env = append(os.Environ(), "RSHELL_USERMOD_SANDBOX_FIXTURE="+dir)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	data, err := os.ReadFile(filepath.Join(dir, "group"))
	require.NoError(t, err)
	require.True(t, bytes.Contains(data, []byte("alice,dd-agent")))
}
