// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Safety and resource limits for the sed builtin.
const (
	// sedMaxIterations limits total command executions per input line,
	// preventing infinite loops from branch commands like ":loop; b loop".
	sedMaxIterations = 10000

	// sedMaxInputLines caps the number of input lines buffered.
	sedMaxInputLines = 100000

	// sedMaxSpaceBytes caps pattern space and hold space size (10 MiB).
	sedMaxSpaceBytes = 10 << 20

	// sedMaxScriptFileBytes caps script file size read via -f (1 MiB).
	sedMaxScriptFileBytes = 1 << 20
)

// Synthetic command bytes for flattened group representation.
const (
	sedCmdGroupStart byte = 0x01 // address check; jumps to jumpTarget on non-match
	sedCmdGroupEnd   byte = 0x02 // no-op marker
)

// sedAddress represents a line address in a sed command.
type sedAddress struct {
	lineNum int            // >0 for line number addresses
	last    bool           // $ address
	regex   *regexp.Regexp // /pattern/ address
	step    int            // for addr~step (GNU extension), 0 = not used
}

// sedSub holds substitution command parameters.
type sedSub struct {
	regex   *regexp.Regexp
	replace string
	global  bool // g flag
	print   bool // p flag
	nth     int  // N flag (replace Nth match only), 0 = not used
	icase   bool // I/i flag (case insensitive)
}

// sedCommand represents a single parsed sed command.
type sedCommand struct {
	addr1      *sedAddress    // nil = no address
	addr2      *sedAddress    // nil = single address or no address
	negated    bool           // ! prefix
	inRange    bool           // runtime state for addr1,addr2 range tracking
	cmd        byte           // command character
	sub        *sedSub        // for 's' command
	transFrom  string         // for 'y' command
	transTo    string         // for 'y' command
	text       string         // for 'a', 'i', 'c' commands
	label      string         // for 'b', 't', 'T', ':' commands
	readFile   string         // for 'r', 'R' commands
	quitCode   int            // for 'q', 'Q' commands
	jumpTarget int            // for sedCmdGroupStart: index of matching group-end
	children   []*sedCommand  // for '{' grouping (used during parsing, flattened before execution)
}

// sedState holds the execution state while processing input.
type sedState struct {
	patternSpace string
	holdSpace    string
	lineNum      int
	lastLine     bool
	subMade      bool // for t/T branching
	appendQueue  []string
	quit         bool
	quitCode     int
	suppress     bool // -n flag

	// Line access for n/N commands.
	lines   []string
	lineIdx *int
}

