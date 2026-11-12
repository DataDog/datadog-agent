// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sha256sum

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

func TestVerifyManifestLineLimit(t *testing.T) {
	callCtx := &builtins.CallContext{Stdout: io.Discard, Stderr: io.Discard}
	totals := checkTotals{}
	err := verifyManifest(context.Background(), callCtx, strings.NewReader(strings.Repeat("x", MaxManifestLineBytes)), options{status: true}, &totals, false)
	require.NoError(t, err)
	assert.Equal(t, 1, totals.malformed)

	totals = checkTotals{}
	err = verifyManifest(context.Background(), callCtx, strings.NewReader(strings.Repeat("x", MaxManifestLineBytes+1)), options{status: true}, &totals, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum line exceeds")
}

func TestManifestByteBudget(t *testing.T) {
	consume := func(size int) error {
		totals := checkTotals{}
		reader := &manifestBudgetReader{
			reader: strings.NewReader(strings.Repeat("x", size)),
			totals: &totals,
		}
		buf := make([]byte, manifestBufferBytes)
		for {
			_, err := reader.Read(buf)
			if err != nil {
				return err
			}
		}
	}

	assert.ErrorIs(t, consume(MaxManifestBytes), io.EOF)
	assert.ErrorIs(t, consume(MaxManifestBytes+1), errManifestTooLarge)
	totals := checkTotals{manifestBytes: MaxManifestBytes + 1}
	reader := &manifestBudgetReader{reader: strings.NewReader("x"), totals: &totals}
	_, err := reader.Read(make([]byte, 1))
	assert.ErrorIs(t, err, errManifestTooLarge)
}

func TestCheckOutputBudget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	callCtx := &builtins.CallContext{Stdout: &stdout, Stderr: &stderr}
	totals := checkTotals{outputBytes: MaxCheckOutputBytes - 2}

	require.NoError(t, writeCheckOutput(callCtx, &totals, "a", "b"))
	assert.Equal(t, "a", stdout.String())
	assert.Equal(t, "b", stderr.String())
	assert.ErrorIs(t, writeCheckOutput(callCtx, &totals, "x", ""), errCheckOutputLimit)
}

func TestFormatCheckName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "plain", want: "plain"},
		{name: "a\\b", want: "\\a\\\\b"},
		{name: "a\nb", want: "\\a\\nb"},
		{name: "a\tb", want: "\\a\\tb"},
		{name: "café", want: "café"},
		{name: string([]byte{'b', 'a', 'd', 0xff}), want: "\\bad\\xff"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, formatCheckName(tt.name))
	}
}

func TestChecksumLineParsingAndEscaping(t *testing.T) {
	const digest = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	tests := []struct {
		line string
		name string
		ok   bool
	}{
		{line: digest + "  plain", name: "plain", ok: true},
		{line: " \t" + strings.ToUpper(digest) + "\tplain", name: "plain", ok: true},
		{line: "\\" + digest + "  a\\\\b", name: "a\\b", ok: true},
		{line: "SHA256 (plain) = " + digest, name: "plain", ok: true},
		{line: "SHA256(plain)= " + digest, name: "plain", ok: true},
		{line: "\\" + digest + "  bad\\q", ok: false},
		{line: digest + "  ", ok: false},
		{line: strings.Repeat("g", sha256HexBytes) + "  plain", ok: false},
	}

	for _, tt := range tests {
		entry, ok := parseChecksumLine(tt.line)
		assert.Equal(t, tt.ok, ok, tt.line)
		if tt.ok {
			assert.Equal(t, digest, entry.digest)
			assert.Equal(t, tt.name, entry.name)
		}
	}

	escaped, changed := escapeManifestName("a\\b\r\n")
	assert.True(t, changed)
	assert.Equal(t, "a\\\\b\\r\\n", escaped)
	name, ok := unescapeManifestName(escaped)
	assert.True(t, ok)
	assert.Equal(t, "a\\b\r\n", name)
}

type noProgressReader struct{}

func (noProgressReader) Read([]byte) (int, error) { return 0, nil }

type fixedErrorReader struct{ err error }

