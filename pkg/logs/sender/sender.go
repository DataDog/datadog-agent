// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

//TODO MOve me
// Strategy should contain all logic to send logs to a remote destination
// and forward them the next stage of the pipeline.
type Strategy interface {
	Start(inputChan chan *message.Message, outputChan chan *message.Payload)
	Flush(ctx context.Context)
}

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
	isRetrying   chan bool
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
		isRetrying:   make(chan bool, 1),
		destinations: destinations,
		done:         make(chan struct{}),
		stop:         make(chan struct{}, 1),
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
	s.stop <- struct{}{}
	<-s.done
}

func (s *Sender) run() {
	defer func() {
		s.done <- struct{}{}
	}()

	reliableDestinations := buildDestinationContexts(s.destinations.Reliable, s.outputChan, s.bufferSize)

	sink := additionalDestinationsSink(s.bufferSize)
	additionalDestinations := buildDestinationContexts(s.destinations.Additionals, sink, s.bufferSize)

	stopped := false

	for payload := range s.inputChan {
		fmt.Println("got payload")
		select {
		case <-s.stop:
			stopped = true
		default:
		}

		sent := false
		for !sent && !stopped { // TODO: Handle stop and in sender
			select {
			case <-s.stop:
				stopped = true
			default:
				for _, destCtx := range reliableDestinations {
					if !destCtx.updateAndGetIsRetrying() {
						destCtx.input <- payload
						sent = true
						fmt.Println("Sent to main")
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
			// if an endpoint is stuck in the previous step, buffer the payloads if we have room to mitigate loss
			// on intermittent failures.
			if destCtx.isRetrying {
				select {
				case destCtx.input <- payload:
				default:
				}
			}
		}

		// Attempt to send to additional destination
		for _, destCtx := range additionalDestinations {
			fmt.Println("sending to additional")
			select {
			case destCtx.input <- payload:
				fmt.Println("additional enqueue")
			default:
				fmt.Println("!!!!! additional miss")
			}
		}
		fmt.Println("finished")
	}
	fmt.Println("Shutting down")

	// Cleanup
	for _, destCtx := range reliableDestinations {
		destCtx.close()
	}
	for _, destCtx := range additionalDestinations {
		destCtx.close()
	}
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
