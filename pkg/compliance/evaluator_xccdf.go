// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Result codes printed by oscap-io, from xccdf_test_result_type_t in OpenSCAP.
const (
	xccdfPass = iota + 1
	xccdfFail
	xccdfError
	xccdfUnknown
	xccdfNotApplicable
	xccdfNotChecked
	xccdfNotSelected
	xccdfInformational
	xccdfFixed
)

const (
	// xccdfTimeout is the longest wait for the next answer from oscap-io.
	xccdfTimeout = 60 * time.Second

	// xccdfWaitDelay bounds the wait for the oscap-io output once the
	// process has exited.
	xccdfWaitDelay = 5 * time.Second

	// xccdfMaxLogged bounds the rule ids and the stderr lines one
	// evaluation logs.
	xccdfMaxLogged = 100
)

var (
	// errStalled reports an oscap-io process killed for staying silent past
	// its timeout.
	errStalled = errors.New("stalled")

	// errNoResult reports an evaluation that produced zero events.
	errNoResult = errors.New("no rule evaluated")

	// errStart marks an oscap-io process that failed to start.
	errStart = errors.New("start oscap-io")
)

// An xccdfRequest asks oscap-io to evaluate the XCCDF rule id of a profile
// on behalf of a benchmark rule.
type xccdfRequest struct {
	profile string
	id      string
	rule    *Rule
}

// An xccdfGroup holds the requests on one datastream.
type xccdfGroup struct {
	file string
	reqs []xccdfRequest
}

// An xccdfResult is the code oscap-io answered for a request, and the time
// oscap-io took to evaluate it.
type xccdfResult struct {
	rule *Rule
	code int
	took time.Duration
}

// An oscapIO evaluates XCCDF requests with oscap-io processes.
type oscapIO struct {
	name     string                                       // benchmark, for logs
	newCmd   func(ctx context.Context) (*exec.Cmd, error) // makes an oscap-io command
	interval time.Duration                                // least time between requests, zero to send them all at once
	timeout  time.Duration                                // longest wait for an answer
	stderr   stderrLog                                    // logs the stderr of its processes
}

// EvaluateXCCDFRules evaluates the XCCDF rules of benchmark with oscap-io,
// probing the filesystem at root, or the running system when root is empty.
// It calls report with the rule of each answer, its event, and the time
// oscap-io took to evaluate it. A rule oscap-io fails to evaluate gets an
// error event, and the event is nil for notchecked, notselected,
// informational and fixed results. With a positive interval,
// it sends a rule once oscap-io has answered the previous one, and at least
// interval after it. With a zero interval, it sends them all at once. A rule
// oscap-io stalls on, or dies on, is logged and skipped. EvaluateXCCDFRules
// returns the errors that ended an evaluation early, or an error when the
// XCCDF rules of benchmark produced zero events.
func EvaluateXCCDFRules(ctx context.Context, hostname, root string, benchmark *Benchmark, interval time.Duration, report func(*Rule, *CheckEvent, time.Duration)) error {
	newCmd := func(ctx context.Context, file string) (*exec.Cmd, error) {
		return oscapIOCommand(ctx, file, root)
	}
	return evaluateXCCDF(ctx, hostname, benchmark, newCmd, interval, xccdfTimeout, report)
}

// evaluateXCCDF is EvaluateXCCDFRules with the oscap-io command and the
// timeout as parameters.
func evaluateXCCDF(ctx context.Context, hostname string, benchmark *Benchmark, newCmd func(ctx context.Context, file string) (*exec.Cmd, error), interval, timeout time.Duration, report func(*Rule, *CheckEvent, time.Duration)) error {
	groups, err := xccdfRequests(benchmark)
	if err != nil {
		return err
	}
	reported := 0
	var errs []error
	for _, g := range groups {
		o := &oscapIO{
			name: benchmark.FrameworkID,
			newCmd: func(ctx context.Context) (*exec.Cmd, error) {
				return newCmd(ctx, g.file)
			},
			interval: interval,
			timeout:  timeout,
		}
		err := o.evaluate(ctx, g.reqs, func(r xccdfResult) {
			e := newXCCDFEvent(hostname, benchmark, r)
			if e != nil {
				reported++
			}
			report(r.rule, e, r.took)
		})
		if err != nil {
			errs = append(errs, err)
		}
		if ctx.Err() != nil {
			break
		}
	}
	if len(groups) > 0 && reported == 0 && len(errs) == 0 {
		errs = append(errs, errNoResult)
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("oscap-io %s: %w", benchmark.FrameworkID, err)
	}
	return nil
}

