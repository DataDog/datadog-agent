// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awk

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

type closeTrackedFile struct {
	*strings.Reader
	closed bool
}

type manuallyReleasedReadCloser struct {
	started     chan struct{}
	release     chan struct{}
	closed      chan struct{}
	finished    chan struct{}
	startedOnce sync.Once
	closedOnce  sync.Once
	releaseOnce sync.Once
}

type closeInterruptedReader struct {
	started     chan struct{}
	closed      chan struct{}
	finished    chan struct{}
	startedOnce sync.Once
	closedOnce  sync.Once
}

type cancelAfterChecksContext struct {
	context.Context
	mu        sync.Mutex
	done      chan struct{}
	remaining int
	canceled  bool
}

func newCancelAfterChecksContext(remaining int) *cancelAfterChecksContext {
	return &cancelAfterChecksContext{
		Context:   context.Background(),
		done:      make(chan struct{}),
		remaining: remaining,
	}
}

func (ctx *cancelAfterChecksContext) Done() <-chan struct{} {
	return ctx.done
}

func (ctx *cancelAfterChecksContext) Err() error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.remaining > 0 {
		ctx.remaining--
		return nil
	}
	if !ctx.canceled {
		close(ctx.done)
		ctx.canceled = true
	}
	return context.Canceled
}

func newManuallyReleasedReadCloser() *manuallyReleasedReadCloser {
	return &manuallyReleasedReadCloser{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		closed:   make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func newCloseInterruptedReader() *closeInterruptedReader {
	return &closeInterruptedReader{
		started:  make(chan struct{}),
		closed:   make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (r *closeInterruptedReader) Read([]byte) (int, error) {
	r.startedOnce.Do(func() { close(r.started) })
	<-r.closed
	close(r.finished)
	return 0, os.ErrClosed
}

func (r *closeInterruptedReader) Close() error {
	r.closedOnce.Do(func() { close(r.closed) })
	return nil
}

func (r *manuallyReleasedReadCloser) Read([]byte) (int, error) {
	r.startedOnce.Do(func() { close(r.started) })
	<-r.release
	close(r.finished)
	return 0, io.EOF
}

func (r *manuallyReleasedReadCloser) Write([]byte) (int, error) {
	return 0, os.ErrInvalid
}

func (r *manuallyReleasedReadCloser) Close() error {
	r.closedOnce.Do(func() { close(r.closed) })
	<-r.release
	return nil
}

func (r *manuallyReleasedReadCloser) unblock() {
	r.releaseOnce.Do(func() { close(r.release) })
}

func (f *closeTrackedFile) Write([]byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *closeTrackedFile) Close() error {
	f.closed = true
	return nil
}

func TestRuntimeClosesInputsOnError(t *testing.T) {
	prog, err := parseProgram(`{ zero = 0; print 1 % zero }`)
	require.NoError(t, err)

	opened := &closeTrackedFile{Reader: strings.NewReader("row\n")}
	var stderr bytes.Buffer
	callCtx := &builtins.CallContext{
		Stderr: &stderr,
		OpenFile: func(context.Context, string, int, os.FileMode) (io.ReadWriteCloser, error) {
			return opened, nil
		},
	}

	result := newRuntime(callCtx, prog).run(context.Background(), []string{"input"})

	assert.Equal(t, uint8(2), result.Code)
	assert.Contains(t, stderr.String(), "division by zero attempted")
	assert.True(t, opened.closed)
}

func TestRuntimeBoundsAggregateStatementExecutions(t *testing.T) {
	prog, err := parseProgram(`BEGIN { print "at-limit"; print "unreachable" }`)
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	rt := newRuntime(&builtins.CallContext{Stdout: &stdout, Stderr: &stderr}, prog)
	rt.stmtExecutions = maxStatementExecutions - 1

	result := rt.run(context.Background(), nil)

	assert.Equal(t, uint8(1), result.Code)
	assert.Equal(t, "at-limit\n", stdout.String())
	assert.Equal(t, "awk: statement execution limit exceeded (maximum 1048576)\n", stderr.String())
}

func TestRuntimeChargesOnlyNormalRulesAgainstMainRuleBudget(t *testing.T) {
	prog, err := parseProgram(`BEGIN { x = 1 } { print $0 } END { x = 2 }`)
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	rt := newRuntime(&builtins.CallContext{
		Stdin:  strings.NewReader("row\n"),
		Stdout: &stdout,
		Stderr: &stderr,
	}, prog)
	rt.mainRuleEvals = maxMainRuleEvaluations - 1

	result := rt.run(context.Background(), nil)

	assert.Equal(t, uint8(0), result.Code)
	assert.Equal(t, "row\n", stdout.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, maxMainRuleEvaluations, rt.mainRuleEvals)
}

func TestRuntimeBoundsAggregateInputBytes(t *testing.T) {
	prog, err := parseProgram(`0`)
	require.NoError(t, err)

	var stderr bytes.Buffer
	rt := newRuntime(&builtins.CallContext{
		Stdin:  strings.NewReader("x\n"),
		Stderr: &stderr,
	}, prog)
	rt.inputBytes = maxInputBytes - 1

	result := rt.run(context.Background(), nil)

	assert.Equal(t, uint8(1), result.Code)
	assert.Equal(t, "awk: -: input byte limit exceeded (maximum 67108864 bytes)\n", stderr.String())
	assert.Equal(t, maxInputBytes-1, rt.inputBytes)
	assert.Zero(t, rt.inputRecords)
}

func TestParagraphRecordDoesNotWaitForFollowingInput(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	rt := newRuntime(&builtins.CallContext{}, &program{})
	require.NoError(t, rt.setVar("RS", stringValue("")))
	src := rt.newRecordSource("input", reader)

	written := make(chan error, 1)
	go func() {
		_, err := io.WriteString(writer, "a\n\n")
		written <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	record, ok, err := src.readRecord(ctx)

	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "a", record)
	require.NoError(t, <-written)
}

func TestRecordSourceFallbackCancellationReturnsPromptly(t *testing.T) {
	reader := newManuallyReleasedReadCloser()
	t.Cleanup(reader.unblock)
	rt := newRuntime(&builtins.CallContext{}, &program{})
	src := rt.newRecordSource("input", reader)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := src.readRecord(ctx)
		done <- err
	}()

	<-reader.started
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("record read did not observe cancellation")
	}
	select {
	case <-reader.closed:
	case <-time.After(time.Second):
		t.Fatal("record source did not close its reader")
	}

	reader.unblock()
	select {
	case <-reader.finished:
	case <-time.After(time.Second):
		t.Fatal("fallback read goroutine did not exit")
	}
}

func TestMainStdinCancellationClosesUnderlyingReader(t *testing.T) {
	reader := newCloseInterruptedReader()
	t.Cleanup(func() { _ = reader.Close() })
	rt := newRuntime(&builtins.CallContext{Stdin: reader}, &program{})
	src, err := rt.openRecordSource(context.Background(), "-")
	require.NoError(t, err)
	rt.mainInput = src
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := rt.readMainRecord(ctx)
		done <- err
	}()

	<-reader.started
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("main stdin read did not observe cancellation")
	}
	select {
	case <-reader.finished:
	case <-time.After(time.Second):
		t.Fatal("main stdin read did not exit after closing its reader")
	}
}

