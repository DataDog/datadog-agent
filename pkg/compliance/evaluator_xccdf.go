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
	// xccdfTimeout is the longest wait for the next result from oscap-io.
	xccdfTimeout = 60 * time.Second

	// xccdfWaitDelay bounds the wait for the oscap-io output once the
	// process has exited.
	xccdfWaitDelay = 5 * time.Second
)

// errStalled reports an oscap-io process killed for going without a
// result past its timeout.
var errStalled = errors.New("stalled")

// An xccdfRequest asks oscap-io to evaluate the XCCDF rule id of a profile
// on behalf of a benchmark rule.
type xccdfRequest struct {
	profile string
	id      string
	rule    *Rule
}

// An xccdfResult is the result code oscap-io printed for a rule.
type xccdfResult struct {
	rule *Rule
	code int
}

// An oscapIO evaluates XCCDF requests with oscap-io processes.
type oscapIO struct {
	name     string                                       // benchmark, for logs
	newCmd   func(ctx context.Context) (*exec.Cmd, error) // makes an oscap-io command
	interval time.Duration                                // between requests, zero for all at once
	timeout  time.Duration                                // longest wait for a result
}

// EvaluateXCCDFRules evaluates the XCCDF rules of benchmark with oscap-io,
// probing the filesystem at root, or the running system when root is empty.
// It sends one rule every interval, or all of them at once when interval is
// zero. A rule oscap-io stalls or dies on is logged and skipped, and the
// evaluation goes on with the rules after it. EvaluateXCCDFRules returns the
// events of the evaluated rules, and the error that ended the evaluation
// early, if any.
func EvaluateXCCDFRules(ctx context.Context, hostname, root string, benchmark *Benchmark, interval time.Duration) ([]*CheckEvent, error) {
	file, reqs, err := xccdfRequests(benchmark)
	if err != nil {
		return nil, err
	}
	o := &oscapIO{
		name: benchmark.FrameworkID,
		newCmd: func(ctx context.Context) (*exec.Cmd, error) {
			return oscapIOCommand(ctx, file, root)
		},
		interval: interval,
		timeout:  xccdfTimeout,
	}
	results, err := o.evaluate(ctx, reqs)
	events := newXCCDFEvents(hostname, benchmark, results)
	if err != nil {
		return events, fmt.Errorf("oscap-io %s: %w", benchmark.FrameworkID, err)
	}
	return events, nil
}

// xccdfRequests returns the datastream of the XCCDF rules of benchmark and
// their requests, in benchmark order.
func xccdfRequests(benchmark *Benchmark) (string, []xccdfRequest, error) {
	var file string
	var reqs []xccdfRequest
	for _, rule := range benchmark.Rules {
		if !rule.IsXCCDF() {
			continue
		}
		spec := rule.InputSpecs[0].XCCDF
		f := filepath.Join(benchmark.dirname, spec.Name)
		if file == "" {
			file = f
		}
		if f != file {
			return "", nil, fmt.Errorf("xccdf benchmark %s uses two datastreams: %s and %s", benchmark.FrameworkID, file, f)
		}
		ids := spec.Rules
		if len(ids) == 0 && spec.Rule != "" {
			ids = []string{spec.Rule}
		}
		for _, id := range ids {
			reqs = append(reqs, xccdfRequest{profile: spec.Profile, id: id, rule: rule})
		}
	}
	return file, reqs, nil
}

// evaluate evaluates reqs. When an oscap-io process ends early, evaluate
// starts another one on the requests left. It skips the request a process
// stalled on, or died on before its first answer, and gives up when the
// first two processes end without an answer.
func (o *oscapIO) evaluate(ctx context.Context, reqs []xccdfRequest) ([]xccdfResult, error) {
	var results []xccdfResult
	answered := false
	for attempt := 1; len(reqs) > 0; attempt++ {
		res, n, err := o.run(ctx, reqs)
		results = append(results, res...)
		answered = answered || n > 0
		if err == nil || ctx.Err() != nil || n >= len(reqs) || (!answered && attempt > 1) {
			return results, err
		}
		if n == 0 || errors.Is(err, errStalled) {
			log.Warnf("oscap-io %s: skipping rule %s: %v", o.name, reqs[n].id, err)
			n++
		} else {
			log.Warnf("oscap-io %s: restarting at rule %s: %v", o.name, reqs[n].id, err)
		}
		reqs = reqs[n:]
	}
	return results, nil
}

