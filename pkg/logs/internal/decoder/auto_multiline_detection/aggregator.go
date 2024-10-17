// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"bytes"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

type bucket struct {
	tagTruncatedLogs bool
	tagMultiLineLogs bool

	message         *message.Message
	originalDataLen int
	buffer          *bytes.Buffer
	lineCount       int
	truncated       bool
}

func (b *bucket) add(msg *message.Message) {
	if b.message == nil {
		b.message = msg
	}
	if b.originalDataLen > 0 {
		b.buffer.Write(message.EscapedLineFeed)
	}
	b.buffer.Write(msg.GetContent())
	b.originalDataLen += msg.RawDataLen
	b.lineCount++
}

func (b *bucket) isEmpty() bool {
	return b.originalDataLen == 0
}

func (b *bucket) truncate(addLine bool) {
	b.buffer.Write(message.TruncatedFlag)
	b.truncated = true
	// Adds a line to the counter (even though it's not a real line)
	// This is to ensure that the truncated flag is counted as a line when truncating multiline logs.
	if addLine {
		b.lineCount++
	}
}

func (b *bucket) flush() *message.Message {
	defer func() {
		b.buffer.Reset()
		b.message = nil
		b.lineCount = 0
		b.originalDataLen = 0
		b.truncated = false
	}()

	data := bytes.TrimSpace(b.buffer.Bytes())
	content := make([]byte, len(data))
	copy(content, data)

	msg := message.NewRawMessage(content, b.message.Status, b.originalDataLen, b.message.ParsingExtra.Timestamp)
	tlmTags := []string{"false", "single_line"}
	logsTag := "single_line"

	if b.lineCount > 1 {
		msg.ParsingExtra.IsMultiLine = true
		tlmTags[1] = "multi_line"
		logsTag = "auto_multiline"
		if b.tagMultiLineLogs {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.MultiLineSourceTag("auto_multiline"))
		}
	}

	if b.truncated {
		msg.ParsingExtra.IsTruncated = true
		tlmTags[0] = "true"
		if b.tagTruncatedLogs {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.TruncatedReasonTag(logsTag))
		}
	}

	metrics.TlmAutoMultilineAggregatorFlush.Inc(tlmTags...)
	return msg
}

// Aggregator aggregates multiline logs with a given label.
type Aggregator struct {
	outputFn           func(m *message.Message)
	bucket             *bucket
	maxContentSize     int
	flushTimeout       time.Duration
	flushTimer         *time.Timer
	multiLineMatchInfo *status.CountInfo
	linesCombinedInfo  *status.CountInfo
}

// NewAggregator creates a new aggregator.
func NewAggregator(outputFn func(m *message.Message), maxContentSize int, flushTimeout time.Duration, tagTruncatedLogs bool, tagMultiLineLogs bool, tailerInfo *status.InfoRegistry) *Aggregator {
	multiLineMatchInfo := status.NewCountInfo("MultiLine matches")
	linesCombinedInfo := status.NewCountInfo("Lines Combined")
	tailerInfo.Register(multiLineMatchInfo)
	tailerInfo.Register(linesCombinedInfo)

	return &Aggregator{
		outputFn:           outputFn,
		bucket:             &bucket{buffer: bytes.NewBuffer(nil), tagTruncatedLogs: tagTruncatedLogs, tagMultiLineLogs: tagMultiLineLogs},
		maxContentSize:     maxContentSize,
		flushTimeout:       flushTimeout,
		multiLineMatchInfo: multiLineMatchInfo,
		linesCombinedInfo:  linesCombinedInfo,
	}
}

// Aggregate aggregates a multiline log using a label.
func (a *Aggregator) Aggregate(msg *message.Message, label Label) {

	a.stopFlushTimerIfNeeded()
	defer a.startFlushTimerIfNeeded()

	// If `noAggregate` - flush the bucket immediately and then flush the next message.
	if label == noAggregate {
		a.Flush()
		a.bucket.add(msg)
		a.Flush()
		return
	}

	// If `aggregate` and the bucket is empty - flush the next message.
	if label == aggregate && a.bucket.isEmpty() {
		a.bucket.add(msg)
		a.Flush()
		return
	}

	// If `startGroup` - flush the bucket.
	if label == startGroup {
		a.multiLineMatchInfo.Add(1)
		a.Flush()
	}

	// At this point we either have `startGroup` with an empty bucket or `aggregate` with a non-empty bucket
	// so we add the message to the bucket or flush if the bucket will overflow the max content size.
	if msg.RawDataLen+a.bucket.buffer.Len() >= a.maxContentSize && !a.bucket.isEmpty() {
		a.bucket.truncate(true) // Truncate the end of the current bucket
		a.Flush()
		a.bucket.truncate(true) // Truncate the start of the next bucket
	}

	if !a.bucket.isEmpty() {
		a.linesCombinedInfo.Add(1)
	}

	a.bucket.add(msg)
}

func (a *Aggregator) stopFlushTimerIfNeeded() {
	if a.flushTimer == nil || a.bucket.isEmpty() {
		return
	}
	// stop the flush timer, as we now have data
	if !a.flushTimer.Stop() {
		<-a.flushTimer.C
	}
}

func (a *Aggregator) startFlushTimerIfNeeded() {
	if a.bucket.isEmpty() {
		return
	}
	// since there's buffered data, start the flush timer to flush it
	if a.flushTimer == nil {
		a.flushTimer = time.NewTimer(a.flushTimeout)
	} else {
		a.flushTimer.Reset(a.flushTimeout)
	}
}

// FlushChan returns the flush timer channel.
func (a *Aggregator) FlushChan() <-chan time.Time {
	if a.flushTimer != nil {
		return a.flushTimer.C
	}
	return nil
}

// Flush flushes the aggregator.
func (a *Aggregator) Flush() {
	if a.bucket.isEmpty() {
		return
	}
	// If the bucket is not truncated, but reaches the maximum size, truncated it.
	// This ensures single line logs are truncated exactly the same way as the single_line_handler.
	if !a.bucket.truncated && a.bucket.buffer.Len() >= a.maxContentSize {
		a.bucket.truncate(false)
	}
	a.outputFn(a.bucket.flush())
}
