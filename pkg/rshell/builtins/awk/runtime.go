// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awk

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"regexp/syntax"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

const (
	MaxProgramBytes          = 256 << 10
	MaxProgramFiles          = 64
	MaxRecordBytes           = 1 << 20
	MaxFields                = 16_384
	MaxVariableBytes         = 1 << 20
	MaxRegexBytes            = 64 << 10
	MaxExpressionBytes       = 5 << 20
	MaxStdoutBytes           = 10 << 20
	maxStatementExecutions   = 1 << 20
	maxLoopIterations        = 1 << 20
	maxFunctionCalls         = 1 << 20
	maxInputRecords          = 1 << 20
	maxMainRuleEvaluations   = 1 << 20
	maxExpressionEvaluations = 1 << 22
	maxStringProcessingBytes = 64 * MaxVariableBytes
	maxFileOpenAttempts      = 1 << 10
	minRegexCompileWork      = 1 << 10
	maxInputBytes            = maxStringProcessingBytes
	maxRegexCacheEntries     = 64
	maxRegexCacheBytes       = MaxProgramBytes
	maxFunctionDepth         = 256
	recordCounterLimit       = 1 << 63
)

var (
	errTooManyFields      = errors.New("too many fields")
	errInputBytesExceeded = errors.New("input byte limit exceeded")
)

type valueKind int

const (
	valueString valueKind = iota
	valueNumber
	valueStrNum
)

type value struct {
	kind valueKind
	s    string
	n    float64
}

func stringValue(s string) value {
	return value{kind: valueString, s: s}
}

func inputStringValue(s string) value {
	if n, ok := parseFullNumericString(s); ok {
		return value{kind: valueStrNum, s: s, n: n}
	}
	return value{kind: valueString, s: s}
}

func unassignedValue() value {
	return value{kind: valueStrNum, s: "", n: 0}
}

func numberValue(n float64) value {
	return value{kind: valueNumber, n: n}
}

func (v value) String() string {
	switch v.kind {
	case valueNumber:
		return formatAwkNumber(v.n)
	default:
		return v.s
	}
}

func formatAwkNumber(n float64) string {
	formatted, _ := formatAwkNumberWithFormat(n, "%.6g")
	return formatted
}

func formatAwkNumberWithFormat(n float64, format string) (string, error) {
	if n == 0 {
		n = 0
	}
	fixed := strconv.FormatFloat(n, 'f', -1, 64)
	if !strings.ContainsRune(fixed, '.') {
		return strconv.FormatFloat(n, 'f', 0, 64), nil
	}
	return formatPrintf(format, []value{numberValue(n)})
}

func (v value) Number() float64 {
	switch v.kind {
	case valueNumber, valueStrNum:
		return v.n
	default:
		if n, ok := parseNumericPrefix(v.s); ok {
			return n
		}
	}
	return 0
}

func (v value) Bool() bool {
	switch v.kind {
	case valueNumber:
		return v.n != 0
	case valueStrNum:
		return v.n != 0
	default:
		return v.s != ""
	}
}

func cloneStoredString(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	b.WriteString(s)
	return b.String()
}

func cloneStoredValue(v value) value {
	if v.kind != valueNumber {
		v.s = cloneStoredString(v.s)
	}
	return v
}

func parseFullNumericString(s string) (float64, bool) {
	trimmed := strings.Trim(s, " \t\n\r\f\v")
	if trimmed == "" || numericPrefix(trimmed) != trimmed {
		return 0, false
	}
	return parseAwkFloat(trimmed)
}

func parseNumericPrefix(s string) (float64, bool) {
	prefix := numericPrefix(trimLeadingAwkSpace(s))
	if prefix == "" {
		return 0, false
	}
	return parseAwkFloat(prefix)
}

func recordCounterValue(v value) value {
	if v.kind == valueStrNum && v.s == "" {
		return v
	}
	n := v.n
	if v.kind == valueString {
		var ok bool
		n, ok = parseNumericPrefix(v.s)
		if !ok {
			return v
		}
	}
	switch {
	case math.IsNaN(n):
		n = 0
	case n >= recordCounterLimit:
		n = recordCounterLimit
	case n <= -recordCounterLimit:
		n = -recordCounterLimit
	default:
		n = math.Trunc(n)
	}
	return numberValue(n)
}

func incrementRecordCounter(v value) value {
	n := v.Number()
	if n >= recordCounterLimit {
		n = -recordCounterLimit
	} else {
		n++
	}
	return numberValue(n)
}

func parseAwkFloat(s string) (float64, bool) {
	n, err := strconv.ParseFloat(s, 64)
	return n, err == nil || errors.Is(err, strconv.ErrRange)
}

func trimLeadingAwkSpace(s string) string {
	for len(s) > 0 {
		switch s[0] {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			s = s[1:]
		default:
			return s
		}
	}
	return s
}

func numericPrefix(s string) string {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return ""
	}
	end := i
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		expDigits := 0
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
			expDigits++
		}
		if expDigits > 0 {
			end = j
		}
	}
	return s[:end]
}

type runtime struct {
	callCtx          *builtins.CallContext
	prog             *program
	vars             map[string]value
	arrays           map[string]map[string]value
	varSizes         map[string]int
	arraySizes       map[arraySlot]int
	varBytes         int
	callArgBytes     int
	exprTempBytes    int
	rangeOn          map[int]bool
	environSet       bool
	environErr       error
	functionDepth    int
	functionCalls    int
	stmtExecutions   int
	loopIterations   int
	inputRecords     int
	inputBytes       int
	mainRuleEvals    int
	exprEvaluations  int
	stringWorkBytes  int
	regexCache       map[regexCacheKey]*awkRegex
	regexCacheOrder  []regexCacheKey
	regexCacheBytes  int
	frames           []callFrame
	ctx              context.Context
	stdoutBytes      int
	inputArgs        []string
	inputIndex       int
	mainInput        *recordSource
	mainHadInput     bool
	mainDefaultStdin bool
	fileOpenAttempts int

	record   string
	fields   []string
	filename value
	nf       value
	nr       value
	fnr      value
	exitCode int
}

type arraySlot struct {
	name string
	key  string
}

type regexCacheKey struct {
	pattern string
}

type callFrame struct {
	locals map[string]*localVar
}

type recordSource struct {
	name           string
	rc             io.ReadCloser
	sc             *bufio.Scanner
	rt             *runtime
	rs             string
	recordAdvance  int
	recordBuffered int
	closeOnce      sync.Once
	asyncRead      bool
	interruptRead  func() bool
	restoreRead    func()
}

type localVar struct {
	value           value
	valueSize       int
	valueSet        bool
	array           map[string]value
	arraySizes      map[string]int
	arrayAlias      *localVar
	globalArrayName string
}

