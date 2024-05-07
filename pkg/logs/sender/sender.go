// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"strconv"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	tlmPayloadsDropped = telemetry.NewCounterWithOpts("logs_sender", "payloads_dropped", []string{"reliable", "destination"}, "Payloads dropped", telemetry.Options{DefaultMetric: true})
	tlmMessagesDropped = telemetry.NewCounterWithOpts("logs_sender", "messages_dropped", []string{"reliable", "destination"}, "Messages dropped", telemetry.Options{DefaultMetric: true})
	tlmSendWaitTime    = telemetry.NewCounter("logs_sender", "send_wait", []string{}, "Time spent waiting for all sends to finish")
)

// Sender sends logs to different destinations. Destinations can be either
// reliable or unreliable. The sender ensures that logs are sent to at least
// one reliable destination and will block the pipeline if they are in an
// error state. Unreliable destinations will only send logs when at least
// one reliable destination is also sending logs. However they do not update
// the auditor or block the pipeline if they fail. There will always be at
// least 1 reliable destination (the main destination).
type Sender struct {
	config       pkgconfigmodel.Reader
	inputChan    chan *message.Payload
	outputChan   chan *message.Payload
	destinations *client.Destinations
	done         chan struct{}
	bufferSize   int
}

// NewSender returns a new sender.
func NewSender(config pkgconfigmodel.Reader, inputChan chan *message.Payload, outputChan chan *message.Payload, destinations *client.Destinations, bufferSize int) *Sender {
	return &Sender{
		config:       config,
		inputChan:    inputChan,
		outputChan:   outputChan,
		destinations: destinations,
		done:         make(chan struct{}),
		bufferSize:   bufferSize,
	}
}

// Start starts the sender.
func (s *Sender) Start() {
	go s.run()
}

// Stop stops the sender,
// this call blocks until inputChan is flushed
func (s *Sender) Stop() {
	close(s.inputChan)
	<-s.done
}

func (s *Sender) run() {
	reliableDestinations := buildDestinationSenders(s.config, s.destinations.Reliable, s.outputChan, s.bufferSize)

	sink := additionalDestinationsSink(s.bufferSize)
	unreliableDestinations := buildDestinationSenders(s.config, s.destinations.Unreliable, sink, s.bufferSize)

	for payload := range s.inputChan {
		var startInUse = time.Now()

		sent := false
		for !sent {
			for _, destSender := range reliableDestinations {
				if destSender.Send(payload) {
					sent = true
				}
			}

			if !sent {
				// Throttle the poll loop while waiting for a send to succeed
				// This will only happen when all reliable destinations
				// are blocked so logs have no where to go.
				time.Sleep(100 * time.Millisecond)
			}
		}

		for i, destSender := range reliableDestinations {
			// If an endpoint is stuck in the previous step, try to buffer the payloads if we have room to mitigate
			// loss on intermittent failures.
			if !destSender.lastSendSucceeded {
				if !destSender.NonBlockingSend(payload) {
					tlmPayloadsDropped.Inc("true", strconv.Itoa(i))
					tlmMessagesDropped.Add(float64(len(payload.Messages)), "true", strconv.Itoa(i))
				}
			}
		}

		// Attempt to send to unreliable destinations
		for i, destSender := range unreliableDestinations {
			if !destSender.NonBlockingSend(payload) {
				tlmPayloadsDropped.Inc("false", strconv.Itoa(i))
				tlmMessagesDropped.Add(float64(len(payload.Messages)), "false", strconv.Itoa(i))
			}
		}

		inUse := float64(time.Since(startInUse) / time.Millisecond)
		tlmSendWaitTime.Add(inUse)
	}

	// Cleanup the destinations
	for _, destSender := range reliableDestinations {
		destSender.Stop()
	}
	for _, destSender := range unreliableDestinations {
		destSender.Stop()
	}
	close(sink)
	s.done <- struct{}{}
}

// Drains the output channel from destinations that don't update the auditor.
func additionalDestinationsSink(bufferSize int) chan *message.Payload {
	sink := make(chan *message.Payload, bufferSize)
	go func() {
		// drain channel, stop when channel is closed
		//nolint:revive // TODO(AML) Fix revive linter
		for range sink {
		}
	}()
	return sink
}

func buildDestinationSenders(config pkgconfigmodel.Reader, destinations []client.Destination, output chan *message.Payload, bufferSize int) []*DestinationSender {
	destinationSenders := []*DestinationSender{}
	for _, destination := range destinations {
		destinationSenders = append(destinationSenders, NewDestinationSender(config, destination, output, bufferSize))
	}
	return destinationSenders
}