// builtinSed implements a safe subset of the POSIX sed command.
// Blocked: -i (in-place), w/W commands, e command, s///w, s///e.
func (r *Runner) builtinSed(ctx context.Context, args []string) exitStatus {
	var exit exitStatus

	var (
		suppress    bool // -n
		extendedRE  bool // -E / -r
		scripts     []string
		scriptFiles []string
	)

	// Parse options — custom parser needed for combined flags like -nE
	// and -e taking rest-of-arg as script value.
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if len(arg) == 0 || arg[0] != '-' {
			break
		}

		// Handle combined short options like -nE
		if len(arg) > 1 && arg[1] != '-' {
			j := 1
			for j < len(arg) {
				switch arg[j] {
				case 'n':
					suppress = true
					j++
				case 'E', 'r':
					extendedRE = true
					j++
				case 'e':
					j++
					if j < len(arg) {
						scripts = append(scripts, arg[j:])
						j = len(arg)
					} else {
						i++
						if i >= len(args) {
							r.errf("sed: option requires an argument -- 'e'\n")
							exit.code = 2
							return exit
						}
						scripts = append(scripts, args[i])
					}
				case 'f':
					j++
					if j < len(arg) {
						scriptFiles = append(scriptFiles, arg[j:])
						j = len(arg)
					} else {
						i++
						if i >= len(args) {
							r.errf("sed: option requires an argument -- 'f'\n")
							exit.code = 2
							return exit
						}
						scriptFiles = append(scriptFiles, args[i])
					}
				case 'i', 'I':
					r.errf("sed: -i (in-place edit) is not available in safe shell\n")
					exit.code = 2
					return exit
				default:
					r.errf("sed: invalid option -- '%c'\n", arg[j])
					exit.code = 2
					return exit
				}
			}
			i++
			continue
		}

		// Long-ish options
		switch {
		case arg == "-n":
			suppress = true
		case arg == "-E" || arg == "-r":
			extendedRE = true
		case arg == "-e":
			i++
			if i >= len(args) {
				r.errf("sed: option requires an argument -- 'e'\n")
				exit.code = 2
				return exit
			}
			scripts = append(scripts, args[i])
		case arg == "-f":
			i++
			if i >= len(args) {
				r.errf("sed: option requires an argument -- 'f'\n")
				exit.code = 2
				return exit
			}
			scriptFiles = append(scriptFiles, args[i])
		case arg == "-i" || strings.HasPrefix(arg, "--in-place"):
			r.errf("sed: -i (in-place edit) is not available in safe shell\n")
			exit.code = 2
			return exit
		default:
			r.errf("sed: invalid option %q\n", arg)
			exit.code = 2
			return exit
		}
		i++
	}

	remaining := args[i:]

	// Read script files with size limit
	for _, sf := range scriptFiles {
		absPath := r.absPath(sf)
		f, err := r.open(ctx, absPath, os.O_RDONLY, 0, true)
		if err != nil {
			exit.code = 2
			return exit
		}
		data, err := io.ReadAll(io.LimitReader(f, sedMaxScriptFileBytes+1))
		f.Close()
		if err != nil {
			r.errf("sed: error reading script file %q: %v\n", sf, err)
			exit.code = 2
			return exit
		}
		if len(data) > sedMaxScriptFileBytes {
			r.errf("sed: script file %q too large (>1MB)\n", sf)
			exit.code = 2
			return exit
		}
		scripts = append(scripts, string(data))
	}

	// If no -e or -f, the first non-option arg is the script
	if len(scripts) == 0 {
		if len(remaining) == 0 {
			r.errf("sed: no script command has been specified\n")
			exit.code = 2
			return exit
		}
		scripts = append(scripts, remaining[0])
		remaining = remaining[1:]
	}

	fullScript := strings.Join(scripts, "\n")

	// Compile: parse then flatten
	parsed, err := sedCompile(fullScript, extendedRE)
	if err != nil {
		r.errf("sed: %v\n", err)
		exit.code = 2
		return exit
	}
	commands := sedFlatten(parsed)

	// Build label map on the flat command list
	labels := make(map[string]int)
	for i, cmd := range commands {
		if cmd.cmd == ':' {
			labels[cmd.label] = i
		}
	}

	state := &sedState{suppress: suppress}

	if len(remaining) == 0 {
		if r.stdin == nil {
			r.errf("sed: cannot read from stdin\n")
			exit.code = 1
			return exit
		}
		exit = r.sedProcessInput(ctx, r.stdin, commands, labels, state)
	} else {
		for _, p := range remaining {
			if state.quit {
				break
			}
			absP := r.absPath(p)
			f, err := r.open(ctx, absP, os.O_RDONLY, 0, true)
			if err != nil {
				exit.code = 2
				return exit
			}
			exit = r.sedProcessInput(ctx, f, commands, labels, state)
			f.Close()
		}
	}

	if state.quit {
		exit.code = uint8(state.quitCode)
	}

	return exit
}

// sedFlatten converts a parsed command tree into a flat instruction list.
// Groups ({...}) become sedCmdGroupStart / sedCmdGroupEnd pairs with a
// jumpTarget on the start pointing to the end. This ensures labels inside
// groups have correct indices and eliminates recursive execution.
func sedFlatten(commands []*sedCommand) []*sedCommand {
	var flat []*sedCommand
	sedFlattenInto(&flat, commands)
	return flat
}

func sedFlattenInto(flat *[]*sedCommand, commands []*sedCommand) {
	for _, cmd := range commands {
		if cmd.cmd == '{' {
			// Insert group-start with the group's address
			gs := &sedCommand{
				cmd:     sedCmdGroupStart,
				addr1:   cmd.addr1,
				addr2:   cmd.addr2,
				negated: cmd.negated,
			}
			gsIdx := len(*flat)
			*flat = append(*flat, gs)

			// Recursively flatten children
			sedFlattenInto(flat, cmd.children)

			// Insert group-end
			ge := &sedCommand{cmd: sedCmdGroupEnd}
			geIdx := len(*flat)
			*flat = append(*flat, ge)

			// Patch jump target
			gs.jumpTarget = geIdx
			_ = gsIdx
		} else {
			*flat = append(*flat, cmd)
		}
	}
}

