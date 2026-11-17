// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package pmap_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func runScriptWithProcPath(t *testing.T, script, procPath string) (stdout, stderr string, code int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		t.Fatal(err)
	}
	var outBuf, errBuf bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &outBuf, &errBuf),
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.ProcPath(procPath),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	runErr := runner.Run(context.Background(), prog)
	exitCode := 0
	if runErr != nil {
		var es interp.ExitStatus
		if errors.As(runErr, &es) {
			exitCode = int(es)
		} else {
			t.Fatalf("unexpected runner error: %v", runErr)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// writeFakePmapProc builds a fake <procPath>/<pid>/{comm,maps,smaps,cmdline}
// tree and returns the fabricated proc root path.
func writeFakePmapProc(t *testing.T, pid int, comm, maps, smaps string, cmdline []byte) string {
	t.Helper()
	procPath := t.TempDir()
	pidDir := filepath.Join(procPath, strconv.Itoa(pid))
	require.NoError(t, os.MkdirAll(pidDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pidDir, "comm"), []byte(comm), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pidDir, "maps"), []byte(maps), 0o644))
	if smaps != "" {
		require.NoError(t, os.WriteFile(filepath.Join(pidDir, "smaps"), []byte(smaps), 0o644))
	}
	if cmdline != nil {
		require.NoError(t, os.WriteFile(filepath.Join(pidDir, "cmdline"), cmdline, 0o644))
	}
	return procPath
}

const fixtureMaps = `00400000-00452000 r-xp 00000000 08:01 173521 /usr/bin/example
00e03000-00e24000 rw-p 00000000 00:00 0 [heap]
7ffe0dd6b000-7ffe0dd8c000 rw-p 00000000 00:00 0 [stack]
`

const fixtureSmaps = `00400000-00452000 r-xp 00000000 08:01 173521 /usr/bin/example
Rss:                 200 kB
Private_Dirty:        50 kB
Shared_Dirty:          10 kB
00e03000-00e24000 rw-p 00000000 00:00 0 [heap]
Rss:                 132 kB
Private_Dirty:       132 kB
Shared_Dirty:          0 kB
00007ffe0dd6b000-7ffe0dd8c000 rw-p 00000000 00:00 0 [stack]
Rss:                  12 kB
Private_Dirty:        12 kB
Shared_Dirty:          0 kB
`

// TestProcPathFakePmapExactOutput asserts pmap's exact stdout against a
// fixed fake proc tree, rather than merely checking substrings.
func TestProcPathFakePmapExactOutput(t *testing.T) {
	procPath := writeFakePmapProc(t, 42, "example\n", fixtureMaps, "", nil)

	stdout, stderr, code := runScriptWithProcPath(t, "pmap 42", procPath)
	require.Equalf(t, 0, code, "stderr: %s", stderr)
	require.Empty(t, stderr)

	want := "42:   example\n" +
		"0000000000400000    328K r-x-- example\n" +
		"0000000000e03000    132K rw--- [heap]\n" +
		"00007ffe0dd6b000    132K rw--- [stack]\n" +
		" total           592K\n"
	require.Equal(t, want, stdout)
}

// TestProcPathFakePmapExtendedExactOutput asserts pmap -x's exact stdout
// against a fixed fake smaps fixture.
func TestProcPathFakePmapExtendedExactOutput(t *testing.T) {
	procPath := writeFakePmapProc(t, 42, "example\n", "", fixtureSmaps, nil)

	stdout, stderr, code := runScriptWithProcPath(t, "pmap -x 42", procPath)
	require.Equalf(t, 0, code, "stderr: %s", stderr)
	require.Empty(t, stderr)

	want := "42:   example\n" +
		"Address           Kbytes     RSS   Dirty Mode  Mapping\n" +
		"0000000000400000     328     200      60 r-x-- example\n" +
		"0000000000e03000     132     132     132 rw--- [heap]\n" +
		"00007ffe0dd6b000     132      12      12 rw--- [stack]\n" +
		"---------------- ------- ------- -------\n" +
		"total kB             592     344     204\n"
	require.Equal(t, want, stdout)
}

// TestProcPathPmapCmdlineArgvNotLeaked ensures pmap never exposes argv from
// <procPath>/<pid>/cmdline, even though procmaps does not currently read
// that file at all — a defense-in-depth regression test mirroring ps's
// equivalent (builtins/ps/ps_procpath_linux_test.go).
func TestProcPathPmapCmdlineArgvNotLeaked(t *testing.T) {
	const secret = "rshell-secret-argv-leak"
	procPath := writeFakePmapProc(t, 42, "safeproc\n", fixtureMaps, "",
		[]byte("safeproc\x00--token="+secret+"\x00--password=hunter2\x00"))

	stdout, stderr, code := runScriptWithProcPath(t, "pmap 42", procPath)
	require.Equalf(t, 0, code, "stderr: %s", stderr)
	require.Contains(t, stdout, "safeproc")
	require.NotContains(t, stdout, secret)
	require.NotContains(t, stdout, "--token")
	require.NotContains(t, stdout, "--password")
}
