// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
)

const (
	testID1 = "0194a1b2-c3d4-8e5f-a617-283940516273"
	testID2 = "0194a1b2-ffff-8e5f-a617-283940516274"

	handshakeTarget = "rc_x509_client::connection::handler::delegate::fsm::handshaking"
)

func handshakeLine(id string) string {
	return "2026-11-18T10:00:00.123456Z DEBUG " + handshakeTarget + ": connection handshake complete connection_id=" + id + "\n"
}

type recordingLogger struct {
	infos chan string
	mu    sync.Mutex
	warns []string
}

func newRecordingLogger() *recordingLogger { return &recordingLogger{infos: make(chan string, 64)} }

func (l *recordingLogger) Infof(format string, params ...interface{}) {
	l.infos <- fmt.Sprintf(format, params...)
}

func (l *recordingLogger) Warnf(format string, params ...interface{}) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, fmt.Sprintf(format, params...))
	return nil
}

func (l *recordingLogger) warnings() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.warns...)
}

func (l *recordingLogger) next(t *testing.T) string {
	t.Helper()
	select {
	case m := <-l.infos:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a log line")
		return ""
	}
}

func (l *recordingLogger) requireNone(t *testing.T) {
	t.Helper()
	select {
	case m := <-l.infos:
		t.Fatalf("unexpected log line: %q", m)
	default:
	}
}

type fakeTicker struct{ c chan time.Time }

func (f *fakeTicker) C() <-chan time.Time { return f.c }
func (f *fakeTicker) Stop()               {}

type reporterHarness struct {
	r      *connectionReporter
	logger *recordingLogger
	ticker *fakeTicker
	sink   *os.File
	env    map[string]string
	// intervals records the ticker intervals requested.
	intervals []time.Duration
	sinkCalls int
}

func newHarness(t *testing.T, env map[string]string, sinkErr error) *reporterHarness {
	t.Helper()
	h := &reporterHarness{logger: newRecordingLogger(), ticker: &fakeTicker{c: make(chan time.Time)}, env: env}
	if h.env == nil {
		h.env = map[string]string{}
	}
	h.r = newConnectionReporter(h.logger, func(f *os.File) error {
		h.sinkCalls++
		if sinkErr != nil {
			return sinkErr
		}
		h.sink = f
		return nil
	})
	h.r.lookupEnv = func(k string) (string, bool) { v, ok := h.env[k]; return v, ok }
	h.r.setEnv = func(k, v string) error { h.env[k] = v; return nil }
	h.r.unsetEnv = func(k string) error { delete(h.env, k); return nil }
	h.r.newTicker = func(d time.Duration) reportTicker {
		h.intervals = append(h.intervals, d)
		return h.ticker
	}
	return h
}

func (h *reporterHarness) write(t *testing.T, s string) {
	t.Helper()
	_, err := h.sink.WriteString(s)
	require.NoError(t, err)
}

// shutdown closes the reporter and requires both goroutines to end.
func (h *reporterHarness) shutdown(t *testing.T) {
	t.Helper()
	h.r.close()
	for name, ch := range map[string]chan struct{}{"reader": h.r.readerDone, "ticker": h.r.tickerDone} {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s goroutine did not stop", name)
		}
	}
}

