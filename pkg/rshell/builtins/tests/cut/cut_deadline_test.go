// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cut_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// cutDeadlineDelim builds a 1 MiB output delimiter in the shell from a
// 512 KiB variable, so the script itself stays small.
const cutDeadlineDelim = `b=x; for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19; do b=$b$b; done; `

// runCutWithDeadline runs script with a short execution deadline and fails
// the test if the run does not return well within a fixed bound. The run
// happens in a goroutine so a regression fails promptly instead of hanging
// the test binary until the go test timeout.
func runCutWithDeadline(t *testing.T, script, dir string) {
	t.Helper()
	const deadline = 500 * time.Millisecond
	const bound = 5 * time.Second

	prog, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	require.NoError(t, err)
	var errBuf bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, io.Discard, &errBuf),
		interp.AllowedPaths([]string{dir}),
		interpoption.AllowAllCommands().(interp.RunnerOption),
	)
	require.NoError(t, err)
	t.Cleanup(func() { runner.Close() })
	runner.Dir = dir

	done := make(chan struct{})
	start := time.Now()
	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), deadline)
		defer cancel()
		_ = runner.Run(ctx, prog)
	}()
	select {
	case <-done:
		t.Logf("run returned after %v (stderr: %q)", time.Since(start), errBuf.String())
	case <-time.After(bound):
		t.Fatalf("cut did not honour a %v deadline: still running after %v", deadline, bound)
	}
}

// One line holding many empty fields, each followed by a 1 MiB output
// delimiter written to the capped run output. The run must end promptly
// instead of copying the delimiter on every field past the deadline.
func TestCutOutputDelimiterFieldsHonoursDeadline(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "in.txt"),
		[]byte(strings.Repeat(",", 100_000)+"\n"), 0644))
	runCutWithDeadline(t, cutDeadlineDelim+`cut -d , -f 1- --output-delimiter="$b$b" in.txt`, dir)
}

// Byte mode emits the output delimiter once per selected range.
func TestCutOutputDelimiterBytesHonoursDeadline(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "in.txt"),
		[]byte(strings.Repeat("a", 100_000)+"\n"), 0644))
	positions := make([]string, 0, 50_000)
	for i := 1; i <= 100_000; i += 2 {
		positions = append(positions, strconv.Itoa(i))
	}
	list := strings.Join(positions, ",")

	t.Run("ranges", func(t *testing.T) {
		runCutWithDeadline(t, cutDeadlineDelim+`cut -b `+list+` --output-delimiter="$b$b" in.txt`, dir)
	})
	t.Run("complement", func(t *testing.T) {
		runCutWithDeadline(t, cutDeadlineDelim+`cut --complement -b `+list+` --output-delimiter="$b$b" in.txt`, dir)
	})
}

// The cancellation checks must not change normal output. Expected values
// match GNU coreutils cut 9.4.
func TestCutOutputDelimiterOutputUnchanged(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "in.txt"),
		[]byte("a,b,c,d\nno-delim\n,,\nabcdef\n"), 0644))
	cases := []struct {
		script string
		want   string
	}{
		{`cut -d , -f 1,3 --output-delimiter=:: in.txt`, "a::c\nno-delim\n::\nabcdef\n"},
		{`cut -d , -f 2- -s --output-delimiter=- in.txt`, "b-c-d\n-\n"},
		{`cut -d , --complement -f 2 in.txt`, "a,c,d\nno-delim\n,\nabcdef\n"},
		{`cut -b 1,3-4 --output-delimiter=. in.txt`, "a.b,\nn.-d\n,\na.cd\n"},
		{`cut --complement -b 2 --output-delimiter=. in.txt`, "a.b,c,d\nn.-delim\n,\na.cdef\n"},
		{`cut -c 2-3 in.txt`, ",b\no-\n,\nbc\n"},
	}
	for _, tc := range cases {
		t.Run(tc.script, func(t *testing.T) {
			stdout, stderr, code := testutil.RunScript(t, tc.script, dir, interp.AllowedPaths([]string{dir}))
			require.Equal(t, 0, code, "stderr: %s", stderr)
			assert.Equal(t, tc.want, stdout)
		})
	}
}
