// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package read

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

// eofWithDataReader is an io.Reader that returns its full payload in
// a single Read call together with io.EOF — a behaviour explicitly
// permitted by the io.Reader contract:
//
//	"a Reader returning a non-zero number of bytes at the end of the
//	 input stream may return either err == EOF or err == nil. The
//	 next Read should return 0, EOF."
//
// We exercise this on the goroutine path used by readInput when the
// caller has a cancellable context but kernel-level cancellation
// can't be wired (non-*os.File stdin), to verify the byte that came
// alongside io.EOF is preserved rather than discarded.
type eofWithDataReader struct {
	data []byte
	pos  int
	done bool
}

func (r *eofWithDataReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos >= len(r.data) && !r.done {
		r.done = true
		return n, io.EOF
	}
	return n, nil
}

// TestReadInput_GoroutinePath_EOFWithData verifies that when stdin
// is a non-*os.File reader that returns the final byte together with
// io.EOF, the goroutine-poll path delivers the byte to the consumer
// before propagating EOF. Regression for codex P2 review feedback.
func TestReadInput_GoroutinePath_EOFWithData(t *testing.T) {
	// Cancellable context is required to take the goroutine path
	// (otherwise readInput uses the direct-Read path which already
	// handles n>0 + EOF correctly via its `if n == 1 { return ...
	// }` short-circuit). A loose deadline keeps the context
	// cancellable without firing during the test.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Read until newline. eofWithDataReader returns "abc\n" + EOF in
	// one call: the goroutine must deliver every byte. With the buggy
	// code the trailing byte (the newline) was paired with err=EOF
	// and dropped; the consumer would then see the b="" plus EOF and
	// also lose the preceding bytes if they hadn't already been
	// flushed (depending on chunking).
	r := &eofWithDataReader{data: []byte("abc\n")}
	const useGoroutinePoll = true
	const charLimit = -1
	const ignoreDelim = false
	line, eof, err := readInput(ctx, r, '\n' /*raw=*/, true, charLimit, ignoreDelim, useGoroutinePoll)
	if err != nil {
		t.Fatalf("readInput error: %v", err)
	}
	if line != "abc" {
		t.Fatalf("got line=%q, want %q", line, "abc")
	}
	// The newline is the delimiter — readInput returns before EOF
	// would propagate. eof should be false here.
	if eof {
		t.Fatalf("eof=true unexpectedly when delimiter was found")
	}
}

// TestReadInput_GoroutinePath_EOFWithDataNoDelim verifies the case
// where the EOF-bearing byte is the last data byte and there is no
// delimiter — readInput must include that final byte in the returned
// string and report eof=true.
func TestReadInput_GoroutinePath_EOFWithDataNoDelim(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r := &eofWithDataReader{data: []byte("abc")}
	line, eof, err := readInput(ctx, r, '\n', true, -1, false, true)
	if err != nil {
		t.Fatalf("readInput error: %v", err)
	}
	if !eof {
		t.Fatalf("expected eof=true at end of stream")
	}
	if line != "abc" {
		t.Fatalf("got line=%q, want %q (last byte must not be dropped)", line, "abc")
	}
}

// stallAfterReader returns up to `data` then blocks on subsequent
// Read calls until the test's context fires. Used to simulate a
// producer that writes a partial UTF-8 sequence then stalls past
// the consumer's `-t` deadline.
type stallAfterReader struct {
	data    []byte
	pos     int
	stallCh chan struct{} // never sent to; closed in test cleanup to unblock the goroutine
}

func (r *stallAfterReader) Read(p []byte) (int, error) {
	if r.pos < len(r.data) {
		n := copy(p, r.data[r.pos:])
		r.pos += n
		return n, nil
	}
	// All buffered bytes delivered; block until the test signals shutdown.
	<-r.stallCh
	return 0, io.EOF
}

// TestReadInput_GoroutinePath_PartialUTF8OnTimeout verifies that
// when a producer writes the first byte(s) of a multi-byte UTF-8
// sequence and then stalls past the read deadline, readInput
// includes those partial bytes in the assigned value before
// returning the deadline error. Bash 5.2.0 preserves the bytes
// (verified empirically); silently dropping them would lose data
// that has already been consumed from stdin.
func TestReadInput_GoroutinePath_PartialUTF8OnTimeout(t *testing.T) {
	stall := make(chan struct{})
	defer close(stall)
	r := &stallAfterReader{data: []byte{0xc3}, stallCh: stall}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	line, eof, err := readInput(ctx, r, '\n', true, -1, false, true)
	if err == nil {
		t.Fatalf("expected deadline error, got nil")
	}
	if eof {
		t.Fatalf("expected eof=false on deadline timeout")
	}
	if got, want := []byte(line), []byte{0xc3}; !bytes.Equal(got, want) {
		t.Fatalf("got line=%x, want %x (partial UTF-8 byte must be preserved)", got, want)
	}
}