// xccdfRequests returns the requests for the XCCDF rules of benchmark, by
// datastream in their order of first use, and in benchmark order within a
// datastream. It logs and leaves out the requests whose profile or rule id
// is empty or holds a blank.
func xccdfRequests(benchmark *Benchmark) ([]xccdfGroup, error) {
	dir, err := filepath.Abs(benchmark.dirname)
	if err != nil {
		return nil, err
	}
	var groups []xccdfGroup
	for _, rule := range benchmark.Rules {
		if !rule.IsXCCDF() {
			continue
		}
		spec := rule.InputSpecs[0].XCCDF
		file := filepath.Join(dir, spec.Name)
		i := slices.IndexFunc(groups, func(g xccdfGroup) bool { return g.file == file })
		if i < 0 {
			i = len(groups)
			groups = append(groups, xccdfGroup{file: file})
		}
		ids := spec.Rules
		if len(ids) == 0 {
			ids = []string{spec.Rule}
		}
		for _, id := range ids {
			if !isOSCAPIOToken(spec.Profile) || !isOSCAPIOToken(id) {
				log.Warnf("xccdf benchmark %s: rule %s: invalid profile %q or XCCDF rule %q", benchmark.FrameworkID, rule.ID, spec.Profile, id)
				continue
			}
			groups[i].reqs = append(groups[i].reqs, xccdfRequest{profile: spec.Profile, id: id, rule: rule})
		}
	}
	return groups, nil
}

// isOSCAPIOToken reports whether s fits in an oscap-io request: a word of
// one or more characters, apart from blanks.
func isOSCAPIOToken(s string) bool {
	return s != "" && !strings.ContainsAny(s, " \t\r\n")
}

// evaluate evaluates reqs and calls report with each result. When an
// oscap-io process ends early, evaluate starts another one on the requests
// left, interval later. It skips the request a process stalled on, or died
// on before answering anything, and gives up when the first two processes
// end before their first answer. A process that fails after answering every
// request has done its work.
func (o *oscapIO) evaluate(ctx context.Context, reqs []xccdfRequest, report func(xccdfResult)) error {
	defer o.stderr.summarize()
	answered := false
	var last error
	for attempt := 1; len(reqs) > 0; attempt++ {
		if attempt > 1 && o.interval > 0 {
			select {
			case <-time.After(o.interval):
			case <-ctx.Done():
				return context.Cause(ctx)
			}
		}
		n, err := o.run(ctx, reqs, report)
		answered = answered || n > 0
		switch {
		case err == nil:
			return nil
		case ctx.Err() != nil, errors.Is(err, errStart), !answered && attempt > 1:
			return err
		case n >= len(reqs):
			log.Warnf("oscap-io %s: %v after the last rule", o.name, err)
			return nil
		case n == 0, errors.Is(err, errStalled):
			log.Warnf("oscap-io %s: skipping rule %s: %v", o.name, reqs[n].id, err)
			n++
		default:
			log.Debugf("oscap-io %s: restarting at rule %s: %v", o.name, reqs[n].id, err)
		}
		last = err
		reqs = reqs[n:]
	}
	if !answered {
		return last
	}
	return nil
}

