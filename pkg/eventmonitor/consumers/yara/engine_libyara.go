// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && yara

package yara

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	goyara "github.com/hillu/go-yara/v4"
)

// EngineName identifies the rule engine compiled into this build
const EngineName = "libyara"

const (
	// libyaraMaxScanners is libyara's YR_MAX_THREADS: the maximum number of scanner objects
	// alive at the same time on one compiled ruleset. It bounds the idle scanners we keep.
	libyaraMaxScanners = 32
	// libyaraNoDeadlineTimeout is the engine timeout used when the scan context has no
	// deadline. A running libyara scan can't be cancelled otherwise, so it must stay bounded.
	libyaraNoDeadlineTimeout = time.Minute
)

// DefaultCompiler returns the rule compiler of this build: libyara, through go-yara.
func DefaultCompiler() Compiler {
	return compileLibyara
}

// compileLibyara compiles the rule sources with libyara. Each source is compiled from its
// text, in its own namespace named after the file, with includes disabled so that a rule file
// can't pull in files from outside the rules directory. Serialized (pre-compiled) rules are
// never loaded: libyara's rule deserializer is not safe on untrusted input.
func compileLibyara(sources []RuleSource) (Scanner, error) {
	compiler, err := goyara.NewCompiler()
	if err != nil {
		return nil, fmt.Errorf("libyara: failed to create compiler: %w", err)
	}
	defer compiler.Destroy()
	compiler.DisableIncludes()

	for _, src := range sources {
		if err := compiler.AddString(string(src.Data), src.Name); err != nil {
			return nil, fmt.Errorf("%s: %w", src.Name, compileError(compiler, err))
		}
	}

	rules, err := compiler.GetRules()
	if err != nil {
		return nil, fmt.Errorf("libyara: failed to get compiled rules: %w", err)
	}
	return &libyaraScanner{
		rules:    rules,
		scanners: make(chan *goyara.Scanner, libyaraMaxScanners),
	}, nil
}

// compileError adds the line number and rule of each compiler error to err. The compiler
// becomes unusable after an error, so this is only used to build the returned error.
func compileError(compiler *goyara.Compiler, err error) error {
	if len(compiler.Errors) == 0 {
		return err
	}
	msgs := make([]string, 0, len(compiler.Errors))
	for _, m := range compiler.Errors {
		msg := fmt.Sprintf("line %d: %s", m.Line, m.Text)
		if m.Rule != "" {
			msg += fmt.Sprintf(" (rule %s)", m.Rule)
		}
		msgs = append(msgs, msg)
	}
	return errors.New(strings.Join(msgs, "; "))
}

// libyaraScanner scans buffers with one compiled ruleset. It is safe for concurrent use.
//
// The compiled rules are immutable and shared by every scan. A libyara scanner object holds
// per-scan state, so it must never be used by two goroutines at once: each Scan borrows its
// own from a free list (a channel), creating one when the list is empty, and gives it back
// after the scan. The ScanPool runs a fixed number of workers, so at most that many scanner
// objects are ever created; the free list holds up to libyaraMaxScanners (libyara's own
// limit of live scanners per ruleset), and a surplus scanner is destroyed.
type libyaraScanner struct {
	rules    *goyara.Rules
	scanners chan *goyara.Scanner

	closeOnce sync.Once
	// closeMu is held for reading by every Scan, and for writing by Close, so that the
	// rules are never destroyed while a scan uses them
	closeMu sync.RWMutex
	closed  bool
}

// Scan implements Scanner.
//
// A libyara scan can't be cancelled once started, so ctx is honored in two ways: its error is
// checked before scanning, and its deadline sets the engine timeout, rounded up to whole
// seconds (the engine's granularity). An engine timeout is returned as
// context.DeadlineExceeded. Cancellation without a deadline can't stop a running scan.
//
// Scan is synchronous on purpose: it returns only once libyara is done reading data, and it
// keeps no reference to data, so the caller may reuse the buffer as soon as it returns.
func (s *libyaraScanner) Scan(ctx context.Context, data []byte) ([]Match, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timeout, err := libyaraTimeout(ctx, time.Now())
	if err != nil {
		return nil, err
	}

	s.closeMu.RLock()
	defer s.closeMu.RUnlock()
	if s.closed {
		return nil, errors.New("libyara: scanner is closed")
	}

	scanner, err := s.getScanner()
	if err != nil {
		return nil, err
	}
	defer s.putScanner(scanner)

	var cb matchCollector
	scanner.SetTimeout(timeout).SetCallback(&cb)
	err = scanner.ScanMem(data)

	var yrErr goyara.Error
	if errors.As(err, &yrErr) && yrErr.Code == goyara.ERROR_SCAN_TIMEOUT {
		return nil, fmt.Errorf("libyara: scan timed out after %s: %w", timeout, context.DeadlineExceeded)
	}
	if err != nil {
		return nil, fmt.Errorf("libyara: scan failed: %w", err)
	}
	return cb.matches, nil
}

// RulesVersion implements Scanner. LoadScanner replaces it with a hash of the rule files.
func (s *libyaraScanner) RulesVersion() string {
	return EngineName
}

// Close frees the idle scanner objects and the compiled rules. It waits for running scans to
// return; later scans fail. It is safe to call more than once.
func (s *libyaraScanner) Close() error {
	s.closeOnce.Do(func() {
		s.closeMu.Lock()
		defer s.closeMu.Unlock()
		s.closed = true
		for {
			select {
			case scanner := <-s.scanners:
				scanner.Destroy()
			default:
				s.rules.Destroy()
				return
			}
		}
	})
	return nil
}

func (s *libyaraScanner) getScanner() (*goyara.Scanner, error) {
	select {
	case scanner := <-s.scanners:
		return scanner, nil
	default:
	}
	scanner, err := goyara.NewScanner(s.rules)
	if err != nil {
		return nil, fmt.Errorf("libyara: failed to create scanner: %w", err)
	}
	return scanner, nil
}

func (s *libyaraScanner) putScanner(scanner *goyara.Scanner) {
	select {
	case s.scanners <- scanner:
	default:
		scanner.Destroy()
	}
}

// libyaraTimeout returns the engine timeout for a scan starting at now: the time left until
// ctx's deadline, rounded up to whole seconds, and at least one second.
// It returns context.DeadlineExceeded when the deadline has already passed.
func libyaraTimeout(ctx context.Context, now time.Time) (time.Duration, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return libyaraNoDeadlineTimeout, nil
	}
	left := deadline.Sub(now)
	if left <= 0 {
		return 0, context.DeadlineExceeded
	}
	seconds := (left + time.Second - 1) / time.Second
	return seconds * time.Second, nil
}

// matchCollector is a go-yara scan callback that records the matching rules. Unlike
// goyara.MatchRules, it doesn't copy the matched string data out of the scanned buffer.
type matchCollector struct {
	matches []Match
}

// RuleMatching implements goyara.ScanCallback
func (c *matchCollector) RuleMatching(_ *goyara.ScanContext, r *goyara.Rule) (bool, error) {
	c.matches = append(c.matches, Match{
		Rule:      r.Identifier(),
		Namespace: r.Namespace(),
		Tags:      r.Tags(),
	})
	return false, nil
}
