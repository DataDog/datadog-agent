// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type destinationContext struct {
	isRetrying        bool
	input             chan *message.Payload
	retryStateChanged chan bool
}

func (d *destinationContext) updateAndGetIsRetrying() bool {
	select {
	case d.isRetrying = <-d.retryStateChanged:
	default:
	}
	return d.isRetrying
}

func (d *destinationContext) close() {
	close(d.input)
	close(d.retryStateChanged)
}

// Sender sends logs to different destinations.
type Sender struct {
	inputChan    chan *message.Payload
	outputChan   chan *message.Payload
	destinations *client.Destinations
	done         chan struct{}
	stop         chan struct{}
	bufferSize   int
}

// NewSender returns a new sender.
func NewSender(inputChan chan *message.Payload, outputChan chan *message.Payload, destinations *client.Destinations, bufferSize int) *Sender {
	return &Sender{
		inputChan:    inputChan,
		outputChan:   outputChan,
		destinations: destinations,
		done:         make(chan struct{}),
		stop:         make(chan struct{}, 1),
		bufferSize:   bufferSize,
	}
}

// Start starts the sender.
func (s *Sender) Start() {
	s.stop = make(chan struct{}, 1)
	go s.run()
}

// Stop stops the sender,
// this call blocks until inputChan is flushed
func (s *Sender) Stop() {
	close(s.inputChan)
	s.stop <- struct{}{}
	<-s.done
}

func (s *Sender) run() {
	reliableDestinations := buildDestinationContexts(s.destinations.Reliable, s.outputChan, s.bufferSize)

	sink := additionalDestinationsSink(s.bufferSize)
	additionalDestinations := buildDestinationContexts(s.destinations.Additionals, sink, s.bufferSize)

payloadLoop:
	for payload := range s.inputChan {
		select {
		case <-s.stop:
			break payloadLoop
		default:
		}

		sent := false
		for !sent {
			select {
			case <-s.stop:
				break payloadLoop
			default:
				for _, destCtx := range reliableDestinations {
					if !destCtx.updateAndGetIsRetrying() {
						destCtx.input <- payload
						sent = true
					}
				}

				if !sent {
					// Using a busy loop is much simpler than trying to join an arbitrary number of channels and
					// wait for just one of them. This is an exceptional case so it has little overhead since it
					// will only happen when there is no possible way to send logs.
					time.Sleep(100 * time.Millisecond)
				}
			}
		}

		for _, destCtx := range reliableDestinations {
			// if an endpoint is stuck in the previous step, try to buffer the payloads if we have room to mitigate
			// loss on intermittent failures.
			if destCtx.isRetrying {
				select {
				case destCtx.input <- payload:
				default:
				}
			}
		}

		// Attempt to send to additional destination
		for _, destCtx := range additionalDestinations {
			select {
			case destCtx.input <- payload:
			default:
			}
		}
	}

	// Cleanup
	for _, destCtx := range reliableDestinations {
		destCtx.close()
	}
	for _, destCtx := range additionalDestinations {
		destCtx.close()
	}
	s.done <- struct{}{}
}

// Drains the output channel from destinations that don't update the auditor.
func additionalDestinationsSink(bufferSize int) chan *message.Payload {
	sink := make(chan *message.Payload, bufferSize)
	go func() {
		for {
			<-sink
		}
	}()
	return sink
}

func buildDestinationContexts(destinations []client.Destination, output chan *message.Payload, bufferSize int) []*destinationContext {
	contexts := []*destinationContext{}
	for _, input := range destinations {
		destCtx := &destinationContext{false, make(chan *message.Payload, bufferSize), make(chan bool, 1)}
		contexts = append(contexts, destCtx)
		input.Start(destCtx.input, destCtx.retryStateChanged, output)
	}
	return contexts
}
