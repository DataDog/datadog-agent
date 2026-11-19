// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package apmattachimpl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFakeHelper writes an executable shell script standing in for the injector's attach helper.
func writeFakeHelper(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "attach")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	return path
}

func TestExecHelperParsesResult(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	path := writeFakeHelper(t, dir, `echo "$@" > `+argsFile+`
echo "helper log line" >&2
printf '%s\n' '{"pid":4242,"language":"java","decision":"attached","reason":"ok","detail":"loaded","rule":"allow java","layer":"user","service":"checkout","tracer_version":"1.69.0","duration_ms":1056}'`)

	h := &execHelper{path: path, timeout: 20 * time.Second, grace: time.Second}
	res := h.run(context.Background(), 4242)

	assert.Equal(t, helperResult{
		PID: 4242, Language: "java", Decision: decisionAttached, Reason: "ok", Detail: "loaded",
		Rule: "allow java", Layer: "user", Service: "checkout", TracerVersion: "1.69.0", DurationMs: 1056,
	}, res)
	args, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	assert.Equal(t, "--pid 4242 --timeout 20s", strings.TrimSpace(string(args)))
}

func TestExecHelperFailedExitCodeKeepsJSON(t *testing.T) {
	path := writeFakeHelper(t, t.TempDir(), `echo '{"pid":7,"language":"java","decision":"failed","reason":"attach_error","detail":"handshake timed out"}'
exit 1`)
	res := (&execHelper{path: path, timeout: time.Second, grace: time.Second}).run(context.Background(), 7)
	assert.Equal(t, decisionFailed, res.Decision)
	assert.Equal(t, "attach_error", res.Reason)
	assert.Equal(t, "handshake timed out", res.Detail)
}

func TestExecHelperSkipped(t *testing.T) {
	path := writeFakeHelper(t, t.TempDir(), `echo '{"pid":8,"language":"java","decision":"skipped","reason":"workload_selection_denied","rule":"deny checkout","layer":"rc_org"}'`)
	res := (&execHelper{path: path, timeout: time.Second, grace: time.Second}).run(context.Background(), 8)
	assert.Equal(t, decisionSkipped, res.Decision)
	assert.Equal(t, "workload_selection_denied", res.Reason)
	assert.Equal(t, "deny checkout", res.Rule)
	assert.Equal(t, "rc_org", res.Layer)
}

func TestExecHelperUsageErrorWithoutJSON(t *testing.T) {
	path := writeFakeHelper(t, t.TempDir(), `echo "attach: must run as root" >&2
exit 2`)
	res := (&execHelper{path: path, timeout: time.Second, grace: time.Second}).run(context.Background(), 9)
	assert.Equal(t, 9, res.PID)
	assert.Equal(t, decisionFailed, res.Decision)
	assert.Equal(t, reasonHelperError, res.Reason)
	assert.Contains(t, res.Detail, "exited with code 2")
	assert.Contains(t, res.Detail, "must run as root")
}

func TestExecHelperTimeout(t *testing.T) {
	path := writeFakeHelper(t, t.TempDir(), `exec sleep 30`)
	start := time.Now()
	res := (&execHelper{path: path, timeout: 100 * time.Millisecond, grace: 100 * time.Millisecond}).run(context.Background(), 10)
	assert.Less(t, time.Since(start), 10*time.Second)
	assert.Equal(t, decisionFailed, res.Decision)
	assert.Equal(t, reasonHelperTimeout, res.Reason)
}

func TestExecHelperRejectsResultForOtherPID(t *testing.T) {
	path := writeFakeHelper(t, t.TempDir(), `echo '{"pid":999,"language":"java","decision":"attached","reason":"ok"}'`)
	res := (&execHelper{path: path, timeout: time.Second, grace: time.Second}).run(context.Background(), 7)
	assert.Equal(t, 7, res.PID)
	assert.Equal(t, decisionFailed, res.Decision)
	assert.Equal(t, reasonHelperError, res.Reason)
	assert.Contains(t, res.Detail, "pid 999")
}

func TestExecHelperMissingBinary(t *testing.T) {
	res := (&execHelper{path: filepath.Join(t.TempDir(), "missing"), timeout: time.Second, grace: time.Second}).run(context.Background(), 11)
	assert.Equal(t, decisionFailed, res.Decision)
	assert.Equal(t, reasonHelperError, res.Reason)
	assert.Contains(t, res.Detail, "could not run attach helper")
}

func TestParseHelperOutput(t *testing.T) {
	tests := []struct {
		name     string
		out      string
		decision string
		wantErr  bool
	}{
		{name: "single object", out: `{"pid":1,"decision":"skipped","reason":"not_java"}` + "\n", decision: decisionSkipped},
		{name: "dry run", out: `{"pid":1,"decision":"allowed","reason":"ok"}`, decision: decisionAllowed},
		{name: "stray output before JSON", out: "warning: something\n" + `{"pid":1,"decision":"attached","reason":"ok"}` + "\n", decision: decisionAttached},
		{name: "empty", out: "", wantErr: true},
		{name: "not JSON", out: "boom", wantErr: true},
		{name: "unknown decision", out: `{"pid":1,"decision":"maybe","reason":"ok"}`, wantErr: true},
		{name: "missing reason", out: `{"pid":1,"decision":"attached"}`, wantErr: true},
		{name: "truncated", out: `{"pid":1,"decision":"att`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := parseHelperOutput([]byte(tt.out))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.decision, res.Decision)
		})
	}
}

func TestCappedBuffer(t *testing.T) {
	b := &cappedBuffer{limit: 4}
	n, err := b.Write([]byte("abc"))
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	n, err = b.Write([]byte("defg"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, "abcd", b.String())
}
