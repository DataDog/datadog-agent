// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/client/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
)

func getNewConfig() pkgconfigmodel.ReaderWriter {
	return pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
}

func newMessage(content []byte, source *sources.LogSource, status string) *message.Payload {
	return &message.Payload{
		Messages: []*message.Message{message.NewMessageWithSource(content, status, source, 0)},
		Encoded:  content,
		Encoding: "identity",
	}
}

func TestSender(t *testing.T) {
	l := mock.NewMockLogsIntake(t)
	defer l.Close()

	source := sources.NewLogSource("", &config.LogsConfig{})

	input := make(chan *message.Payload, 1)
	output := make(chan *message.Payload, 1)

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()

	destination := tcp.AddrToDestination(l.Addr(), destinationsCtx, statusinterface.NewStatusProviderMock())
	destinations := client.NewDestinations([]client.Destination{destination}, nil)

	cfg := getNewConfig()
	sender := NewSender(cfg, input, output, destinations, 0, nil, nil)
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

//nolint:revive // TODO(AML) Fix revive linter
func TestSenderSingleDestination(t *testing.T) {
	cfg := getNewConfig()
	input := make(chan *message.Payload, 1)
	output := make(chan *message.Payload, 1)

	respondChan := make(chan int)

	server := http.NewTestServerWithOptions(200, 0, true, respondChan, cfg)

	destinations := client.NewDestinations([]client.Destination{server.Destination}, nil)

	sender := NewSender(cfg, input, output, destinations, 10, nil, nil)
	sender.Start()

	input <- &message.Payload{}
	input <- &message.Payload{}

	<-respondChan
	<-output

	<-respondChan
	<-output

	server.Stop()
	sender.Stop()
}

//nolint:revive // TODO(AML) Fix revive linter
func TestSenderDualReliableDestination(t *testing.T) {
	cfg := getNewConfig()
	input := make(chan *message.Payload, 1)
	output := make(chan *message.Payload, 1)

	respondChan1 := make(chan int)
	server1 := http.NewTestServerWithOptions(200, 0, true, respondChan1, cfg)

	respondChan2 := make(chan int)
	server2 := http.NewTestServerWithOptions(200, 0, true, respondChan2, cfg)

	destinations := client.NewDestinations([]client.Destination{server1.Destination, server2.Destination}, nil)

	sender := NewSender(cfg, input, output, destinations, 10, nil, nil)
	sender.Start()

	input <- &message.Payload{}
	input <- &message.Payload{}

	<-respondChan1
	<-respondChan2
	<-output
	<-output

	<-respondChan1
	<-respondChan2
	<-output
	<-output

	server1.Stop()
	server2.Stop()
	sender.Stop()
}

//nolint:revive // TODO(AML) Fix revive linter
func TestSenderUnreliableAdditionalDestination(t *testing.T) {
	cfg := getNewConfig()
	input := make(chan *message.Payload, 1)
	output := make(chan *message.Payload, 1)

	respondChan1 := make(chan int)
	server1 := http.NewTestServerWithOptions(200, 0, true, respondChan1, cfg)

	respondChan2 := make(chan int)
	server2 := http.NewTestServerWithOptions(200, 0, false, respondChan2, cfg)

	destinations := client.NewDestinations([]client.Destination{server1.Destination}, []client.Destination{server2.Destination})

	sender := NewSender(cfg, input, output, destinations, 10, nil, nil)
	sender.Start()

	input <- &message.Payload{}
	input <- &message.Payload{}

	<-respondChan1
	<-respondChan2
	<-output

	<-respondChan1
	<-respondChan2
	<-output

	server1.Stop()
	server2.Stop()
	sender.Stop()
}

func TestSenderUnreliableStopsWhenMainFails(t *testing.T) {
	cfg := getNewConfig()
	input := make(chan *message.Payload, 1)
	output := make(chan *message.Payload, 1)

	reliableRespond := make(chan int)
	reliableServer := http.NewTestServerWithOptions(200, 0, true, reliableRespond, cfg)

	unreliableRespond := make(chan int)
	unreliableServer := http.NewTestServerWithOptions(200, 0, false, unreliableRespond, cfg)

	destinations := client.NewDestinations([]client.Destination{reliableServer.Destination}, []client.Destination{unreliableServer.Destination})

	sender := NewSender(cfg, input, output, destinations, 10, nil, nil)
	sender.Start()

	input <- &message.Payload{}

	<-reliableRespond
	<-unreliableRespond
	<-output

	reliableServer.ChangeStatus(500)

	input <- &message.Payload{}

	<-reliableRespond   // let it respond 500 once
	<-unreliableRespond // unreliable gets this log line because it hasn't fallen into a retry loop yet.
	<-reliableRespond   // its in a loop now, once we respond 500 a second time we know the sender has marked the endpoint as retrying

	// send another log
	input <- &message.Payload{}

	// reliable still stuck in retry loop - responding 500 over and over again.
	<-reliableRespond

	// unreliable should not be sending since all the reliable endpoints are failing.
	select {
	case <-unreliableRespond:
		assert.Fail(t, "unreliable sender should be waiting for main sender")
	default:
	}

	reliableServer.Stop()
	unreliableServer.Stop()
	sender.Stop()
}

//nolint:revive // TODO(AML) Fix revive linter
func TestSenderReliableContinuseWhenOneFails(t *testing.T) {
	cfg := getNewConfig()
	input := make(chan *message.Payload, 1)
	output := make(chan *message.Payload, 1)

	reliableRespond1 := make(chan int)
	reliableServer1 := http.NewTestServerWithOptions(200, 0, true, reliableRespond1, cfg)

	reliableRespond2 := make(chan int)
	reliableServer2 := http.NewTestServerWithOptions(200, 0, false, reliableRespond2, cfg)

	destinations := client.NewDestinations([]client.Destination{reliableServer1.Destination, reliableServer2.Destination}, nil)

	sender := NewSender(cfg, input, output, destinations, 10, nil, nil)
	sender.Start()

	input <- &message.Payload{}

	<-reliableRespond1
	<-reliableRespond2
	<-output
	<-output

	reliableServer1.ChangeStatus(500)

	input <- &message.Payload{}

	<-reliableRespond1 // let it respond 500 once
	<-reliableRespond2 // Second endpoint gets the log line
	<-output
	<-reliableRespond1 // its in a loop now, once we respond 500 a second time we know the sender has marked the endpoint as retrying

	// send another log
	input <- &message.Payload{}

	// reliable still stuck in retry loop - responding 500 over and over again.
	<-reliableRespond1
	<-reliableRespond2 // Second output gets the line again
	<-output

	reliableServer1.Stop()
	reliableServer2.Stop()
	sender.Stop()
}

//nolint:revive // TODO(AML) Fix revive linter
func TestSenderReliableWhenOneFailsAndRecovers(t *testing.T) {
	cfg := getNewConfig()
	input := make(chan *message.Payload, 1)
	output := make(chan *message.Payload, 1)

	reliableRespond1 := make(chan int)
	reliableServer1 := http.NewTestServerWithOptions(200, 0, true, reliableRespond1, cfg)

	reliableRespond2 := make(chan int)
	reliableServer2 := http.NewTestServerWithOptions(200, 0, false, reliableRespond2, cfg)

	destinations := client.NewDestinations([]client.Destination{reliableServer1.Destination, reliableServer2.Destination}, nil)

	sender := NewSender(cfg, input, output, destinations, 10, nil, nil)
	sender.Start()

	input <- &message.Payload{}

	<-reliableRespond1
	<-reliableRespond2
	<-output
	<-output

	reliableServer1.ChangeStatus(500)

	input <- &message.Payload{}

	<-reliableRespond1 // let it respond 500 once
	<-reliableRespond2 // Second endpoint gets the log line
	<-output
	<-reliableRespond1 // its in a loop now, once we respond 500 a second time we know the sender has marked the endpoint as retrying

	// send another log
	input <- &message.Payload{}

	// reliable still stuck in retry loop - responding 500 over and over again.
	<-reliableRespond1
	<-reliableRespond2 // Second output gets the line again
	<-output

	// Recover the first server
	reliableServer1.ChangeStatus(200)

	// Drain any retries
	for {
		if (<-reliableRespond1) == 200 {
			break
		}
	}

	<-output // get the buffered log line that was stuck

	// Make sure everything is unblocked
	input <- &message.Payload{}

	<-reliableRespond1
	<-reliableRespond2
	<-output
	<-output

	reliableServer1.Stop()
	reliableServer2.Stop()
	sender.Stop()
}