// sedProcessInput reads lines from the reader and applies sed commands.
func (r *Runner) sedProcessInput(ctx context.Context, reader io.Reader, commands []*sedCommand, labels map[string]int, state *sedState) exitStatus {
	var exit exitStatus
	scanner := bufio.NewScanner(reader)

	// Collect lines with cap to prevent unbounded memory growth.
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) >= sedMaxInputLines {
			r.errf("sed: input truncated at %d lines\n", sedMaxInputLines)
			break
		}
	}
	if err := scanner.Err(); err != nil {
		r.errf("sed: read error: %v\n", err)
		exit.code = 2
		return exit
	}

	state.lines = lines
	state.lineIdx = new(int)

	for idx := 0; idx < len(lines); idx++ {
		select {
		case <-ctx.Done():
			return exit
		default:
		}

		if state.quit {
			return exit
		}

		state.lineNum++
		*state.lineIdx = idx
		state.lastLine = (idx == len(lines)-1)
		state.patternSpace = lines[idx]
		state.subMade = false
		state.appendQueue = state.appendQueue[:0]

		action := r.sedExecCommands(ctx, commands, labels, state)

		// n/N may have advanced lineIdx
		idx = *state.lineIdx
		state.lastLine = (idx == len(lines)-1)

		switch action {
		case sedActionQuit:
			if !state.suppress {
				r.outf("%s\n", state.patternSpace)
			}
			r.sedFlushAppend(state)
			state.quit = true
			return exit
		case sedActionQuitNoprint:
			r.sedFlushAppend(state)
			state.quit = true
			return exit
		case sedActionEndCycle:
			// End of cycle: print (unless suppressed) and advance
		case sedActionDelete:
			r.sedFlushAppend(state)
			continue
		case sedActionDeleteFirstLine:
			if nl := strings.IndexByte(state.patternSpace, '\n'); nl >= 0 {
				state.patternSpace = state.patternSpace[nl+1:]
				state.subMade = false
				state.appendQueue = state.appendQueue[:0]
				action = r.sedExecCommands(ctx, commands, labels, state)
				if action == sedActionDelete || action == sedActionDeleteFirstLine {
					r.sedFlushAppend(state)
					continue
				}
			}
			if !state.suppress {
				r.outf("%s\n", state.patternSpace)
			}
			r.sedFlushAppend(state)
			continue
		}

		if !state.suppress {
			r.outf("%s\n", state.patternSpace)
		}
		r.sedFlushAppend(state)
	}

	return exit
}

func (r *Runner) sedFlushAppend(state *sedState) {
	for _, a := range state.appendQueue {
		r.outf("%s\n", a)
	}
}

type sedAction int

const (
	sedActionContinue       sedAction = iota
	sedActionQuit                     // q
	sedActionQuitNoprint              // Q
	sedActionEndCycle                 // end current cycle (b with no label, t/T restart)
	sedActionDelete                   // d
	sedActionDeleteFirstLine          // D
)

// sedExecCommands executes the flat command list against the current state.
// It enforces an iteration limit and checks for context cancellation to
// prevent infinite loops from branch commands.
func (r *Runner) sedExecCommands(ctx context.Context, commands []*sedCommand, labels map[string]int, state *sedState) sedAction {
	iterations := 0
	for i := 0; i < len(commands); i++ {
		iterations++
		if iterations > sedMaxIterations {
			r.errf("sed: execution limit exceeded (possible infinite loop)\n")
			return sedActionEndCycle
		}
		select {
		case <-ctx.Done():
			return sedActionEndCycle
		default:
		}

		cmd := commands[i]

		// Handle synthetic group-start: check address, jump past on non-match
		if cmd.cmd == sedCmdGroupStart {
			if !sedMatchAddress(cmd, state) {
				i = cmd.jumpTarget // jump to group-end
			}
			continue
		}
		if cmd.cmd == sedCmdGroupEnd {
			continue
		}

		if !sedMatchAddress(cmd, state) {
			continue
		}

		action := r.sedExecOne(ctx, cmd, labels, state, &i)
		if action != sedActionContinue {
			return action
		}
	}
	return sedActionContinue
}