func newRuntime(callCtx *builtins.CallContext, prog *program) *runtime {
	rt := &runtime{
		callCtx:    callCtx,
		prog:       prog,
		filename:   stringValue(""),
		nf:         numberValue(0),
		nr:         numberValue(0),
		fnr:        numberValue(0),
		regexCache: make(map[regexCacheKey]*awkRegex),
		vars:       make(map[string]value),
		arrays:     make(map[string]map[string]value),
		varSizes:   make(map[string]int),
		arraySizes: make(map[arraySlot]int),
		rangeOn:    make(map[int]bool),
	}
	rt.vars["FS"] = stringValue(" ")
	rt.vars["RS"] = stringValue("\n")
	rt.vars["OFS"] = stringValue(" ")
	rt.vars["ORS"] = stringValue("\n")
	rt.vars["OFMT"] = stringValue("%.6g")
	rt.vars["CONVFMT"] = stringValue("%.6g")
	rt.vars["SUBSEP"] = stringValue("\034")
	rt.vars["RSTART"] = numberValue(0)
	rt.vars["RLENGTH"] = numberValue(0)
	return rt
}

func (rt *runtime) run(ctx context.Context, files []string) builtins.Result {
	prevCtx := rt.ctx
	rt.ctx = ctx
	defer func() { rt.ctx = prevCtx }()
	rt.inputArgs = append([]string{}, files...)
	defer rt.closeAllInputs()
	exited := false
	if err := rt.runRules(ctx, ruleBegin); err != nil {
		if code, ok := exitCodeFromError(err); ok {
			rt.exitCode = code
			exited = true
		} else {
			return rt.errorResult(ctx, err)
		}
	}
	if !exited && rt.needsInput() {
		for {
			rec, ok, err := rt.readMainRecord(ctx)
			if err != nil {
				if code, ok := exitCodeFromError(err); ok {
					rt.exitCode = code
					break
				}
				return rt.errorResult(ctx, err)
			}
			if !ok {
				break
			}
			if err := rt.setRecord(rec); err != nil {
				return rt.errorResult(ctx, err)
			}
			if err := rt.runRules(ctx, ruleNormal); err != nil {
				if errors.Is(err, errNextRecord) {
					continue
				}
				if code, ok := exitCodeFromError(err); ok {
					rt.exitCode = code
					break
				}
				return rt.errorResult(ctx, err)
			}
		}
	}
	if err := rt.runRules(ctx, ruleEnd); err != nil {
		if code, ok := exitCodeFromError(err); ok {
			rt.exitCode = code
		} else {
			return rt.errorResult(ctx, err)
		}
	}
	return builtins.Result{Code: normalizeAwkExitCode(rt.exitCode)}
}

func (rt *runtime) errorResult(_ context.Context, err error) builtins.Result {
	rt.callCtx.Errf("awk: %v\n", err)
	code := uint8(1)
	if isFatalError(err) {
		code = 2
	}
	return builtins.Result{Code: code}
}

func isFatalError(err error) bool {
	const prefix = "fatal: "
	msg := err.Error()
	return len(msg) >= len(prefix) && msg[:len(prefix)] == prefix
}

func exitCodeFromError(err error) (int, bool) {
	exit, ok := err.(*exitError)
	if ok {
		return exit.code, true
	}
	return 0, false
}

func normalizeAwkExitCode(code int) uint8 {
	code %= 256
	if code < 0 {
		code += 256
	}
	return uint8(code)
}

func (rt *runtime) ensureEnviron() error {
	if rt.environSet {
		return rt.environErr
	}
	rt.environSet = true
	rt.arrays["ENVIRON"] = make(map[string]value)
	if rt.ctx != nil {
		if err := rt.ctx.Err(); err != nil {
			rt.environErr = err
			return err
		}
	}
	if rt.callCtx.Env != nil {
		rt.callCtx.Env(func(name, value string) bool {
			if rt.environErr != nil {
				return false
			}
			if rt.ctx != nil {
				if err := rt.ctx.Err(); err != nil {
					rt.environErr = err
					return false
				}
			}
			if err := rt.setGlobalArrayElem("ENVIRON", name, inputStringValue(value)); err != nil {
				rt.environErr = err
				return false
			}
			return true
		})
	}
	if rt.environErr == nil && rt.ctx != nil {
		rt.environErr = rt.ctx.Err()
	}
	return rt.environErr
}

func (rt *runtime) applyOperandAssignment(arg string) (bool, error) {
	name, value, ok := strings.Cut(arg, "=")
	if !ok || !validIdentifierName(name) {
		return false, nil
	}
	if !validCommandLineAssignmentName(name, rt.prog) {
		return true, fmt.Errorf("fatal: invalid variable assignment %q", arg)
	}
	if err := rt.setVar(name, inputStringValue(DecodeAwkEscapes(value))); err != nil {
		return true, err
	}
	return true, nil
}

func (rt *runtime) needsInput() bool {
	for _, r := range rt.prog.rules {
		if r.kind == ruleNormal || r.kind == ruleEnd {
			return true
		}
	}
	return false
}

func (rt *runtime) readMainRecord(ctx context.Context) (string, bool, error) {
	for {
		if rt.mainInput == nil {
			ok, err := rt.openNextMainInput(ctx)
			if err != nil || !ok {
				return "", false, err
			}
		}
		rec, ok, err := rt.mainInput.readRecord(ctx)
		if err != nil {
			return "", false, fmt.Errorf("%s: %w", rt.mainInput.name, err)
		}
		if ok {
			if rt.inputRecords >= maxInputRecords {
				return "", false, fmt.Errorf("input record limit exceeded (maximum %d)", maxInputRecords)
			}
			rt.inputRecords++
			if err := rt.setVar("NR", incrementRecordCounter(rt.nr)); err != nil {
				return "", false, err
			}
			if err := rt.setVar("FNR", incrementRecordCounter(rt.fnr)); err != nil {
				return "", false, err
			}
			return rec, true, nil
		}
		rt.mainInput.close()
		rt.mainInput = nil
	}
}

func (rt *runtime) openNextMainInput(ctx context.Context) (bool, error) {
	for rt.inputIndex < len(rt.inputArgs) {
		arg := rt.inputArgs[rt.inputIndex]
		rt.inputIndex++
		assigned, err := rt.applyOperandAssignment(arg)
		if err != nil {
			return false, err
		}
		if assigned {
			continue
		}
		return rt.openMainInput(ctx, arg)
	}
	if !rt.mainHadInput && !rt.mainDefaultStdin {
		rt.mainDefaultStdin = true
		return rt.openMainInput(ctx, "-")
	}
	return false, nil
}

func (rt *runtime) openMainInput(ctx context.Context, file string) (bool, error) {
	if err := rt.chargeFileOpenAttempt(file); err != nil {
		return false, err
	}
	src, err := rt.openRecordSource(ctx, file)
	if err != nil {
		return false, fmt.Errorf("fatal: cannot open file `%s' for reading: %v", file, err)
	}
	rt.mainHadInput = true
	if err := rt.setVar("FILENAME", stringValue(file)); err != nil {
		src.close()
		return false, err
	}
	if !rt.mainDefaultStdin {
		if err := rt.setVar("FNR", numberValue(0)); err != nil {
			src.close()
			return false, err
		}
	}
	rt.mainInput = src
	return true, nil
}

// newRecordSource keeps at most one read in flight so context cancellation can
// return even when the underlying reader is blocked. As with the read builtin's
// fallback path, a reader that cannot be interrupted may retain that one
// goroutine until it eventually returns; no speculative record is started.
func (rt *runtime) newRecordSource(name string, rc io.ReadCloser) *recordSource {
	src := rt.newRecordSourceBase(name, rc)
	src.asyncRead = true
	return src
}

