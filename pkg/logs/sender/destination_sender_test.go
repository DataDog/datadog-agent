// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package sender

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type mockDestination struct {
	input      chan *message.Payload
	output     chan *message.Payload
	isRetrying chan bool
	stopChan   chan struct{}
	isMRF      bool
}

func (m *mockDestination) IsMRF() bool {
	return m.isMRF
}

func (m *mockDestination) Target() string {
	return "mock-dest"
}

func (m *mockDestination) Start(input chan *message.Payload, output chan *message.Payload, isRetrying chan bool) (stopChan <-chan struct{}) {
	m.input = input
	m.output = output
	m.isRetrying = isRetrying
	m.stopChan = make(chan struct{})
	return m.stopChan
}

func newDestinationSenderWithBufferSize(bufferSize int) (*mockDestination, *DestinationSender) {
	output := make(chan *message.Payload)
	dest := &mockDestination{}
	cfg := getNewConfig()
	d := NewDestinationSender(cfg, dest, output, bufferSize)
	return dest, d
}

func newDestinationSenderWithConfigAndBufferSize(cfg pkgconfigmodel.Reader, bufferSize int) (*mockDestination, *DestinationSender) {
	output := make(chan *message.Payload)
	dest := &mockDestination{}
	dest.isMRF = cfg.GetBool("multi_region_failover.enabled")
	d := NewDestinationSender(cfg, dest, output, bufferSize)
	return dest, d
}

func TestDestinationSender(_ *testing.T) {
	dest, destSender := newDestinationSenderWithBufferSize(1)

	destSender.Send(&message.Payload{})

	<-dest.input

	didStop := make(chan struct{})
	go func() {
		<-dest.stopChan
		didStop <- struct{}{}
	}()
}

func TestDestinationSenderCanBeCanceled(t *testing.T) {
	dest, destSender := newDestinationSenderWithBufferSize(0)

	sendSucceeded := make(chan bool)

	// Send should block because input is full.
	go func() {
		sendSucceeded <- destSender.Send(&message.Payload{})
	}()
	// trigger a retry state change to unblock it
	dest.isRetrying <- true

	assert.False(t, <-sendSucceeded)
}

func TestDestinationSenderAlreadyRetrying(t *testing.T) {
	dest, destSender := newDestinationSenderWithBufferSize(0)
	dest.isRetrying <- true

	assert.False(t, destSender.Send(&message.Payload{}))
}

func TestDestinationSenderStopsRetrying(t *testing.T) {
	dest, destSender := newDestinationSenderWithBufferSize(0)
	dest.isRetrying <- true

	assert.False(t, destSender.Send(&message.Payload{}))

	dest.isRetrying <- false

	gotPayload := make(chan struct{})
	go func() {
		<-dest.input
		gotPayload <- struct{}{}

	}()

	// retry the send until it succeeds
	for !destSender.Send(&message.Payload{}) { //revive:disable-line:empty-block
	}

	<-gotPayload
}

func TestDestinationSenderDeadlock(_ *testing.T) {
	dest, destSender := newDestinationSenderWithBufferSize(100)

	go func() {
		for range dest.input { //revive:disable-line:empty-block
		}
	}()

	syn := make(chan struct{})
	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		<-syn
		for i := 0; i < 1000; i++ {
			dest.isRetrying <- false
		}
		wg.Done()
	}()

	go func() {
		<-syn
		for i := 0; i < 1000; i++ {
			destSender.Send(&message.Payload{})
		}
		wg.Done()
	}()

	close(syn)
	wg.Wait()
	close(dest.input)
}

func TestDestinationSenderDisabled(t *testing.T) {
	cfg := getNewConfig()
	cfg.SetWithoutSource("multi_region_failover.enabled", true)
	cfg.SetWithoutSource("multi_region_failover.failover_logs", false)

	dest, destSender := newDestinationSenderWithConfigAndBufferSize(cfg, 1)

	assert.True(t, destSender.Send(&message.Payload{}), "sender should always indicate success when disabled in MRF mode")
	assert.Len(t, dest.input, 0, "sender should not send anything when disabled")
}

func TestDestinationSenderDisabledToEnabled(t *testing.T) {
	cfg := getNewConfig()
	cfg.SetWithoutSource("multi_region_failover.enabled", true)
	cfg.SetWithoutSource("multi_region_failover.failover_logs", false)

	dest, destSender := newDestinationSenderWithConfigAndBufferSize(cfg, 1)

	assert.True(t, destSender.Send(&message.Payload{}), "sender should always indicate success when disabled in MRF mode")
	assert.Len(t, dest.input, 0, "sender should not send payload when disabled")

	cfg.SetWithoutSource("multi_region_failover.failover_logs", true)

	assert.True(t, destSender.Send(&message.Payload{}), "sender should have buffer space to accept payload when enabled")
	assert.Len(t, dest.input, 1, "sender should send payload when enabled")
}

func TestDestinationSenderEnabledToDisabled(t *testing.T) {
	cfg := getNewConfig()
	cfg.SetWithoutSource("multi_region_failover.enabled", true)
	cfg.SetWithoutSource("multi_region_failover.failover_logs", true)

	dest, destSender := newDestinationSenderWithConfigAndBufferSize(cfg, 1)

	assert.True(t, destSender.Send(&message.Payload{}), "sender should have buffer space to accept payload when enabled")
	assert.Len(t, dest.input, 1, "sender should send payload when enabled")

	// drain input channel and set to disabled
	<-dest.input
	cfg.SetWithoutSource("multi_region_failover.failover_logs", false)

	assert.True(t, destSender.Send(&message.Payload{}), "sender should always indicate success when disabled in MRF mode")
	assert.Len(t, dest.input, 0, "sender should not send payload when disabled")
}
