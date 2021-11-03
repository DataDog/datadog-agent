// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type mockStrategy struct {
}

func (m *mockStrategy) Send(inputChan chan *message.Message, outputChan chan *message.Message, send func([]byte) error) {
	for msg := range inputChan {
		outputChan <- msg
	}
}

func (m *mockStrategy) Flush(ctx context.Context) {}

func newMessage(content []byte, source *config.LogSource, status string) *message.Message {
	return message.NewMessageWithSource(content, status, source, 0)
}

func assertNoMessage(t *testing.T, channel chan *message.Message) {
	timer := time.NewTimer(10 * time.Millisecond)
loop:
	for {
		select {
		case <-channel:
			t.Fail()
		case <-timer.C:
			break loop
		}
	}
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

	sender := NewSender(input, output, destinations, StreamStrategy)
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

	sender := NewSender(input, output, destinations, StreamStrategy)
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

func TestSplitSender(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})

	senderError := func(sender *Sender, fail bool) {
		semaChan := make(chan bool)
		go func() {
			semaChan <- true
			sender.hasError <- fail
		}()
		<-semaChan
	}

	input := make(chan *message.Message, 1)
	mainInput := make(chan *message.Message, 1)
	backupInput := make(chan *message.Message, 1)
	mainOutput := make(chan *message.Message, 1)
	backupOutput := make(chan *message.Message, 1)

	mainStrategy := &mockStrategy{}
	backupStrategy := &mockStrategy{}
	mainSender := NewSender(mainInput, mainOutput, nil, mainStrategy)
	backupSender := NewSender(backupInput, backupOutput, nil, backupStrategy)

	SplitSenders(input, mainSender, backupSender)

	mainSender.Start()
	backupSender.Start()

	// Scenario 1: Both senders fail, and then both recover

	// Test both output's get the line
	input <- newMessage([]byte("fake line"), source, "")
	<-mainOutput
	<-backupOutput

	senderError(backupSender, true)

	// Main should get the message - backup should not.
	input <- newMessage([]byte("fake line"), source, "")
	<-mainOutput
	assertNoMessage(t, backupOutput)

	senderError(mainSender, true)

	// Both senders are in a failed state. SplitSenders will now block until
	// one succeeds regardless of the error state. Simulate main starting to recover.
	input <- newMessage([]byte("fake line"), source, "")
	<-mainOutput

	senderError(mainSender, false)

	// Main has recovered and should should get the message - backup should not.
	input <- newMessage([]byte("fake line"), source, "")
	<-mainOutput
	assertNoMessage(t, backupOutput)

	senderError(backupSender, false)

	// Both senders are now recovered - both get the messsage
	input <- newMessage([]byte("fake line"), source, "")
	<-mainOutput
	<-backupOutput

	// Scenario 2: One sender fails and then recovers

	senderError(mainSender, true)

	input <- newMessage([]byte("fake line"), source, "")
	assertNoMessage(t, mainOutput)
	<-backupOutput

	senderError(mainSender, false)

	input <- newMessage([]byte("fake line"), source, "")
	<-mainOutput
	<-backupOutput

}