func TestParseHandshakeComplete(t *testing.T) {
	ansi := "\x1b[2m2026-11-18T10:00:00.123456Z\x1b[0m \x1b[34mDEBUG\x1b[0m \x1b[2m" + handshakeTarget + "\x1b[0m\x1b[2m:\x1b[0m connection handshake complete \x1b[3mconnection_id\x1b[0m\x1b[2m=\x1b[0m" + testID1
	for name, tc := range map[string]struct {
		line string
		id   string
		ok   bool
	}{
		"plain":                        {handshakeLine(testID1), testID1, true},
		"no newline":                   {strings.TrimSuffix(handshakeLine(testID1), "\n"), testID1, true},
		"ansi":                         {ansi, testID1, true},
		"span prefix":                  {"2026-11-18T10:00:00Z DEBUG run{app=core-agent}:connect{attempt=2}: " + handshakeTarget + ": connection handshake complete connection_id=" + testID1, testID1, true},
		"extra fields":                 {"2026 DEBUG " + handshakeTarget + ": connection handshake complete peer=x connection_id=" + testID1 + " attempt=3", testID1, true},
		"quoted":                       {"2026 DEBUG " + handshakeTarget + ": connection handshake complete connection_id=\"" + testID1 + "\"", testID1, true},
		"uppercase":                    {handshakeLine(strings.ToUpper(testID1)), testID1, true},
		"other message":                {"2026 DEBUG " + handshakeTarget + ": connection handshake started connection_id=" + testID1, "", false},
		"other target":                 {"2026 DEBUG rc_x509_client::connection::other: connection handshake complete connection_id=" + testID1, "", false},
		"id in message":                {"2026 INFO agent: sent payload connection_id=" + testID1, "", false},
		"too short":                    {handshakeLine(testID1[:35]), "", false},
		"too long":                     {handshakeLine(testID1 + "0"), "", false},
		"not hex":                      {handshakeLine("0194a1b2-c3d4-8e5f-a617-28394051627z"), "", false},
		"no hyphens":                   {handshakeLine(strings.ReplaceAll(testID1, "-", "")), "", false},
		"empty id":                     {handshakeLine(""), "", false},
		"missing field":                {"2026 DEBUG " + handshakeTarget + ": connection handshake complete", "", false},
		"unrelated":                    {"2026 WARN something else entirely\n", "", false},
		"empty":                        {"", "", false},
		"connection_id in prefix only": {"connection_id=" + testID1 + " " + handshakeTarget + ": connection handshake complete", "", false},
	} {
		t.Run(name, func(t *testing.T) {
			id, ok := parseHandshakeComplete(tc.line)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.id, id)
		})
	}
}

func TestReporterEmitsOnHandshakeAndEveryInterval(t *testing.T) {
	h := newHarness(t, nil, nil)
	require.True(t, h.r.install())
	require.Equal(t, []time.Duration{5 * time.Minute}, h.intervals)

	// A tick before any handshake has nothing to report.
	h.r.reemit()
	h.logger.requireNone(t)

	h.write(t, handshakeLine(testID1))
	require.Equal(t, "remote queries tunnel connection ready connection_id="+testID1+" execute_handler=false", h.logger.next(t))

	h.r.markExecuteHandlerRegistered()
	h.ticker.c <- time.Time{}
	require.Equal(t, "remote queries tunnel connection ready connection_id="+testID1+" execute_handler=true", h.logger.next(t))

	// A reconnect replaces the current ID, and later ticks report the new one.
	h.write(t, handshakeLine(strings.ToUpper(testID2)))
	require.Equal(t, "remote queries tunnel connection ready connection_id="+testID2+" execute_handler=true", h.logger.next(t))
	h.ticker.c <- time.Time{}
	require.Equal(t, "remote queries tunnel connection ready connection_id="+testID2+" execute_handler=true", h.logger.next(t))

	h.shutdown(t)
	h.logger.requireNone(t)
	require.Empty(t, h.logger.warnings())
}

func TestReporterLogsNothingElseFromTheSink(t *testing.T) {
	h := newHarness(t, nil, nil)
	require.True(t, h.r.install())

	h.write(t, "2026 DEBUG "+handshakeTarget+": connection handshake started connection_id="+testID2+"\n")
	h.write(t, "2026 WARN rc_x509_client: SELECT secret FROM t password=hunter2 api_key=abc\n")
	h.write(t, "\x1b[31mgarbage\x1b[0m\n\n")
	h.write(t, handshakeLine("not-a-uuid"))
	h.write(t, handshakeLine(testID1))

	// The reader is sequential, so the first message proves the lines before
	// it were consumed without being logged.
	msg := h.logger.next(t)
	require.Equal(t, "remote queries tunnel connection ready connection_id="+testID1+" execute_handler=false", msg)
	require.NotContains(t, msg, "hunter2")
	h.shutdown(t)
	h.logger.requireNone(t)
	require.Empty(t, h.logger.warnings())
}

func TestReporterBoundsLineLengthWithoutBlockingWriters(t *testing.T) {
	h := newHarness(t, nil, nil)
	require.True(t, h.r.install())

	// Far more than the pipe buffer and the line limit: the write would block
	// forever unless the reader keeps draining.
	huge := strings.Repeat("x", 10*maxSinkLineBytes)
	h.write(t, huge+" "+handshakeTarget+": connection handshake complete connection_id="+testID2+"\n")
	h.write(t, handshakeLine(testID1))

	require.Equal(t, "remote queries tunnel connection ready connection_id="+testID1+" execute_handler=false", h.logger.next(t))
	h.shutdown(t)
	h.logger.requireNone(t)
}

