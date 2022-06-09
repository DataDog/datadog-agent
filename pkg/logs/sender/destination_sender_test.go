// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package sender

import (
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
)

type mockDestination struct {
	input      chan *message.Payload
	output     chan *message.Payload
	isRetrying chan bool
	stopChan   chan struct{}
}

func (m *mockDestination) Start(input chan *message.Payload, output chan *message.Payload, isRetrying chan bool) (stopChan <-chan struct{}) {
	m.input = input
	m.output = output
	m.isRetrying = isRetrying
	m.stopChan = make(chan struct{})
	return m.stopChan
}

func TestDestinationSender(t *testing.T) {

	output := make(chan *message.Payload)
	dest := &mockDestination{}
	d := NewDestinationSender(dest, output, 1)

	d.Send(&message.Payload{})

	<-dest.input

	didStop := make(chan struct{})
	go func() {
		<-dest.stopChan
		didStop <- struct{}{}
	}()
}

func TestDestinationSenderCanBeCanceled(t *testing.T) {

	output := make(chan *message.Payload)
	dest := &mockDestination{}
	d := NewDestinationSender(dest, output, 0)

	sendSucceeded := make(chan bool)

	// Send should block because intput is full.
	go func() {
		sendSucceeded <- d.Send(&message.Payload{})
	}()
	// trigger a retry state change to unblock it
	dest.isRetrying <- true

	assert.False(t, <-sendSucceeded)
}

func TestDestinationSenderAlreadyRetrying(t *testing.T) {

	output := make(chan *message.Payload)
	dest := &mockDestination{}
	d := NewDestinationSender(dest, output, 0)
	dest.isRetrying <- true

	assert.False(t, d.Send(&message.Payload{}))
}

func TestDestinationSenderStopsRetrying(t *testing.T) {

	output := make(chan *message.Payload)
	dest := &mockDestination{}
	d := NewDestinationSender(dest, output, 0)
	dest.isRetrying <- true

	assert.False(t, d.Send(&message.Payload{}))

	dest.isRetrying <- false

	gotPayload := make(chan struct{})
	go func() {
		<-dest.input
		gotPayload <- struct{}{}

	}()

	// retry the send until it succeeds
	for !d.Send(&message.Payload{}) {
	}

	<-gotPayload
}

func TestDestinationSenderDeadlock(t *testing.T) {
	output := make(chan *message.Payload)
	dest := &mockDestination{}
	d := NewDestinationSender(dest, output, 100)

	go func() {
		for range dest.input {
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
			d.Send(&message.Payload{})
		}
		wg.Done()
	}()

	close(syn)
	wg.Wait()
	close(dest.input)
}
