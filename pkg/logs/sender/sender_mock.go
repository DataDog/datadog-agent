// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sender

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

// NewMockSender generates a mock sender
func NewMockSender() *Mock {
	return &Mock{
		inChan:  make(chan *message.Payload, 1),
		monitor: metrics.NewNoopPipelineMonitor("mock_monitor"),
	}
}

// Mock represents a mocked sender that fulfills the pipeline component interface
type Mock struct {
	inChan  chan *message.Payload
	monitor metrics.PipelineMonitor
}

// In returns a self-emptying chan
func (s *Mock) In() chan *message.Payload {
	return s.inChan
}

// PipelineMonitor returns an instance of NoopPipelineMonitor
func (s *Mock) PipelineMonitor() metrics.PipelineMonitor {
	return s.monitor
}

// Start begins the routine that empties the In channel
func (s *Mock) Start() {
	go func() {
		for range s.inChan { //revive:disable-line:empty-block
		}
	}()
}

// Stop closes the in channel
func (s *Mock) Stop() {
	close(s.inChan)

}
