// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package awk implements a restricted, POSIX-oriented awk interpreter.
package awk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the awk builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "awk",
	Description: "pattern scanning and text processing",
	MakeFlags:   registerFlags,
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}
func (s *stringList) Type() string { return "string" }

type orderedOptionKind int

const (
	orderedOptionFieldSeparator orderedOptionKind = iota
	orderedOptionAssignment
)

type orderedOption struct {
	kind  orderedOptionKind
	value string
}

type fieldSeparatorOption struct {
	options *[]orderedOption
	value   string
}

func (f *fieldSeparatorOption) String() string { return f.value }
func (f *fieldSeparatorOption) Set(v string) error {
	f.value = v
	*f.options = append(*f.options, orderedOption{kind: orderedOptionFieldSeparator, value: v})
	return nil
}
func (f *fieldSeparatorOption) Type() string { return "string" }

type assignmentOption struct {
	options *[]orderedOption
	values  []string
}

func (a *assignmentOption) String() string { return strings.Join(a.values, ",") }
func (a *assignmentOption) Set(v string) error {
	a.values = append(a.values, v)
	*a.options = append(*a.options, orderedOption{kind: orderedOptionAssignment, value: v})
	return nil
}
func (a *assignmentOption) Type() string { return "string" }

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	fs.SetInterspersed(false)
	help := fs.BoolP("help", "h", false, "print usage and exit")
	var orderedOptions []orderedOption
	fieldSep := fieldSeparatorOption{options: &orderedOptions}
	fs.VarP(&fieldSep, "field-separator", "F", "use an input field separator regular expression")
	var programFiles stringList
	fs.VarP(&programFiles, "file", "f", "read awk program from file")
	assignments := assignmentOption{options: &orderedOptions}
	fs.VarP(&assignments, "assign", "v", "assign awk variable before execution")

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *help {
			printHelp(callCtx, fs)
			return builtins.Result{}
		}
		for _, opt := range orderedOptions {
			if opt.kind != orderedOptionAssignment {
				continue
			}
			name, _, ok := strings.Cut(opt.value, "=")
			if !ok {
				callCtx.Errf("awk: invalid -v assignment %q\n", opt.value)
				return builtins.Result{Code: 1}
			}
			if !validCommandLineAssignmentName(name, nil) {
				callCtx.Errf("awk: invalid -v assignment %q\n", opt.value)
				return builtins.Result{Code: 2}
			}
		}
		programText, files, err := loadProgram(ctx, callCtx, args, programFiles)
		if err != nil {
			callCtx.Errf("awk: %v\n", err)
			code := uint8(1)
			if isFatalError(err) {
				code = 2
			}
			return builtins.Result{Code: code}
		}
		prog, err := parseProgramContext(ctx, programText)
		if err != nil {
			callCtx.Errf("awk: %v\n", err)
			return builtins.Result{Code: 1}
		}
		rt := newRuntime(callCtx, prog)
		for _, opt := range orderedOptions {
			name := "FS"
			value := opt.value
			if opt.kind == orderedOptionAssignment {
				name, value, _ = strings.Cut(opt.value, "=")
				if !validCommandLineAssignmentName(name, prog) {
					callCtx.Errf("awk: invalid -v assignment %q\n", opt.value)
					return builtins.Result{Code: 1}
				}
			}
			if err := rt.setVar(name, inputStringValue(DecodeAwkEscapes(value))); err != nil {
				callCtx.Errf("awk: %v\n", err)
				code := uint8(1)
				if isFatalError(err) {
					code = 2
				}
				return builtins.Result{Code: code}
			}
		}
		return rt.run(ctx, files)
	}
}

func printHelp(callCtx *builtins.CallContext, fs *builtins.FlagSet) {
	callCtx.Out("Usage: awk [OPTION]... 'program' [FILE]...\n")
	callCtx.Out("Pattern scanning and text processing.\n")
	callCtx.Out("This is a practical rshell awk profile, not a full GNU awk clone.\n")
	callCtx.Out("With no FILE, or when FILE is -, read standard input.\n\n")

	callCtx.Out("Supported profile:\n")
	callCtx.Out("  - Inline programs, -f program files, -F separators, -v assignments, FILE args, and - for stdin.\n")
	callCtx.Out("  - BEGIN/main/END rules; regex, comparison, boolean, and range patterns.\n")
	callCtx.Out("  - Fields and records: $0, $1..$NF, NF, NR, FNR, FILENAME, FS, RS, OFS, ORS, OFMT, CONVFMT, SUBSEP, RSTART, RLENGTH.\n")
	callCtx.Out("  - Scalars, associative arrays, composite keys, ENVIRON, arithmetic, comparisons, POSIX-oriented regex matching, ternary, and string concatenation.\n")
	callCtx.Out("  - if/else, for, for-in, while, break, continue, next, exit, and user-defined functions with return.\n")
	callCtx.Out("  - Evaluated expression nodes have a 4,194,304-operation limit per awk run. Main-input records, per-record rule evaluations, executed statements, explicit loop iterations, and user-function calls each have a 1,048,576-operation limit; function depth is capped at 256.\n")
	callCtx.Out("  - Evaluated strings and regex cache misses share a 67,108,864-unit aggregate work limit per awk run.\n")
	callCtx.Out("  - Stdout is capped at 10,485,760 bytes per awk run.\n")
	callCtx.Out("  - Substitution calls retain at most 32,768 aggregate match indices.\n")
	callCtx.Out("  - print, printf, sprintf, length, substr, index, tolower, toupper, int, split, sub, gsub, match, and delete.\n\n")

	callCtx.Out("Not supported:\n")
	callCtx.Out("  - system(), getline in every form, close(), command input pipes, and print/printf output pipes. awk programs cannot execute commands.\n")
	callCtx.Out("  - print/printf file output redirection, such as print x > \"file\" or printf ... >> \"file\".\n")
	callCtx.Out("  - GNU-only builtins and variables including gensub, asort/asorti, strtonum, patsplit, match's third capture-array argument, and IGNORECASE.\n")
	callCtx.Out("  - GNU regex boundary extensions (\\y, \\B, \\<, \\>), malformed-UTF-8 byte matching, and nondecimal source literals.\n")
	callCtx.Out("  - ARGV/ARGC mutation, BEGINFILE/ENDFILE, nextfile, do/while, switch, include/load, namespaces, and indirect function calls.\n")
	callCtx.Out("  - GNU awk CSV mode, FIELDWIDTHS, FPAT, PROCINFO, SYMTAB, FUNCTAB, typed regexps, and extension loading.\n")
	callCtx.Out("  - Exact cross-implementation printf/numeric edge compatibility, including NaN/infinity spellings and uncommon flag combinations.\n")
	callCtx.Out("  - Math/time/random helpers, bitwise functions, typeof, and i18n functions.\n\n")

	fs.SetOutput(callCtx.Stdout)
	fs.PrintDefaults()
}

