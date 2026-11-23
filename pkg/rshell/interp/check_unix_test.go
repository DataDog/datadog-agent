// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package interp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckDoesNotOpenFIFO(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, unix.Mkfifo(filepath.Join(root, "fifo"), 0600))
	runner := checkRunner(t, allowAllCommandsOpt(), AllowedPaths([]string{root + ":rw"}), WithMode(ModeRemediation))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan *CheckResult, 1)
	go func() {
		result, _ := runner.Check(ctx, "cat fifo; echo hi > fifo")
		done <- result
	}()
	select {
	case result := <-done:
		require.NotNil(t, result)
		assert.Equal(t, CheckDenied, result.Status)
		assert.True(t, checkHasIssue(result, CheckPathNotAllowed))
	case <-ctx.Done():
		t.Fatal("Check blocked opening a FIFO")
	}
}

func TestCheckCDParentTraversal(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "link")))
	for _, mode := range []string{"", "-L ", "-P "} {
		t.Run(mode, func(t *testing.T) {
			runner := checkRunner(t, AllowedCommands([]string{"rshell:cd"}), AllowedPaths([]string{root}))
			script := "cd " + mode + "link/.."
			report := checkScript(t, runner, script)
			assert.Equal(t, CheckIndeterminate, report.Status)
			require.Len(t, report.Commands, 1)
			assert.Equal(t, "link/..", report.Commands[0].Issues[0].Path)
			program, err := ParseScript(script, "")
			require.NoError(t, err)
			require.Error(t, runner.Run(context.Background(), program), "execution must still deny the intermediate sandbox escape")
			remote := checkRemoteScript(t, script, RemotePolicy{AllowedCommands: []string{"rshell:cd"}, AllowedPaths: []string{root}, Dir: root})
			assert.Equal(t, CheckIndeterminate, remote.Status)
		})
	}
}

func TestCheckRemoteWithDeletedLocalCWDAndHardLinks(t *testing.T) {
	local := t.TempDir()
	root := filepath.Join(local, "root")
	require.NoError(t, os.Mkdir(root, 0700))
	file := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(file, []byte("preserve"), 0600))
	policy := RemotePolicy{AllowedCommands: []string{"rshell:echo", "rshell:cat"}, AllowedPaths: []string{root + ":rw"}, Mode: ModeRemediation}
	script := "echo hi > " + quoteCheckPath(file) + "; cat relative"
	before := checkRemoteScript(t, script, policy)
	require.NoError(t, os.Link(file, filepath.Join(local, "outside-alias")))
	deleted := filepath.Join(local, "deleted-cwd")
	require.NoError(t, os.Mkdir(deleted, 0700))
	t.Chdir(deleted)
	require.NoError(t, os.Remove(deleted))
	assert.Equal(t, before, checkRemoteScript(t, script, policy), "remote checking must never call Getwd or inspect hard links")
}
