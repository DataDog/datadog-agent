// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package decoder

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const header = "HEADER"

type MockLineHandler struct {
	ch chan *message.Message
}

func NewMockLineHandler() *MockLineHandler {
	return &MockLineHandler{
		ch: make(chan *message.Message, 20),
	}
}

func (m *MockLineHandler) process(msg *message.Message) {
	m.ch <- msg
}

func (m *MockLineHandler) flushChan() <-chan time.Time {
	return nil
}
func (m *MockLineHandler) flush() {}

type MockFailingParser struct {
	header []byte
}

func NewMockFailingParser(header string) parsers.Parser {
	return &MockFailingParser{header: []byte(header)}
}

// Parse removes header from line, returns a message if its header matches the Parser header
// or returns an error and flags the line as partial if it does not end up by \n
func (u *MockFailingParser) Parse(input *message.Message) (*message.Message, error) {
	if bytes.HasPrefix(input.GetContent(), u.header) {
		content := bytes.Replace(input.GetContent(), u.header, []byte(""), 1)
		l := len(content)
		if l > 1 && content[l-2] == '\\' && content[l-1] == 'n' {
			msg := message.NewMessage(content[:l-2], nil, "", 0)
			return msg, nil
		}
		msg := message.NewMessage(content, nil, "", 0)
		msg.ParsingExtra = message.ParsingExtra{
			IsPartial: true,
		}
		return msg, nil
	}
	msg := message.NewMessage(input.GetContent(), nil, "", 0)
	return msg, errors.New("error")
}

func (u *MockFailingParser) SupportsPartialLine() bool {
	return true
}

func TestSingleLineParser(t *testing.T) {
	p := NewMockFailingParser(header)

	lineHandler := NewMockLineHandler()
	lineParser := NewSingleLineParser(lineHandler, p)

	line := header
	logMessage := message.NewMessage([]byte(line), nil, "", 0)

	inputLen := len(logMessage.GetContent()) + 1
	lineParser.process(logMessage, inputLen)
	message := <-lineHandler.ch
	assert.Equal(t, "", string(message.GetContent()))
	assert.Equal(t, inputLen, message.RawDataLen)

	logMessage.SetContent([]byte(line + "one message"))
	inputLen = len(logMessage.GetContent()) + 1
	lineParser.process(logMessage, inputLen)
	message = <-lineHandler.ch
	assert.Equal(t, []byte("one message"), message.GetContent())
	assert.Equal(t, inputLen, message.RawDataLen)
}

func TestSingleLineParserSendsRawInvalidMessages(t *testing.T) {
	p := NewMockFailingParser(header)

	lineHandler := NewMockLineHandler()
	lineParser := NewSingleLineParser(lineHandler, p)

	logMessage := message.NewMessage([]byte("one message"), nil, "", 0)

	lineParser.process(logMessage, 12)
	message := <-lineHandler.ch
	assert.Equal(t, "one message", string(message.GetContent()))
}

func TestMultilineParser(t *testing.T) {
	p := NewMockFailingParser(header)
	timeout := 1000 * time.Millisecond
	contentLenLimit := 256 * 100

	lineHandler := NewMockLineHandler()
	lineParser := NewMultiLineParser(lineHandler, timeout, p, contentLenLimit)

	logMessage := message.NewMessage([]byte(header+"one "), nil, "", 0)

	lineParser.process(logMessage, 11)

	logMessage.SetContent([]byte(header + "long "))
	lineParser.process(logMessage, 12)

	logMessage.SetContent([]byte(header + "line\\n"))
	lineParser.process(logMessage, 14)

	message := <-lineHandler.ch

	assert.Equal(t, "one long line", string(message.GetContent()))
	assert.Equal(t, message.RawDataLen, 11+12+14)
}

func TestMultilineParserForwardsCompleteEmptyLine(t *testing.T) {
	// A complete (non-partial) blank line must be forwarded to the line
	// handler rather than dropped. Downstream aggregators (e.g. the Go stack
	// trace aggregator) rely on observing the blank line that separates a
	// panic header from its goroutine block. Regression test for blank lines
	// being silently swallowed on the partial-line parser path used by
	// container/CRI log sources.
	p := NewMockFailingParser(header)
	timeout := 1000 * time.Millisecond
	contentLenLimit := 256 * 100

	lineHandler := NewMockLineHandler()
	lineParser := NewMultiLineParser(lineHandler, timeout, p, contentLenLimit)

	// empty content, but a non-zero raw length (the trailing newline)
	logMessage := message.NewMessage([]byte(""), nil, "", 0)
	lineParser.process(logMessage, 1)

	select {
	case msg := <-lineHandler.ch:
		assert.Equal(t, "", string(msg.GetContent()))
		assert.Equal(t, 1, msg.RawDataLen)
	case <-time.After(time.Second):
		t.Fatal("expected the complete blank line to be forwarded to the line handler")
	}
}

