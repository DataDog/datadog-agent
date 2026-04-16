// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StackTraceParser defines language-specific stack trace recognition.
// Implementations are stateful: Reset() begins a new trace, then AcceptLine()
// validates each continuation line, and ShouldCombine() decides the outcome.
type StackTraceParser interface {
	// IsStart returns true if the line begins a new stack trace (stateless).
	IsStart(content []byte) bool
	// Reset prepares the parser to validate a new trace.
	Reset()
	// AcceptLine returns true if the line is a valid continuation of the
	// current trace. Must only be called after Reset().
	AcceptLine(line []byte) bool
	// ShouldCombine returns true if the lines accepted so far constitute a
	// real stack trace worth combining (e.g. at least one goroutine block).
	ShouldCombine() bool
	// Uncommitted returns the number of trailing accepted lines that have not
	// yet been confirmed as structurally valid. On resolve the aggregator
	// strips these from the combined message and emits them individually.
	Uncommitted() int
}

// stackTraceParserRegistry maps parser names (used in configuration) to their
// constructors. To add a new language parser, add one entry here and implement
// the StackTraceParser interface.
var stackTraceParserRegistry = map[string]func() StackTraceParser{
	"go": func() StackTraceParser { return NewGoStackTraceParser() },
}

// StackTraceAggregator is the interface for the stack trace aggregation stage
// of the Preprocessor. It sits between the JSONAggregator and the
// Tokenizer/Labeler, buffering stack trace lines and combining them into a
// single message before downstream stages see them.
type StackTraceAggregator interface {
	Process(msg *message.Message) []*message.Message
	Flush() []*message.Message
	IsEmpty() bool
}

// stackTraceAggregator buffers lines that belong to a stack trace and combines
// them into a single message when the trace ends or a non-continuation line
// arrives. Language-specific recognition is delegated to the StackTraceParser.
type stackTraceAggregator struct {
	parser         StackTraceParser
	messageBuf     []*message.Message
	buffer         *bytes.Buffer
	rawDataLen     int
	maxContentSize int
	collected      []*message.Message
}

// NewStackTraceAggregator creates a new StackTraceAggregator using the given
// parser for language-specific pattern recognition.
func NewStackTraceAggregator(parser StackTraceParser, maxContentSize int) StackTraceAggregator {
	return &stackTraceAggregator{
		parser:         parser,
		buffer:         bytes.NewBuffer(nil),
		maxContentSize: maxContentSize,
	}
}

// NewStackTraceAggregatorFromNames builds a StackTraceAggregator from a list
// of parser names (e.g. ["go", "java"]). Unknown names are silently skipped.
// Returns a NoopStackTraceAggregator if no valid parsers are resolved.
func NewStackTraceAggregatorFromNames(names []string, maxContentSize int) StackTraceAggregator {
	var parsers []StackTraceParser
	for _, name := range names {
		if ctor, ok := stackTraceParserRegistry[name]; ok {
			parsers = append(parsers, ctor())
		} else {
			log.Warnf("Unknown stack trace parser %q in logs_config.auto_multi_line.stack_trace_parsers, skipping", name)
		}
	}
	if len(parsers) == 0 {
		return NewNoopStackTraceAggregator()
	}
	if len(parsers) == 1 {
		return NewStackTraceAggregator(parsers[0], maxContentSize)
	}
	return NewStackTraceAggregator(NewCompositeStackTraceParser(parsers), maxContentSize)
}

// Process handles one incoming message.
func (s *stackTraceAggregator) Process(msg *message.Message) []*message.Message {
	s.collected = s.collected[:0]
	content := msg.GetContent()

	if s.isBuffering() {
		if s.parser.AcceptLine(content) {
			if s.buffer.Len()+len(message.EscapedLineFeed)+len(content) >= s.maxContentSize {
				s.abandonWithTag("overflow")
				s.collected = append(s.collected, msg)
				return s.collected
			}
			s.appendToBuffer(msg)
			return s.collected
		}
		s.resolve()
		if s.parser.IsStart(content) {
			s.startBuffering(msg)
		} else {
			s.collected = append(s.collected, msg)
		}
		return s.collected
	}

	if s.parser.IsStart(content) {
		s.startBuffering(msg)
		return s.collected
	}

	s.collected = append(s.collected, msg)
	return s.collected
}

// Flush returns any buffered trace, resolved by the parser's ShouldCombine.
func (s *stackTraceAggregator) Flush() []*message.Message {
	if !s.isBuffering() {
		return nil
	}
	s.collected = s.collected[:0]
	s.resolve()
	return s.collected
}

// IsEmpty reports whether the aggregator has no buffered state.
func (s *stackTraceAggregator) IsEmpty() bool {
	return !s.isBuffering()
}

func (s *stackTraceAggregator) isBuffering() bool {
	return len(s.messageBuf) > 0
}

func (s *stackTraceAggregator) startBuffering(msg *message.Message) {
	s.messageBuf = append(s.messageBuf[:0], msg)
	s.buffer.Reset()
	s.buffer.Write(msg.GetContent())
	s.rawDataLen = msg.RawDataLen
	s.parser.Reset()
}