// sedExecOne executes a single sed command. It may modify *cmdIdx for branches.
func (r *Runner) sedExecOne(ctx context.Context, cmd *sedCommand, labels map[string]int, state *sedState, cmdIdx *int) sedAction {
	switch cmd.cmd {
	case 's':
		made := sedExecSubstitute(cmd.sub, state)
		if made && cmd.sub.print {
			r.outf("%s\n", state.patternSpace)
		}
		state.subMade = made || state.subMade
	case 'y':
		state.patternSpace = sedExecTransliterate(state.patternSpace, cmd.transFrom, cmd.transTo)
	case 'd':
		return sedActionDelete
	case 'D':
		return sedActionDeleteFirstLine
	case 'p':
		r.outf("%s\n", state.patternSpace)
	case 'P':
		line := state.patternSpace
		if nl := strings.IndexByte(line, '\n'); nl >= 0 {
			line = line[:nl]
		}
		r.outf("%s\n", line)
	case 'n':
		// Print current pattern space (unless suppressed), then read next line.
		// Per POSIX: if no next input line is available, branch to end of script.
		if !state.suppress {
			r.outf("%s\n", state.patternSpace)
		}
		r.sedFlushAppend(state)
		state.appendQueue = state.appendQueue[:0]
		if *state.lineIdx+1 < len(state.lines) {
			*state.lineIdx++
			state.lineNum++
			state.patternSpace = state.lines[*state.lineIdx]
			state.lastLine = (*state.lineIdx == len(state.lines)-1)
		} else {
			return sedActionEndCycle
		}
	case 'N':
		// Append next line to pattern space
		if *state.lineIdx+1 >= len(state.lines) {
			// No more input — default print and exit
			if !state.suppress {
				r.outf("%s\n", state.patternSpace)
			}
			state.quit = true
			return sedActionQuitNoprint
		}
		*state.lineIdx++
		state.lineNum++
		state.lastLine = (*state.lineIdx == len(state.lines)-1)
		state.patternSpace = state.patternSpace + "\n" + state.lines[*state.lineIdx]
		if len(state.patternSpace) > sedMaxSpaceBytes {
			r.errf("sed: pattern space exceeded size limit\n")
			return sedActionEndCycle
		}
	case 'g':
		state.patternSpace = state.holdSpace
	case 'G':
		state.patternSpace = state.patternSpace + "\n" + state.holdSpace
		if len(state.patternSpace) > sedMaxSpaceBytes {
			r.errf("sed: pattern space exceeded size limit\n")
			return sedActionEndCycle
		}
	case 'h':
		state.holdSpace = state.patternSpace
	case 'H':
		state.holdSpace = state.holdSpace + "\n" + state.patternSpace
		if len(state.holdSpace) > sedMaxSpaceBytes {
			r.errf("sed: hold space exceeded size limit\n")
			return sedActionEndCycle
		}
	case 'x':
		state.patternSpace, state.holdSpace = state.holdSpace, state.patternSpace
	case 'a':
		state.appendQueue = append(state.appendQueue, cmd.text)
	case 'i':
		r.outf("%s\n", cmd.text)
	case 'c':
		r.outf("%s\n", cmd.text)
		return sedActionDelete
	case 'l':
		r.outf("%s$\n", sedVisual(state.patternSpace))
	case '=':
		r.outf("%d\n", state.lineNum)
	case 'q':
		state.quitCode = cmd.quitCode
		return sedActionQuit
	case 'Q':
		state.quitCode = cmd.quitCode
		return sedActionQuitNoprint
	case 'r':
		data, err := r.sedReadFile(ctx, cmd.readFile)
		if err == nil {
			state.appendQueue = append(state.appendQueue, data)
		}
	case 'R':
		line, err := r.sedReadFirstLine(ctx, cmd.readFile)
		if err == nil {
			state.appendQueue = append(state.appendQueue, line)
		}
	case 'b':
		if cmd.label == "" {
			return sedActionEndCycle
		}
		if idx, ok := labels[cmd.label]; ok {
			*cmdIdx = idx - 1 // will be incremented by the loop
			return sedActionContinue
		}
		return sedActionEndCycle
	case 't':
		if state.subMade {
			state.subMade = false
			if cmd.label == "" {
				return sedActionEndCycle
			}
			if idx, ok := labels[cmd.label]; ok {
				*cmdIdx = idx - 1
				return sedActionContinue
			}
			return sedActionEndCycle
		}
	case 'T':
		if !state.subMade {
			if cmd.label == "" {
				return sedActionEndCycle
			}
			if idx, ok := labels[cmd.label]; ok {
				*cmdIdx = idx - 1
				return sedActionContinue
			}
			return sedActionEndCycle
		}
	case ':':
		// Label definition — no-op at execution time
	}
	return sedActionContinue
}

// sedMatchAddress returns true if the command should execute for the current line.
func sedMatchAddress(cmd *sedCommand, state *sedState) bool {
	matched := sedMatchAddressRaw(cmd, state)
	if cmd.negated {
		return !matched
	}
	return matched
}

func sedMatchAddressRaw(cmd *sedCommand, state *sedState) bool {
	if cmd.addr1 == nil {
		return true // no address = match all
	}

	if cmd.addr2 == nil {
		return sedAddrMatches(cmd.addr1, state) // single address
	}

	// Range: addr1,addr2 — proper toggle logic
	if cmd.inRange {
		if sedAddrMatches(cmd.addr2, state) {
			cmd.inRange = false
		}
		return true
	}
	if sedAddrMatches(cmd.addr1, state) {
		cmd.inRange = true
		return true
	}
	return false
}