// run evaluates reqs with one oscap-io process. It returns the results, the
// index of the request after the last one answered, and the reason the
// process ended before answering every request, if any.
func (o *oscapIO) run(ctx context.Context, reqs []xccdfRequest) ([]xccdfResult, int, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	cmd, err := o.newCmd(ctx)
	if err != nil {
		return nil, 0, err
	}
	cmd.Stderr = stderrLog{}
	cmd.WaitDelay = xccdfWaitDelay
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, 0, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, err
	}
	log.Debugf("oscap-io %s: evaluating %d rules with %s", o.name, len(reqs), cmd)
	if err := cmd.Start(); err != nil {
		return nil, 0, err
	}
	// Cancelling also closes stdout, so the read ends even while a child of
	// oscap-io holds the pipe open.
	stop := context.AfterFunc(ctx, func() { stdout.Close() })
	defer stop()

	// The watchdog kills oscap-io once it has gone o.timeout without a result.
	watchdog := time.AfterFunc(o.timeout, func() { cancel(errStalled) })
	defer watchdog.Stop()

	done := make(chan struct{})
	written := make(chan struct{})
	go func() {
		defer close(written)
		o.write(stdin, reqs, done)
	}()

	// oscap-io answers in request order and prints nothing for a request
	// it fails to evaluate, so a result matches the first pending request
	// with its id, and the requests before that one got no result.
	var results []xccdfResult
	var silent []string
	next := 0
	s := bufio.NewScanner(stdout)
	for s.Scan() {
		id, code, ok := parseXCCDFResult(s.Text())
		i := slices.IndexFunc(reqs[next:], func(r xccdfRequest) bool { return r.id == id })
		if !ok || i < 0 {
			log.Warnf("oscap-io %s: unexpected output %q", o.name, s.Text())
			continue
		}
		watchdog.Reset(o.timeout)
		for _, r := range reqs[next : next+i] {
			silent = append(silent, r.id)
		}
		next += i
		results = append(results, xccdfResult{rule: reqs[next].rule, code: code})
		next++
	}
	if err := s.Err(); err != nil {
		cancel(err)
	}
	close(done)
	err = cmd.Wait()
	<-written
	if cause := context.Cause(ctx); cause != nil {
		err = cause
	}
	if err == nil {
		for _, r := range reqs[next:] {
			silent = append(silent, r.id)
		}
	}
	if len(silent) > 0 {
		log.Warnf("oscap-io %s: no result for %d rules: %s", o.name, len(silent), strings.Join(silent, " "))
	}
	return results, next, err
}

// write sends reqs to oscap-io on w, one every o.interval, then closes w.
// It returns early when done is closed or a write fails. A write or close
// fails once oscap-io has exited, which run reports.
func (o *oscapIO) write(w io.WriteCloser, reqs []xccdfRequest, done <-chan struct{}) {
	defer w.Close()
	var tick <-chan time.Time
	if o.interval > 0 {
		t := time.NewTicker(o.interval)
		defer t.Stop()
		tick = t.C
	}
	for i, r := range reqs {
		if i > 0 && tick != nil {
			select {
			case <-tick:
			case <-done:
				return
			}
		}
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

// newXCCDFEvents returns the events of the results on the host hostname.
func newXCCDFEvents(hostname string, benchmark *Benchmark, results []xccdfResult) []*CheckEvent {
	var events []*CheckEvent
	for _, r := range results {
		switch r.code {
		case xccdfPass:
			events = append(events, NewCheckEvent(XCCDFEvaluator, CheckPassed, map[string]any{}, hostname, "host", r.rule, benchmark))
		case xccdfFail:
			events = append(events, NewCheckEvent(XCCDFEvaluator, CheckFailed, map[string]any{}, hostname, "host", r.rule, benchmark))
		case xccdfError, xccdfUnknown:
			events = append(events, NewCheckError(XCCDFEvaluator, errors.New("XCCDF_RESULT_ERROR"), hostname, "host", r.rule, benchmark))
		case xccdfNotApplicable:
			events = append(events, NewCheckSkipped(XCCDFEvaluator, errors.New("XCCDF_RESULT_NOT_APPLICABLE"), hostname, "host", r.rule, benchmark))
		case xccdfNotChecked, xccdfNotSelected, xccdfInformational, xccdfFixed:
		}
	}
	return events
}

// stderrLog logs what oscap-io writes to its standard error.
type stderrLog struct{}

func (stderrLog) Write(p []byte) (int, error) {
	log.Warnf("oscap-io: %s", bytes.TrimSpace(p))
	return len(p), nil
}
