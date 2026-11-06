// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// MaxNestingDepth bounds the depth of a parsed script's AST.
//
// rshell's AST passes ([validateNode] via [syntax.Walk]) and its executor
// recurse once per AST level. Some constructs — long pipelines (a|b|c…),
// logical lists (a&&b&&c…), and left-associative arithmetic — are built
// iteratively by the parser, so their AST can be far deeper than the parser's
// own recursion. Without this bound a sub-[MaxScriptBytes] script could drive
// those recursions past the Go runtime's 1 GiB goroutine stack limit, which
// aborts the process with an uncatchable "fatal error: stack overflow".
//
// The parser's own recursion is bounded separately, while it runs, by
// [maxParseFrames].
const MaxNestingDepth = 5000

// maxParseFrames bounds how many call frames mvdan.cc/sh's recursive-descent
// parser may stack below [ParseScript]. It is measured, not predicted, so it
// holds for every construct the parser recurses on. Per nesting level the
// parser uses about 6 frames for subshells, brace groups and compound
// commands, 8 for command substitutions, and 30 for arithmetic parentheses and
// array indexes, so the bound rejects roughly 1300, 1000 and 270 levels of
// those respectively. Depth is checked only when the parser reads more input,
// so nesting within the last [parseReadChunk] bytes read is not yet counted
// and the exact level at which a script is rejected varies by up to that much
// input.
const maxParseFrames = 8192

// parseReadChunk caps the bytes handed to the parser per read. The parser
// descends only while consuming input, so between two depth checks it recurses
// on at most this many bytes. mvdan.cc/sh v3.14.1 already reads at most 1 KiB
// at a time; the cap keeps the overshoot small if a later version reads larger
// chunks on demand. The guard relies on the parser reading lazily from inside
// its recursion; a parser that buffers all input up front would defeat it.
const parseReadChunk = 1024

// errParseTooDeep is returned by [ParseScript] when parsing would exceed
// [maxParseFrames].
var errParseTooDeep = errors.New("script nesting exceeds the parser's maximum depth; simplify deeply nested constructs")

// errNestingTooDeep is the error reported when a parsed script exceeds
// [MaxNestingDepth].
func errNestingTooDeep() error {
	return fmt.Errorf("script nesting exceeds maximum depth of %d; simplify deeply nested constructs", MaxNestingDepth)
}

// parseDepthBounded parses script with the parser's call depth bounded by
// [maxParseFrames]. The parser pulls input from its reader at the point in its
// recursion where it needs more bytes, so the reader measures the stack depth
// on every read and fails once it is exceeded. The parser then stops reading,
// unwinds, and returns the read error.
func parseDepthBounded(script, name string) (*syntax.File, error) {
	r := &depthBoundedReader{
		src:  strings.NewReader(script),
		skip: callerFrames() + maxParseFrames,
	}
	return syntax.NewParser().Parse(r, name)
}

// depthBoundedReader is the parser input used by [parseDepthBounded].
type depthBoundedReader struct {
	src io.Reader
	// skip is the frame count, from Read's own frame to the goroutine root,
	// beyond which the parse is too deep.
	skip int
	pc   [1]uintptr
}

func (r *depthBoundedReader) Read(p []byte) (int, error) {
	// runtime.Callers stores a PC only if the stack holds more than skip
	// frames above runtime.Callers itself.
	if runtime.Callers(r.skip, r.pc[:]) > 0 {
		return 0, errParseTooDeep
	}
	if len(p) > parseReadChunk {
		p = p[:parseReadChunk]
	}
	return r.src.Read(p)
}

// callerFrames returns the number of call frames from its caller to the
// goroutine root, inclusive.
func callerFrames() int {
	var pcs [64]uintptr
	n := 0
	for {
		// Skip runtime.Callers and callerFrames.
		got := runtime.Callers(2+n, pcs[:])
		n += got
		if got < len(pcs) {
			return n
		}
	}
}

// checkNestingDepth bounds the structural depth of a parsed AST. It is called
// from [ParseScript] and [validateNode] — and therefore on every [Runner.Run]
// and [Runner.Check] path — before the recursive [syntax.Walk] validation
// pass and before any recursive execution, so that none of those recursions
// can exceed [MaxNestingDepth] stack frames.
//
// It is itself iterative: it never recurses deeper than one level (to read a
// node's direct children), so it cannot overflow the stack regardless of how
// deep the AST is.
func checkNestingDepth(root syntax.Node) error {
	if root == nil {
		return nil
	}
	type item struct {
		node  syntax.Node
		depth int
	}
	stack := []item{{root, 0}}
	for len(stack) > 0 {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if it.depth > MaxNestingDepth {
			return errNestingTooDeep()
		}
		for _, child := range directChildren(it.node) {
			stack = append(stack, item{child, it.depth + 1})
		}
	}
	return nil
}

// directChildren returns the immediate child nodes of parent without
// descending further. It uses [syntax.Walk], whose visitor is invoked for the
// node itself first and then for each child; returning true only for parent
// and false for every child stops the walk at depth one, so this call recurses
// a single frame regardless of the overall tree depth.
func directChildren(parent syntax.Node) []syntax.Node {
	var children []syntax.Node
	syntax.Walk(parent, func(n syntax.Node) bool {
		if n == nil {
			return false
		}
		if n == parent {
			return true // descend into parent's direct children
		}
		children = append(children, n)
		return false // but no deeper
	})
	return children
}
