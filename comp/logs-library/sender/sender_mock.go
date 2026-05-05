// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sender

import (
	"sync"

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

// NewMockServerlessMeta returns a new MockServerlessMeta
func NewMockServerlessMeta(isEnabled bool) *MockServerlessMeta {
	if isEnabled {
		return &MockServerlessMeta{
			wg:        &sync.WaitGroup{},
			doneChan:  make(chan *sync.WaitGroup),
			isEnabled: isEnabled,
		}
	}
	return &MockServerlessMeta{
		wg:        nil,
		doneChan:  nil,
		isEnabled: isEnabled,
	}
}

// MockServerlessMeta is a struct that contains essential control structures for serverless mode.
// Do not access any methods on this struct without checking IsEnabled first.
type MockServerlessMeta struct {
	wg        *sync.WaitGroup
	doneChan  chan *sync.WaitGroup
	isEnabled bool
}

// IsEnabled returns true if the serverless mode is enabled.
func (s *MockServerlessMeta) IsEnabled() bool {
	return s.isEnabled
}

// Lock is a no-op for the mock serverless meta.
func (s *MockServerlessMeta) Lock() {
}

// Unlock is a no-op for the mock serverless meta.
func (s *MockServerlessMeta) Unlock() {
}

// WaitGroup returns the wait group for the serverless mode.
func (s *MockServerlessMeta) WaitGroup() *sync.WaitGroup {
	return s.wg
}

// SenderDoneChan returns the channel is used to transfer wait groups from the sync_destination to the sender.
func (s *MockServerlessMeta) SenderDoneChan() chan *sync.WaitGroup {
	return s.doneChan
}