// run evaluates reqs with one oscap-io process and calls report with each
// result. It returns the index of the request after the last one answered,
// and the reason the process ended before answering every request, if any.
func (o *oscapIO) run(ctx context.Context, reqs []xccdfRequest, report func(xccdfResult)) (int, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	cmd, err := o.newCmd(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errStart, err)
	}
	o.stderr.name = o.name
	cmd.Stderr = &o.stderr
	cmd.WaitDelay = xccdfWaitDelay
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errStart, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errStart, err)
	}
	log.Debugf("oscap-io %s: evaluating %d rules with %s", o.name, len(reqs), cmd)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("%w: %w", errStart, err)
	}

	// The watchdog kills oscap-io once o.timeout passes after its start or
	// its last answer, and bounds the wait for its exit.
	watchdog := time.AfterFunc(o.timeout, func() { cancel(errStalled) })
	defer watchdog.Stop()

	next, silent, err := o.talk(ctx, watchdog, stdin, stdout, reqs, report)
	if err != nil {
		cancel(err)
	}
	err = cmd.Wait()
	o.stderr.flush()
	if errors.Is(err, exec.ErrWaitDelay) {
		// oscap-io exited 0, and a child of it held stderr open.
		err = nil
	}
	// The cause tells why a killed oscap-io failed. A clean exit stands,
	// even when the watchdog fired after it.
	if cause := context.Cause(ctx); cause != nil && err != nil {
		err = cause
	}
	if err == nil {
		for _, r := range reqs[next:] {
			silent = append(silent, r.id)
		}
	}
	if len(silent) > 0 {
		log.Warnf("oscap-io %s: no result for %d rules: %s", o.name, len(silent), strings.Join(silent[:min(len(silent), xccdfMaxLogged)], " "))
	}
	return next, err
}

// talk sends reqs to oscap-io on stdin, reads its answers on stdout, and
// calls report with each. It stops the watchdog, which cancels ctx, while
// report runs, and restarts it at each answer. talk returns the index of the
// request after the last one answered, the ids of the requests passed over
// before it, and the error that ended the read, if any.
func (o *oscapIO) talk(ctx context.Context, watchdog *time.Timer, stdin io.WriteCloser, stdout io.ReadCloser, reqs []xccdfRequest, report func(xccdfResult)) (int, []string, error) {
	// Cancelling also closes stdout, so the read ends even while a child of
	// oscap-io holds the pipe open.
	stop := context.AfterFunc(ctx, func() { stdout.Close() })
	defer stop()

	// The writer puts the time of each request on sent before the request
	// goes out, and paces the requests on the answers put on answered.
	sent := make(chan time.Time, len(reqs))
	answered := make(chan struct{}, len(reqs))
	done := make(chan struct{})
	written := make(chan struct{})
	go func() {
		defer close(written)
		o.write(stdin, reqs, sent, answered, done)
	}()
	// Closing stdin also ends a write that oscap-io has stopped reading.
	defer func() {
		close(done)
		stdin.Close()
		<-written
	}()

	// oscap-io answers each request in order, with error for a rule it
	// fails to evaluate and notselected for a rule whose results it filters
	// out. Older versions print nothing for those, so an answer goes to the
	// first request sent and pending with its id, and the requests before
	// it count as silent.
	var silent []string
	var prev time.Time
	next := 0
	s := bufio.NewScanner(stdout)
	for s.Scan() && ctx.Err() == nil {
		id, code, ok := parseXCCDFResult(s.Text())
		i := slices.IndexFunc(reqs[next:next+len(sent)], func(r xccdfRequest) bool { return r.id == id })
		if !ok || i < 0 {
			log.Warnf("oscap-io %s: unexpected output %q", o.name, s.Text())
			continue
		}
		// oscap-io starts on a request once it has the request and has
		// answered the one before.
		var start time.Time
		for range i + 1 {
			start = <-sent
		}
		if start.Before(prev) {
			start = prev
		}
		prev = time.Now()
		for _, r := range reqs[next : next+i] {
			silent = append(silent, r.id)
		}
		next += i
		// The watchdog waits on oscap-io, so it stops while report runs.
		watchdog.Stop()
		report(xccdfResult{rule: reqs[next].rule, code: code, took: prev.Sub(start)})
		watchdog.Reset(o.timeout)
		next++
		answered <- struct{}{}
	}
	return next, silent, s.Err()
}