func TestMainStdinCleanupDoesNotCloseCallerReader(t *testing.T) {
	reader := &closeTrackedFile{Reader: strings.NewReader("")}
	rt := newRuntime(&builtins.CallContext{Stdin: reader}, &program{})
	src, err := rt.openRecordSource(context.Background(), "-")
	require.NoError(t, err)

	src.close()

	assert.False(t, reader.closed)
}

func TestRecordSourceAcceptsMaxRecordWithMultibyteSeparator(t *testing.T) {
	const separator = "\U0010ffff"
	record := strings.Repeat("x", MaxRecordBytes)
	rt := newRuntime(&builtins.CallContext{}, &program{})
	require.NoError(t, rt.setVar("RS", stringValue(separator)))
	src := rt.newRecordSource("input", io.NopCloser(strings.NewReader(record+separator)))
	t.Cleanup(src.close)

	got, ok, err := src.readRecord(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	assert.Len(t, got, MaxRecordBytes)

	_, ok, err = src.readRecord(context.Background())
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRecordSourceRejectsRecordOverLimitWithLargerScanBuffer(t *testing.T) {
	const separator = "\U0010ffff"
	rt := newRuntime(&builtins.CallContext{}, &program{})
	require.NoError(t, rt.setVar("RS", stringValue(separator)))
	src := rt.newRecordSource("input", io.NopCloser(strings.NewReader(strings.Repeat("x", MaxRecordBytes+1))))
	t.Cleanup(src.close)

	_, _, err := src.readRecord(context.Background())
	require.EqualError(t, err, "record exceeds 1048576 bytes")
}

func TestRecordRebuildChargesAggregateStringWorkBeforeMutation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		assign func(*runtime) error
	}{
		{name: "field", assign: func(rt *runtime) error { return rt.setField(1, stringValue("x")) }},
		{name: "NF", assign: func(rt *runtime) error { return rt.setNF(numberValue(2)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := newRuntime(&builtins.CallContext{}, &program{})
			rt.record = "a b c"
			rt.fields = []string{"a", "b", "c"}
			rt.stringWorkBytes = maxStringProcessingBytes - 2

			err := tc.assign(rt)

			require.EqualError(t, err, "string processing limit exceeded (maximum 67108864 bytes)")
			assert.Equal(t, "a b c", rt.record)
			assert.Equal(t, []string{"a", "b", "c"}, rt.fields)
		})
	}
}

func TestNumericConversionChargesAggregateStringWork(t *testing.T) {
	rt := newRuntime(&builtins.CallContext{}, &program{})
	require.NoError(t, rt.setVar("CONVFMT", stringValue("%8.2f")))
	rt.stringWorkBytes = maxStringProcessingBytes - 8

	s, err := rt.conversionString(numberValue(1.5), "CONVFMT")
	require.NoError(t, err)
	assert.Equal(t, "    1.50", s)
	assert.Equal(t, maxStringProcessingBytes, rt.stringWorkBytes)

	_, err = rt.conversionString(numberValue(1.5), "CONVFMT")
	require.EqualError(t, err, "string processing limit exceeded (maximum 67108864 bytes)")
	assert.Equal(t, maxStringProcessingBytes, rt.stringWorkBytes)
}

func TestRuntimeRegexCacheBoundsStorage(t *testing.T) {
	rt := newRuntime(&builtins.CallContext{}, &program{})

	first, err := rt.compileRegex("abc")
	require.NoError(t, err)
	again, err := rt.compileRegex("abc")
	require.NoError(t, err)
	assert.Same(t, first, again)

	for i := 0; i <= maxRegexCacheEntries; i++ {
		rt.rememberRegex(regexCacheKey{pattern: fmt.Sprintf("pattern-%d", i)}, &awkRegex{})
	}
	assert.LessOrEqual(t, len(rt.regexCache), maxRegexCacheEntries)
	assert.LessOrEqual(t, rt.regexCacheBytes, maxRegexCacheBytes)
}

func TestRegexCacheMissesChargeAggregateWork(t *testing.T) {
	rt := newRuntime(&builtins.CallContext{}, &program{})
	rt.stringWorkBytes = maxStringProcessingBytes - minRegexCompileWork

	first, err := rt.compileRegex("x")
	require.NoError(t, err)
	used := rt.stringWorkBytes
	again, err := rt.compileRegex("x")
	require.NoError(t, err)
	assert.Same(t, first, again)
	assert.Equal(t, used, rt.stringWorkBytes)

	_, err = rt.compileRegex("y")
	require.EqualError(t, err, "string processing limit exceeded (maximum 67108864 bytes)")
}

func TestRegexMatchingChecksCancellation(t *testing.T) {
	const count = 1024
	pattern := strings.Repeat("a?", count)
	input := strings.Repeat("a", count)

	t.Run("single match", func(t *testing.T) {
		re, err := compileRegex(pattern)
		require.NoError(t, err)
		re.ctx = newCancelAfterChecksContext(3)

		loc, err := re.FindStringIndex(input)

		assert.Nil(t, loc)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("all matches", func(t *testing.T) {
		re, err := compileRegex(pattern)
		require.NoError(t, err)
		re.ctx = newCancelAfterChecksContext(3)

		locs, err := re.FindAllStringIndex(input, -1)

		assert.Nil(t, locs)
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestCancellableRegexFindAllMatchesNativeResults(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		n       int
	}{
		{pattern: `^a|b`, input: "abb", n: -1},
		{pattern: `^`, input: "ab", n: -1},
		{pattern: `$`, input: "ab", n: -1},
		{pattern: `a*`, input: "baa", n: -1},
		{pattern: `.`, input: "é🙂", n: -1},
		{pattern: `a?`, input: "aa", n: 1},
		{pattern: `a?`, input: "aa", n: 0},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s/%d", tc.pattern, tc.n), func(t *testing.T) {
			re, err := compileRegex(tc.pattern)
			require.NoError(t, err)
			want := re.re.FindAllStringIndex(tc.input, tc.n)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			re.ctx = ctx

			got, err := re.FindAllStringIndex(tc.input, tc.n)

			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestEnvironInitializationAccountsStorageAndCachesLimitError(t *testing.T) {
	envCalls := 0
	yields := 0
	stopped := false
	rt := newRuntime(&builtins.CallContext{
		Env: func(yield func(name, value string) bool) {
			envCalls++
			for _, entry := range [][2]string{{"A", "x"}, {"B", "y"}, {"C", "z"}} {
				yields++
				if !yield(entry[0], entry[1]) {
					stopped = true
					return
				}
			}
		},
	}, &program{})
	rt.varBytes = MaxVariableBytes - 2
	wantErr := fmt.Sprintf("variable storage limit exceeded (%d bytes total)", MaxVariableBytes+2)

	_, err := rt.evalLength(&callExpr{args: []expr{&varExpr{name: "ENVIRON"}}})
	require.EqualError(t, err, wantErr)
	assert.Equal(t, 1, envCalls)
	assert.Equal(t, 2, yields)
	assert.True(t, stopped)
	assert.Equal(t, MaxVariableBytes, rt.varBytes)
	assert.Equal(t, 2, rt.arraySizes[arraySlot{name: "ENVIRON", key: "A"}])
	assert.Equal(t, inputStringValue("x"), rt.arrays["ENVIRON"]["A"])
	assert.NotContains(t, rt.arrays["ENVIRON"], "B")

	accesses := []struct {
		name string
		call func() error
	}{
		{name: "indexed read", call: func() error { _, err := rt.getArrayElem("ENVIRON", "A"); return err }},
		{name: "membership", call: func() error { _, err := rt.hasArrayElem("ENVIRON", "A"); return err }},
		{name: "assignment", call: func() error { return rt.setArrayElem("ENVIRON", "A", stringValue("new")) }},
		{name: "element deletion", call: func() error { return rt.deleteArrayElem("ENVIRON", "A") }},
		{name: "array deletion", call: func() error { return rt.deleteArray("ENVIRON") }},
		{name: "iteration", call: func() error { _, err := rt.arrayKeys("ENVIRON"); return err }},
	}
	for _, access := range accesses {
		t.Run(access.name, func(t *testing.T) {
			require.EqualError(t, access.call(), wantErr)
		})
	}
	assert.Equal(t, 1, envCalls)
	assert.Equal(t, 2, yields)
}

func TestEnvironInitializationHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	envCalls := 0
	yields := 0
	stopped := false
	rt := newRuntime(&builtins.CallContext{
		Env: func(yield func(name, value string) bool) {
			envCalls++
			yields++
			if !yield("A", "x") {
				stopped = true
				return
			}
			cancel()
			yields++
			if !yield("B", "y") {
				stopped = true
				return
			}
			yields++
			yield("C", "z")
		},
	}, &program{})
	rt.ctx = ctx

	_, err := rt.arrayStorage("ENVIRON")
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, envCalls)
	assert.Equal(t, 2, yields)
	assert.True(t, stopped)
	assert.Equal(t, 2, rt.varBytes)
	assert.Equal(t, 2, rt.arraySizes[arraySlot{name: "ENVIRON", key: "A"}])
	assert.Equal(t, inputStringValue("x"), rt.arrays["ENVIRON"]["A"])
	assert.NotContains(t, rt.arrays["ENVIRON"], "B")

	_, err = rt.hasArrayElem("ENVIRON", "A")
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, envCalls)
	assert.Equal(t, 2, yields)
}

func TestEmptyMainInputFileOpenAttemptsAreBounded(t *testing.T) {
	openCalls := 0
	rt := newRuntime(&builtins.CallContext{
		OpenFile: func(context.Context, string, int, os.FileMode) (io.ReadWriteCloser, error) {
			openCalls++
			return &closeTrackedFile{Reader: strings.NewReader("")}, nil
		},
	}, &program{})
	rt.inputArgs = []string{"empty", "empty", "empty"}
	rt.fileOpenAttempts = maxFileOpenAttempts - 2

	_, ok, err := rt.readMainRecord(context.Background())

	require.EqualError(t, err, "file open attempt limit exceeded (maximum 1024)")
	assert.False(t, ok)
	assert.Equal(t, 2, openCalls)
	assert.Equal(t, maxFileOpenAttempts, rt.fileOpenAttempts)
}
