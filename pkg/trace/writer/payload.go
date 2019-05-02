package writer

import (
	"container/list"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/trace/writer/backoff"
	writerconfig "github.com/DataDog/datadog-agent/pkg/trace/writer/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// payload represents a data payload to be sent to some endpoint
type payload struct {
	creationDate time.Time
	bytes        []byte
	headers      map[string]string
}

// newPayload constructs a new payload object with the provided data and with CreationDate initialized to the current
// time.
func newPayload(bytes []byte, headers map[string]string) *payload {
	return &payload{
		creationDate: time.Now(),
		bytes:        bytes,
		headers:      headers,
	}
}

// eventType is a type of event sent down the monitor channel.
type eventType int

const (
	eventTypeSuccess eventType = iota
	eventTypeFailure
	eventTypeRetry
)

var eventTypeStrings = map[eventType]string{
	eventTypeSuccess: "success",
	eventTypeFailure: "failure",
	eventTypeRetry:   "retry",
}

func (e eventType) String() string { return eventTypeStrings[e] }

type monitorEvent struct {
	typ        eventType
	payload    *payload
	stats      sendStats
	err        error
	retryDelay time.Duration
	retryNum   int
}

// sendStats represents basic stats related to the sending of a payload.
type sendStats struct {
	sendTime time.Duration
	host     string
}

// payloadSender represents an object capable of asynchronously sending payloads to some endpoint.
type payloadSender interface {
	Start()
	Run()
	Stop()
	Send(payload *payload)
	Monitor() <-chan monitorEvent

	setEndpoint(endpoint)
}

// queuableSender is a specific implementation of a payloadSender that will queue new payloads on error and
// retry sending them according to some configurable BackoffTimer.
type queuableSender struct {
	conf              writerconfig.QueuablePayloadSenderConf
	queuedPayloads    *list.List
	queuing           bool
	currentQueuedSize int64

	backoffTimer backoff.Timer

	// Test helper
	syncBarrier <-chan interface{}

	in        chan *payload
	monitorCh chan monitorEvent
	endpoint  endpoint

	exit chan struct{}
}

// newSender constructs a new QueuablePayloadSender with custom configuration to send payloads to
// the provided endpoint.
func newSender(e endpoint, conf writerconfig.QueuablePayloadSenderConf) *queuableSender {
	return &queuableSender{
		conf:           conf,
		queuedPayloads: list.New(),
		backoffTimer:   backoff.NewCustomExponentialTimer(conf.ExponentialBackoff),
		in:             make(chan *payload, conf.InChannelSize),
		monitorCh:      make(chan monitorEvent),
		endpoint:       e,
		exit:           make(chan struct{}),
	}
}

// Send sends a single isolated payload through this sender.
func (s *queuableSender) Send(payload *payload) {
	s.in <- payload
}

// Stop asks this sender to stop and waits until it correctly stops.
func (s *queuableSender) Stop() {
	s.exit <- struct{}{}
	<-s.exit
	close(s.in)
	close(s.monitorCh)
}

func (s *queuableSender) setEndpoint(e endpoint) {
	s.endpoint = e
}

// Monitor allows an external entity to monitor events of this sender by receiving Sender*Event structs.
func (s *queuableSender) Monitor() <-chan monitorEvent {
	return s.monitorCh
}

// send will send the provided payload without any checks.
func (s *queuableSender) doSend(payload *payload) (sendStats, error) {
	if payload == nil {
		return sendStats{}, nil
	}

	startFlush := time.Now()
	err := s.endpoint.write(payload)

	sendStats := sendStats{
		sendTime: time.Since(startFlush),
		host:     s.endpoint.baseURL(),
	}

	return sendStats, err
}

// Start asynchronously starts this QueueablePayloadSender.
func (s *queuableSender) Start() {
	go func() {
		defer watchdog.LogOnPanic()
		s.Run()
	}()
}

// Run executes the queuableSender main logic synchronously.
func (s *queuableSender) Run() {
	defer close(s.exit)

	for {
		select {
		case payload := <-s.in:
			if stats, err := s.sendOrQueue(payload); err != nil {
				log.Debugf("Error while sending or queueing payload. err=%v", err)
				s.notifyError(payload, err, stats)
			}
		case <-s.backoffTimer.ReceiveTick():
			s.flushQueue()
		case <-s.syncBarrier:
			// TODO: Is there a way of avoiding this? I want Promises in Go :(((
			// This serves as a barrier (assuming syncBarrier is an unbuffered channel). Used for testing
			continue
		case <-s.exit:
			log.Info("Exiting payload sender, try flushing whatever is left")
			s.flushQueue()
			return
		}
	}
}

// NumQueuedPayloads returns the number of payloads currently waiting in the queue for a retry
func (s *queuableSender) NumQueuedPayloads() int {
	return s.queuedPayloads.Len()
}

// sendOrQueue sends the provided payload or queues it if this sender is currently queueing payloads.
func (s *queuableSender) sendOrQueue(payload *payload) (sendStats, error) {
	var stats sendStats

	if payload == nil {
		return stats, nil
	}

	var err error

	if !s.queuing {
		if stats, err = s.doSend(payload); err != nil {
			if _, ok := err.(*retriableError); ok {
				// If error is retriable, start a queue and schedule a retry
				retryNum, delay := s.backoffTimer.ScheduleRetry(err)
				log.Debugf("Got retriable error. Starting a queue. delay=%s, err=%v", delay, err)
				s.notifyRetry(payload, err, delay, retryNum)
				return stats, s.enqueue(payload)
			}
		} else {
			// If success, notify
			log.Tracef("Successfully sent direct payload: %v", payload)
			s.notifySuccess(payload, stats)
		}
	} else {
		return stats, s.enqueue(payload)
	}

	return stats, err
}

func (s *queuableSender) enqueue(payload *payload) error {
	if !s.queuing {
		s.queuing = true
	}

	// Start by discarding payloads that are too old, freeing up memory
	s.discardOldPayloads()

	for s.conf.MaxQueuedPayloads > 0 && s.queuedPayloads.Len() >= s.conf.MaxQueuedPayloads {
		log.Debugf("Dropping existing payload because max queued payloads reached: %d", s.conf.MaxQueuedPayloads)
		if _, err := s.dropOldestPayload("max queued payloads reached"); err != nil {
			panic(fmt.Errorf("unable to respect max queued payloads value of %d", s.conf.MaxQueuedPayloads))
		}
	}

	newPayloadSize := int64(len(payload.bytes))

	if s.conf.MaxQueuedBytes > 0 && newPayloadSize > s.conf.MaxQueuedBytes {
		log.Debugf("Payload bigger than max size: size=%d, max size=%d", newPayloadSize, s.conf.MaxQueuedBytes)
		return fmt.Errorf("unable to queue payload bigger than max size: payload size=%d, max size=%d",
			newPayloadSize, s.conf.MaxQueuedBytes)
	}

	for s.conf.MaxQueuedBytes > 0 && s.currentQueuedSize+newPayloadSize > s.conf.MaxQueuedBytes {
		if _, err := s.dropOldestPayload("max queued bytes reached"); err != nil {
			// Should never happen because we know we can fit it in
			panic(fmt.Errorf("unable to find space for queueing payload of size %d: %v", newPayloadSize, err))
		}
	}

	log.Tracef("Queuing new payload: %v", payload)
	s.queuedPayloads.PushBack(payload)
	s.currentQueuedSize += newPayloadSize

	return nil
}

func (s *queuableSender) flushQueue() error {
	log.Debugf("Attempting to flush queue with %d payloads", s.NumQueuedPayloads())

	// Start by discarding payloads that are too old
	s.discardOldPayloads()

	// For the remaining ones, try to send them one by one
	var next *list.Element
	for e := s.queuedPayloads.Front(); e != nil; e = next {
		payload := e.Value.(*payload)

		var err error
		var stats sendStats

		if stats, err = s.doSend(payload); err != nil {
			if _, ok := err.(*retriableError); ok {
				// If send failed due to a retriable error, retry flush later
				retryNum, delay := s.backoffTimer.ScheduleRetry(err)
				log.Debugf("Got retriable error. Retrying flush later: retry=%d, delay=%s, err=%v",
					retryNum, delay, err)
				s.notifyRetry(payload, err, delay, retryNum)
				// Don't try to send following. We'll flush all later.
				return err
			}

			// If send failed due to non-retriable error, notify error and drop it
			log.Debugf("Dropping payload due to non-retriable error: err=%v, payload=%v", err, payload)
			s.notifyError(payload, err, stats)
			next = s.removeQueuedPayload(e)
			// Try sending next ones
			continue
		}

		// If successful, remove payload from queue
		log.Tracef("Successfully sent a queued payload: %v", payload)
		s.notifySuccess(payload, stats)
		next = s.removeQueuedPayload(e)
	}

	s.queuing = false
	s.backoffTimer.Reset()

	return nil
}

func (s *queuableSender) removeQueuedPayload(e *list.Element) *list.Element {
	next := e.Next()
	payload := e.Value.(*payload)
	s.currentQueuedSize -= int64(len(payload.bytes))
	s.queuedPayloads.Remove(e)
	return next
}

// Discard those payloads that are older than max age.
func (s *queuableSender) discardOldPayloads() {
	// If MaxAge <= 0 then age limitation is disabled so do nothing
	if s.conf.MaxAge <= 0 {
		return
	}

	var next *list.Element

	for e := s.queuedPayloads.Front(); e != nil; e = next {
		payload := e.Value.(*payload)

		age := time.Since(payload.creationDate)

		// Payloads are kept in order so as soon as we find one that isn't, we can break out
		if age < s.conf.MaxAge {
			break
		}

		err := fmt.Errorf("payload is older than max age: age=%v, max age=%v", age, s.conf.MaxAge)
		log.Tracef("Discarding payload: err=%v, payload=%v", err, payload)
		s.notifyError(payload, err, sendStats{})
		next = s.removeQueuedPayload(e)
	}
}

// Payloads are kept in order so dropping the one at the front guarantees we're dropping the oldest
func (s *queuableSender) dropOldestPayload(reason string) (*payload, error) {
	if s.queuedPayloads.Len() == 0 {
		return nil, fmt.Errorf("no queued payloads")
	}

	err := fmt.Errorf("payload dropped: %s", reason)
	droppedPayload := s.queuedPayloads.Front().Value.(*payload)
	s.removeQueuedPayload(s.queuedPayloads.Front())
	s.notifyError(droppedPayload, err, sendStats{})

	return droppedPayload, nil
}

func (s *queuableSender) notifySuccess(payload *payload, sendStats sendStats) {
	s.sendEvent(&monitorEvent{
		typ:     eventTypeSuccess,
		payload: payload,
		stats:   sendStats,
	})
}

func (s *queuableSender) notifyError(payload *payload, err error, sendStats sendStats) {
	s.sendEvent(&monitorEvent{
		typ:     eventTypeFailure,
		payload: payload,
		err:     err,
	})
}

func (s *queuableSender) notifyRetry(payload *payload, err error, delay time.Duration, retryNum int) {
	s.sendEvent(&monitorEvent{
		typ:        eventTypeRetry,
		payload:    payload,
		err:        err,
		retryDelay: delay,
		retryNum:   retryNum,
	})
}

func (s *queuableSender) sendEvent(event *monitorEvent) {
	s.monitorCh <- *event
}
