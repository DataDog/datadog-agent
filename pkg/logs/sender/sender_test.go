// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
)

type mockDestination struct {
	sync.Mutex
	isRetrying bool
	started    chan bool
}

func newMockDestination() *mockDestination {
	return &mockDestination{
		isRetrying: false,
		started:    make(chan bool),
	}
}

func (m *mockDestination) Start(input chan *message.Payload, output chan *message.Payload) (stopChan chan struct{}) {
	stopChan = make(chan struct{})
	go func() {
		for payload := range input {
			_ = payload
		}
		stopChan <- struct{}{}
	}()
	m.started <- true
	return stopChan
}

func (m *mockDestination) setRetrying(val bool) {
	m.Lock()
	m.isRetrying = val
	m.Unlock()
}

func (m *mockDestination) GetIsRetrying() bool {
	m.Lock()
	defer m.Unlock()
	return m.isRetrying
}

func newMessage(content []byte, source *config.LogSource, status string) *message.Payload {
	return &message.Payload{
		Messages: []*message.Message{message.NewMessageWithSource(content, status, source, 0)},
		Encoded:  content,
		Encoding: "identity",
	}
}

func TestSender(t *testing.T) {
	l := mock.NewMockLogsIntake(t)
	defer l.Close()

	source := config.NewLogSource("", &config.LogsConfig{})

	input := make(chan *message.Payload, 1)
	output := make(chan *message.Payload, 1)

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()

	destination := tcp.AddrToDestination(l.Addr(), destinationsCtx)
	destinations := client.NewDestinations([]client.Destination{destination}, nil)

	sender := NewSender(input, output, destinations, 0)
	sender.Start()

	expectedMessage := newMessage([]byte("fake line"), source, "")

	// Write to the output should relay the message to the output (after sending it on the wire)
	input <- expectedMessage
	message, ok := <-output

	assert.True(t, ok)
	assert.Equal(t, message, expectedMessage)

	sender.Stop()
	destinationsCtx.Stop()
}

func TestReliableSendersApplyBackpressure(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})

	input := make(chan *message.Payload)

	// the channel config for this tests allows a buffering of 2 payloads. so send groups of 3 to
	// ensure we are unblocked
	send3 := func() {
		input <- newMessage([]byte("fake line"), source, "")
		input <- newMessage([]byte("fake line"), source, "")
		input <- newMessage([]byte("fake line"), source, "")
	}

	reliableDest1 := newMockDestination()
	reliableDest2 := newMockDestination()
	unreliableDest := newMockDestination()
	destinations := client.NewDestinations([]client.Destination{reliableDest1, reliableDest2}, []client.Destination{unreliableDest})

	dualSender := NewSender(input, make(chan *message.Payload), destinations, 1)
	dualSender.Start()
	<-reliableDest1.started
	<-reliableDest2.started
	<-unreliableDest.started

	send3()

	reliableDest2.setRetrying(true)
	send3()

	reliableDest1.setRetrying(true)

	// first one lands in the buffered channel
	input <- newMessage([]byte("fake line"), source, "")

	// input should now be blocked - even though unreliableDest isn't retrying (because the sender only blocks on reliable destinations)
	select {
	case input <- newMessage([]byte("fake line"), source, ""):
		assert.Fail(t, "Input should be blocked")
	default:
	}

	// unblock
	reliableDest2.setRetrying(false)
	send3()
}
