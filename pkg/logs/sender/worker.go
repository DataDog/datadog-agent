// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"strconv"
	"sync"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tlmPayloadsDropped = telemetry.NewCounterWithOpts("logs_sender", "payloads_dropped", []string{"reliable", "destination"}, "Payloads dropped", telemetry.Options{DefaultMetric: true})
	tlmMessagesDropped = telemetry.NewCounterWithOpts("logs_sender", "messages_dropped", []string{"reliable", "destination"}, "Messages dropped", telemetry.Options{DefaultMetric: true})
	tlmSendWaitTime    = telemetry.NewCounter("logs_sender", "send_wait", []string{}, "Time spent waiting for all sends to finish")
)

// retrierInterface defines the interface for disk retry functionality
type retrierInterface interface {
	WritePayloadToDisk(payload *message.Payload) (bool, error)
	Stop()
	ReplayFromDisk(payloadChan chan *message.Payload, done chan struct{})
}

// noopRetrier is a no-op implementation used when disk retry is not configured.
// When backpressure occurs, payloads are dropped instead of being written to disk.
type noopRetrier struct{}

func (n *noopRetrier) WritePayloadToDisk(_ *message.Payload) (bool, error) { return false, nil }
func (n *noopRetrier) Stop()                                               {}
func (n *noopRetrier) ReplayFromDisk(_ chan *message.Payload, done chan struct{}) {
	// Block until done signal to match the real retrier's behavior
	<-done
}

// worker sends logs to different destinations. Destinations can be either
// reliable or unreliable. The worker ensures that logs are sent to at least
// one reliable destination and will block the pipeline if they are in an
// error state. Unreliable destinations will only send logs when at least
// one reliable destination is also sending logs. However they do not update
// the auditor or block the pipeline if they fail. There will always be at
// least 1 reliable destination (the main destination).
type worker struct {
	config         pkgconfigmodel.Reader
	inputChan      chan *message.Payload
	payloadChan    chan *message.Payload
	outputChan     chan *message.Payload
	destinations   *client.Destinations
	done           chan struct{}
	finished       chan struct{}
	bufferSize     int
	senderDoneChan chan *sync.WaitGroup
	flushWg        *sync.WaitGroup
	sink           Sink
	workerID       string
	retrier        retrierInterface

	pipelineMonitor metrics.PipelineMonitor
	utilization     metrics.UtilizationMonitor
}

func newWorker(
	config pkgconfigmodel.Reader,
	inputChan chan *message.Payload,
	sink Sink,
	destinationFactory DestinationFactory,
	bufferSize int,
	serverlessMeta ServerlessMeta,
	pipelineMonitor metrics.PipelineMonitor,
	workerID string,
	retrier retrierInterface,
) *worker {
	var senderDoneChan chan *sync.WaitGroup
	var flushWg *sync.WaitGroup

	if serverlessMeta.IsEnabled() {
		senderDoneChan = serverlessMeta.SenderDoneChan()
		flushWg = serverlessMeta.WaitGroup()
	}

	// Use no-op retrier if disk retry is not configured
	// This eliminates nil checks in the hot path
	var retrierImpl retrierInterface
	if retrier != nil {
		retrierImpl = retrier
	} else {
		retrierImpl = &noopRetrier{}
	}

	// Ensure payloadChan has at least buffer of 1 for non-blocking send in routePayloads()
	payloadChanSize := bufferSize
	if payloadChanSize == 0 {
		payloadChanSize = 1
	}

	return &worker{
		config:         config,
		inputChan:      inputChan,
		payloadChan:    make(chan *message.Payload, payloadChanSize),
		sink:           sink,
		destinations:   destinationFactory(workerID),
		bufferSize:     bufferSize,
		senderDoneChan: senderDoneChan,
		flushWg:        flushWg,
		done:           make(chan struct{}),
		finished:       make(chan struct{}),
		workerID:       workerID,
		retrier:        retrierImpl,

		// Telemetry
		pipelineMonitor: pipelineMonitor,
		utilization:     pipelineMonitor.MakeUtilizationMonitor(metrics.WorkerTlmName, workerID),
	}
}

// Start starts the worker.
func (s *worker) start() {
	s.outputChan = s.sink.Channel()

	go s.run()
	go s.retrier.ReplayFromDisk(s.payloadChan, s.done)
	go s.routePayloads()
}

// Stop stops the worker,
// this call blocks until inputChan is flushed
func (s *worker) stop() {
	close(s.done)
	<-s.finished
}

