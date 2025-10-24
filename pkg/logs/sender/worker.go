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
	"github.com/DataDog/datadog-agent/pkg/logs/sender/diskretry"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tlmPayloadsDropped = telemetry.NewCounterWithOpts("logs_sender", "payloads_dropped", []string{"reliable", "destination"}, "Payloads dropped", telemetry.Options{DefaultMetric: true})
	tlmMessagesDropped = telemetry.NewCounterWithOpts("logs_sender", "messages_dropped", []string{"reliable", "destination"}, "Messages dropped", telemetry.Options{DefaultMetric: true})
	tlmSendWaitTime    = telemetry.NewCounter("logs_sender", "send_wait", []string{}, "Time spent waiting for all sends to finish")
)

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
	outputChan     chan *message.Payload
	destinations   *client.Destinations
	done           chan struct{}
	finished       chan struct{}
	bufferSize     int
	senderDoneChan chan *sync.WaitGroup
	flushWg        *sync.WaitGroup
	sink           Sink
	workerID       string
	diskRetryQueue *diskretry.Queue

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
	diskRetryQueue *diskretry.Queue,
) *worker {
	var senderDoneChan chan *sync.WaitGroup
	var flushWg *sync.WaitGroup

	if serverlessMeta.IsEnabled() {
		senderDoneChan = serverlessMeta.SenderDoneChan()
		flushWg = serverlessMeta.WaitGroup()
	}
	return &worker{
		config:         config,
		inputChan:      inputChan,
		sink:           sink,
		destinations:   destinationFactory(workerID),
		bufferSize:     bufferSize,
		senderDoneChan: senderDoneChan,
		flushWg:        flushWg,
		done:           make(chan struct{}),
		finished:       make(chan struct{}),
		workerID:       workerID,
		diskRetryQueue: diskRetryQueue,

		// Telemetry
		pipelineMonitor: pipelineMonitor,
		utilization:     pipelineMonitor.MakeUtilizationMonitor(metrics.WorkerTlmName, workerID),
	}
}

// Start starts the worker.
func (s *worker) start() {
	s.outputChan = s.sink.Channel()

	go s.run()
}

// Stop stops the worker,
// this call blocks until inputChan is flushed
func (s *worker) stop() {
	s.done <- struct{}{}
	<-s.finished
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
	wasFailingBefore := false // Track if we were writing to disk in previous iteration
	for continueLoop {
		// Check if destinations have recovered and we need to replay from disk
		if wasFailingBefore && s.canSendToDestinations(reliableDestinations) {
			log.Infof("Worker %s: Destinations recovered, replaying from disk before processing new logs", s.workerID)
			s.replayFromDisk(reliableDestinations)
			wasFailingBefore = false
		}

		select {
		case payload := <-s.inputChan:
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
					// All reliable destinations have failed. Write to disk if enabled.
					if s.diskRetryQueue != nil {
						if err := s.diskRetryQueue.Add(payload, s.workerID); err != nil {
							log.Errorf("Failed to persist payload to disk: %v", err)
							// Fall through to throttle and retry
						} else {
							log.Debugf("Persisted payload to disk (%d messages) after all destinations failed", payload.Count())
							// Mark that we're in failure mode - will trigger replay when destinations recover
							wasFailingBefore = true
							// Successfully persisted - break out of retry loop
							sent = true
							break
						}
					}

					// Throttle the poll loop while waiting for a send to succeed
					// This will only happen when all reliable destinations
					// are blocked and we either have no disk queue or disk write failed.
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

// canSendToDestinations checks if at least one reliable destination is ready to accept payloads
func (s *worker) canSendToDestinations(destinations []*DestinationSender) bool {
	for _, destSender := range destinations {
		// Check if destination is not in retry state
		destSender.retryLock.Lock()
		isRetrying := destSender.lastRetryState
		destSender.retryLock.Unlock()

		if !isRetrying {
			return true
		}
	}
	return false
}

// replayFromDisk attempts to replay all persisted payloads from disk before processing new logs
// This ensures logs are delivered in order: old logs from disk first, then new logs from pipeline
func (s *worker) replayFromDisk(reliableDestinations []*DestinationSender) {
	if s.diskRetryQueue == nil {
		return
	}

	payloads, err := s.diskRetryQueue.List()
	if err != nil {
		log.Warnf("Failed to list payloads for replay: %v", err)
		return
	}

	if len(payloads) == 0 {
		return
	}

	log.Infof("Worker %s: Replaying %d payload(s) from disk before processing new logs", s.workerID, len(payloads))

	replayedCount := 0
	for _, pp := range payloads {
		// Check if payload should still be retried
		config := s.diskRetryQueue.GetConfig()
		if !pp.ShouldRetry(config.MaxAge, config.MaxRetries) {
			log.Infof("Worker %s: Discarding stale payload: age=%s, retries=%d", s.workerID, pp.Age(), pp.RetryCount)
			if err := s.diskRetryQueue.Delete(pp); err != nil {
				log.Warnf("Failed to delete stale payload: %v", err)
			}
			continue
		}

		payload := pp.ToPayload()

		// Try to send to destinations
		sent := false
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
				break
			}
		}

		if sent {
			// Successfully replayed - delete from disk
			if err := s.diskRetryQueue.Delete(pp); err != nil {
				log.Warnf("Failed to delete replayed payload: %v", err)
			} else {
				replayedCount++
				log.Debugf("Worker %s: Successfully replayed payload (age: %s, retries: %d)",
					s.workerID, pp.Age(), pp.RetryCount)
			}
		} else {
			// Failed to send - update retry count and stop replaying
			log.Infof("Worker %s: Replay stopped after %d payloads, destinations failing again", s.workerID, replayedCount)
			if err := s.diskRetryQueue.UpdateRetryMetadata(pp); err != nil {
				log.Warnf("Failed to update retry metadata: %v", err)
			}
			return
		}
	}

	if replayedCount > 0 {
		log.Infof("Worker %s: Successfully replayed %d payload(s) from disk", s.workerID, replayedCount)
	}
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
