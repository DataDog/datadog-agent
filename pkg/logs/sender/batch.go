// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sender provides log message sending functionality
package sender

import (
	"bytes"
	"io"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tlmDroppedTooLarge = telemetry.NewCounter("logs_sender_batch_strategy", "dropped_too_large", []string{"pipeline"}, "Number of payloads dropped due to being too large")
)

type batch struct {
	buffer         *MessageBuffer
	serializer     Serializer
	compression    compression.Compressor
	compressor     compression.StreamCompressor
	writeCounter   *writerCounter
	encodedPayload *bytes.Buffer
	// pipelineName provides a name for the strategy to differentiate it from other instances in other internal pipelines
	pipelineName   string
	serverlessMeta ServerlessMeta

	// Telemetry
	pipelineMonitor metrics.PipelineMonitor
	utilization     metrics.UtilizationMonitor
	instanceID      string
}

func makeBatch(
	compression compression.Compressor,
	maxBatchSize int,
	maxContentSize int,
	pipelineName string,
	serverlessMeta ServerlessMeta,
	pipelineMonitor metrics.PipelineMonitor,
	utilization metrics.UtilizationMonitor,
	instanceID string,
) *batch {
	var encodedPayload bytes.Buffer
	compressor := compression.NewStreamCompressor(&encodedPayload)
	wc := newWriterWithCounter(compressor)
	buffer := NewMessageBuffer(maxBatchSize, maxContentSize)
	serializer := NewArraySerializer()

	batch := &batch{
		buffer:          buffer,
		serializer:      serializer,
		compression:     compression,
		compressor:      compressor,
		writeCounter:    wc,
		encodedPayload:  &encodedPayload,
		pipelineName:    pipelineName,
		pipelineMonitor: pipelineMonitor,
		instanceID:      instanceID,
		utilization:     utilization,
		serverlessMeta:  serverlessMeta,
	}
	return batch
}

func (b *batch) resetBatch() {
	b.buffer.Clear()
	b.serializer.Reset()
	var encodedPayload bytes.Buffer
	compressor := b.compression.NewStreamCompressor(&encodedPayload)

	wc := newWriterWithCounter(compressor)
	b.writeCounter = wc
	b.compressor = compressor
	b.encodedPayload = &encodedPayload
}

func (b *batch) processMessage(m *message.Message, outputChan chan *message.Payload) {
	if m.Origin != nil {
		m.Origin.LogSource.LatencyStats.Add(m.GetLatency())
	}
	added, err := b.addMessage(m)
	if err != nil {
		log.Warn("Encoding failed - dropping payload", err)
		b.resetBatch()
		return
	}
	if !added || b.buffer.IsFull() {
		b.flushBuffer(outputChan)
	}
	if !added {
		// it's possible that the m could not be added because the buffer was full
		// so we need to retry once again
		added, err = b.addMessage(m)
		if err != nil {
			log.Warn("Encoding failed - dropping payload", err)
			b.resetBatch()
			return
		}
		if !added {
			log.Warnf("Dropped message in pipeline=%s reason=too-large ContentLength=%d ContentSizeLimit=%d", b.pipelineName, len(m.GetContent()), b.buffer.ContentSizeLimit())
			tlmDroppedTooLarge.Inc(b.pipelineName)
		}

	}
}

func (b *batch) addMessage(m *message.Message) (bool, error) {
	b.utilization.Start()
	defer b.utilization.Stop()

	if b.buffer.AddMessage(m) {
		err := b.serializer.Serialize(m, b.writeCounter)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// flushBuffer sends all of the messages that are stored in the buffer and
// forwards them to the the next stage of the pipeline
func (b *batch) flushBuffer(outputChan chan *message.Payload) {
	if b.buffer.IsEmpty() {
		return
	}

	b.utilization.Start()
	if err := b.serializer.Finish(b.writeCounter); err != nil {
		log.Warn("Encoding failed - dropping payload", err)
		b.resetBatch()
		b.utilization.Stop()
		return
	}

	messagesMetadata := b.buffer.GetMessages()
	b.buffer.Clear()
	// Logging specifically for DBM pipelines, which seem to fail to send more often than other pipelines.
	// pipelineName comes from epforwarder.passthroughPipelineDescs.eventType, and these names are constants in the epforwarder package.
	if b.pipelineName == "dbm-samples" || b.pipelineName == "dbm-metrics" || b.pipelineName == "dbm-activity" {
		log.Debugf("Flushing buffer and sending %d messages for pipeline %s", len(messagesMetadata), b.pipelineName)
	}
	b.sendMessages(messagesMetadata, outputChan)
}

func (b *batch) sendMessages(messagesMetadata []*message.MessageMetadata, outputChan chan *message.Payload) {
	defer b.resetBatch()

	if err := b.compressor.Close(); err != nil {
		log.Warn("Encoding failed - dropping payload", err)
		b.utilization.Stop()
		return
	}

	unencodedSize := b.writeCounter.getWrittenBytes()
	log.Debugf("Send messages for pipeline %s (msg_count:%d, content_size=%d, avg_msg_size=%.2f)", b.pipelineName, len(messagesMetadata), unencodedSize, float64(unencodedSize)/float64(len(messagesMetadata)))

	if b.serverlessMeta.IsEnabled() {
		// Increment the wait group so the flush doesn't finish until all payloads are sent to all destinations
		// The lock is needed to ensure that the wait group is not incremented while the flush is in progress
		b.serverlessMeta.Lock()
		b.serverlessMeta.WaitGroup().Add(1)
		b.serverlessMeta.Unlock()
	}

	p := message.NewPayload(messagesMetadata, b.encodedPayload.Bytes(), b.compression.ContentEncoding(), unencodedSize)

	b.utilization.Stop()
	outputChan <- p
	b.pipelineMonitor.ReportComponentEgress(p, metrics.StrategyTlmName, b.instanceID)
	b.pipelineMonitor.ReportComponentIngress(p, metrics.SenderTlmName, metrics.SenderTlmInstanceID)
}

// writerCounter is a simple io.Writer that counts the number of bytes written to it
type writerCounter struct {
	io.Writer
	counter int
}

func newWriterWithCounter(w io.Writer) *writerCounter {
	return &writerCounter{Writer: w}
}

// Write writes the given bytes and increments the counter
func (wc *writerCounter) Write(b []byte) (int, error) {
	n, err := wc.Writer.Write(b)
	wc.counter += n
	return n, err
}

// getWrittenBytes returns the number of bytes written to the writer
func (wc *writerCounter) getWrittenBytes() int {
	return wc.counter
}