func TestMultilineParserForwardsBlankLineBetweenAggregatedLines(t *testing.T) {
	// A blank line arriving between two aggregated (partial) lines must be
	// forwarded on its own once complete, preserving the blank line for
	// downstream aggregators instead of collapsing it away.
	p := NewMockFailingParser(header)
	timeout := 1000 * time.Millisecond
	contentLenLimit := 256 * 100

	lineHandler := NewMockLineHandler()
	lineParser := NewMultiLineParser(lineHandler, timeout, p, contentLenLimit)

	// complete line "foo"
	logMessage := message.NewMessage([]byte(header+"foo\\n"), nil, "", 0)
	lineParser.process(logMessage, 5)
	msg := <-lineHandler.ch
	assert.Equal(t, "foo", string(msg.GetContent()))

	// complete blank line
	logMessage = message.NewMessage([]byte(""), nil, "", 0)
	lineParser.process(logMessage, 1)
	msg = <-lineHandler.ch
	assert.Equal(t, "", string(msg.GetContent()))
	assert.Equal(t, 1, msg.RawDataLen)

	// complete line "bar"
	logMessage = message.NewMessage([]byte(header+"bar\\n"), nil, "", 0)
	lineParser.process(logMessage, 5)
	msg = <-lineHandler.ch
	assert.Equal(t, "bar", string(msg.GetContent()))
}

func TestMultilineParserTimeout(t *testing.T) {
	p := NewMockFailingParser(header)
	timeout := 100 * time.Millisecond
	contentLenLimit := 256 * 100

	lineHandler := NewMockLineHandler()
	lineParser := NewMultiLineParser(lineHandler, timeout, p, contentLenLimit)

	logMessage := message.NewMessage([]byte(header+"message"), nil, "", 0)

	lineParser.process(logMessage, 14)

	// shouldn't be anything here yet
	select {
	case <-lineHandler.ch:
		panic("shouldn't be a message")
	default:
	}

	lineParser.buffers[""].deadline = time.Now().Add(-time.Second)
	lineParser.flushTimedOut()

	message := <-lineHandler.ch

	assert.Equal(t, "message", string(message.GetContent()))
	assert.Equal(t, message.RawDataLen, 14)
	assert.Equal(t, 14, message.RawDataLenForCheckpoint())
}

func TestMultilineParserCompleteLineImmediatelyAdvancesCheckpoint(t *testing.T) {
	lineHandler := NewMockLineHandler()
	lineParser := NewMultiLineParser(lineHandler, time.Hour, noop.New(), 256*100)

	stderr := message.NewMessage([]byte("stderr"), nil, "", 0)
	stderr.ParsingExtra.Stream = message.StreamStderr
	lineParser.process(stderr, 6)

	complete := <-lineHandler.ch
	assert.Equal(t, "stderr", string(complete.GetContent()))
	assert.Equal(t, 6, complete.RawDataLenForCheckpoint())
	assert.Empty(t, lineParser.buffers)
}

func TestMultilineParserCompleteLineDoesNotExtendPartialStreamDeadline(t *testing.T) {
	lineHandler := NewMockLineHandler()
	lineParser := NewMultiLineParser(lineHandler, time.Hour, noop.New(), 256*100)

	stdout := message.NewMessage([]byte("stdout partial"), nil, "", 0)
	stdout.ParsingExtra.Stream = message.StreamStdout
	stdout.ParsingExtra.IsPartial = true
	lineParser.process(stdout, 14)
	stdoutDeadline := lineParser.buffers[message.StreamStdout].deadline

	stderr := message.NewMessage([]byte("stderr complete"), nil, "", 0)
	stderr.ParsingExtra.Stream = message.StreamStderr
	lineParser.process(stderr, 15)
	complete := <-lineHandler.ch
	assert.Zero(t, complete.RawDataLenForCheckpoint())
	assert.Equal(t, stdoutDeadline, lineParser.buffers[message.StreamStdout].deadline)

	lineParser.buffers[message.StreamStdout].deadline = time.Now().Add(-time.Second)
	lineParser.flushTimedOut()
	partial := <-lineHandler.ch
	assert.Equal(t, "stdout partial", string(partial.GetContent()))
	assert.Equal(t, 29, partial.RawDataLenForCheckpoint())
}

