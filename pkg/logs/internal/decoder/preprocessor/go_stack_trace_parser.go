// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import "bytes"

// GoStackTraceParser implements StackTraceParser for Go runtime crash dumps.
// It uses a 4-state machine (inHeader, betweenChunks, inChunk, inRegDump)
// ported from dev/crash_patterns/parser/parser.go.
type GoStackTraceParser struct {
	st          goTraceStateID
	chunkCount  int
	expectFile  bool
	uncommitted int
}

// NewGoStackTraceParser returns a new GoStackTraceParser.
func NewGoStackTraceParser() *GoStackTraceParser {
	return &GoStackTraceParser{}
}

var goColonSpace = []byte(": ")

// IsStart returns true if the line is a Go crash header (panic:, fatal error:,
// runtime:, unexpected fault address, or SIG*:).
func (g *GoStackTraceParser) IsStart(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	switch content[0] {
	case 'p':
		return goHasPrefix(content, "panic:")
	case 'f':
		return goHasPrefix(content, "fatal error:")
	case 'r':
		return goHasPrefix(content, "runtime:")
	case 'u':
		return goHasPrefix(content, "unexpected fault address")
	case 'S':
		return goHasPrefix(content, "SIG") && bytes.Contains(content, goColonSpace)
	}
	return false
}

// Reset prepares the parser to validate a new Go crash trace.
func (g *GoStackTraceParser) Reset() {
	g.st = goStateInHeader
	g.chunkCount = 0
	g.expectFile = false
	g.uncommitted = 0
}

// AcceptLine returns true if the line is a valid continuation of the current
// Go crash dump.
func (g *GoStackTraceParser) AcceptLine(line []byte) bool {
	if len(line) == 0 {
		switch g.st {
		case goStateInHeader:
			g.st = goStateBetweenChunks
			g.uncommitted = 0
			return true
		case goStateBetweenChunks:
			return true
		case goStateInChunk:
			if g.expectFile {
				g.uncommitted++
			} else {
				g.uncommitted = 0
			}
			g.st = goStateBetweenChunks
			return true
		case goStateInRegDump:
			g.st = goStateBetweenChunks
			g.uncommitted = 0
			return true
		}
	}

	switch g.st {
	case goStateInHeader:
		if goValidHeaderContinuation(line) {
			g.uncommitted = 0
			return true
		}
		return false

	case goStateBetweenChunks:
		ct, ok := goDetectChunkStart(line)
		if !ok {
			return false
		}
		g.expectFile = false
		g.chunkCount++
		g.uncommitted = 0
		if ct == goChunkRegDump {
			g.st = goStateInRegDump
		} else {
			g.st = goStateInChunk
		}
		return true

	case goStateInChunk:
		return g.validStackLine(line)

	case goStateInRegDump:
		if goValidRegisterLine(line) {
			g.uncommitted = 0
			return true
		}
		return false
	}
	return false
}

// ShouldCombine returns true if at least one goroutine/stack chunk was seen.
func (g *GoStackTraceParser) ShouldCombine() bool {
	return g.chunkCount > 0
}

// Uncommitted returns the number of trailing accepted lines that are tentative
// (e.g. a function name line awaiting its tab-indented file line).
func (g *GoStackTraceParser) Uncommitted() int {
	return g.uncommitted
}

// ---------------------------------------------------------------------------
// Internal types and helpers
// ---------------------------------------------------------------------------

type goTraceStateID int

const (
	goStateInHeader goTraceStateID = iota
	goStateBetweenChunks
	goStateInChunk
	goStateInRegDump
)

type goChunkType int

const (
	goChunkStack goChunkType = iota
	goChunkRegDump
)

func goHasPrefix(line []byte, prefix string) bool {
	return len(line) >= len(prefix) && string(line[:len(prefix)]) == prefix
}

func goValidHeaderContinuation(line []byte) bool {
	if len(line) == 0 {
		return false
	}
	switch line[0] {
	case '\t':
		return true
	case '[':
		return goHasPrefix(line, "[signal ")
	case 'f':
		return goHasPrefix(line, "fatal error:")
	case 'P':
		return goHasPrefix(line, "PC=")
	case 's':
		return goHasPrefix(line, "signal ")
	case 'r':
		return goHasPrefix(line, "runtime:")
	}
	return false
}

func goDetectChunkStart(line []byte) (goChunkType, bool) {
	if len(line) == 0 {
		return 0, false
	}
	switch line[0] {
	case 'g':
		if goHasPrefix(line, "goroutine ") {
			return goChunkStack, true
		}
	case 'r':
		if goHasPrefix(line, "runtime stack:") {
			return goChunkStack, true
		}
		if goIsRegisterDumpStart(line) {
			return goChunkRegDump, true
		}
	}
	return 0, false
}

func goIsRegisterDumpStart(line []byte) bool {
	if len(line) < 3 {
		return false
	}
	if line[0] == 'r' && line[1] == '0' && (line[2] == ' ' || line[2] == '\t') {
		return true
	}
	if len(line) >= 4 && line[0] == 'r' && line[1] == 'a' && line[2] == 'x' && (line[3] == ' ' || line[3] == '\t') {
		return true
	}
	return false
}

func (g *GoStackTraceParser) validStackLine(line []byte) bool {
	b := line[0]
	if g.expectFile {
		if b != '\t' {
			return false
		}
		g.expectFile = false
		g.uncommitted = 0
		return true
	}
	if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') {
		g.expectFile = true
		g.uncommitted = 1
		return true
	}
	// "...N frames elided..."
	if b == '.' && len(line) >= 3 && line[1] == '.' && line[2] == '.' {
		g.uncommitted = 0
		return true
	}
	return false
}

func goValidRegisterLine(line []byte) bool {
	return len(line) > 0 && line[0] >= 'a' && line[0] <= 'z'
}