func (s *stackTraceAggregator) appendToBuffer(msg *message.Message) {
	s.messageBuf = append(s.messageBuf, msg)
	s.buffer.Write(message.EscapedLineFeed)
	s.buffer.Write(msg.GetContent())
	s.rawDataLen += msg.RawDataLen
}

func (s *stackTraceAggregator) resolve() {
	if len(s.messageBuf) == 0 {
		return
	}
	rollback := s.parser.Uncommitted()
	var tail []*message.Message
	if rollback > 0 && rollback < len(s.messageBuf) {
		tail = make([]*message.Message, rollback)
		copy(tail, s.messageBuf[len(s.messageBuf)-rollback:])
		s.messageBuf = s.messageBuf[:len(s.messageBuf)-rollback]
		s.rebuildBuffer()
	}
	if s.parser.ShouldCombine() && len(s.messageBuf) > 1 {
		s.combine()
	} else {
		s.abandon()
	}
	if len(tail) > 0 {
		s.collected = append(s.collected, tail...)
	}
}

func (s *stackTraceAggregator) rebuildBuffer() {
	s.buffer.Reset()
	s.rawDataLen = 0
	for i, m := range s.messageBuf {
		if i > 0 {
			s.buffer.Write(message.EscapedLineFeed)
		}
		s.buffer.Write(m.GetContent())
		s.rawDataLen += m.RawDataLen
	}
}

func (s *stackTraceAggregator) combine() {
	combined := make([]byte, s.buffer.Len())
	copy(combined, s.buffer.Bytes())

	out := s.messageBuf[0]
	out.SetContent(combined)
	out.RawDataLen = s.rawDataLen
	out.ParsingExtra.IsMultiLine = true

	s.collected = append(s.collected, out)
	s.resetBuffer()
	metrics.TlmAutoMultilineStackTraceAggregatorFlush.Inc("combined")
}

func (s *stackTraceAggregator) abandon() {
	s.abandonWithTag("abandoned")
}

func (s *stackTraceAggregator) abandonWithTag(tag string) {
	s.collected = append(s.collected, s.messageBuf...)
	s.resetBuffer()
	metrics.TlmAutoMultilineStackTraceAggregatorFlush.Inc(tag)
}

func (s *stackTraceAggregator) resetBuffer() {
	s.messageBuf = s.messageBuf[:0]
	s.buffer.Reset()
	s.rawDataLen = 0
}

// ---------------------------------------------------------------------------
// CompositeStackTraceParser — multiplexes N parsers
// ---------------------------------------------------------------------------

// CompositeStackTraceParser wraps multiple StackTraceParser implementations
// and delegates to whichever one matches the current trace. Only one parser
// is active per trace; the list order determines priority when multiple
// parsers match the same start line.
type CompositeStackTraceParser struct {
	parsers []StackTraceParser
	active  StackTraceParser
}

// NewCompositeStackTraceParser creates a composite parser from the given list.
// The list order determines priority for IsStart matching.
func NewCompositeStackTraceParser(parsers []StackTraceParser) *CompositeStackTraceParser {
	return &CompositeStackTraceParser{parsers: parsers}
}

// IsStart returns true if any child parser recognises the line as a trace
// start. The first match (by list order) is remembered for the subsequent
// Reset call.
func (c *CompositeStackTraceParser) IsStart(content []byte) bool {
	for _, p := range c.parsers {
		if p.IsStart(content) {
			c.active = p
			return true
		}
	}
	c.active = nil
	return false
}

// Reset prepares the active parser (set during the preceding IsStart) for a
// new trace.
func (c *CompositeStackTraceParser) Reset() {
	if c.active != nil {
		c.active.Reset()
	}
}

// AcceptLine delegates to the active parser.
func (c *CompositeStackTraceParser) AcceptLine(line []byte) bool {
	if c.active != nil {
		return c.active.AcceptLine(line)
	}
	return false
}

// ShouldCombine delegates to the active parser.
func (c *CompositeStackTraceParser) ShouldCombine() bool {
	if c.active != nil {
		return c.active.ShouldCombine()
	}
	return false
}

// Uncommitted delegates to the active parser.
func (c *CompositeStackTraceParser) Uncommitted() int {
	if c.active != nil {
		return c.active.Uncommitted()
	}
	return 0
}

// ---------------------------------------------------------------------------
// NoopStackTraceAggregator — pass-through
// ---------------------------------------------------------------------------

// NoopStackTraceAggregator is a pass-through that never buffers messages.
type NoopStackTraceAggregator struct {
	collected []*message.Message
}

// NewNoopStackTraceAggregator returns a new NoopStackTraceAggregator.
func NewNoopStackTraceAggregator() *NoopStackTraceAggregator {
	return &NoopStackTraceAggregator{}
}

// Process passes the message through unchanged.
func (n *NoopStackTraceAggregator) Process(msg *message.Message) []*message.Message {
	n.collected = append(n.collected[:0], msg)
	return n.collected
}

// Flush is a no-op.
func (n *NoopStackTraceAggregator) Flush() []*message.Message {
	return nil
}

// IsEmpty always returns true.
func (n *NoopStackTraceAggregator) IsEmpty() bool {
	return true
}
