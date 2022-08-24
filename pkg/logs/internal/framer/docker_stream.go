// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

const (
	headerPrefixLength = 4
)

// dockerStreamMatcher matches the stream format documented here:
// https://pkg.go.dev/github.com/moby/moby/client?utm_source=godoc#Client.ContainerLogs
//
// Well, sort-of.  It ignores the size in the headers, and groups multiple "frames",
// including their headers, into a single log message, based on a search for newlines.
type dockerStreamMatcher struct {
	// contentLenLimit is the maximum content length that will be returned.
	// Lines longer than this value will be split into multiple frames, without regard
	// for the header framing.
	contentLenLimit int
}

// FindFrame implements EndLineMatcher#FindFrame.
func (s *dockerStreamMatcher) FindFrame(buf []byte, seen int) ([]byte, int) {
	for i := seen; i < len(buf); i++ {
		if buf[i] == '\n' && !s.matchHeader([]byte{}, buf[:i]) {
			return buf[:i], i + 1
		}
	}
	return nil, 0
}

// When a newline (in byte is 10) is matching, an additional check need to
// be done to make sure this is not part of docker header.
// [1|2 0 0 0 size1 size2 size3 size4], where size1 size2 size3 size4 can be
// 10 in byte.
// case [1|2 0 0 0 10 size2 size3 size4]
// case [1|2 0 0 0 size1 10 size3 size4]
// case [1|2 0 0 0 size1 size2 10 size4]
// case [1|2 0 0 0 size1 size2 size3 10]
func (s *dockerStreamMatcher) matchHeader(exists []byte, bs []byte) bool {
	l := len(exists) + len(bs)
	if l < headerPrefixLength {
		return false
	}

	for i := 0; i < 4; i++ {
		// possible start of header offset
		idx := l - (headerPrefixLength + i)
		if idx < 0 {
			// less than 4 + i bytes
			continue
		}
		// We test for {1, 0, 0, 0} (stdout log) and {2, 0, 0, 0} (stderr)
		if s.checkByte(exists, bs, idx, 1) || s.checkByte(exists, bs, idx, 2) {
			if s.checkByte(exists, bs, idx+1, 0) &&
				s.checkByte(exists, bs, idx+2, 0) &&
				s.checkByte(exists, bs, idx+3, 0) {
				return true
			}
		}
	}
	return false
}

func (s *dockerStreamMatcher) checkByte(exists []byte, bs []byte, i int, val byte) bool {
	l := len(exists) + len(bs)
	if i < l {
		if i < len(exists) {
			return exists[i] == val
		}
		return bs[i-len(exists)] == val
	}
	return false
}