func (r fixedErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestDigestReaderErrorsAndCancellation(t *testing.T) {
	digest, err := digestReader(context.Background(), strings.NewReader("abc"))
	require.NoError(t, err)
	assert.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", digest)

	_, err = digestReader(context.Background(), noProgressReader{})
	assert.ErrorIs(t, err, errNoReadProgress)

	wantErr := errors.New("read failed")
	_, err = digestReader(context.Background(), fixedErrorReader{err: wantErr})
	assert.ErrorIs(t, err, wantErr)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = digestReader(ctx, strings.NewReader("abc"))
	assert.ErrorIs(t, err, context.Canceled)
}

func TestVerifyManifestFailurePropagation(t *testing.T) {
	const abcDigest = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	const emptyDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	openErr := errors.New("open failed")
	callCtx := &builtins.CallContext{
		Stdout: io.Discard,
		Stderr: io.Discard,
		OpenRegularFile: func(_ context.Context, path string) (io.ReadCloser, error) {
			if path == "missing" {
				return nil, openErr
			}
			return io.NopCloser(strings.NewReader("abc")), nil
		},
	}

	for _, line := range []string{
		abcDigest + "  target\n",
		emptyDigest + "  target\n",
		abcDigest + "  missing\n",
	} {
		totals := checkTotals{outputBytes: MaxCheckOutputBytes}
		err := verifyManifest(context.Background(), callCtx, strings.NewReader(line), options{}, &totals, false)
		assert.ErrorIs(t, err, errCheckOutputLimit)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	err := verifyManifest(cancelled, callCtx, strings.NewReader(""), options{}, &checkTotals{}, false)
	assert.ErrorIs(t, err, context.Canceled)

	err = verifyManifest(context.Background(), callCtx, strings.NewReader("\n"), options{}, &checkTotals{}, false)
	require.NoError(t, err)

	totals := checkTotals{manifestBytes: MaxManifestBytes}
	err = verifyManifest(context.Background(), callCtx, strings.NewReader("x"), options{}, &totals, false)
	assert.ErrorIs(t, err, errManifestTooLarge)

	err = verifyManifest(context.Background(), callCtx, strings.NewReader(strings.Repeat("x", MaxManifestLineBytes+2)), options{}, &checkTotals{}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum line exceeds")

	wantErr := errors.New("manifest read failed")
	err = verifyManifest(context.Background(), callCtx, fixedErrorReader{err: wantErr}, options{}, &checkTotals{}, false)
	assert.ErrorIs(t, err, wantErr)
}

type deadlineTestReader struct {
	io.Reader
	fail  bool
	calls int
}

func (r *deadlineTestReader) SetReadDeadline(time.Time) error {
	r.calls++
	if r.fail {
		return errors.New("deadlines unsupported")
	}
	return nil
}

type blockingReader struct {
	started chan struct{}
	release chan struct{}
}

func (r *blockingReader) Read([]byte) (int, error) {
	close(r.started)
	<-r.release
	return 0, io.EOF
}

func TestCancellableReaderFallback(t *testing.T) {
	reader := &blockingReader{started: make(chan struct{}), release: make(chan struct{})}
	defer close(reader.release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- withCancellableReader(ctx, reader, func(r io.Reader) error {
			_, err := io.ReadAll(r)
			return err
		})
	}()

	select {
	case <-reader.started:
	case <-time.After(time.Second):
		t.Fatal("reader did not start")
	}
	cancel()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("reader did not stop after cancellation")
	}
}

func TestCancellableReaderWaitsForContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	reader := &deadlineTestReader{Reader: fixedErrorReader{err: os.ErrDeadlineExceeded}}

	err := withCancellableReader(ctx, reader, func(r io.Reader) error {
		_, readErr := r.Read(make([]byte, 1))
		return readErr
	})
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
}

func TestReaderCapabilityBranches(t *testing.T) {
	callCtx := &builtins.CallContext{}
	err := withReader(context.Background(), callCtx, "", func(io.Reader) error { return nil })
	assert.Contains(t, err.Error(), "zero-length")

	var got string
	err = withReader(context.Background(), callCtx, "-", func(r io.Reader) error {
		data, readErr := io.ReadAll(r)
		got = string(data)
		return readErr
	})
	require.NoError(t, err)
	assert.Empty(t, got)

	err = withReader(context.Background(), callCtx, "file", func(io.Reader) error { return nil })
	assert.Contains(t, err.Error(), "capability")

	wantErr := errors.New("open failed")
	callCtx.OpenRegularFile = func(context.Context, string) (io.ReadCloser, error) { return nil, wantErr }
	err = withReader(context.Background(), callCtx, "file", func(io.Reader) error { return nil })
	assert.ErrorIs(t, err, wantErr)

	called := false
	err = withCancellableReader(context.Background(), strings.NewReader("x"), func(io.Reader) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = withCancellableReader(ctx, strings.NewReader("x"), func(io.Reader) error { return nil })
	require.NoError(t, err)

	failingDeadline := &deadlineTestReader{Reader: strings.NewReader("x"), fail: true}
	err = withCancellableReader(ctx, failingDeadline, func(io.Reader) error { return nil })
	require.NoError(t, err)
	assert.Equal(t, 1, failingDeadline.calls)

	deadlineReader := &deadlineTestReader{Reader: strings.NewReader("x")}
	err = withCancellableReader(ctx, deadlineReader, func(io.Reader) error { return nil })
	require.NoError(t, err)
	assert.GreaterOrEqual(t, deadlineReader.calls, 2)
}

func TestSmallDefensiveHelpers(t *testing.T) {
	assert.Equal(t, "items", plural(2, "item", "items"))
	assert.Equal(t, "'standard input'", sourceLabel("-"))
	assert.Equal(t, "checksums", sourceLabel("checksums"))
	wantErr := errors.New("raw error")
	assert.Equal(t, "raw error", portableError(&builtins.CallContext{}, wantErr))
	assert.Equal(t, "portable", portableError(&builtins.CallContext{PortableErr: func(error) string { return "portable" }}, wantErr))

	assert.False(t, validHexDigest("short"))
	_, ok := unescapeManifestName("trailing\\")
	assert.False(t, ok)

	const digest = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	for _, line := range []string{
		"SHA256 (x) nope " + digest,
		"SHA256 (x) = " + strings.Repeat("g", sha256HexBytes),
		"SHA256 (x)",
	} {
		_, ok := parseTaggedLine(line)
		assert.False(t, ok)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	callCtx := &builtins.CallContext{Stdout: io.Discard, Stderr: io.Discard}
	assert.Equal(t, uint8(1), generateDigests(cancelled, callCtx, []string{"-"}).Code)
	assert.Equal(t, uint8(1), verifyManifests(cancelled, callCtx, []string{"-"}, options{}).Code)
}
