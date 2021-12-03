// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
)

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
