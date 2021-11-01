// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"context"
	"sync"

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

// Sender sends logs to different destinations.
type Sender struct {
	inputChan    chan *message.Message
	outputChan   chan *message.Message
	destinations *client.Destinations
	strategy     Strategy
	done         chan struct{}
	lastError    error
	sync.Mutex
}

// NewSender returns a new sender.
func NewSender(inputChan chan *message.Message, outputChan chan *message.Message, destinations *client.Destinations, strategy Strategy) *Sender {
	return &Sender{
		inputChan:    inputChan,
		outputChan:   outputChan,
		destinations: destinations,
		strategy:     strategy,
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
	s.strategy.Send(s.inputChan, s.outputChan, s.send)
}

// send sends a payload to multiple destinations,
// it will forever retry for the main destination unless the error is not retryable
// and only try once for additionnal destinations.
func (s *Sender) send(payload []byte) error {
	for {
		err := s.destinations.Main.Send(payload)
		if err != nil {
			s.Lock()
			s.lastError = err
			s.Unlock()

			metrics.DestinationErrors.Add(1)
			metrics.TlmDestinationErrors.Inc()
			if _, ok := err.(*client.RetryableError); ok {

				// could not send the payload because of a client issue,
				// let's retry
				continue
			}
			return err
		}
		s.Lock()
		s.lastError = nil
		s.Unlock()
		break
	}

	for _, destination := range s.destinations.Additionals {
		// send in the background so that the agent does not fall behind
		// for the main destination
		destination.SendAsync(payload)
	}

	return nil
}
func (s *Sender) HasError() bool {
	s.Lock()
	defer s.Unlock()
	return s.lastError != nil
}

// shouldStopSending returns true if a component should stop sending logs.
func shouldStopSending(err error) bool {
	return err == context.Canceled
}

// SplitChannel splits a single stream of message into 2 equal streams.
// Acts like an AND gate in that the input will only block if both outputs block.
// This ensures backpressure is propagated to the input to prevent loss of measages in the pipeline.
func SplitChannel(inputChan chan *message.Message, main *Sender, backup *Sender) {
	go func() {
		for v := range inputChan {
			copy := *v

			mainSenderHasErr := main.HasError()
			backupSenderHasErr := backup.HasError()
			sentMain := false
			sentBackup := false

			if mainSenderHasErr && backupSenderHasErr {
				select {
				case main.inputChan <- v:
					sentMain = true
				case backup.inputChan <- &copy:
					sentBackup = true
				}
			}

			if !sentMain {
				if !mainSenderHasErr {
					main.inputChan <- v
				} else {
					select {
					case main.inputChan <- v:
					default:
						break
					}
				}
			}

			if !sentBackup {
				if !backupSenderHasErr {
					backup.inputChan <- &copy
				} else {
					select {
					case backup.inputChan <- &copy:
					default:
						break
					}
				}
			}
		}
	}()
}