func (rt *runtime) newBufferedRecordSource(name string, rc io.ReadCloser) *recordSource {
	return rt.newRecordSourceBase(name, rc)
}

func (rt *runtime) newRecordSourceBase(name string, rc io.ReadCloser) *recordSource {
	src := &recordSource{name: name, rc: rc, rt: rt}
	sc := bufio.NewScanner(rc)
	sc.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if len(data) > src.recordBuffered {
			src.recordBuffered = len(data)
		}
		advance, token, err := src.splitRecord(data, atEOF)
		if err != nil {
			return advance, token, err
		}
		pending := advance
		if advance == 0 && token == nil {
			pending = len(data)
		}
		remaining := maxInputBytes - src.rt.inputBytes
		if src.recordAdvance > remaining || pending > remaining-src.recordAdvance {
			return 0, nil, errInputBytesExceeded
		}
		if advance > 0 {
			src.recordAdvance += advance
			if token == nil {
				src.recordBuffered = 0
			}
		}
		return advance, token, err
	})
	sc.Buffer(make([]byte, 4096), MaxRecordBytes+utf8.UTFMax)
	src.sc = sc
	return src
}

func (src *recordSource) recordSeparator() string {
	return src.rt.getVar("RS").String()
}

func (src *recordSource) readRecord(ctx context.Context) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	src.rs = src.recordSeparator()
	if src.asyncRead && ctx.Done() != nil {
		result := make(chan recordReadResult, 1)
		go func() {
			result <- src.scanRecord()
		}()
		select {
		case scanned := <-result:
			if err := ctx.Err(); err != nil {
				return "", false, err
			}
			return src.finishRecordRead(scanned)
		case <-ctx.Done():
			restore := false
			if src.interruptRead != nil {
				restore = src.interruptRead()
			}
			src.closeAsync()
			if restore && src.restoreRead != nil {
				go func() {
					<-result
					src.restoreRead()
				}()
			}
			return "", false, ctx.Err()
		}
	}
	scanned := src.scanRecord()
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return src.finishRecordRead(scanned)
}

type recordReadResult struct {
	record  string
	advance int
	ok      bool
	err     error
}

func (src *recordSource) scanRecord() recordReadResult {
	src.recordAdvance = 0
	src.recordBuffered = 0
	if !src.sc.Scan() {
		return recordReadResult{advance: src.recordAdvance + src.recordBuffered, err: src.sc.Err()}
	}
	rec := src.sc.Text()
	if len(rec) > MaxRecordBytes {
		return recordReadResult{advance: src.recordAdvance, err: fmt.Errorf("record exceeds %d bytes", MaxRecordBytes)}
	}
	return recordReadResult{record: rec, advance: src.recordAdvance, ok: true}
}

func (src *recordSource) finishRecordRead(scanned recordReadResult) (string, bool, error) {
	if err := src.rt.chargeInputBytes(scanned.advance); err != nil {
		return "", false, err
	}
	if scanned.err != nil || !scanned.ok {
		return scanned.record, scanned.ok, scanned.err
	}
	return scanned.record, true, nil
}

func (rt *runtime) chargeInputBytes(n int) error {
	if n > maxInputBytes || rt.inputBytes > maxInputBytes-n {
		return fmt.Errorf("%w (maximum %d bytes)", errInputBytesExceeded, maxInputBytes)
	}
	rt.inputBytes += n
	return nil
}

func (src *recordSource) close() {
	if src != nil && src.rc != nil {
		src.closeOnce.Do(func() {
			src.rc.Close() //nolint:errcheck
		})
	}
}

func (src *recordSource) closeAsync() {
	if src != nil && src.rc != nil {
		src.closeOnce.Do(func() {
			go func() {
				src.rc.Close() //nolint:errcheck
			}()
		})
	}
}