func TestMultilineParserFlushesOnlyExpiredStream(t *testing.T) {
	timeout := time.Hour
	lineHandler := NewMockLineHandler()
	lineParser := NewMultiLineParser(lineHandler, timeout, noop.New(), 256*100)

	stderr := message.NewMessage([]byte("stderr"), nil, "", 0)
	stderr.ParsingExtra.Stream = message.StreamStderr
	stderr.ParsingExtra.IsPartial = true
	lineParser.process(stderr, 6)

	stdout := message.NewMessage([]byte("stdout"), nil, "", 0)
	stdout.ParsingExtra.Stream = message.StreamStdout
	stdout.ParsingExtra.IsPartial = true
	lineParser.process(stdout, 6)

	now := time.Now()
	lineParser.buffers[message.StreamStderr].deadline = now.Add(-time.Second)
	lineParser.buffers[message.StreamStdout].deadline = now.Add(time.Hour)
	lineParser.flushTimedOut()

	flushed := <-lineHandler.ch
	assert.Equal(t, "stderr", string(flushed.GetContent()))
	assert.Equal(t, message.StreamStderr, flushed.ParsingExtra.Stream)
	assert.Zero(t, flushed.RawDataLenForCheckpoint())
	select {
	case msg := <-lineHandler.ch:
		t.Fatalf("unexpected unexpired stream flush: %q", msg.GetContent())
	default:
	}

	lineParser.flush()
	remaining := <-lineHandler.ch
	assert.Equal(t, "stdout", string(remaining.GetContent()))
	assert.Equal(t, message.StreamStdout, remaining.ParsingExtra.Stream)
	assert.Equal(t, 12, remaining.RawDataLenForCheckpoint())
}

func TestMultilineParserLimitIsIndependentPerStream(t *testing.T) {
	timeout := time.Hour
	lineHandler := NewMockLineHandler()
	lineParser := NewMultiLineParser(lineHandler, timeout, noop.New(), 5)

	stderr := message.NewMessage([]byte("1234"), nil, "", 0)
	stderr.ParsingExtra.Stream = message.StreamStderr
	stderr.ParsingExtra.IsPartial = true
	lineParser.process(stderr, 4)

	stdout := message.NewMessage([]byte("a"), nil, "", 0)
	stdout.ParsingExtra.Stream = message.StreamStdout
	stdout.ParsingExtra.IsPartial = true
	lineParser.process(stdout, 1)

	stderr = message.NewMessage([]byte("56"), nil, "", 0)
	stderr.ParsingExtra.Stream = message.StreamStderr
	stderr.ParsingExtra.IsPartial = true
	lineParser.process(stderr, 2)

	truncated := <-lineHandler.ch
	assert.Equal(t, "123456", string(truncated.GetContent()))
	assert.Equal(t, message.StreamStderr, truncated.ParsingExtra.Stream)
	assert.True(t, truncated.ParsingExtra.IsTruncated)
	assert.Equal(t, 6, truncated.RawDataLen)
	assert.Zero(t, truncated.RawDataLenForCheckpoint())

	stdout = message.NewMessage([]byte("b"), nil, "", 0)
	stdout.ParsingExtra.Stream = message.StreamStdout
	lineParser.process(stdout, 1)

	complete := <-lineHandler.ch
	assert.Equal(t, "ab", string(complete.GetContent()))
	assert.Equal(t, message.StreamStdout, complete.ParsingExtra.Stream)
	assert.False(t, complete.ParsingExtra.IsTruncated)
	assert.Equal(t, 2, complete.RawDataLen)
	assert.Equal(t, 8, complete.RawDataLenForCheckpoint())
}

func TestMultilineParserLimit(t *testing.T) {
	// Allow buffering to ensure the line_parser does not timeout
	p := NewMockFailingParser(header)
	timeout := 1000 * time.Millisecond
	contentLenLimit := 64
	line := strings.Repeat("a", contentLenLimit)

	lineHandler := NewMockLineHandler()
	lineParser := NewMultiLineParser(lineHandler, timeout, p, contentLenLimit)

	for i := 0; i < 10; i++ {
		logMessage := message.NewMessage([]byte(header+line), nil, "", 0)
		lineParser.process(logMessage, 7+len(line))
	}

	logMessage := message.NewMessage([]byte(header+"aaaa\\n"), nil, "", 0)
	lineParser.process(logMessage, 13)

	// oversized chunks: raw content (no flag bytes), IsTruncated=true
	// SingleLineHandler adds ...TRUNCATED... markers downstream
	for i := 0; i < 10; i++ {
		msg := <-lineHandler.ch
		assert.Equal(t, line, string(msg.GetContent()))
		assert.True(t, msg.ParsingExtra.IsTruncated)
		assert.Equal(t, msg.RawDataLen, 7+len(line))
	}

	// final short chunk: raw content, IsTruncated=false (not oversized itself)
	msg := <-lineHandler.ch
	assert.Equal(t, "aaaa", string(msg.GetContent()))
	assert.False(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.RawDataLen, 13)

	// next normal message stays clean
	logMessage = message.NewMessage([]byte(header+"clean\\n"), nil, "", 0)
	lineParser.process(logMessage, 14)

	msg = <-lineHandler.ch
	assert.Equal(t, "clean", string(msg.GetContent()))
	assert.False(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.RawDataLen, 14)
}
