// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sender

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/client"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/client/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func newMessage(content []byte, source *config.LogSource, status string) *message.Message {
	return message.NewPartialMessage2(content, source, status)
}

func TestSender(t *testing.T) {
	l := mock.NewMockLogsIntake(t)
	defer l.Close()

	source := config.NewLogSource("", &config.LogsConfig{})

	input := make(chan *message.Message, 1)
	output := make(chan *message.Message, 1)

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()

	destination := client.AddrToDestination(l.Addr(), destinationsCtx)
	destinations := client.NewDestinations(destination, nil)

	sender := NewSender(input, output, destinations)
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

	mainDestination := client.AddrToDestination(l.Addr(), destinationsCtx)
	// This destination doesn't exists
	additionalDestination := client.NewDestination(client.Endpoint{Host: "dont.exist.local", Port: 0}, destinationsCtx)
	destinations := client.NewDestinations(mainDestination, []*client.Destination{additionalDestination})

	sender := NewSender(input, output, destinations)
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