func sedAddrMatches(addr *sedAddress, state *sedState) bool {
	if addr.last {
		return state.lastLine
	}
	if addr.lineNum > 0 {
		if addr.step > 0 {
			return state.lineNum >= addr.lineNum && (state.lineNum-addr.lineNum)%addr.step == 0
		}
		return state.lineNum == addr.lineNum
	}
	if addr.regex != nil {
		return addr.regex.MatchString(state.patternSpace)
	}
	return false
}

// sedExecSubstitute performs s/re/repl/flags. Returns true if a substitution was made.
func sedExecSubstitute(sub *sedSub, state *sedState) bool {
	if sub == nil {
		return false
	}

	re := sub.regex
	ps := state.patternSpace

	if sub.global {
		result := re.ReplaceAllString(ps, sedGoReplace(sub.replace))
		if result != ps {
			state.patternSpace = result
			return true
		}
		return false
	}

	if sub.nth > 0 {
		count := 0
		result := re.ReplaceAllStringFunc(ps, func(match string) string {
			count++
			if count == sub.nth {
				return re.ReplaceAllString(match, sedGoReplace(sub.replace))
			}
			return match
		})
		if result != ps {
			state.patternSpace = result
			return true
		}
		return false
	}

	// Replace first match
	loc := re.FindStringIndex(ps)
	if loc == nil {
		return false
	}

	matched := ps[loc[0]:loc[1]]
	replacement := re.ReplaceAllString(matched, sedGoReplace(sub.replace))
	state.patternSpace = ps[:loc[0]] + replacement + ps[loc[1]:]
	return true
}