func loadProgram(ctx context.Context, callCtx *builtins.CallContext, args []string, programFiles []string) (string, []string, error) {
	var parts []string
	var files []string
	total := 0
	if len(programFiles) > 0 {
		if len(programFiles) > MaxProgramFiles {
			return "", nil, fmt.Errorf("too many program files (maximum %d)", MaxProgramFiles)
		}
		total = len(programFiles) - 1 // strings.Join inserts one newline between each file.
		for _, path := range programFiles {
			text, err := readProgramFile(ctx, callCtx, path, &total)
			if err != nil {
				return "", nil, err
			}
			parts = append(parts, text)
		}
		files = args
	} else {
		if len(args) == 0 {
			return "", nil, errors.New("missing program")
		}
		total += len(args[0])
		if total > MaxProgramBytes {
			return "", nil, fmt.Errorf("program exceeds %d bytes", MaxProgramBytes)
		}
		parts = append(parts, args[0])
		files = args[1:]
	}
	if len(parts) == 0 {
		return "", nil, errors.New("missing program")
	}
	return strings.Join(parts, "\n"), files, nil
}

func readProgramFile(ctx context.Context, callCtx *builtins.CallContext, path string, total *int) (string, error) {
	if path == "-" {
		if callCtx.Stdin == nil {
			return "", nil
		}
		return readProgramCancellable(ctx, callCtx.Stdin, total)
	}
	rc, err := callCtx.OpenFile(ctx, path, os.O_RDONLY, 0)
	if err != nil {
		message := err.Error()
		if callCtx.PortableErr != nil {
			message = callCtx.PortableErr(err)
		}
		return "", fmt.Errorf("fatal: cannot open source file %q for reading: %s", path, message)
	}
	defer rc.Close()
	return readProgramCancellable(ctx, rc, total)
}

type byteReader interface {
	Read([]byte) (int, error)
}

type programFile interface {
	byteReader
	SetReadDeadline(time.Time) error
}

func readProgramCancellable(ctx context.Context, r byteReader, total *int) (string, error) {
	if ctx.Done() == nil {
		return readProgram(ctx, r, total)
	}
	if file, ok := r.(programFile); ok {
		if text, handled, err := readProgramFileFallback(ctx, file, total); handled {
			return text, err
		}
	}
	closer, ok := r.(interface{ Close() error })
	if !ok {
		// A generic reader cannot be interrupted; keep the caller cancellable
		// while allowing at most one in-flight Read to outlive the invocation.
		type readResult struct {
			text  string
			total int
			err   error
		}
		result := make(chan readResult, 1)
		readTotal := *total
		go func() {
			text, err := readProgram(ctx, r, &readTotal)
			result <- readResult{text: text, total: readTotal, err: err}
		}()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case res := <-result:
			if err := ctx.Err(); err != nil {
				return "", err
			}
			*total = res.total
			return res.text, res.err
		}
	}
	type deadlineSetter interface {
		SetReadDeadline(time.Time) error
	}
	setter, hasDeadline := r.(deadlineSetter)
	stop := make(chan struct{})
	watchDone := make(chan bool, 1)
	go func() {
		select {
		case <-ctx.Done():
			if hasDeadline && setter.SetReadDeadline(time.Unix(1, 0)) == nil {
				watchDone <- true
				return
			}
			// Closing a reader that cannot use deadlines is the only way to
			// interrupt Read without abandoning a blocked goroutine.
			_ = closer.Close()
			watchDone <- false
		case <-stop:
			watchDone <- false
		}
	}()
	readTotal := *total
	text, err := readProgram(ctx, r, &readTotal)
	close(stop)
	if restoreDeadline := <-watchDone; restoreDeadline {
		_ = setter.SetReadDeadline(time.Time{})
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	*total = readTotal
	return text, err
}

func readProgram(ctx context.Context, r byteReader, total *int) (string, error) {
	var b strings.Builder
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := r.Read(buf)
		if n > 0 {
			*total += n
			if *total > MaxProgramBytes {
				return "", fmt.Errorf("program exceeds %d bytes", MaxProgramBytes)
			}
			b.WriteString(string(buf[:n]))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

func validVarName(name string) bool {
	return validIdentifierName(name) && !isSpecialPatternName(name)
}

func validIdentifierName(name string) bool {
	if name == "" || !isIdentStart(rune(name[0])) {
		return false
	}
	for _, ch := range name[1:] {
		if !isIdentPart(ch) {
			return false
		}
	}
	return true
}

func isSpecialPatternName(name string) bool {
	return name == "BEGIN" || name == "END"
}