// write sends reqs to oscap-io on w, then closes w. It puts the time of each
// request on sent before the request goes out. With a positive interval, it
// sends a request once the previous one is answered on answered, and at
// least interval after the previous one. It returns early when done is
// closed or a write fails, which happens once oscap-io has exited.
func (o *oscapIO) write(w io.WriteCloser, reqs []xccdfRequest, sent chan<- time.Time, answered, done <-chan struct{}) {
	defer w.Close()
	var last time.Time
	for i, r := range reqs {
		if i > 0 && o.interval > 0 {
			select {
			case <-answered:
			case <-done:
				return
			}
			select {
			case <-time.After(time.Until(last.Add(o.interval))):
			case <-done:
				return
			}
		}
		last = time.Now()
		sent <- last
		if _, err := fmt.Fprintf(w, "%s %s\n", r.profile, r.id); err != nil {
			return
		}
	}
}

// parseXCCDFResult parses an oscap-io result line, "id code".
func parseXCCDFResult(line string) (string, int, bool) {
	id, s, ok := strings.Cut(line, " ")
	if !ok {
		return "", 0, false
	}
	code, err := strconv.Atoi(s)
	if err != nil || code < xccdfPass || code > xccdfFixed {
		return "", 0, false
	}
	return id, code, true
}

// newXCCDFEvent returns the event of the result r on the host hostname. It
// returns nil for notchecked, notselected, informational and fixed.
func newXCCDFEvent(hostname string, benchmark *Benchmark, r xccdfResult) *CheckEvent {
	switch r.code {
	case xccdfPass:
		return NewCheckEvent(XCCDFEvaluator, CheckPassed, map[string]any{}, hostname, "host", r.rule, benchmark)
	case xccdfFail:
		return NewCheckEvent(XCCDFEvaluator, CheckFailed, map[string]any{}, hostname, "host", r.rule, benchmark)
	case xccdfError, xccdfUnknown:
		return NewCheckError(XCCDFEvaluator, errors.New("XCCDF_RESULT_ERROR"), hostname, "host", r.rule, benchmark)
	case xccdfNotApplicable:
		return NewCheckSkipped(XCCDFEvaluator, errors.New("XCCDF_RESULT_NOT_APPLICABLE"), hostname, "host", r.rule, benchmark)
	default:
		return nil
	}
}

// A stderrLog logs what oscap-io writes to its standard error, line by
// line, up to xccdfMaxLogged lines.
type stderrLog struct {
	name    string
	partial []byte
	lines   int
}

func (w *stderrLog) Write(p []byte) (int, error) {
	w.partial = append(w.partial, p...)
	rest := w.partial
	for {
		i := bytes.IndexByte(rest, '\n')
		if i < 0 {
			break
		}
		w.line(rest[:i])
		rest = rest[i+1:]
	}
	if len(rest) > bufio.MaxScanTokenSize {
		w.line(rest)
		rest = nil
	}
	w.partial = append(w.partial[:0], rest...)
	return len(p), nil
}

// flush logs the partial line a process left at its end.
func (w *stderrLog) flush() {
	if len(w.partial) > 0 {
		w.line(w.partial)
		w.partial = w.partial[:0]
	}
}

// summarize logs the number of lines left out of the log.
func (w *stderrLog) summarize() {
	if w.lines > xccdfMaxLogged {
		log.Warnf("oscap-io %s: %d more lines on stderr", w.name, w.lines-xccdfMaxLogged)
	}
}

func (w *stderrLog) line(b []byte) {
	w.lines++
	if w.lines <= xccdfMaxLogged {
		log.Warnf("oscap-io %s: %s", w.name, bytes.TrimSpace(b))
	}
}