// routPayloads takes all incoming payloads and routes them to one of two places:
// 1. Further in the pipeline if the pipeline is healthy
// 2. To disk if the pipeline is blocking (retrier will write to disk or no-op)
func (s *worker) routePayloads() {
	for {
		select {
		case payload := <-s.inputChan:
			// Try non-blocking send to run(), fallback to disk on backpressure
			select {
			case s.payloadChan <- payload:
				// Successfully sent to run()
			default:
				// Backpressure: write to disk (or drop if no retrier configured)
				if _, err := s.retrier.WritePayloadToDisk(payload); err != nil {
					log.Warnf("Failed to write payload to disk: %v", err)
				}
			}
		case <-s.done:
			return
		}
	}
}

func (s *worker) run() {
	noopSink := noopDestinationsSink(s.bufferSize)
	reliableOutputChan := s.outputChan
	if reliableOutputChan == nil {
		reliableOutputChan = noopSink
	}

	reliableDestinations := buildDestinationSenders(s.config, s.destinations.Reliable, reliableOutputChan, s.bufferSize)
	unreliableDestinations := buildDestinationSenders(s.config, s.destinations.Unreliable, noopSink, s.bufferSize)
	continueLoop := true
	for continueLoop {
		select {
		case payload, ok := <-s.payloadChan:
			if !ok {
				continueLoop = false
				continue
			}
			s.pipelineMonitor.ReportComponentEgress(payload, metrics.SenderTlmName, metrics.SenderTlmInstanceID)
			s.pipelineMonitor.ReportComponentIngress(payload, metrics.WorkerTlmName, s.workerID)
			s.utilization.Start()
			var startInUse = time.Now()
			senderDoneWg := &sync.WaitGroup{}

			sent := false
			for !sent {
				for _, destSender := range reliableDestinations {
					// Drop non-MRF payloads to MRF destinations
					if destSender.destination.IsMRF() && !payload.IsMRF() {
						log.Debugf("Dropping non-MRF payload to MRF destination: %s", destSender.destination.Target())
						sent = true
						continue
					}

					if destSender.Send(payload) {
						if destSender.destination.Metadata().ReportingEnabled {
							s.pipelineMonitor.ReportComponentIngress(payload, destSender.destination.Metadata().MonitorTag(), s.workerID)
						}
						sent = true
						if s.senderDoneChan != nil {
							senderDoneWg.Add(1)
							s.senderDoneChan <- senderDoneWg
						}
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
				// Drop non-MRF payloads to MRF destinations
				if destSender.destination.IsMRF() && !payload.IsMRF() {
					log.Debugf("Dropping non-MRF payload to MRF destination: %s", destSender.destination.Target())
					sent = true
					continue
				}
				// If an endpoint is stuck in the previous step, try to buffer the payloads if we have room to mitigate
				// loss on intermittent failures.
				if !destSender.lastSendSucceeded {
					if !destSender.NonBlockingSend(payload) {
						tlmPayloadsDropped.Inc("true", strconv.Itoa(i))
						tlmMessagesDropped.Add(float64(payload.Count()), "true", strconv.Itoa(i))
					}
				}
			}

			// Attempt to send to unreliable destinations
			for i, destSender := range unreliableDestinations {
				// Drop non-MRF payloads to MRF destinations
				if destSender.destination.IsMRF() && !payload.IsMRF() {
					log.Debugf("Dropping non-MRF payload to MRF destination: %s", destSender.destination.Target())
					sent = true
					continue
				}
				if !destSender.NonBlockingSend(payload) {
					tlmPayloadsDropped.Inc("false", strconv.Itoa(i))
					tlmMessagesDropped.Add(float64(payload.Count()), "false", strconv.Itoa(i))
					if s.senderDoneChan != nil {
						senderDoneWg.Add(1)
						s.senderDoneChan <- senderDoneWg
					}
				}
			}

			inUse := float64(time.Since(startInUse) / time.Millisecond)
			tlmSendWaitTime.Add(inUse)
			s.utilization.Stop()

			if s.senderDoneChan != nil && s.flushWg != nil {
				// Wait for all destinations to finish sending the payload
				senderDoneWg.Wait()
				// Decrement the wait group when this payload has been sent
				s.flushWg.Done()
			}
			s.pipelineMonitor.ReportComponentEgress(payload, metrics.WorkerTlmName, s.workerID)
		case <-s.done:
			continueLoop = false
		}
	}

	// Cleanup the destinations
	for _, destSender := range reliableDestinations {
		destSender.Stop()
	}
	for _, destSender := range unreliableDestinations {
		destSender.Stop()
	}
	close(noopSink)
	s.finished <- struct{}{}
}

// Drains the output channel from destinations that don't update the auditor.
func noopDestinationsSink(bufferSize int) chan *message.Payload {
	sink := make(chan *message.Payload, bufferSize)
	go func() {
		// drain channel, stop when channel is closed
		for msg := range sink {
			// Consume messages from channel until closed
			_ = msg
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
