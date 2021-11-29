// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

// Strategy should contain all logic to send logs to a remote destination
// and forward them the next stage of the pipeline.
type Strategy interface {
	Send(inputChan chan *message.Message, outputChan chan *message.Message, send func([]byte) error)
	Flush(ctx context.Context)
}

// Sender contains all the logic to manage a stream of messages to destinations
type Sender interface {
	Start()
	Stop()
	Flush(ctx context.Context)
}

// SingleSender sends logs to different destinations.
type SingleSender struct {
	inputChan    chan *message.Message
	outputChan   chan *message.Message
	hasError     chan bool
	destinations *client.Destinations
	strategy     Strategy
	done         chan struct{}
	lastError    error
	trackErrors  bool
}

// NewSingleSender returns a new sender.
func NewSingleSender(inputChan chan *message.Message, outputChan chan *message.Message, destinations *client.Destinations, strategy Strategy) *SingleSender {
	return &SingleSender{
		inputChan:    inputChan,
		outputChan:   outputChan,
		hasError:     make(chan bool, 1),
		destinations: destinations,
		strategy:     strategy,
		done:         make(chan struct{}),
		trackErrors:  false,
	}
}

// Start starts the sender.
func (s *SingleSender) Start() {
	go s.run()
}

// Stop stops the sender,
// this call blocks until inputChan is flushed
func (s *SingleSender) Stop() {
	close(s.inputChan)
	<-s.done
}

// Flush sends synchronously the messages that this sender has to send.
func (s *SingleSender) Flush(ctx context.Context) {
	s.strategy.Flush(ctx)
}

func (s *SingleSender) run() {
	defer func() {
		s.done <- struct{}{}
	}()
	s.strategy.Send(s.inputChan, s.outputChan, s.send)
}

// send sends a payload to multiple destinations,
// it will forever retry for the main destination unless the error is not retryable
// and only try once for additional destinations.
func (s *SingleSender) send(payload []byte) error {
	for {
		err := s.destinations.Main.Send(payload)
		if err != nil {
			if s.trackErrors && s.lastError == nil {
				s.hasError <- true
			}
			s.lastError = err

			metrics.DestinationErrors.Add(1)
			metrics.TlmDestinationErrors.Inc()
			if _, ok := err.(*client.RetryableError); ok {

				// could not send the payload because of a client issue,
				// let's retry
				continue
			}
			return err
		}
		if s.trackErrors && s.lastError != nil {
			s.lastError = nil
			s.hasError <- false
		}
		break
	}

	for _, destination := range s.destinations.Additionals {
		// send in the background so that the agent does not fall behind
		// for the main destination
		destination.SendAsync(payload)
	}

	return nil
}

// shouldStopSending returns true if a component should stop sending logs.
func shouldStopSending(err error) bool {
	return err == context.Canceled
}

// DualSender wraps 2 single senders to manage sending logs to 2 primary destinations
type DualSender struct {
	inputChan        chan *message.Message
	mainSender       *SingleSender
	additionalSender *SingleSender
	done             chan struct{}
}

// NewDualSender creates a new dual sender
func NewDualSender(inputChan chan *message.Message, mainSender *SingleSender, additionalSender *SingleSender) Sender {
	mainSender.trackErrors = true
	additionalSender.trackErrors = true
	return &DualSender{
		inputChan:        inputChan,
		mainSender:       mainSender,
		additionalSender: additionalSender,
	}
}

// Start starts the child senders and manages any errors/back pressure.
func (s *DualSender) Start() {
	s.mainSender.Start()
	s.additionalSender.Start()

	// Splits a single stream of message into 2 equal streams.
	// Acts like an AND gate in that the input will only block if both outputs block.
	// This ensures backpressure is propagated to the input to prevent loss of measages in the pipeline.
	go func() {
		mainSenderHasErr := false
		additionalSenderHasErr := false

		for message := range s.inputChan {
			sentMain := false
			sentAdditional := false

			// First collect any errors from the senders
			select {
			case mainSenderHasErr = <-s.mainSender.hasError:
			default:
			}

			select {
			case additionalSenderHasErr = <-s.additionalSender.hasError:
			default:
			}

			// If both senders are failing, we want to block the pipeline until at least one succeeds
			if mainSenderHasErr && additionalSenderHasErr {
				select {
				case s.mainSender.inputChan <- message:
					sentMain = true
				case s.additionalSender.inputChan <- message:
					sentAdditional = true
				case mainSenderHasErr = <-s.mainSender.hasError:
				case additionalSenderHasErr = <-s.additionalSender.hasError:
				}
			}

			if !sentMain {
				mainSenderHasErr = sendMessage(mainSenderHasErr, s.mainSender, message)
			}

			if !sentAdditional {
				additionalSenderHasErr = sendMessage(additionalSenderHasErr, s.additionalSender, message)
			}
		}
		s.done <- struct{}{}
	}()
}

// Stop stops the sender,
// this call blocks until inputChan is flushed
func (s *DualSender) Stop() {
	close(s.inputChan)
	<-s.done
	s.mainSender.Stop()
	s.additionalSender.Stop()
}

// Flush sends synchronously the messages that the child senders have to send
func (s *DualSender) Flush(ctx context.Context) {
	s.mainSender.Flush(ctx)
	s.additionalSender.Flush(ctx)
}

func sendMessage(hasError bool, sender *SingleSender, message *message.Message) bool {
	if !hasError {
		// If there is no error - block and write to the buffered channel until it succeeds or we get an error.
		// If we don't block, the input can fill the buffered channels faster than sender can
		// drain them - causing missing logs.
		select {
		case sender.inputChan <- message:
		case hasError = <-sender.hasError:
		}
	} else {
		// Even if there is an error, try to put the log line in the buffered channel in case the
		// error resolves quickly and there is room in the channel.
		select {
		case sender.inputChan <- message:
		default:
		}
	}
	return hasError
}