func TestReporterSetsDefaultRCLogWhenUnset(t *testing.T) {
	h := newHarness(t, nil, nil)
	require.True(t, h.r.install())
	require.Equal(t, defaultRCLog, h.env[rcLogEnv])
	require.Equal(t, "warn,rc_x509_client::connection::handler::delegate::fsm::handshaking=debug", defaultRCLog)
	require.Empty(t, h.logger.warnings())
	h.shutdown(t)
}

func TestReporterRespectsExistingRCLogAndWarnsOnce(t *testing.T) {
	h := newHarness(t, map[string]string{rcLogEnv: "trace"}, nil)
	require.True(t, h.r.install())
	require.Equal(t, "trace", h.env[rcLogEnv])
	warns := h.logger.warnings()
	require.Len(t, warns, 1)
	require.Contains(t, warns[0], "RC_LOG is already set")
	require.Contains(t, warns[0], "handshake lines may be missing")

	// It still reports when the existing filter lets the line through.
	h.write(t, handshakeLine(testID1))
	require.Contains(t, h.logger.next(t), "connection_id="+testID1)
	h.shutdown(t)
	require.Len(t, h.logger.warnings(), 1)
}

func TestReporterTreatsEmptyRCLogAsUnset(t *testing.T) {
	h := newHarness(t, map[string]string{rcLogEnv: ""}, nil)
	require.True(t, h.r.install())
	require.Equal(t, defaultRCLog, h.env[rcLogEnv])
	h.shutdown(t)
}

func TestReporterSinkAlreadySetWarnsOnceAndReportsNothing(t *testing.T) {
	alreadySet := errors.New("ddrc: log sink already installed for this process")
	h := newHarness(t, nil, alreadySet)
	require.False(t, h.r.install())
	require.Equal(t, 1, h.sinkCalls)
	warns := h.logger.warnings()
	require.Len(t, warns, 1)
	require.Contains(t, warns[0], "not reporting the connection ID")
	require.Contains(t, warns[0], alreadySet.Error())

	// Nothing was started and RC_LOG is back to what it was.
	require.Empty(t, h.intervals)
	require.NotContains(t, h.env, rcLogEnv)
	select {
	case <-h.r.readerDone:
		t.Fatal("reader goroutine must not run when the sink was not installed")
	default:
	}

	// Closing is safe and idempotent, and nothing is ever reported.
	h.r.close()
	h.r.close()
	h.logger.requireNone(t)
	require.Len(t, h.logger.warnings(), 1)
}

func TestReporterFailedInstallKeepsExistingRCLog(t *testing.T) {
	h := newHarness(t, map[string]string{rcLogEnv: "info"}, errors.New("boom"))
	require.False(t, h.r.install())
	require.Equal(t, "info", h.env[rcLogEnv])
	require.Len(t, h.logger.warnings(), 2) // already set, then the failure
}

func TestReporterStopsReportingAfterClose(t *testing.T) {
	h := newHarness(t, nil, nil)
	require.True(t, h.r.install())
	h.write(t, handshakeLine(testID1))
	require.Contains(t, h.logger.next(t), testID1)

	h.shutdown(t)
	h.r.observe(testID2)
	h.logger.requireNone(t)
	h.r.close() // idempotent
}

func TestNilReporterIsInert(_ *testing.T) {
	var r *connectionReporter
	r.markExecuteHandlerRegistered()
	r.close()
}

type closeRecorder struct {
	rcx509client.Client
	closed int
}

func (c *closeRecorder) Close() error { c.closed++; return nil }

func TestReportingClientClosesTheReporterWithTheClient(t *testing.T) {
	h := newHarness(t, nil, nil)
	require.True(t, h.r.install())
	inner := &closeRecorder{}
	c := &reportingClient{Client: inner, reporter: h.r}

	require.NoError(t, c.Close())
	require.Equal(t, 1, inner.closed)
	select {
	case <-h.r.tickerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("ticker goroutine did not stop with the client")
	}
	select {
	case <-h.r.readerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("reader goroutine did not stop with the client")
	}
}

func TestRealTickerUsesTheTickerChannel(t *testing.T) {
	tk := newRealTicker(time.Hour)
	defer tk.Stop()
	require.NotNil(t, tk.C())
}
