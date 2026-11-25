// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"bufio"
	"errors"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
)

const (
	// rcLogEnv is the EnvFilter-syntax variable libdd-rc reads when the log
	// sink is installed.
	rcLogEnv = "RC_LOG"
	// defaultRCLog keeps the library quiet except for the handshake lines.
	defaultRCLog = "warn,rc_x509_client::connection::handler::delegate::fsm::handshaking=debug"

	// connectionReportInterval is how often the current ID is logged again so
	// that a log search over a short window always finds it.
	connectionReportInterval = 5 * time.Minute

	// maxSinkLineBytes bounds the memory used for one library log line. Longer
	// lines are dropped without being parsed.
	maxSinkLineBytes = 16 * 1024
)

var (
	ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

	// handshakeCompleteRE matches only the handshake-complete event of the
	// handshaking state machine. tracing_subscriber's fmt layer prints
	// `<time> <LEVEL> <spans>: <target>: <message> <field>=<value>...`, so the
	// target and message must be adjacent and connection_id must be a field
	// that follows the message; the value must be a canonical UUID.
	handshakeCompleteRE = regexp.MustCompile(`(?i)(?:^|\s|:)rc_x509_client::connection::handler::delegate::fsm::handshaking: connection handshake complete(?:\s.*)?\sconnection_id="?([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:["\s]|$)`)
)

// parseHandshakeComplete returns the lowercase connection ID of a
// handshake-complete line, or false for every other line.
func parseHandshakeComplete(line string) (string, bool) {
	line = ansiEscapeRE.ReplaceAllString(line, "")
	m := handshakeCompleteRE.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return lowerASCII(m[1]), true
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'F' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// reportLogger is the subset of the Agent logger the reporter uses.
type reportLogger interface {
	Infof(format string, params ...interface{})
	Warnf(format string, params ...interface{}) error
}

// reportTicker is the clock seam for the periodic re-emission.
type reportTicker interface {
	C() <-chan time.Time
	Stop()
}

type realTicker struct{ *time.Ticker }

func (t realTicker) C() <-chan time.Time { return t.Ticker.C }

func newRealTicker(d time.Duration) reportTicker { return realTicker{time.NewTicker(d)} }

// connectionReporter captures the connection ID from the client library's
// handshake debug line and has two outputs: the Agent log line (when
// logConnection is set) and the tunnel connection ID in check instance
// inventory metadata (when a publisher is set and the Execute handler is
// registered). It is the only consumer of the library log sink and logs nothing
// else from it.
type connectionReporter struct {
	logger reportLogger

	// logConnection enables the INFO log line and its periodic re-emission.
	logConnection bool
	// publisher, if set, publishes the ID in inventory metadata, but only once
	// the Execute handler is registered.
	publisher *tunnelMetadataPublisher

	// Seams so tests need neither the native library nor real time.
	enableSink func(*os.File) error
	lookupEnv  func(string) (string, bool)
	setEnv     func(string, string) error
	unsetEnv   func(string) error
	newTicker  func(time.Duration) reportTicker

	// tracker, if set, follows the current connection ID for the tunnel authority.
	tracker *ConnectionTracker

	// executeHandler is set once the Execute handler is actually registered.
	executeHandler atomic.Bool

	// publishNow wakes the ticker goroutine to publish right away, so the
	// pipe reader never waits on the collector or the inventory.
	publishNow chan struct{}
	// publishMu serializes publishes so that an older ID is never applied after
	// a newer one.
	publishMu sync.Mutex

	mu      sync.Mutex
	id      string
	stopped bool

	writeEnd   *os.File
	stop       chan struct{}
	stopOnce   sync.Once
	tickerDone chan struct{}
	// readerDone is closed when the pipe reader returns, which happens only
	// once every write end of the pipe is closed.
	readerDone chan struct{}
}

func newConnectionReporter(logger reportLogger, enableSink func(*os.File) error) *connectionReporter {
	return &connectionReporter{
		logger:        logger,
		logConnection: true,
		publishNow:    make(chan struct{}, 1),
		enableSink:    enableSink,
		lookupEnv:     os.LookupEnv,
		setEnv:        os.Setenv,
		unsetEnv:      os.Unsetenv,
		newTicker:     newRealTicker,
		stop:          make(chan struct{}),
		tickerDone:    make(chan struct{}),
		readerDone:    make(chan struct{}),
	}
}

// install sets RC_LOG when unset, hands the library the write end of a pipe and
// starts the reader and the re-emission ticker. On any failure it warns once,
// leaves nothing running and returns false.
func (r *connectionReporter) install() bool {
	setRCLog := false
	if v, ok := r.lookupEnv(rcLogEnv); ok && v != "" {
		_ = r.logger.Warnf("remote config x509 client: %s is already set, so connection handshake lines may be missing and the connection ID may not be reported", rcLogEnv)
	} else if err := r.setEnv(rcLogEnv, defaultRCLog); err != nil {
		_ = r.logger.Warnf("remote config x509 client: unable to set %s, not reporting the connection ID: %v", rcLogEnv, err)
		return false
	} else {
		setRCLog = true
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		r.restoreEnv(setRCLog)
		_ = r.logger.Warnf("remote config x509 client: unable to create the log pipe, not reporting the connection ID: %v", err)
		return false
	}
	if err := r.enableSink(pw); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		r.restoreEnv(setRCLog)
		_ = r.logger.Warnf("remote config x509 client: unable to install the log sink, not reporting the connection ID: %v", err)
		return false
	}

	r.writeEnd = pw
	go r.readLoop(pr)
	var logTicker, publishTicker reportTicker
	if r.logConnection {
		logTicker = r.newTicker(connectionReportInterval)
	}
	if r.publisher != nil {
		publishTicker = r.newTicker(tunnelMetadataInterval)
	}
	go r.tickLoop(logTicker, publishTicker)
	return true
}

