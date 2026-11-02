// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package tee

import (
	"bytes"
	"context"
	"errors"
	"io"
	iofs "io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// fakeCloser is an io.Closer whose Close call is scripted for the test,
// modeling a delayed write-back failure surfaced only at Close time (e.g.
// ENOSPC, EIO, or a quota error on some filesystems, including several
// network filesystems) — a real os.File cannot be made to fail Close on
// demand portably, so this is a Go-level unit test of closeAllDests rather
// than an end-to-end scenario.
type fakeCloser struct {
	err error
}

func (f *fakeCloser) Close() error { return f.err }

func TestCloseAllDestsReportsFailure(t *testing.T) {
	var errBuf bytes.Buffer
	callCtx := &builtins.CallContext{
		Stderr:      &errBuf,
		PortableErr: func(err error) string { return err.Error() },
	}
	closeErr := errors.New("write-back failed: no space left on device")
	dests := []dest{
		{name: "standard output", w: nil, closer: nil}, // never closed
		{name: "good.txt", w: nil, closer: &fakeCloser{err: nil}},
		{name: "bad.txt", w: nil, closer: &fakeCloser{err: closeErr}},
	}

	ok := closeAllDests(callCtx, dests)

	if ok {
		t.Fatal("expected closeAllDests to report failure")
	}
	if !bytes.Contains(errBuf.Bytes(), []byte("bad.txt")) {
		t.Errorf("stderr missing failing destination name: %q", errBuf.String())
	}
	if !bytes.Contains(errBuf.Bytes(), []byte("no space left on device")) {
		t.Errorf("stderr missing close error text: %q", errBuf.String())
	}
	if bytes.Contains(errBuf.Bytes(), []byte("good.txt")) {
		t.Errorf("stderr must not report the successfully closed destination: %q", errBuf.String())
	}
}

func TestCloseAllDestsAllSucceed(t *testing.T) {
	var errBuf bytes.Buffer
	callCtx := &builtins.CallContext{
		Stderr:      &errBuf,
		PortableErr: func(err error) string { return err.Error() },
	}
	dests := []dest{
		{name: "standard output", w: nil, closer: nil},
		{name: "a.txt", w: nil, closer: &fakeCloser{err: nil}},
		{name: "b.txt", w: nil, closer: &fakeCloser{err: nil}},
	}

	ok := closeAllDests(callCtx, dests)

	if !ok {
		t.Fatal("expected closeAllDests to report success")
	}
	if errBuf.Len() != 0 {
		t.Errorf("expected no stderr output, got %q", errBuf.String())
	}
}

// TestCloseAllDestsClosesEveryDestinationDespiteEarlierFailure verifies that
// one destination's close failure does not skip closing the rest — every
// closer must still be invoked exactly once.
func TestCloseAllDestsClosesEveryDestinationDespiteEarlierFailure(t *testing.T) {
	var errBuf bytes.Buffer
	callCtx := &builtins.CallContext{
		Stderr:      &errBuf,
		PortableErr: func(err error) string { return err.Error() },
	}
	closed := make([]bool, 3)
	makeCloser := func(i int, err error) *trackedCloser {
		return &trackedCloser{err: err, onClose: func() { closed[i] = true }}
	}
	dests := []dest{
		{name: "a.txt", w: nil, closer: makeCloser(0, errors.New("boom"))},
		{name: "b.txt", w: nil, closer: makeCloser(1, nil)},
		{name: "c.txt", w: nil, closer: makeCloser(2, errors.New("boom2"))},
	}

	closeAllDests(callCtx, dests)

	for i, c := range closed {
		if !c {
			t.Errorf("destination %d was not closed", i)
		}
	}
}

type trackedCloser struct {
	err     error
	onClose func()
}

func (t *trackedCloser) Close() error {
	t.onClose()
	return t.err
}

// TestSafeErrEscapesControlCharsWithoutDoublingBackslash pins three
// properties of safeErr at once: it must neutralize an embedded newline
// and other control characters like ESC (the actual line-forging /
// terminal-injection risk PortablePathError's double-normalization gap
// can leak — see the package doc comment on safeErr), and it must NOT
// touch a literal backslash, since a formatted error message can
// legitimately contain one as a Windows path separator (e.g. "openat
// missing_dir\out.txt: ..."). Two earlier versions of this helper each
// fixed one property while breaking the other: builtins.SafeOperand
// escapes backslashes too and broke Windows CI by doubling every
// separator in the message; a bare strings.ReplaceAll("\n", ...) escaped
// only the newline and left ESC/other control characters leaking raw.
func TestSafeErrEscapesControlCharsWithoutDoublingBackslash(t *testing.T) {
	callCtx := &builtins.CallContext{
		PortableErr: func(err error) string { return err.Error() },
	}

	got := safeErr(callCtx, errors.New("evil\nFORGED LINE: no such file or directory"))
	want := `evil\nFORGED LINE: no such file or directory`
	if got != want {
		t.Errorf("newline case: got %q want %q", got, want)
	}

	got = safeErr(callCtx, errors.New("evil\x1b[31mRED\x1b[0m: no such file or directory"))
	want = `evil\x1b[31mRED\x1b[0m: no such file or directory`
	if got != want {
		t.Errorf("ESC sequence case: got %q want %q", got, want)
	}

	got = safeErr(callCtx, errors.New(`openat missing_dir\out.txt: no such file or directory`))
	want = `openat missing_dir\out.txt: no such file or directory`
	if got != want {
		t.Errorf("windows path case: got %q want %q", got, want)
	}
}

// TestRegisterFlagsHandlerDoesNotPanicOnMissingCapabilities is the
// regression test for calling the exported command factory directly
// (bypassing normal interp dispatch) with RemediationMode set but
// OpenFile/StatFile/AllowedPathsList left nil — e.g. a hand-built
// *builtins.CallContext in an embedding application or test. Before the
// capability check was added, this panicked with a nil pointer
// dereference inside rejectNonRegularTarget's callCtx.StatFile call
// instead of reporting a missing-capability diagnostic.
func TestRegisterFlagsHandlerDoesNotPanicOnMissingCapabilities(t *testing.T) {
	fs := pflag.NewFlagSet("tee", pflag.ContinueOnError)
	handler := registerFlags(fs)
	if err := fs.Parse([]string{"out.txt"}); err != nil {
		t.Fatal(err)
	}

	var errBuf bytes.Buffer
	callCtx := &builtins.CallContext{
		RemediationMode: true,
		Stderr:          &errBuf,
		// OpenFile, StatFile, AllowedPathsList intentionally left nil.
	}

	result := handler(context.Background(), callCtx, fs.Args())

	if result.Code != 1 {
		t.Errorf("expected exit 1, got %d (stderr %q)", result.Code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "no writable path is configured") {
		t.Errorf("expected no-writable-root diagnostic, got %q", errBuf.String())
	}
}

// TestFilesLoopChecksCancellationBeforeEachOperand is the direct unit-level
// regression test for the per-operand ctx.Err() check: it calls the
// handler with an already-expired deadline and a fake OpenFile that counts
// its own invocations, bypassing the interpreter's own r.stop(ctx)
// short-circuit (which would otherwise refuse to dispatch the command at
// all and never exercise this code path — see the equivalent
// interp-level TestTeePentestChecksCancellationBeforeEachDestination for
// why an end-to-end scenario test cannot observe this directly). Confirmed
// this test fails (openCalls > 0) when the ctx.Err() check is removed from
// the loop.
func TestFilesLoopChecksCancellationBeforeEachOperand(t *testing.T) {
	dir := t.TempDir()
	fs := pflag.NewFlagSet("tee", pflag.ContinueOnError)
	handler := registerFlags(fs)
	if err := fs.Parse([]string{"a.txt", "b.txt", "c.txt"}); err != nil {
		t.Fatal(err)
	}

	var errBuf bytes.Buffer
	var openCalls int
	callCtx := &builtins.CallContext{
		RemediationMode: true,
		Stderr:          &errBuf,
		PortableErr:     func(err error) string { return err.Error() },
		AllowedPathsList: func() []builtins.AllowedPath {
			return []builtins.AllowedPath{{Path: dir, Access: builtins.AllowedPathReadWrite}}
		},
		StatFile: func(_ context.Context, _ string) (iofs.FileInfo, error) {
			return nil, os.ErrNotExist
		},
		OpenFile: func(_ context.Context, _ string, _ int, _ os.FileMode) (io.ReadWriteCloser, error) {
			openCalls++
			return nil, os.ErrPermission
		},
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	handler(ctx, callCtx, fs.Args())

	if openCalls != 0 {
		t.Errorf("expected no OpenFile calls once the context was already expired, got %d", openCalls)
	}
}
