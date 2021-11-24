// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type Payload struct {
	messages []*message.Message
	payload  []byte
}

// Strategy should contain all logic to send logs to a remote destination
// and forward them the next stage of the pipeline.
type Strategy interface {
	Start(inputChan chan *message.Message, outputChan chan *Payload)
	Flush(ctx context.Context)
}

type destinationContext struct {
	hasError     bool
	input        chan []byte
	errorChanged chan bool
}

func (d *destinationContext) updateAndGetHasError() bool {
	select {
	case d.hasError = <-d.errorChanged:
	default:
	}
	return d.hasError
}

// Sender sends logs to different destinations.
type Sender struct {
	inputChan    chan *Payload
	outputChan   chan *message.Message
	hasError     chan bool
	destinations *client.Destinations
	strategy     Strategy
	done         chan struct{}
}

// NewSender returns a new sender.
func NewSender(inputChan chan *Payload, outputChan chan *message.Message, destinations *client.Destinations) *Sender {
	return &Sender{
		inputChan:    inputChan,
		outputChan:   outputChan,
		hasError:     make(chan bool, 1),
		destinations: destinations,
		done:         make(chan struct{}),
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

// Flush sends synchronously the messages that this sender has to send.
func (s *Sender) Flush(ctx context.Context) {
	s.strategy.Flush(ctx)
}

func (s *Sender) run() {
	defer func() {
		s.done <- struct{}{}
	}()

	destinationContetexts := []*destinationContext{}
	for _, input := range s.destinations.Reliable {
		destCtx := &destinationContext{false, make(chan []byte, 100), make(chan bool, 1)}
		destinationContetexts = append(destinationContetexts, destCtx)
		input.Start(destCtx.input, destCtx.errorChanged)
	}

	additionalContexts := []*destinationContext{}
	for _, input := range s.destinations.Additionals {
		destCtx := &destinationContext{false, make(chan []byte, 100), make(chan bool, 1)}
		additionalContexts = append(additionalContexts, destCtx)
		input.Start(destCtx.input, destCtx.errorChanged)
	}

	for payload := range s.inputChan {

		sent := false
		for !sent {
			for _, destCtx := range destinationContetexts {
				if !destCtx.updateAndGetHasError() {
					sent = true
					destCtx.input <- payload.payload
				}
			}

			if !sent {
				// Using a busy loop is much simpler than trying to join an arbitrary number of channels.
				// This has little overhead since it will only happen when there is no possible way to send logs
				time.Sleep(100 * time.Millisecond)
			}
		}

		for _, destCtx := range destinationContetexts {
			// if an endpoint is stuck in the previous step, buffer the payloads if we have room to mitigate loss
			// on intermittent failures.
			if destCtx.hasError {
				select {
				case destCtx.input <- payload.payload:
				default:
				}
			}
		}

		// Attempt to send to additional destination
		for _, destCtx := range additionalContexts {
			select {
			case destCtx.input <- payload.payload:
			default:
			}
		}

	}
}
