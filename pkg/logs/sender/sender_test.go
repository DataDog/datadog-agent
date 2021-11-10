// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type mockDestination struct {
	errChan  chan bool
	hasError bool
}

func newMockDestination() *mockDestination {
	return &mockDestination{
		errChan:  make(chan bool, 1),
		hasError: false,
	}
}

func (m *mockDestination) Send(payload []byte) error {
	select {
	case m.hasError = <-m.errChan:
	default:
	}

	if m.hasError {
		return errors.New("Test error")
	}
	return nil

}
func (m *mockDestination) SendAsync(payload []byte) {
}

type mockStrategy struct {
	sendFailed chan bool
}

func newMockStrategy() *mockStrategy {
	return &mockStrategy{
		sendFailed: make(chan bool),
	}

}

func (m *mockStrategy) Send(inputChan chan *message.Message, outputChan chan *message.Message, send func([]byte) error) {
	for msg := range inputChan {
		if send(msg.Content) == nil {
			outputChan <- msg
			continue
		}
		m.sendFailed <- true
	}
}

func (m *mockStrategy) Flush(ctx context.Context) {}

func newMessage(content []byte, source *config.LogSource, status string) *message.Message {
	return message.NewMessageWithSource(content, status, source, 0)
}

func TestSender(t *testing.T) {
	l := mock.NewMockLogsIntake(t)
	defer l.Close()

	source := config.NewLogSource("", &config.LogsConfig{})

	input := make(chan *message.Message, 1)
	output := make(chan *message.Message, 1)

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()

	destination := tcp.AddrToDestination(l.Addr(), destinationsCtx)
	destinations := client.NewDestinations(destination, nil)

	sender := NewSingleSender(input, output, destinations, StreamStrategy)
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

func TestSenderNotBlockedByAdditional(t *testing.T) {
	l := mock.NewMockLogsIntake(t)
	defer l.Close()

	source := config.NewLogSource("", &config.LogsConfig{})

	input := make(chan *message.Message, 1)
	output := make(chan *message.Message, 1)

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()

	mainDestination := tcp.AddrToDestination(l.Addr(), destinationsCtx)
	// This destination doesn't exists
	additionalDestination := tcp.NewDestination(config.Endpoint{Host: "dont.exist.local", Port: 0}, true, destinationsCtx)
	destinations := client.NewDestinations(mainDestination, []client.Destination{additionalDestination})

	sender := NewSingleSender(input, output, destinations, StreamStrategy)
	sender.Start()

	expectedMessage1 := newMessage([]byte("fake line"), source, "")
	input <- expectedMessage1
	message, ok := <-output
	assert.True(t, ok)
	assert.Equal(t, message, expectedMessage1)

	expectedMessage2 := newMessage([]byte("fake line 2"), source, "")
	input <- expectedMessage2
	message, ok = <-output
	assert.True(t, ok)
	assert.Equal(t, message, expectedMessage2)

	sender.Stop()
	destinationsCtx.Stop()
}

func TestDualShipEndpoints(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})

	input := make(chan *message.Message, 1)
	mainInput := make(chan *message.Message, 1)
	backupInput := make(chan *message.Message, 1)
	mainOutput := make(chan *message.Message)
	backupOutput := make(chan *message.Message)

	mainDest := newMockDestination()
	mainDests := client.NewDestinations(mainDest, []client.Destination{})
	backupDest := newMockDestination()
	backupDests := client.NewDestinations(backupDest, []client.Destination{})

	mainMockStrategy := newMockStrategy()
	backupMockStrategy := newMockStrategy()

	mainSender := NewSingleSender(mainInput, mainOutput, mainDests, mainMockStrategy)
	additionalSender := NewSingleSender(backupInput, backupOutput, backupDests, backupMockStrategy)

	dualSender := NewDualSender(input, mainSender, additionalSender)
	dualSender.Start()

	// Scenario 1: Both senders fail, and then both recover

	// Test both output's get the line
	input <- newMessage([]byte("fake line"), source, "")
	<-mainOutput
	<-backupOutput

	backupDest.errChan <- true

	// Main should get the message - backup should not.
	input <- newMessage([]byte("fake line"), source, "")
	<-mainOutput
	<-backupMockStrategy.sendFailed

	mainDest.errChan <- true

	// Both senders are in a failed state. SplitSenders will now block until
	// one succeeds regardless of the error state.
	input <- newMessage([]byte("fake line"), source, "")
	<-mainMockStrategy.sendFailed
	<-backupMockStrategy.sendFailed

	mainDest.errChan <- false

	// Main has recovered and should should get the message - backup should not.
	input <- newMessage([]byte("1"), source, "")
	<-mainOutput
	<-backupMockStrategy.sendFailed

	backupDest.errChan <- false

	// Both senders are now recovered - both get the messsage
	input <- newMessage([]byte("fake line"), source, "")
	<-mainOutput
	<-backupOutput

	// Scenario 2: One sender fails and then recovers

	mainDest.errChan <- true

	input <- newMessage([]byte("fake line"), source, "")
	<-mainMockStrategy.sendFailed
	<-backupOutput

	mainDest.errChan <- false

	input <- newMessage([]byte("fake line"), source, "")
	<-mainOutput
	<-backupOutput
}

func TestSingleFailsThenRecovers(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})

	input := make(chan *message.Message, 1)
	mainOutput := make(chan *message.Message)

	mainDest := newMockDestination()
	mainDests := client.NewDestinations(mainDest, []client.Destination{})

	mainMockStrategy := newMockStrategy()

	mainSender := NewSingleSender(input, mainOutput, mainDests, mainMockStrategy)

	mainSender.Start()

	input <- newMessage([]byte("fake line"), source, "")
	<-mainOutput

	mainDest.errChan <- true

	input <- newMessage([]byte("fake line"), source, "")
	<-mainMockStrategy.sendFailed

	mainDest.errChan <- false

	input <- newMessage([]byte("fake line"), source, "")
	<-mainOutput
}