// sedGoReplace converts sed replacement syntax to Go regexp replacement syntax.
// Sed uses \1..\9 for backreferences; Go uses $1..$9.
// Sed uses & for the full match; Go uses $0.
func sedGoReplace(sedRepl string) string {
	var b strings.Builder
	b.Grow(len(sedRepl))
	for i := 0; i < len(sedRepl); i++ {
		ch := sedRepl[i]
		switch ch {
		case '\\':
			if i+1 < len(sedRepl) {
				next := sedRepl[i+1]
				if next >= '1' && next <= '9' {
					b.WriteByte('$')
					b.WriteByte(next)
					i++
				} else if next == 'n' {
					b.WriteByte('\n')
					i++
				} else if next == 't' {
					b.WriteByte('\t')
					i++
				} else if next == '\\' {
					b.WriteByte('\\')
					i++
				} else if next == '&' {
					b.WriteByte('&')
					i++
				} else {
					b.WriteByte(next)
					i++
				}
			} else {
				b.WriteByte('\\')
			}
		case '&':
			b.WriteString("${0}")
		case '$':
			b.WriteString("$$")
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}

// sedExecTransliterate performs y/src/dst/ transliteration.
func sedExecTransliterate(input, from, to string) string {
	fromRunes := []rune(from)
	toRunes := []rune(to)

	mapping := make(map[rune]rune, len(fromRunes))
	for i, fr := range fromRunes {
		if i < len(toRunes) {
			mapping[fr] = toRunes[i]
		}
	}

	var b strings.Builder
	b.Grow(len(input))
	for _, r := range input {
		if rep, ok := mapping[r]; ok {
			b.WriteRune(rep)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sedVisual creates a visual dump of a string (for the 'l' command).
func sedVisual(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '\a':
			b.WriteString("\\a")
		case '\b':
			b.WriteString("\\b")
		case '\f':
			b.WriteString("\\f")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		case '\v':
			b.WriteString("\\v")
		default:
			if r < 32 || r == 127 {
				fmt.Fprintf(&b, "\\%03o", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// sedReadFile reads a file for the 'r' command with a size limit.
func (r *Runner) sedReadFile(ctx context.Context, path string) (string, error) {
	absPath := r.absPath(path)
	f, err := r.open(ctx, absPath, os.O_RDONLY, 0, false)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxOutputBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxOutputBytes {
		return "", fmt.Errorf("file too large (>1MB)")
	}
	s := strings.TrimSuffix(string(data), "\n")
	return s, nil
}

// sedReadFirstLine reads the first line of a file for the 'R' command.
func (r *Runner) sedReadFirstLine(ctx context.Context, path string) (string, error) {
	absPath := r.absPath(path)
	f, err := r.open(ctx, absPath, os.O_RDONLY, 0, false)
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	return "", scanner.Err()
}

// --- Compiler: parse sed script text into sedCommand list ---

// sedCompile parses a sed script string into a list of commands.
func sedCompile(script string, extendedRE bool) ([]*sedCommand, error) {
	p := &sedParser{
		script:     script,
		pos:        0,
		extendedRE: extendedRE,
	}
	return p.parseCommands(false)
}

type sedParser struct {
	script     string
	pos        int
	extendedRE bool
}

func (p *sedParser) peek() byte {
	if p.pos >= len(p.script) {
		return 0
	}
	return p.script[p.pos]
}

func (p *sedParser) next() byte {
	if p.pos >= len(p.script) {
		return 0
	}
	b := p.script[p.pos]
	p.pos++
	return b
}

func (p *sedParser) skipSpaces() {
	for p.pos < len(p.script) && (p.script[p.pos] == ' ' || p.script[p.pos] == '\t') {
		p.pos++
	}
}

func (p *sedParser) skipWhitespaceAndSemicolons() {
	for p.pos < len(p.script) {
		c := p.script[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == ';' {
			p.pos++
		} else {
			break
		}
	}
}

// parseCommands parses commands until EOF or closing '}'.
func (p *sedParser) parseCommands(inGroup bool) ([]*sedCommand, error) {
	var commands []*sedCommand

	for {
		p.skipWhitespaceAndSemicolons()
		if p.pos >= len(p.script) {
			if inGroup {
				return nil, fmt.Errorf("unmatched '{'")
			}
			return commands, nil
		}

		if p.peek() == '}' {
			if !inGroup {
				return nil, fmt.Errorf("unexpected '}'")
			}
			p.next() // consume '}'
			return commands, nil
		}

		if p.peek() == '#' {
			for p.pos < len(p.script) && p.script[p.pos] != '\n' {
				p.pos++
			}
			continue
		}

		cmd, err := p.parseOneCommand()
		if err != nil {
			return nil, err
		}
		if cmd != nil {
			commands = append(commands, cmd)
		}
	}
}

// parseOneCommand parses a single sed command (with optional addresses).
func (p *sedParser) parseOneCommand() (*sedCommand, error) {
	cmd := &sedCommand{}

	p.skipSpaces()

	addr1, err := p.parseAddress()
	if err != nil {
		return nil, err
	}
	if addr1 != nil {
		cmd.addr1 = addr1
		p.skipSpaces()

		if p.peek() == ',' {
			p.next()
			p.skipSpaces()
			addr2, err := p.parseAddress()
			if err != nil {
				return nil, err
			}
			if addr2 == nil {
				return nil, fmt.Errorf("expected address after ','")
			}
			cmd.addr2 = addr2
			p.skipSpaces()
		}
	}

	if p.peek() == '!' {
		cmd.negated = true
		p.next()
		p.skipSpaces()
	}

	if p.pos >= len(p.script) {
		return nil, fmt.Errorf("expected command")
	}

	c := p.next()
	cmd.cmd = c

	switch c {
	case 's':
		sub, err := p.parseSubstitution()
		if err != nil {
			return nil, err
		}
		cmd.sub = sub
	case 'y':
		from, to, err := p.parseTransliterate()
		if err != nil {
			return nil, err
		}
		cmd.transFrom = from
		cmd.transTo = to
	case 'a', 'i', 'c':
		text, err := p.parseText()
		if err != nil {
			return nil, err
		}
		cmd.text = text
	case ':':
		label := p.parseLabel()
		if label == "" {
			return nil, fmt.Errorf("missing label for ':'")
		}
		cmd.label = label
	case 'b', 't', 'T':
		p.skipSpaces()
		cmd.label = p.parseLabel()
	case 'r', 'R':
		p.skipSpaces()
		file := p.parsePath()
		if file == "" {
			return nil, fmt.Errorf("missing filename for '%c' command", c)
		}
		cmd.readFile = file
	case 'w':
		return nil, fmt.Errorf("'w' command is not available in safe shell")
	case 'W':
		return nil, fmt.Errorf("'W' command is not available in safe shell")
	case 'e':
		return nil, fmt.Errorf("'e' command is not available in safe shell")
	case 'q', 'Q':
		p.skipSpaces()
		code := 0
		if p.pos < len(p.script) && p.peek() >= '0' && p.peek() <= '9' {
			numStr := ""
			for p.pos < len(p.script) && p.peek() >= '0' && p.peek() <= '9' {
				numStr += string(p.next())
			}
			n, err := strconv.Atoi(numStr)
			if err == nil {
				if n < 0 {
					n = 0
				} else if n > 255 {
					n = 255
				}
				code = n
			}
		}
		cmd.quitCode = code
	case '{':
		children, err := p.parseCommands(true)
		if err != nil {
			return nil, err
		}
		cmd.children = children
	case 'd', 'D', 'p', 'P', 'g', 'G', 'h', 'H', 'x', 'n', 'N', 'l', '=':
		// No arguments
	default:
		return nil, fmt.Errorf("unknown command: '%c'", c)
	}

	return cmd, nil
}

// parseAddress parses a single address: line number, $, /regex/, or addr~step.
func (p *sedParser) parseAddress() (*sedAddress, error) {
	if p.pos >= len(p.script) {
		return nil, nil
	}

	c := p.peek()

	switch {
	case c == '$':
		p.next()
		return &sedAddress{last: true}, nil
	case c == '/' || c == '\\':
		re, err := p.parseRegexAddress()
		if err != nil {
			return nil, err
		}
		return &sedAddress{regex: re}, nil
	case c >= '0' && c <= '9':
		num := p.parseNumber()
		addr := &sedAddress{lineNum: num}
		if p.pos < len(p.script) && p.peek() == '~' {
			p.next()
			step := p.parseNumber()
			if step <= 0 {
				return nil, fmt.Errorf("invalid step in address")
			}
			addr.step = step
		}
		return addr, nil
	default:
		return nil, nil
	}
}

func (p *sedParser) parseNumber() int {
	start := p.pos
	for p.pos < len(p.script) && p.script[p.pos] >= '0' && p.script[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == start {
		return 0
	}
	n, _ := strconv.Atoi(p.script[start:p.pos])
	return n
}

// parseRegexAddress parses /regex/ or \Xregex\X addresses.
func (p *sedParser) parseRegexAddress() (*regexp.Regexp, error) {
	delim := p.next()
	if delim == '\\' {
		if p.pos >= len(p.script) {
			return nil, fmt.Errorf("unterminated address regex")
		}
		delim = p.next()
	}
	return p.parseRegexUntil(delim)
}

func (p *sedParser) parseRegexUntil(delim byte) (*regexp.Regexp, error) {
	var pattern strings.Builder
	for p.pos < len(p.script) {
		c := p.script[p.pos]
		if c == '\\' && p.pos+1 < len(p.script) {
			next := p.script[p.pos+1]
			if next == delim {
				pattern.WriteByte(delim)
				p.pos += 2
				continue
			}
			pattern.WriteByte(c)
			pattern.WriteByte(next)
			p.pos += 2
			continue
		}
		if c == delim {
			p.pos++
			patStr := pattern.String()
			if !p.extendedRE {
				patStr = sedBREtoERE(patStr)
			}
			re, err := regexp.Compile(patStr)
			if err != nil {
				return nil, fmt.Errorf("invalid regex %q: %v", pattern.String(), err)
			}
			return re, nil
		}
		pattern.WriteByte(c)
		p.pos++
	}
	return nil, fmt.Errorf("unterminated regex")
}

// sedBREtoERE converts basic regex to extended regex for Go's regexp engine.
// In BRE: \( \) \{ \} \+ \? are special, while ( ) { } + ? are literal.
// Go's regexp uses ERE syntax, so we need to adjust.
func sedBREtoERE(pattern string) string {
	var result strings.Builder
	result.Grow(len(pattern))
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' && i+1 < len(pattern) {
			next := pattern[i+1]
			switch next {
			case '(', ')', '{', '}', '+', '?', '|':
				result.WriteByte(next)
				i++
			default:
				result.WriteByte('\\')
				result.WriteByte(next)
				i++
			}
		} else {
			switch pattern[i] {
			case '(', ')', '{', '}', '+', '?', '|':
				result.WriteByte('\\')
				result.WriteByte(pattern[i])
			default:
				result.WriteByte(pattern[i])
			}
		}
	}
	return result.String()
}

// parseSubstitution parses s/regex/replacement/flags
func (p *sedParser) parseSubstitution() (*sedSub, error) {
	if p.pos >= len(p.script) {
		return nil, fmt.Errorf("unterminated 's' command")
	}

	delim := p.next()
	if delim == '\\' || delim == '\n' {
		return nil, fmt.Errorf("invalid delimiter for 's' command")
	}

	var patternBuf strings.Builder
	for p.pos < len(p.script) {
		c := p.script[p.pos]
		if c == '\\' && p.pos+1 < len(p.script) {
			next := p.script[p.pos+1]
			if next == delim {
				patternBuf.WriteByte(delim)
				p.pos += 2
				continue
			}
			patternBuf.WriteByte(c)
			patternBuf.WriteByte(next)
			p.pos += 2
			continue
		}
		if c == delim {
			p.pos++
			break
		}
		patternBuf.WriteByte(c)
		p.pos++
	}

	var replBuf strings.Builder
	for p.pos < len(p.script) {
		c := p.script[p.pos]
		if c == '\\' && p.pos+1 < len(p.script) {
			next := p.script[p.pos+1]
			if next == delim {
				replBuf.WriteByte(delim)
				p.pos += 2
				continue
			}
			replBuf.WriteByte(c)
			replBuf.WriteByte(next)
			p.pos += 2
			continue
		}
		if c == delim {
			p.pos++
			break
		}
		replBuf.WriteByte(c)
		p.pos++
	}

	sub := &sedSub{
		replace: replBuf.String(),
	}

	icase := false
	for p.pos < len(p.script) {
		c := p.peek()
		if c == ';' || c == '\n' || c == '}' || c == ' ' || c == '\t' {
			break
		}
		p.next()
		switch c {
		case 'g':
			sub.global = true
		case 'p':
			sub.print = true
		case 'i', 'I':
			icase = true
		case 'w':
			return nil, fmt.Errorf("'w' flag in substitution is not available in safe shell")
		case 'e':
			return nil, fmt.Errorf("'e' flag in substitution is not available in safe shell")
		default:
			if c >= '1' && c <= '9' {
				numStr := string(c)
				for p.pos < len(p.script) && p.peek() >= '0' && p.peek() <= '9' {
					numStr += string(p.next())
				}
				n, err := strconv.Atoi(numStr)
				if err == nil && n > 0 {
					sub.nth = n
				}
			}
		}
	}

	sub.icase = icase
	patStr := patternBuf.String()
	if !p.extendedRE {
		patStr = sedBREtoERE(patStr)
	}
	if icase {
		patStr = "(?i)" + patStr
	}

	re, err := regexp.Compile(patStr)
	if err != nil {
		return nil, fmt.Errorf("invalid regex in substitution: %v", err)
	}
	sub.regex = re

	return sub, nil
}

// parseTransliterate parses y/src/dst/
func (p *sedParser) parseTransliterate() (string, string, error) {
	if p.pos >= len(p.script) {
		return "", "", fmt.Errorf("unterminated 'y' command")
	}

	delim := p.next()

	from, err := p.readUntilDelim(delim)
	if err != nil {
		return "", "", fmt.Errorf("unterminated 'y' command")
	}

	to, err := p.readUntilDelim(delim)
	if err != nil {
		return "", "", fmt.Errorf("unterminated 'y' command")
	}

	fromRunes := utf8.RuneCountInString(from)
	toRunes := utf8.RuneCountInString(to)
	if fromRunes != toRunes {
		return "", "", fmt.Errorf("'y' command strings have different lengths")
	}

	return from, to, nil
}

func (p *sedParser) readUntilDelim(delim byte) (string, error) {
	var buf strings.Builder
	for p.pos < len(p.script) {
		c := p.script[p.pos]
		if c == '\\' && p.pos+1 < len(p.script) {
			next := p.script[p.pos+1]
			if next == delim {
				buf.WriteByte(delim)
				p.pos += 2
				continue
			}
			if next == 'n' {
				buf.WriteByte('\n')
				p.pos += 2
				continue
			}
			buf.WriteByte(c)
			buf.WriteByte(next)
			p.pos += 2
			continue
		}
		if c == delim {
			p.pos++
			return buf.String(), nil
		}
		buf.WriteByte(c)
		p.pos++
	}
	return "", fmt.Errorf("unterminated delimiter")
}

// parseText parses text for a/i/c commands.
// Supports "a\ text" and "a text" syntax.
func (p *sedParser) parseText() (string, error) {
	if p.pos < len(p.script) && p.peek() == '\\' {
		p.next()
	}
	if p.pos < len(p.script) && p.peek() == '\n' {
		p.next()
	} else {
		p.skipSpaces()
	}

	var buf strings.Builder
	first := true
	trailingBackslash := false
	for p.pos < len(p.script) {
		c := p.script[p.pos]
		if first && (c == '\n' || c == ';') {
			break
		}
		first = false
		if c == '\n' {
			if trailingBackslash {
				// Continuation: replace trailing backslash with newline
				s := buf.String()
				buf.Reset()
				buf.WriteString(s[:len(s)-1])
				buf.WriteByte('\n')
				trailingBackslash = false
				p.pos++
				continue
			}
			break
		}
		trailingBackslash = (c == '\\')
		buf.WriteByte(c)
		p.pos++
	}
	return buf.String(), nil
}

func (p *sedParser) parseLabel() string {
	p.skipSpaces()
	start := p.pos
	for p.pos < len(p.script) {
		c := p.script[p.pos]
		if c == ';' || c == '\n' || c == '}' || c == ' ' || c == '\t' {
			break
		}
		p.pos++
	}
	return p.script[start:p.pos]
}

func (p *sedParser) parsePath() string {
	start := p.pos
	for p.pos < len(p.script) {
		c := p.script[p.pos]
		if c == ';' || c == '\n' || c == '}' {
			break
		}
		p.pos++
	}
	return strings.TrimSpace(p.script[start:p.pos])
}