func (r *connectionReporter) restoreEnv(set bool) {
	if set {
		_ = r.unsetEnv(rcLogEnv)
	}
}

// markExecuteHandlerRegistered records that the Execute handler is registered
// on the client, which later reports reflect.
func (r *connectionReporter) markExecuteHandlerRegistered() {
	if r != nil {
		r.executeHandler.Store(true)
		select {
		case r.publishNow <- struct{}{}:
		default:
		}
	}
}

// readLoop reads the pipe until every write end is closed. The library holds a
// duplicate of the write end for the life of the process, so the loop keeps
// draining (and discarding) after the reporter is closed: a library write must
// never block.
func (r *connectionReporter) readLoop(pr *os.File) {
	defer close(r.readerDone)
	defer pr.Close()
	reader := bufio.NewReaderSize(pr, maxSinkLineBytes)
	for {
		line, err := reader.ReadSlice('\n')
		switch {
		case err == nil:
			r.handleLine(string(line))
		case errors.Is(err, bufio.ErrBufferFull):
			// Over-long line: discard it up to its end without parsing.
			for errors.Is(err, bufio.ErrBufferFull) {
				_, err = reader.ReadSlice('\n')
			}
			if err != nil {
				return
			}
		default:
			if len(line) > 0 {
				r.handleLine(string(line))
			}
			return
		}
	}
}

func (r *connectionReporter) handleLine(line string) {
	if id, ok := parseHandshakeComplete(line); ok {
		r.observe(id)
	}
}

// observe records the current connection ID and reports it.
func (r *connectionReporter) observe(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return
	}
	r.id = id
	r.tracker.set(id)
	r.emitLocked()
	select {
	case r.publishNow <- struct{}{}:
	default:
	}
}

// tickLoop re-emits the log line and re-applies the metadata. Either ticker
// may be nil, which disables that output.
func (r *connectionReporter) tickLoop(logTicker, publishTicker reportTicker) {
	defer close(r.tickerDone)
	var logC, publishC <-chan time.Time
	if logTicker != nil {
		defer logTicker.Stop()
		logC = logTicker.C()
	}
	if publishTicker != nil {
		defer publishTicker.Stop()
		publishC = publishTicker.C()
	}
	for {
		select {
		case <-logC:
			r.reemit()
		case <-publishC:
			r.publish()
		case <-r.publishNow:
			r.publish()
		case <-r.stop:
			return
		}
	}
}

// publish applies the current connection ID to the eligible check instances.
// It does nothing before a handshake, before the Execute handler is
// registered, or after close.
func (r *connectionReporter) publish() {
	if r.publisher == nil || !r.executeHandler.Load() {
		return
	}
	r.publishMu.Lock()
	defer r.publishMu.Unlock()
	r.mu.Lock()
	id, stopped := r.id, r.stopped
	r.mu.Unlock()
	if stopped || id == "" {
		return
	}
	r.publisher.publish(id)
}

// reemit logs the current ID again, if there is one.
func (r *connectionReporter) reemit() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.stopped && r.id != "" {
		r.emitLocked()
	}
}

func (r *connectionReporter) emitLocked() {
	if !r.logConnection {
		return
	}
	r.logger.Infof("remote queries tunnel connection ready connection_id=%s execute_handler=%t", r.id, r.executeHandler.Load())
}

// close stops reporting and the ticker. It is safe to call more than once and
// on a nil reporter. Closing our write end lets the reader exit when the
// library does not hold its own duplicate.
func (r *connectionReporter) close() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		r.mu.Lock()
		r.stopped = true
		r.mu.Unlock()
		r.tracker.set("")
		close(r.stop)
		if r.writeEnd != nil {
			<-r.tickerDone
			_ = r.writeEnd.Close()
		}
	})
}

// reportingClient stops the reporter when the wrapped client is closed.
type reportingClient struct {
	rcx509client.Client
	reporter *connectionReporter
}

func (c *reportingClient) Close() error {
	err := c.Client.Close()
	c.reporter.close()
	return err
}
