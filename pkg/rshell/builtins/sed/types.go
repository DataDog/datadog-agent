// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sed

import (
	"fmt"
	"regexp"
)

// --- Error types ---

// quitError signals a q or Q command with an exit code.
type quitError struct {
	code uint8
}

func (e *quitError) Error() string {
	return fmt.Sprintf("quit with code %d", e.code)
}

// --- Address types ---

// addrType distinguishes different address kinds.
type addrType int

const (
	addrNone   addrType = iota
	addrLine            // specific line number
	addrLast            // $ (last line)
	addrRegexp          // /regex/
	addrStep            // first~step (GNU extension)
)

// address represents a sed address (line number, regex, or $).
type address struct {
	kind  addrType
	line  int64          // for addrLine
	re    *regexp.Regexp // for addrRegexp
	first int64          // for addrStep
	step  int64          // for addrStep
}

// --- Command types ---

// cmdType identifies the sed command.
type cmdType int

const (
	cmdSubstitute cmdType = iota
	cmdPrint
	cmdDelete
	cmdQuit
	cmdQuitNoprint
	cmdTransliterate
	cmdAppend
	cmdInsert
	cmdChange
	cmdLineNum
	cmdPrintUnambig
	cmdNext
	cmdNextAppend
	cmdHoldCopy
	cmdHoldAppend
	cmdGetCopy
	cmdGetAppend
	cmdExchange
	cmdBranch
	cmdLabel
	cmdBranchIfSub
	cmdBranchIfNoSub
	cmdPrintFirstLine  // P: print up to first embedded newline
	cmdDeleteFirstLine // D: delete up to first embedded newline, restart cycle
	cmdGroup
	cmdNoop
)

// sedCmd represents a single parsed sed command.
type sedCmd struct {
	addr1   *address
	addr2   *address
	negated bool
	inRange bool // stateful: tracks whether we're inside a two-address range
	kind    cmdType

	// For s command:
	subRe          *regexp.Regexp // nil means "reuse last regex"
	subReplacement string
	subGlobal      bool
	subPrint       bool
	subNth         int
	// Note: case-insensitive flag (i/I) on empty regexp is rejected at parse time
	// (GNU sed: "cannot specify modifiers on empty regexp"), so no deferred flag is needed.

	// For y command:
	transMap map[rune]rune // precomputed rune mapping for O(1) lookup

	// For a, i, c commands:
	text string

	// For q, Q commands:
	quitCode uint8

	// For b, t, T commands:
	label string

	// For { ... } grouping:
	children []*sedCmd
}

// --- Action types ---

// actionType signals how to proceed after executing a command.
type actionType int

const (
	actionContinue actionType = iota
	actionDelete              // d/D command: skip auto-print, start next cycle
	actionRestart             // D command: restart the cycle (used internally)
	actionBranch              // b/t/T command completed a non-local jump
)
