// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Tests added by vuln-hunt campaign 2026-05-18-initial-audit /
// interp-redirect-handling to pin the runner's redirect-implementation
// invariants at the subsystem level (the implementation behind the
// already-audited `redirections`, `allowed_redirects`, `blocked_redirects`
// shell-feature scenarios).

package interp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"
)

// recordingOpenHandler is a test double that records every call made to
// openHandler so the test can assert which redirect target paths the
// interpreter routed through the sandbox.
type recordingOpenHandler struct {
	calls []openCall
	root  string
}

type openCall struct {
	path string
	flag int
}

func (h *recordingOpenHandler) handle(_ context.Context, path string, flag int, _ os.FileMode) (io.ReadWriteCloser, error) {
	h.calls = append(h.calls, openCall{path: path, flag: flag})
	// Open the underlying file relative to the recorded root, so cat reads
	// real contents and the test exercises the post-open code path too.
	full := path
	if !filepath.IsAbs(path) {
		full = filepath.Join(h.root, path)
	}
	return os.OpenFile(full, flag, 0)
}

// TestVulnHuntSubsystemInterpRedirectHandling_InputRedirectGoesThroughOpenHandler
// asserts that an input redirect target (`cat <file`) reaches the runner's
// openHandler with the expected path and a read-only flag. If a future
// regression in runner_redir.go bypassed openHandler with a direct os.Open,
// the recorded calls slice would be empty and this test would fail — the
// AllowedPaths sandbox is built on top of openHandler, so any bypass would
// also bypass AllowedPaths.
func TestVulnHuntSubsystemInterpRedirectHandling_InputRedirectGoesThroughOpenHandler(t *testing.T) {
	dir := t.TempDir()
	const fileName = "input.txt"
	const fileBody = "redirect-payload\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, fileName), []byte(fileBody), 0o600))

	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Reset()

	rec := &recordingOpenHandler{root: dir}
	r.openHandler = rec.handle
	r.Dir = dir

	err = r.Run(context.Background(), parseScript(t, "cat <"+fileName))
	require.NoError(t, err, "stderr=%q", stderr.String())

	require.Len(t, rec.calls, 1, "expected exactly one open call routed through openHandler")
	assert.Equal(t, fileName, rec.calls[0].path,
		"openHandler must receive the redirect target path verbatim, got: %v", rec.calls)
	assert.Equal(t, os.O_RDONLY, rec.calls[0].flag,
		"input redirect must request read-only access; a writable flag would mean the redirect path widens the sandbox")
	assert.Equal(t, fileBody, stdout.String(),
		"cat output should match file body — proves the recorded handler also returned a working file")
}

type closeRecordingFile struct {
	io.ReadWriteCloser
	closed *int
}

func (f *closeRecordingFile) Close() error {
	*f.closed++
	return f.ReadWriteCloser.Close()
}

func TestVulnHuntSubsystemInterpRedirectHandling_FailedSecondRedirectClosesFirstAndRestoresStdio(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "first.txt"), []byte("should-not-print\n"), 0o600))

	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Reset()
	r.Dir = dir

	closeCount := 0
	r.openHandler = func(_ context.Context, path string, flag int, _ os.FileMode) (io.ReadWriteCloser, error) {
		if path == "blocked.txt" {
			return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrPermission}
		}
		f, err := os.OpenFile(filepath.Join(dir, path), flag, 0)
		if err != nil {
			return nil, err
		}
		return &closeRecordingFile{ReadWriteCloser: f, closed: &closeCount}, nil
	}

	err = r.Run(context.Background(), parseScript(t, "cat < first.txt < blocked.txt\necho after\n"))
	require.NoError(t, err, "stderr=%q", stderr.String())
	assert.Equal(t, "after\n", stdout.String(),
		"failed redirect must skip the cat command and restore stdout for the next statement")
	assert.Contains(t, stderr.String(), "blocked.txt")
	assert.Equal(t, 1, closeCount, "first redirect file must be closed when a later redirect fails")
}

func TestVulnHuntSubsystemInterpRedirectHandling_RecoveredPanicClosesRedirectAndRestoresStdio(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
		WithMode(ModeRemediation),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Reset()
	r.Dir = dir

	closeCount := 0
	r.openHandler = func(_ context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
		f, err := os.OpenFile(filepath.Join(dir, path), flag, perm)
		if err != nil {
			return nil, err
		}
		return &closeRecordingFile{ReadWriteCloser: f, closed: &closeCount}, nil
	}
	r.execHandler = func(context.Context, []string) error {
		panic("redirect panic sentinel")
	}

	err = r.Run(context.Background(), parseScript(t, "paniccmd 2> panic.txt"))
	require.EqualError(t, err, "internal error")
	assert.Equal(t, 1, closeCount, "redirect file must be closed while unwinding the recovered panic")
	assert.Contains(t, stderr.String(), "redirect panic sentinel",
		"panic diagnostic must use the restored stderr")
}

