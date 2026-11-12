// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sender provides log message sending functionality
package sender

import (
	"context"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const mainBatch string = "main"
const mrfBatch string = "mrf"

// batchStrategy contains all the logic to send logs in batch.
type batchStrategy struct {
	inputChan      chan *message.Message
	outputChan     chan *message.Payload
	flushChan      chan struct{}
	flushRequests  chan chan struct{} // see Flush
	serverlessMeta ServerlessMeta
	// pipelineName provides a name for the strategy to differentiate it from other instances in other internal pipelines
	pipelineName   string
	batchWait      time.Duration
	stopChan       chan struct{} // closed when the goroutine has finished
	clock          clock.Clock
	maxBatchSize   int
	maxContentSize int
	compression    compression.Compressor
	batches        map[string]*batch

	// Telemetry
	pipelineMonitor metrics.PipelineMonitor
	utilization     metrics.UtilizationMonitor
	instanceID      string
}

// NewBatchStrategy returns a new batch concurrent strategy with the specified batch & content size limits
func NewBatchStrategy(
	inputChan chan *message.Message,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	serverlessMeta ServerlessMeta,
	batchWait time.Duration,
	maxBatchSize int,
	maxContentSize int,
	pipelineName string,
	compression compression.Compressor,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string,
) Strategy {
	return newBatchStrategyWithClock(inputChan, outputChan, flushChan, serverlessMeta, batchWait, maxBatchSize, maxContentSize, pipelineName, clock.New(), compression, pipelineMonitor, instanceID)
}

func newBatchStrategyWithClock(
	inputChan chan *message.Message,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	serverlessMeta ServerlessMeta,
	batchWait time.Duration,
	maxBatchSize int,
	maxContentSize int,
	pipelineName string,
	clock clock.Clock,
	compression compression.Compressor,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string,
) Strategy {
	return &batchStrategy{
		inputChan:       inputChan,
		outputChan:      outputChan,
		flushChan:       flushChan,
		flushRequests:   make(chan chan struct{}),
		serverlessMeta:  serverlessMeta,
		batchWait:       batchWait,
		compression:     compression,
		stopChan:        make(chan struct{}),
		pipelineName:    pipelineName,
		clock:           clock,
		pipelineMonitor: pipelineMonitor,
		utilization:     pipelineMonitor.MakeUtilizationMonitor(metrics.StrategyTlmName, instanceID),
		maxBatchSize:    maxBatchSize,
		maxContentSize:  maxContentSize,
		instanceID:      instanceID,
		batches:         make(map[string]*batch),
	}
}

// Stop flushes the buffer and stops the strategy
func (s *batchStrategy) Stop() {
	close(s.inputChan)
	<-s.stopChan
}

// Start reads the incoming messages and forwards them to the appropriate batch
func (s *batchStrategy) Start() {
	go func() {
		flushTicker := s.clock.Ticker(s.batchWait)
		defer func() {
			s.flushAllBatches("shutdown")
			flushTicker.Stop()
			close(s.stopChan)
		}()
		for {
			select {
			case m, isOpen := <-s.inputChan:

				if !isOpen {
					// inputChan has been closed, no more payloads are expected
					return
				}

				s.processMessage(m)
			case <-flushTicker.C:
				// flush the payloads at a regular interval so pending messages don't wait here for too long.
				s.flushAllBatches("timer")
			case <-s.flushChan:
				// flush payloads on demand, used for infrequently running serverless functions
				s.flushAllBatches("flush")
			case done := <-s.flushRequests:
				// Include messages already queued, since select may pick this case before them,
				// then signal Flush that the payloads have been handed to the sender.
				s.drainInput()
				s.flushAllBatches("flush")
				close(done)
			}
		}
	}()
}

// Flush flushes the buffered messages, including those already queued on the input channel,
// and returns once the resulting payloads have been handed to the output channel or ctx is done.
// Unlike a send on flushChan, a caller waiting for in-flight sends after Flush returns also
// waits for these payloads.
func (s *batchStrategy) Flush(ctx context.Context) {
	done := make(chan struct{})
	select {
	case s.flushRequests <- done:
	case <-ctx.Done():
		return
	}
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// processMessage adds the message to the multi-region failover batch if it may be sent to the failover
// destination, or to the main batch otherwise.
func (s *batchStrategy) processMessage(m *message.Message) {
	if m.IsMRFAllow {
		s.getBatch(mrfBatch).processMessage(m, s.outputChan)
	} else {
		s.getBatch(mainBatch).processMessage(m, s.outputChan)
	}
}

// drainInput processes the messages already queued on inputChan without waiting for more.
func (s *batchStrategy) drainInput() {
	for {
		select {
		case m, isOpen := <-s.inputChan:
			if !isOpen {
				return
			}
			s.processMessage(m)
		default:
			return
		}
	}
}

func (s *batchStrategy) getBatch(key string) *batch {
	if b, exists := s.batches[key]; exists {
		return b
	}

	log.Debugf("Creating batch for key: %s", key)
	s.batches[key] = makeBatch(s.compression, s.maxBatchSize, s.maxContentSize, s.pipelineName, s.serverlessMeta, s.pipelineMonitor, s.utilization, s.instanceID)
	return s.batches[key]
}

func (s *batchStrategy) flushAllBatches(reason string) {
	for _, batch := range s.batches {
		batch.flushBuffer(s.outputChan, reason)
	}
}