func scanAwkRecord(data []byte, atEOF bool, rs string) (int, []byte, error) {
	if err := validateRS(rs); err != nil {
		return 0, nil, err
	}
	sep := []byte(rs)
	if i := bytes.Index(data, sep); i >= 0 {
		return i + len(sep), data[:i], nil
	}
	if atEOF {
		if len(data) == 0 {
			return 0, nil, nil
		}
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (src *recordSource) splitRecord(data []byte, atEOF bool) (int, []byte, error) {
	if src.rs == "" {
		return splitAwkParagraphRecord(data, atEOF)
	}
	return scanAwkRecord(data, atEOF, src.rs)
}

func splitAwkParagraphRecord(data []byte, atEOF bool) (int, []byte, error) {
	leading := 0
	for leading < len(data) && data[leading] == '\n' {
		leading++
	}
	if leading > 0 {
		if !atEOF || leading == len(data) {
			return leading, nil, nil
		}
		data = data[leading:]
	}

	if separator := bytes.Index(data, []byte("\n\n")); separator >= 0 {
		return leading + separator + 2, data[:separator], nil
	}

	if atEOF {
		if len(data) == 0 {
			return 0, nil, nil
		}
		end := len(data)
		if data[end-1] == '\n' {
			end--
		}
		return leading + len(data), data[:end], nil
	}
	return 0, nil, nil
}

func (rt *runtime) openRecordSource(ctx context.Context, file string) (*recordSource, error) {
	if file == "-" {
		if rt.callCtx.Stdin == nil {
			return rt.newBufferedRecordSource(file, io.NopCloser(strings.NewReader(""))), nil
		}
		stdin := rt.callCtx.Stdin
		// Normal cleanup must not close caller-owned stdin; cancellation may.
		src := rt.newRecordSource(file, io.NopCloser(stdin))
		closer, _ := stdin.(interface{ Close() error })
		setter, _ := stdin.(interface {
			SetReadDeadline(time.Time) error
		})
		if setter != nil || closer != nil {
			src.interruptRead = func() bool {
				if setter != nil && setter.SetReadDeadline(time.Unix(1, 0)) == nil {
					return true
				}
				if closer != nil {
					_ = closer.Close()
				}
				return false
			}
		}
		if setter != nil {
			src.restoreRead = func() {
				_ = setter.SetReadDeadline(time.Time{})
			}
		}
		return src, nil
	}
	f, err := rt.openRecordFile(ctx, file)
	if err != nil {
		return nil, err
	}
	return rt.newRecordSource(file, f), nil
}

func (rt *runtime) openRecordFile(ctx context.Context, file string) (io.ReadCloser, error) {
	f, err := rt.callCtx.OpenFile(ctx, file, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (rt *runtime) writeStdoutString(s string) error {
	if err := rt.reserveStdout(len(s)); err != nil {
		return err
	}
	rt.callCtx.Out(s)
	return nil
}

func (rt *runtime) reserveStdout(n int) error {
	if n > MaxStdoutBytes-rt.stdoutBytes {
		return fmt.Errorf("stdout output exceeds %d bytes", MaxStdoutBytes)
	}
	rt.stdoutBytes += n
	return nil
}

func (rt *runtime) chargeFileOpenAttempt(name string) error {
	if name == "-" {
		return nil
	}
	if rt.fileOpenAttempts >= maxFileOpenAttempts {
		return fmt.Errorf("file open attempt limit exceeded (maximum %d)", maxFileOpenAttempts)
	}
	rt.fileOpenAttempts++
	return nil
}

func (rt *runtime) closeAllInputs() {
	if rt.mainInput != nil {
		rt.mainInput.close()
		rt.mainInput = nil
	}
}

func (rt *runtime) chargeMainRuleEvaluation() error {
	if rt.mainRuleEvals >= maxMainRuleEvaluations {
		return fmt.Errorf("main-input rule evaluation limit exceeded (maximum %d)", maxMainRuleEvaluations)
	}
	rt.mainRuleEvals++
	return nil
}

func (rt *runtime) runRules(ctx context.Context, kind ruleKind) error {
	prevCtx := rt.ctx
	rt.ctx = ctx
	defer func() { rt.ctx = prevCtx }()
	for i := range rt.prog.rules {
		r := &rt.prog.rules[i]
		if err := ctx.Err(); err != nil {
			return err
		}
		if r.kind != kind {
			continue
		}
		if kind == ruleNormal {
			if err := rt.chargeMainRuleEvaluation(); err != nil {
				return err
			}
		}
		if kind == ruleNormal && r.pattern != nil {
			ok, err := rt.matchPattern(i, r.pattern)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
		}
		if r.action == nil {
			v := rt.field(0)
			if err := rt.chargeStringValue(v); err != nil {
				return err
			}
			out, err := rt.formatPrintValues([]value{v})
			if err != nil {
				return err
			}
			if err := rt.chargeStringProcessing(len(out)); err != nil {
				return err
			}
			if err := rt.writeStdoutString(out); err != nil {
				return err
			}
			continue
		}
		if err := rt.execStatements(ctx, r.action); err != nil {
			if errors.Is(err, errNextRecord) {
				if kind == ruleNormal {
					return err
				}
				return errors.New("fatal: next is not allowed in BEGIN or END")
			}
			return err
		}
	}
	return nil
}

func (rt *runtime) matchPattern(ruleIndex int, x expr) (bool, error) {
	if rx, ok := x.(*rangeExpr); ok {
		return rt.matchRangePattern(ruleIndex, rx)
	}
	return rt.matchSimplePattern(x)
}

func (rt *runtime) matchRangePattern(ruleIndex int, x *rangeExpr) (bool, error) {
	if rt.rangeOn[ruleIndex] {
		end, err := rt.matchSimplePattern(x.end)
		if err != nil {
			return false, err
		}
		if end {
			rt.rangeOn[ruleIndex] = false
		}
		return true, nil
	}
	start, err := rt.matchSimplePattern(x.start)
	if err != nil {
		return false, err
	}
	if !start {
		return false, nil
	}
	end, err := rt.matchSimplePattern(x.end)
	if err != nil {
		return false, err
	}
	if !end {
		rt.rangeOn[ruleIndex] = true
	}
	return true, nil
}

func (rt *runtime) matchSimplePattern(x expr) (bool, error) {
	v, err := rt.eval(x)
	if err != nil {
		return false, err
	}
	return v.Bool(), nil
}

func (rt *runtime) setRecord(rec string) error {
	if err := validateRecordSize(rec); err != nil {
		return err
	}
	rec = cloneStoredString(rec)
	rt.record = rec
	fs := rt.getVar("FS").String()
	fields, err := rt.splitAwkRecordFields(rec, fs)
	if err != nil {
		if errors.Is(err, errTooManyFields) {
			return errors.New("record has too many fields")
		}
		return err
	}
	for i, field := range fields {
		fields[i] = cloneStoredString(field)
	}
	rt.fields = fields
	if len(rt.fields) > MaxFields {
		return errors.New("record has too many fields")
	}
	rt.setComputedNF(len(rt.fields))
	return nil
}

func validateRecordSize(rec string) error {
	if len(rec) > MaxRecordBytes {
		return fmt.Errorf("record exceeds %d bytes", MaxRecordBytes)
	}
	return nil
}

func validateRebuiltRecordSize(fields []string, fieldCount, replacementIndex int, replacement, ofs string) (int, error) {
	total := 0
	for i := 0; i < fieldCount; i++ {
		if i > 0 {
			total += len(ofs)
			if total > MaxRecordBytes {
				return 0, fmt.Errorf("record exceeds %d bytes", MaxRecordBytes)
			}
		}
		field := ""
		if i < len(fields) {
			field = fields[i]
		}
		if replacementIndex == i+1 {
			field = replacement
		}
		total += len(field)
		if total > MaxRecordBytes {
			return 0, fmt.Errorf("record exceeds %d bytes", MaxRecordBytes)
		}
	}
	return total, nil
}

func (rt *runtime) rebuildRecordFromFields() {
	ofs := rt.getVar("OFS").String()
	rt.record = strings.Join(rt.fields, ofs)
	for i, field := range rt.fields {
		rt.fields[i] = cloneStoredString(field)
	}
}

func (rt *runtime) setField(n int, v value) error {
	if n < 0 {
		return errors.New("invalid field index")
	}
	if n > MaxFields {
		return errors.New("record has too many fields")
	}
	s, err := rt.conversionString(v, "CONVFMT")
	if err != nil {
		return err
	}
	if n == 0 {
		return rt.setRecord(s)
	}
	oldCount := len(rt.fields)
	fieldCount := max(len(rt.fields), n)
	recordSize, err := validateRebuiltRecordSize(rt.fields, fieldCount, n, s, rt.getVar("OFS").String())
	if err != nil {
		return err
	}
	if err := rt.chargeStringProcessing(max(recordSize, fieldCount)); err != nil {
		return err
	}
	for len(rt.fields) < n {
		rt.fields = append(rt.fields, "")
	}
	rt.fields[n-1] = s
	rt.rebuildRecordFromFields()
	if n > oldCount {
		rt.setComputedNF(len(rt.fields))
	}
	return nil
}

func (rt *runtime) setNF(v value) error {
	n := int(v.Number())
	if n < 0 {
		return errors.New("fatal: invalid NF value")
	}
	if n > MaxFields {
		return errors.New("record has too many fields")
	}
	recordSize, err := validateRebuiltRecordSize(rt.fields, n, 0, "", rt.getVar("OFS").String())
	if err != nil {
		return err
	}
	if err := rt.chargeStringProcessing(max(recordSize, n)); err != nil {
		return err
	}
	size := len(v.String())
	if size > MaxVariableBytes {
		return fmt.Errorf("variable value exceeds %d bytes", MaxVariableBytes)
	}
	old := rt.varSizes["NF"]
	if rt.varBytes-old+size > MaxVariableBytes {
		return fmt.Errorf("variable storage limit exceeded (%d bytes total)", rt.varBytes-old+size)
	}
	stored := cloneStoredValue(v)
	if n < len(rt.fields) {
		rt.fields = rt.fields[:n]
	} else {
		for len(rt.fields) < n {
			rt.fields = append(rt.fields, "")
		}
	}
	rt.rebuildRecordFromFields()
	rt.varBytes = rt.varBytes - old + size
	rt.varSizes["NF"] = size
	rt.nf = stored
	return nil
}

func (rt *runtime) setComputedNF(n int) {
	old := rt.varSizes["NF"]
	rt.varBytes -= old
	delete(rt.varSizes, "NF")
	rt.nf = numberValue(float64(n))
}

func (rt *runtime) splitAwkFields(s, fs string) ([]string, error) {
	if fs == " " {
		return splitAwkWhitespaceFields(s)
	}
	if err := rt.validateFS(fs); err != nil {
		return nil, err
	}
	if s == "" {
		return nil, nil
	}
	if isSingleRune(fs) {
		parts := strings.SplitN(s, fs, MaxFields+1)
		if len(parts) > MaxFields {
			return nil, errTooManyFields
		}
		return parts, nil
	}
	return rt.splitAwkRegex(s, fs)
}

func (rt *runtime) splitAwkRecordFields(s, fs string) ([]string, error) {
	if rt.getVar("RS").String() == "" && fs != " " && fs != "\n" && isSingleRune(fs) {
		if s == "" {
			return nil, nil
		}
		return splitAwkParagraphFields(s, fs)
	}
	return rt.splitAwkFields(s, fs)
}

func splitAwkParagraphFields(s, fs string) ([]string, error) {
	fields := make([]string, 0, min(len(s), MaxFields))
	start := 0
	for i := 0; i < len(s); {
		separatorSize := 0
		if s[i] == '\n' {
			separatorSize = 1
		} else if strings.HasPrefix(s[i:], fs) {
			separatorSize = len(fs)
		}
		if separatorSize == 0 {
			i++
			continue
		}
		if len(fields) >= MaxFields {
			return nil, errTooManyFields
		}
		fields = append(fields, s[start:i])
		i += separatorSize
		start = i
	}
	if len(fields) >= MaxFields {
		return nil, errTooManyFields
	}
	return append(fields, s[start:]), nil
}

func splitAwkWhitespaceFields(rec string) ([]string, error) {
	var fields []string
	for i := 0; i < len(rec); {
		for i < len(rec) && isAwkFieldBlank(rec[i]) {
			i++
		}
		start := i
		for i < len(rec) && !isAwkFieldBlank(rec[i]) {
			i++
		}
		if start < i {
			if len(fields) >= MaxFields {
				return nil, errTooManyFields
			}
			fields = append(fields, rec[start:i])
		}
	}
	return fields, nil
}

func isAwkFieldBlank(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n'
}

func splitAwkChars(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	chars := make([]string, 0, min(len(s), MaxFields))
	for len(s) > 0 {
		if len(chars) >= MaxFields {
			return nil, errTooManyFields
		}
		_, size := utf8.DecodeRuneInString(s)
		chars = append(chars, s[:size])
		s = s[size:]
	}
	return chars, nil
}

func (rt *runtime) splitAwkRegex(s, pattern string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	if pattern == "" {
		return splitAwkChars(s)
	}
	re, err := rt.compileRegex(pattern)
	if err != nil {
		return nil, err
	}
	matches, err := re.FindAllStringIndex(s, MaxFields)
	if err != nil {
		return nil, err
	}
	fields := make([]string, 0, min(len(s), MaxFields))
	last := 0
	for i, match := range matches {
		if i%256 == 0 {
			if err := rt.ctx.Err(); err != nil {
				return nil, err
			}
		}
		start, end := match[0], match[1]
		if start == end {
			continue
		}
		if len(fields) >= MaxFields-1 {
			return nil, errTooManyFields
		}
		fields = append(fields, s[last:start])
		last = end
	}
	if len(fields) == 0 {
		return []string{s}, nil
	}
	fields = append(fields, s[last:])
	return fields, nil
}

func (rt *runtime) field(n int) value {
	if n == 0 {
		return inputStringValue(rt.record)
	}
	if n < 0 || n > len(rt.fields) {
		return stringValue("")
	}
	return inputStringValue(rt.fields[n-1])
}

func (rt *runtime) currentFrame() *callFrame {
	if len(rt.frames) == 0 {
		return nil
	}
	return &rt.frames[len(rt.frames)-1]
}

func (rt *runtime) lookupLocal(name string) *localVar {
	frame := rt.currentFrame()
	if frame == nil {
		return nil
	}
	return frame.locals[name]
}

func rootLocalVar(v *localVar) *localVar {
	for v != nil && v.arrayAlias != nil {
		v = v.arrayAlias
	}
	return v
}

func (rt *runtime) localIsArray(v *localVar) bool {
	root := rootLocalVar(v)
	if root == nil {
		return false
	}
	if root.globalArrayName != "" {
		return rt.isGlobalArray(root.globalArrayName) || isBuiltinArrayName(root.globalArrayName)
	}
	return root.array != nil
}

func (rt *runtime) getVar(name string) value {
	if local := rt.lookupLocal(name); local != nil {
		root := rootLocalVar(local)
		if rt.localIsArray(root) {
			return unassignedValue()
		}
		if local.valueSet {
			return local.value
		}
		rt.markLocalScalarRead(local)
		return unassignedValue()
	}
	switch name {
	case "NF":
		return rt.nf
	case "NR":
		return rt.nr
	case "FNR":
		return rt.fnr
	case "FILENAME":
		return rt.filename
	default:
		if v, ok := rt.vars[name]; ok {
			return v
		}
		if isBuiltinArrayName(name) {
			return unassignedValue()
		}
		v := unassignedValue()
		rt.vars[name] = v
		return v
	}
}

func (rt *runtime) conversionString(v value, formatVar string) (string, error) {
	if v.kind != valueNumber {
		return v.s, nil
	}
	s, err := formatAwkNumberWithFormat(v.n, rt.getVar(formatVar).String())
	if err != nil {
		return "", err
	}
	if err := rt.chargeStringProcessing(len(s)); err != nil {
		return "", err
	}
	return s, nil
}

func (rt *runtime) setVar(name string, v value) error {
	if local := rt.lookupLocal(name); local != nil {
		root := rootLocalVar(local)
		if rt.localIsArray(root) {
			return fmt.Errorf("fatal: cannot use array %s as scalar", name)
		}
		return rt.setLocalScalar(local, v)
	}
	if rt.isArray(name) {
		return fmt.Errorf("fatal: cannot use array %s as scalar", name)
	}
	if isBuiltinArrayName(name) {
		return fmt.Errorf("fatal: cannot use array %s as scalar", name)
	}
	switch name {
	case "NF":
		return rt.setNF(v)
	case "NR", "FNR":
		v = recordCounterValue(v)
	case "FS", "RS", "OFS", "ORS", "SUBSEP":
		s, err := rt.conversionString(v, "CONVFMT")
		if err != nil {
			return err
		}
		v = stringValue(s)
	case "OFMT", "CONVFMT":
		if v.kind == valueNumber {
			s, err := rt.conversionString(v, "CONVFMT")
			if err != nil {
				return err
			}
			// Keep the original number while materializing its string facet.
			v = value{kind: valueStrNum, s: s, n: v.n}
		}
	}
	switch name {
	case "FS":
		if err := rt.validateFS(v.String()); err != nil {
			return err
		}
	case "RS":
		if err := validateRS(v.String()); err != nil {
			return err
		}
	}
	size := len(v.String())
	if size > MaxVariableBytes {
		return fmt.Errorf("variable value exceeds %d bytes", MaxVariableBytes)
	}
	old := rt.varSizes[name]
	if rt.varBytes-old+size > MaxVariableBytes {
		return fmt.Errorf("variable storage limit exceeded (%d bytes total)", rt.varBytes-old+size)
	}
	rt.varBytes = rt.varBytes - old + size
	rt.varSizes[name] = size
	stored := cloneStoredValue(v)
	switch name {
	case "NR":
		rt.nr = stored
	case "FNR":
		rt.fnr = stored
	case "FILENAME":
		rt.filename = stored
	default:
		rt.vars[name] = stored
	}
	return nil
}

func (rt *runtime) setLocalScalar(local *localVar, v value) error {
	root := rootLocalVar(local)
	size := len(v.String())
	if size > MaxVariableBytes {
		return fmt.Errorf("variable value exceeds %d bytes", MaxVariableBytes)
	}
	if rt.varBytes-local.valueSize+size > MaxVariableBytes {
		return fmt.Errorf("variable storage limit exceeded (%d bytes total)", rt.varBytes-local.valueSize+size)
	}
	rt.varBytes = rt.varBytes - local.valueSize + size
	local.valueSize = size
	local.value = cloneStoredValue(v)
	local.valueSet = true
	if root != nil && root != local && !rt.localIsArray(root) {
		root.valueSet = true
		if root.globalArrayName != "" {
			rt.markGlobalScalarName(root.globalArrayName)
		}
	}
	local.arrayAlias = nil
	local.globalArrayName = ""
	local.array = nil
	local.arraySizes = nil
	return nil
}

func (rt *runtime) markLocalScalarRead(local *localVar) {
	root := rootLocalVar(local)
	if root == nil || rt.localIsArray(root) {
		return
	}
	root.value = unassignedValue()
	root.valueSet = true
	if root.globalArrayName != "" {
		rt.markGlobalScalarName(root.globalArrayName)
	}
}

func (rt *runtime) isArray(name string) bool {
	if local := rt.lookupLocal(name); local != nil {
		return rt.localIsArray(local)
	}
	return rt.isGlobalArray(name)
}

func (rt *runtime) isGlobalArray(name string) bool {
	_, ok := rt.arrays[name]
	return ok
}

func (rt *runtime) localArrayStorage(name string, create bool) (map[string]value, *localVar, string, bool, error) {
	local := rt.lookupLocal(name)
	if local == nil {
		return nil, nil, "", false, nil
	}
	root := rootLocalVar(local)
	if root.valueSet && root.array == nil {
		return nil, nil, "", true, fmt.Errorf("fatal: cannot use scalar %s as array", name)
	}
	if root.globalArrayName != "" {
		actual := root.globalArrayName
		if err := rt.ensureBuiltinArray(actual); err != nil {
			return nil, nil, "", true, err
		}
		if err := rt.validateArrayName(actual); err != nil {
			return nil, nil, "", true, err
		}
		if create || rt.arrays[actual] != nil {
			rt.markArrayName(actual)
		}
		return rt.arrays[actual], root, actual, true, nil
	}
	if root.array == nil && create {
		root.array = make(map[string]value)
		root.arraySizes = make(map[string]int)
	}
	return root.array, root, "", true, nil
}

func (rt *runtime) ensureLocalArray(name string) (map[string]value, *localVar, string, bool, error) {
	elems, local, globalName, handled, err := rt.localArrayStorage(name, true)
	if handled || err != nil {
		return elems, local, globalName, handled, err
	}
	if err := rt.ensureBuiltinArray(name); err != nil {
		return nil, nil, "", false, err
	}
	if err := rt.validateArrayName(name); err != nil {
		return nil, nil, "", false, err
	}
	rt.markArrayName(name)
	return rt.arrays[name], nil, name, false, nil
}

func (rt *runtime) getArrayElem(name, key string) (value, error) {
	elems, local, globalName, handled, err := rt.ensureLocalArray(name)
	if err != nil {
		return value{}, err
	}
	if v, ok := elems[key]; ok {
		return v, nil
	}
	v := unassignedValue()
	if handled {
		if err := rt.setLocalArrayElem(local, globalName, key, v); err != nil {
			return value{}, err
		}
		return v, nil
	}
	if err := rt.setGlobalArrayElem(name, key, v); err != nil {
		return value{}, err
	}
	return v, nil
}

func (rt *runtime) hasArrayElem(name, key string) (bool, error) {
	elems, _, _, handled, err := rt.localArrayStorage(name, true)
	if err != nil {
		return false, err
	}
	if !handled {
		if err := rt.ensureBuiltinArray(name); err != nil {
			return false, err
		}
		if err := rt.validateArrayName(name); err != nil {
			return false, err
		}
		rt.markArrayName(name)
		elems = rt.arrays[name]
	}
	_, ok := elems[key]
	return ok, nil
}

func (rt *runtime) setArrayElem(name, key string, v value) error {
	_, local, globalName, handled, err := rt.ensureLocalArray(name)
	if err != nil {
		return err
	}
	if handled {
		return rt.setLocalArrayElem(local, globalName, key, v)
	}
	return rt.setGlobalArrayElem(name, key, v)
}

func (rt *runtime) setGlobalArrayElem(name, key string, v value) error {
	if err := rt.ensureBuiltinArray(name); err != nil {
		return err
	}
	if err := rt.validateArrayName(name); err != nil {
		return err
	}
	rt.markArrayName(name)
	size := len(key) + len(v.String())
	if size > MaxVariableBytes {
		return fmt.Errorf("array element exceeds %d bytes", MaxVariableBytes)
	}
	old := rt.arraySizes[arraySlot{name: name, key: key}]
	if rt.varBytes-old+size > MaxVariableBytes {
		return fmt.Errorf("variable storage limit exceeded (%d bytes total)", rt.varBytes-old+size)
	}
	key = cloneStoredString(key)
	v = cloneStoredValue(v)
	slot := arraySlot{name: name, key: key}
	rt.varBytes = rt.varBytes - old + size
	rt.arraySizes[slot] = size
	rt.arrays[name][key] = v
	return nil
}

func (rt *runtime) setLocalArrayElem(local *localVar, globalName, key string, v value) error {
	if globalName != "" {
		return rt.setGlobalArrayElem(globalName, key, v)
	}
	root := rootLocalVar(local)
	if root.array == nil {
		root.array = make(map[string]value)
		root.arraySizes = make(map[string]int)
	}
	size := len(key) + len(v.String())
	if size > MaxVariableBytes {
		return fmt.Errorf("array element exceeds %d bytes", MaxVariableBytes)
	}
	old := root.arraySizes[key]
	if rt.varBytes-old+size > MaxVariableBytes {
		return fmt.Errorf("variable storage limit exceeded (%d bytes total)", rt.varBytes-old+size)
	}
	key = cloneStoredString(key)
	v = cloneStoredValue(v)
	rt.varBytes = rt.varBytes - old + size
	root.arraySizes[key] = size
	root.array[key] = v
	return nil
}

func (rt *runtime) replaceArray(name string, elems map[string]value) error {
	if err := rt.deleteArray(name); err != nil {
		return err
	}
	for key, v := range elems {
		if err := rt.setArrayElem(name, key, v); err != nil {
			return err
		}
	}
	return nil
}

func (rt *runtime) deleteArrayElem(name, key string) error {
	elems, local, globalName, handled, err := rt.ensureLocalArray(name)
	if err != nil {
		return err
	}
	if handled {
		if globalName != "" {
			return rt.deleteGlobalArrayElem(globalName, key)
		}
		root := rootLocalVar(local)
		if root.array == nil {
			return nil
		}
		if old := root.arraySizes[key]; old > 0 {
			rt.varBytes -= old
			if rt.varBytes < 0 {
				rt.varBytes = 0
			}
		}
		delete(root.arraySizes, key)
		delete(elems, key)
		return nil
	}
	return rt.deleteGlobalArrayElem(name, key)
}

func (rt *runtime) deleteGlobalArrayElem(name, key string) error {
	if err := rt.ensureBuiltinArray(name); err != nil {
		return err
	}
	if err := rt.validateArrayName(name); err != nil {
		return err
	}
	rt.markArrayName(name)
	slot := arraySlot{name: name, key: key}
	if old := rt.arraySizes[slot]; old > 0 {
		rt.varBytes -= old
		if rt.varBytes < 0 {
			rt.varBytes = 0
		}
	}
	delete(rt.arraySizes, slot)
	delete(rt.arrays[name], key)
	return nil
}

func (rt *runtime) deleteArray(name string) error {
	_, local, globalName, handled, err := rt.ensureLocalArray(name)
	if err != nil {
		return err
	}
	if handled {
		if globalName != "" {
			return rt.deleteGlobalArray(globalName)
		}
		root := rootLocalVar(local)
		for _, size := range root.arraySizes {
			rt.varBytes -= size
		}
		if rt.varBytes < 0 {
			rt.varBytes = 0
		}
		root.array = make(map[string]value)
		root.arraySizes = make(map[string]int)
		root.valueSet = false
		root.valueSize = 0
		return nil
	}
	return rt.deleteGlobalArray(name)
}

func (rt *runtime) deleteGlobalArray(name string) error {
	if err := rt.ensureBuiltinArray(name); err != nil {
		return err
	}
	if err := rt.validateArrayName(name); err != nil {
		return err
	}
	rt.markArrayName(name)
	for slot, size := range rt.arraySizes {
		if slot.name != name {
			continue
		}
		rt.varBytes -= size
		delete(rt.arraySizes, slot)
	}
	if rt.varBytes < 0 {
		rt.varBytes = 0
	}
	rt.arrays[name] = make(map[string]value)
	return nil
}

func (rt *runtime) arrayKeys(name string) ([]string, error) {
	return rt.arrayKeysSorted(name)
}

func (rt *runtime) arrayStorage(name string) (map[string]value, error) {
	elems, _, _, handled, err := rt.localArrayStorage(name, true)
	if err != nil {
		return nil, err
	}
	if !handled {
		if err := rt.ensureBuiltinArray(name); err != nil {
			return nil, err
		}
		if err := rt.validateArrayName(name); err != nil {
			return nil, err
		}
		rt.markArrayName(name)
		elems = rt.arrays[name]
	}
	return elems, nil
}

func (rt *runtime) arrayLen(name string) (int, error) {
	elems, err := rt.arrayStorage(name)
	if err != nil {
		return 0, err
	}
	return len(elems), nil
}

func (rt *runtime) arrayKeysSorted(name string) ([]string, error) {
	elems, err := rt.arrayStorage(name)
	if err != nil {
		return nil, err
	}
	if err := rt.chargeArraySort(elems); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(elems))
	for key := range elems {
		keys = append(keys, key)
	}
	sortStringKeys(keys)
	return keys, nil
}

func (rt *runtime) chargeArraySort(elems map[string]value) error {
	remaining := maxStringProcessingBytes - rt.stringWorkBytes
	bytesUsed := 0
	for key := range elems {
		keyBytes := max(1, len(key))
		if keyBytes > remaining-bytesUsed {
			return rt.chargeStringProcessing(remaining + 1)
		}
		bytesUsed += keyBytes
	}
	return rt.chargeStringSort(len(elems), bytesUsed)
}

func (rt *runtime) chargeStringSort(count, bytesUsed int) error {
	levels := 1
	for n := count; n > 1; n = n/2 + n%2 {
		levels++
	}
	remaining := maxStringProcessingBytes - rt.stringWorkBytes
	if bytesUsed > remaining/levels {
		return rt.chargeStringProcessing(remaining + 1)
	}
	return rt.chargeStringProcessing(bytesUsed * levels)
}

func sortStringKeys(keys []string) {
	slices.SortFunc(keys, func(left, right string) int {
		return strings.Compare(left, right)
	})
}

func (rt *runtime) ensureBuiltinArray(name string) error {
	if name == "ENVIRON" {
		return rt.ensureEnviron()
	}
	return nil
}

func (rt *runtime) markArrayName(name string) {
	if rt.arrays[name] == nil {
		rt.arrays[name] = make(map[string]value)
	}
}

func (rt *runtime) validateArrayName(name string) error {
	if isBuiltinScalarName(name) {
		return fmt.Errorf("fatal: cannot use scalar %s as array", name)
	}
	if _, ok := rt.vars[name]; ok {
		return fmt.Errorf("fatal: cannot use scalar %s as array", name)
	}
	return nil
}

func (rt *runtime) markGlobalScalarName(name string) {
	if _, ok := rt.vars[name]; !ok {
		rt.vars[name] = unassignedValue()
		rt.varSizes[name] = 0
	}
}

func isBuiltinScalarName(name string) bool {
	switch name {
	case "NF", "NR", "FNR", "FILENAME":
		return true
	default:
		return false
	}
}

func isBuiltinArrayName(name string) bool {
	return name == "ENVIRON"
}

func isReservedAwkVariableName(name string) bool {
	return isBuiltinScalarName(name) || isBuiltinArrayName(name) || isWritableSpecialScalarName(name)
}

func isWritableSpecialScalarName(name string) bool {
	switch name {
	case "FS", "RS", "OFS", "ORS", "OFMT", "CONVFMT", "SUBSEP", "RSTART", "RLENGTH":
		return true
	default:
		return false
	}
}

func (rt *runtime) validateFS(fs string) error {
	if fs == " " {
		return nil
	}
	if fs == "" {
		return errors.New("empty FS is not supported")
	}
	if isSingleRune(fs) {
		return nil
	}
	_, err := rt.compileRegex(fs)
	if err != nil {
		return err
	}
	return nil
}

func validateRS(rs string) error {
	if rs != "" && !isSingleRune(rs) {
		return errors.New("multi-character RS is not supported")
	}
	return nil
}

func isSingleRune(s string) bool {
	if s == "" {
		return false
	}
	_, size := utf8.DecodeRuneInString(s)
	return size == len(s)
}

type awkRegex struct {
	re           *regexp.Regexp
	continuation *regexp.Regexp
	ctx          context.Context
}

func (rt *runtime) compileRegex(pattern string) (*awkRegex, error) {
	key := regexCacheKey{pattern: pattern}
	if re, ok := rt.regexCache[key]; ok {
		re.ctx = rt.ctx
		return re, nil
	}
	if err := rt.chargeStringProcessing(max(minRegexCompileWork, len(pattern))); err != nil {
		return nil, err
	}
	re, err := compileRegexContext(rt.ctx, pattern)
	if err != nil {
		const invalidPrefix = "invalid regular expression"
		msg := err.Error()
		if strings.HasPrefix(msg, invalidPrefix) {
			return nil, fmt.Errorf("fatal: %w", err)
		}
		return nil, err
	}
	rt.rememberRegex(key, re)
	return re, nil
}

func (rt *runtime) rememberRegex(key regexCacheKey, re *awkRegex) {
	if len(key.pattern) > maxRegexCacheBytes {
		return
	}
	key.pattern = cloneStoredString(key.pattern)
	for len(rt.regexCacheOrder) >= maxRegexCacheEntries || rt.regexCacheBytes+len(key.pattern) > maxRegexCacheBytes {
		oldest := rt.regexCacheOrder[0]
		rt.regexCacheOrder = rt.regexCacheOrder[1:]
		delete(rt.regexCache, oldest)
		rt.regexCacheBytes -= len(oldest.pattern)
	}
	rt.regexCache[key] = re
	rt.regexCacheOrder = append(rt.regexCacheOrder, key)
	rt.regexCacheBytes += len(key.pattern)
}

func (rt *runtime) indexString(haystack, needle string) (int, error) {
	if err := rt.ctx.Err(); err != nil {
		return 0, err
	}
	if needle == "" {
		return 1, nil
	}
	pos := strings.Index(haystack, needle)
	if pos < 0 {
		return 0, nil
	}
	return runeLen(haystack[:pos]) + 1, nil
}

func compileRegex(pattern string) (*awkRegex, error) {
	return compileRegexContext(context.Background(), pattern)
}

func compileRegexContext(ctx context.Context, pattern string) (*awkRegex, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(pattern) > MaxRegexBytes {
		return nil, fmt.Errorf("regular expression exceeds %d bytes", MaxRegexBytes)
	}
	if escape := unsupportedRegexEscape(pattern); escape != "" {
		return nil, fmt.Errorf("GNU regular expression escape %s is not supported", escape)
	}
	parsed, err := syntax.Parse(pattern, syntax.POSIX|syntax.MatchNL|syntax.OneLine)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression %q: %v", pattern, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	normalized := parsed.String()
	if len(normalized) > MaxRegexBytes {
		return nil, fmt.Errorf("regular expression exceeds %d bytes", MaxRegexBytes)
	}
	compiled, err := regexp.Compile(normalized)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression %q: %v", pattern, err)
	}
	compiled.Longest()
	continuation, err := regexp.Compile("(?s:.)(?:" + normalized + ")")
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression %q: %v", pattern, err)
	}
	continuation.Longest()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &awkRegex{re: compiled, continuation: continuation, ctx: ctx}, nil
}

func unsupportedRegexEscape(pattern string) string {
	for i := 0; i < len(pattern); {
		if pattern[i] != '\\' {
			i++
			continue
		}
		start := i
		for i < len(pattern) && pattern[i] == '\\' {
			i++
		}
		if (i-start)%2 == 0 || i == len(pattern) {
			continue
		}
		switch pattern[i] {
		case 'y', 'B', '<', '>':
			return pattern[i-1 : i+1]
		}
		i++
	}
	return ""
}

func (re *awkRegex) MatchString(s string) (bool, error) {
	loc, err := re.FindStringIndex(s)
	return loc != nil, err
}

func (re *awkRegex) FindStringIndex(s string) ([]int, error) {
	ctx := re.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ctx.Done() == nil {
		return re.re.FindStringIndex(s), nil
	}
	return findRegexReaderIndex(ctx, re.re, s)
}

func (re *awkRegex) FindAllStringIndex(s string, n int) ([][]int, error) {
	ctx := re.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ctx.Done() == nil {
		return re.re.FindAllStringIndex(s, n), nil
	}
	if n == 0 {
		return nil, nil
	}

	var matches [][]int
	for pos, previousEnd := 0, -1; pos <= len(s) && (n < 0 || len(matches) < n); {
		loc, err := re.findStringIndexFrom(ctx, s, pos)
		if err != nil {
			return nil, err
		}
		if loc == nil {
			break
		}

		accept := true
		if loc[1] == pos {
			if loc[0] == previousEnd {
				accept = false
			}
			_, width := utf8.DecodeRuneInString(s[pos:])
			if width > 0 {
				pos += width
			} else {
				pos = len(s) + 1
			}
		} else {
			pos = loc[1]
		}
		previousEnd = loc[1]
		if accept {
			matches = append(matches, loc)
		}
	}
	return matches, nil
}

type regexContextReader struct {
	ctx   context.Context
	text  string
	index int
	err   error
}

func (r *regexContextReader) ReadRune() (rune, int, error) {
	if err := r.ctx.Err(); err != nil {
		r.err = err
		return 0, 0, err
	}
	if r.index >= len(r.text) {
		return 0, 0, io.EOF
	}
	decoded, size := utf8.DecodeRuneInString(r.text[r.index:])
	r.index += size
	return decoded, size, nil
}

func findRegexReaderIndex(ctx context.Context, re *regexp.Regexp, s string) ([]int, error) {
	reader := &regexContextReader{ctx: ctx, text: s}
	loc := re.FindReaderIndex(reader)
	if reader.err != nil {
		return nil, reader.err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return loc, nil
}

func (re *awkRegex) findStringIndexFrom(ctx context.Context, s string, start int) ([]int, error) {
	if start == 0 {
		return findRegexReaderIndex(ctx, re.re, s)
	}
	previousSize := previousRegexRuneSize(s, start)
	base := start - previousSize
	loc, err := findRegexReaderIndex(ctx, re.continuation, s[base:])
	if err != nil || loc == nil {
		return nil, err
	}
	_, prefixSize := utf8.DecodeRuneInString(s[base+loc[0]:])
	return []int{base + loc[0] + prefixSize, base + loc[1]}, nil
}

func previousRegexRuneSize(s string, end int) int {
	for size := min(utf8.UTFMax, end); size > 1; size-- {
		_, decodedSize := utf8.DecodeRuneInString(s[end-size : end])
		if decodedSize == size {
			return size
		}
	}
	return 1
}

func runeRangeForByteRange(s string, startByte, endByte int) (int, int) {
	if startByte < 0 {
		startByte = 0
	}
	if startByte > len(s) {
		startByte = len(s)
	}
	if endByte < startByte {
		endByte = startByte
	}
	if endByte > len(s) {
		endByte = len(s)
	}
	if startByte == endByte {
		idx := runeIndexForByteOffset(s, startByte)
		return idx, idx
	}
	return runeIndexForByteOffset(s, startByte), runeIndexAfterByteOffset(s, endByte)
}

func runeIndexForByteOffset(s string, offset int) int {
	if offset <= 0 {
		return 0
	}
	runeIndex := 0
	for i := 0; i < len(s); runeIndex++ {
		_, size := utf8.DecodeRuneInString(s[i:])
		next := i + size
		if offset < next {
			return runeIndex
		}
		if offset == next {
			return runeIndex + 1
		}
		i = next
	}
	return runeIndex
}

func runeIndexAfterByteOffset(s string, offset int) int {
	if offset <= 0 {
		return 0
	}
	runeIndex := 0
	for i := 0; i < len(s); runeIndex++ {
		_, size := utf8.DecodeRuneInString(s[i:])
		next := i + size
		if offset <= next {
			return runeIndex + 1
		}
		i = next
	}
	return runeIndex
}
