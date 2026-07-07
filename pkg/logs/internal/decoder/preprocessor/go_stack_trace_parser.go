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

	// Once past the header, a new start marker always signals a separate
	// trace — never a continuation of the current goroutine stack.
	if g.st != goStateInHeader && g.IsStart(line) {
		return false
	}

	switch g.st {
	case goStateInHeader:
		if goValidHeaderContinuation(line) {
			g.uncommitted = 0
			return true
		}
		// The Go runtime prints a blank line between the header and the first
		// goroutine/stack chunk, and normally that blank line is what ends the
		// header (handled by the empty-line case above). But many log sources
		// (container runtimes, loggers) strip blank lines before they reach the
		// aggregator, so the chunk-start line arrives directly after the header.
		// Treat a chunk start as an implicit end of the header; otherwise the
		// trace could never reach betweenChunks and would always be abandoned.
		if _, ok := goDetectChunkStart(line); !ok {
			return false
		}
		g.st = goStateBetweenChunks
		fallthrough

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
		// A chunk-start line ends the current chunk just as a blank line would.
		// Sources that strip the header's blank-line separator strip the
		// separators between later chunks too, so without this a multi-goroutine
		// dump would combine only its first chunk. Re-dispatch through
		// betweenChunks (which resets uncommitted and counts the new chunk),
		// yielding the same end-state as the blank-line path.
		if _, ok := goDetectChunkStart(line); ok {
			g.st = goStateBetweenChunks
			return g.AcceptLine(line)
		}
		return g.validStackLine(line)

	case goStateInRegDump:
		if goValidRegisterLine(line) {
			g.uncommitted = 0
			return true
		}
		// As above: a chunk-start line directly after a register dump (no blank
		// line) begins a new chunk rather than ending the trace.
		if _, ok := goDetectChunkStart(line); ok {
			g.st = goStateBetweenChunks
			return g.AcceptLine(line)
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

// goHasDigitAt reports whether line has an ASCII digit at index i.
func goHasDigitAt(line []byte, i int) bool {
	return i < len(line) && line[i] >= '0' && line[i] <= '9'
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
		// A real goroutine header is always "goroutine <goid> [<status>]:",
		// where <goid> is numeric — runtime/traceback.go goroutineheader emits
		// print("goroutine ", gp.goid). Requiring a digit immediately after the
		// prefix rejects benign prose that merely starts with "goroutine "
		// (e.g. "goroutine to watchExperiments has terminated", "goroutine
		// profile: total 5"), which would otherwise be mistaken for the first
		// chunk of a trace and let an errant start marker combine unrelated
		// lines. A digit is the only reliable discriminator: the verbose
		// GOTRACEBACK=system form ("goroutine 1 gp=0x.. m=0 mp=0x.. [running]:")
		// still leads with the numeric goid, so we must not require " [" here.
		if goHasPrefix(line, "goroutine ") && goHasDigitAt(line, len("goroutine ")) {
			return goChunkStack, true
		}
	case 'r':
		// "runtime stack:" header, or a register dump led by r0/rax/ra
		// (arm64, amd64, ppc64x, mips*, s390x, loong64, riscv64).
		if goHasPrefix(line, "runtime stack:") {
			return goChunkStack, true
		}
		if goIsRegisterDumpStart(line) {
			return goChunkRegDump, true
		}
	case 'e':
		// 386 register dumps begin with "eax".
		if goIsRegisterDumpStart(line) {
			return goChunkRegDump, true
		}
	case 't':
		// arm register dumps begin with "trap".
		if goIsRegisterDumpStart(line) {
			return goChunkRegDump, true
		}
	}
	return 0, false
}

// goIsRegisterDumpStart reports whether `line` is the FIRST line of a Go
// runtime register dump. dumpregs() emits a fixed, arch-specific register
// order, so the dump always begins with a known leading register:
//
//	r0    arm64, ppc64x, mips64x, mipsx, s390x, loong64
//	rax   amd64
//	eax   386
//	trap  arm   (dumpregs prints trap, error, oldmask, then r0...)
//	ra    riscv64 (prints "ra ...\tsp ...")
//
// We require the line to also be a structurally valid register line (a hex
// value, correct shape) so that ordinary log lines that merely start with one
// of these words — e.g. "trap handler installed" or "ra debug enabled" — are
// not mistaken for the start of a register dump.
func goIsRegisterDumpStart(line []byte) bool {
	if !goValidRegisterLine(line) {
		return false
	}
	switch string(goLeadingRegisterName(line)) {
	case "r0", "rax", "eax", "trap", "ra":
		return true
	default:
		return false
	}
}

// goLeadingRegisterName returns the leading register-name token of `line`:
// the run of [a-z][a-z0-9]* before the first whitespace. The line is assumed
// to have already passed goValidRegisterLine, so a name is always present.
func goLeadingRegisterName(line []byte) []byte {
	i := 0
	for i < len(line) {
		c := line[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			i++
			continue
		}
		break
	}
	return line[:i]
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

// goValidRegisterLine reports whether `line` looks like a register dump line
// emitted by the Go runtime's per-arch dumpregs(). The runtime always prints
// "<name> <hex>" pairs, where <name> is a short lowercase identifier
// (registers across all GOARCHes are 2-7 chars: lowercase letters optionally
// followed by digits; the longest is arm's "oldmask") and <hex> begins with
// "0x". Several architectures (ppc64x, mips64x, mipsx, s390x, loong64,
// riscv64) pack two register pairs per line separated by '\t'; both halves
// must validate.
//
// The strict shape rules out unrelated log lines that merely happen to
// begin with a lowercase letter (e.g. "error=denied", "level=info ts=..."),
// which would otherwise be silently absorbed into an in-flight register
// dump and contaminate the eventual combined message.
func goValidRegisterLine(line []byte) bool {
	if len(line) == 0 {
		return false
	}
	rest, ok := goConsumeRegisterPair(line)
	if !ok {
		return false
	}
	// Accept either end-of-line, or "<tab>second pair" (packed-pair layout
	// used by ppc64x, mips64x, loong64, riscv64).
	if len(rest) == 0 {
		return true
	}
	if rest[0] != '\t' {
		return false
	}
	rest, ok = goConsumeRegisterPair(rest[1:])
	return ok && len(rest) == 0
}

// goConsumeRegisterPair consumes one "<name><ws>0x<hex>" pair from the head
// of `b`, returning the unconsumed remainder and ok=true on success.
//
// Grammar:
//   - name: 2-8 bytes; first byte [a-z]; subsequent bytes [a-z0-9].
//     The 8-byte upper bound accommodates the longest register name
//     emitted by upstream Go's dumpregs() across all GOARCHes
//     (arm/linux's "oldmask", 7 chars), with one byte of headroom.
//   - ws:   one or more spaces or tabs.
//   - val:  "0x" followed by one or more hex digits ([0-9a-fA-F]).
//
// On failure the original slice is returned unchanged.
func goConsumeRegisterPair(b []byte) ([]byte, bool) {
	if len(b) == 0 || b[0] < 'a' || b[0] > 'z' {
		return b, false
	}
	const maxNameLen = 8
	i := 1
	for i < len(b) && i < maxNameLen {
		c := b[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			i++
			continue
		}
		break
	}
	if i < 2 {
		return b, false
	}

	wsStart := i
	for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
		i++
	}
	if i == wsStart {
		return b, false
	}

	if i+2 > len(b) || b[i] != '0' || b[i+1] != 'x' {
		return b, false
	}
	i += 2

	hexStart := i
	for i < len(b) {
		c := b[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			i++
			continue
		}
		break
	}
	if i == hexStart {
		return b, false
	}
	return b[i:], true
}