func TestVulnHuntSubsystemInterpRedirectHandling_UnsupportedRedirectsAndProcSubstDoNotOpenFiles(t *testing.T) {
	cases := []string{
		"cat <&0\n",
		"cat <> file.txt\n",
		"cat <<< hello\n",
		"cat <(echo secret)\n",
		"echo hello 3>&1\n",
		"echo hello >&-\n",
	}
	for _, script := range cases {
		t.Run(script, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			r, err := New(
				allowAllCommandsOpt(),
				StdIO(nil, &stdout, &stderr),
			)
			require.NoError(t, err)
			t.Cleanup(func() { _ = r.Close() })
			r.Reset()

			openCalls := 0
			r.openHandler = func(context.Context, string, int, os.FileMode) (io.ReadWriteCloser, error) {
				openCalls++
				return nil, errors.New("openHandler should not be called")
			}

			err = r.Run(context.Background(), parseScript(t, script))
			require.Error(t, err)
			var es ExitStatus
			require.True(t, errors.As(err, &es), "expected validation exit status, got %v", err)
			assert.Equal(t, ExitStatus(2), es)
			assert.Equal(t, 0, openCalls, "unsupported redirect reached openHandler; stderr=%q stdout=%q", stderr.String(), stdout.String())
			assert.Empty(t, stdout.String())
		})
	}
}

func TestVulnHuntSubsystemInterpRedirectHandling_RuntimeDevNullDefenseRejectsMutatedTarget(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Reset()

	target := filepath.Join(dir, "created.txt")
	_, err = r.redir(context.Background(), &syntax.Redirect{
		Op:   syntax.RdrOut,
		Word: &syntax.Word{Parts: []syntax.WordPart{&syntax.Lit{Value: target}}},
	})
	require.Error(t, err)
	assert.Contains(t, stderr.String(), "file redirection is only supported")
	_, statErr := os.Stat(target)
	assert.True(t, errors.Is(statErr, os.ErrNotExist), "runtime redirect defense created file, stat err=%v", statErr)
}

func TestVulnHuntSubsystemInterpRedirectHandling_DevNullOutputDoesNotOpenFileHandler(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Reset()

	openCalls := 0
	r.openHandler = func(context.Context, string, int, os.FileMode) (io.ReadWriteCloser, error) {
		openCalls++
		return nil, errors.New("openHandler should not be called for /dev/null output")
	}

	err = r.Run(context.Background(), parseScript(t, "echo hidden >/dev/null\necho visible\n"))
	require.NoError(t, err, "stderr=%q", stderr.String())
	assert.Equal(t, "visible\n", stdout.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, 0, openCalls, "/dev/null output redirect must use io.Discard, not openHandler")
}

func TestVulnHuntSubsystemInterpRedirectHandling_HeredocCommandSubstitutionUsesSandbox(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.MkdirAll(allowed, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("classified\n"), 0o600))

	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
		AllowedPaths([]string{allowed}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = allowed

	script := "cat <<EOF\n$(<../outside/secret.txt)\nEOF\necho after\n"
	err = r.Run(context.Background(), parseScript(t, script))
	require.NoError(t, err, "stderr=%q", stderr.String())
	assert.NotContains(t, stdout.String(), "classified")
	assert.NotContains(t, stderr.String(), "classified")
	assert.Contains(t, stdout.String(), "after\n")
	assert.NotEmpty(t, stderr.String(), "sandbox denial should be reported")
}

func TestVulnHuntSubsystemInterpRedirectHandling_QuotedHeredocStaysLiteral(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	script := "cat <<'EOF'\n$(echo should-not-run)\n~/literal\nEOF\n"
	err = r.Run(context.Background(), parseScript(t, script))
	require.NoError(t, err, "stderr=%q", stderr.String())
	assert.Equal(t, "$(echo should-not-run)\n~/literal\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestVulnHuntSubsystemInterpRedirectHandling_NoDirectUserPathOpenInRedirectRuntime(t *testing.T) {
	src, err := os.ReadFile("runner_redir.go")
	require.NoError(t, err)
	text := string(src)
	assert.NotContains(t, text, "os.Open(")
	assert.NotContains(t, text, "os.OpenFile(")
	assert.True(t, strings.Contains(text, "os.Pipe()"),
		"redirect runtime may use os.Pipe for heredocs but must not directly open user paths")
}
