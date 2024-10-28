// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package prometheus

import (
	"io"
	"strings"
)

// A Reader implements the io.Reader interfaces by reading from
// a byte slice.
// Unlike a Buffer, a Reader is read-only and supports seeking.
// The zero value for Reader operates like a Reader of an empty slice.
type Reader struct {
	s           []byte
	i           int64 // current reading index
	filters     []string
	startOfLine bool
}

// Read implements the io.Reader interface.
func (r *Reader) Read(b []byte) (n int, err error) {
	if r.i >= int64(len(r.s)) {
		return 0, io.EOF
	}
	// we need to do 2 things:
	// 1) need to filter out \r character from output stream, as it breaks prometheus parser
	// 2) if any filters are present, we need to check if we are at the start of a line. If we are, then before we start to
	//    populate the buffer, we need to scan until the next newline character. Once we have the current line loaded into
	//    memory, we can check the filter list against it. If anything matches, we increment the index to the start of the
	//    next line. But the buffer size isn't necessarily equal to one line, it could be more than one line, so we need to
	//    keep doing this check as we fill the buffer if we encounter a newline.
	seek := r.i
	for n < len(b) {
		if seek >= int64(len(r.s)) {
			break
		}

		if r.startOfLine {
			if next := r.checkLine(seek); seek != next {
				seek = next
				continue
			}
		}

		c := r.s[seek]
		// case: \r, skip char
		// case: \n, start of line = true, append char
		// case: anything else, start of line = false, append char
		seek++
		if c == '\r' {
			// the prometheus TextParser does not support windows line separators, so we need to explicitly remove them
			continue
		} else if c == '\n' {
			r.startOfLine = true
		} else {
			r.startOfLine = false
		}

		b[n] = c
		n++
	}
	r.i = seek
	return
}

func (r *Reader) checkLine(n int64) int64 {
	if !r.startOfLine {
		return n
	}

	seek := n
	sb := strings.Builder{}
	var endOfLine bool
	for !endOfLine {
		if seek >= int64(len(r.s)) {
			break
		}

		if b := r.s[seek]; b == '\n' {
			endOfLine = true
		} else {
			sb.WriteByte(b)
			seek++
		}
	}

	line := sb.String()
	for i := range r.filters {
		if strings.Contains(line, r.filters[i]) {
			// filter exists, so we should skip this line
			return seek
		}
	}
	// no filter was found, so safe to process this line
	return n
}

// NewReader returns a new Reader reading from b.
func NewReader(b []byte, filters []string) *Reader { return &Reader{b, 0, filters, true} }
