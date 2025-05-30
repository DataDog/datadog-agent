// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package writer contains the logic for sending payloads to the Datadog intake.
package writer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// newSenders returns a list of senders based on the given agent configuration, using climit
// as the maximum number of concurrent outgoing connections, writing to path.
func newSenders(cfg *config.AgentConfig, r eventRecorder, path string, climit, qsize int, telemetryCollector telemetry.TelemetryCollector, statsd statsd.ClientInterface) []*sender {
	if e := cfg.Endpoints; len(e) == 0 || e[0].Host == "" || e[0].APIKey == "" {
		panic(errors.New("config was not properly validated"))
	}
	maxConns := maxConns(climit, cfg.Endpoints)
	senders := make([]*sender, len(cfg.Endpoints))
	for i, endpoint := range cfg.Endpoints {
		url, err := url.Parse(endpoint.Host + path)
		if err != nil {
			telemetryCollector.SendStartupError(telemetry.InvalidIntakeEndpoint, err)
			log.Criticalf("Invalid host endpoint: %q", endpoint.Host)
			os.Exit(1)
		}
		senders[i] = newSender(&senderConfig{
			client:         cfg.NewHTTPClient(),
			maxConns:       int(maxConns),
			maxQueued:      qsize,
			maxRetries:     cfg.MaxSenderRetries,
			url:            url,
			apiKey:         endpoint.APIKey,
			recorder:       r,
			userAgent:      fmt.Sprintf("Datadog Trace Agent/%s/%s", cfg.AgentVersion, cfg.GitCommit),
			isMRF:          endpoint.IsMRF,
			MRFFailoverAPM: cfg.MRFFailoverAPM,
		}, statsd)
	}
	return senders
}

func maxConns(climit int, endpoints []*config.Endpoint) int {
	// spread out the the maximum connection limit (climit) between senders.
	// We exclude multi-region failover senders from this calculation, since they
	// will be inactive most of the time.

	// short-circuit the most common setup
	if len(endpoints) == 1 {
		return climit
	}
	n := 0
	for _, e := range endpoints {
		if !e.IsMRF {
			n++
		}
	}
	return int(math.Max(1, float64(climit/n)))
}

// eventRecorder implementations are able to take note of events happening in
// the sender.
type eventRecorder interface {
	// recordEvent notifies that event t has happened, passing details about
	// the event in data.
	recordEvent(t eventType, data *eventData)
}

// eventType specifies an event which occurred in the sender.
type eventType int

const (
	// eventTypeRetry specifies that a send failed with a retriable error (5xx).
	eventTypeRetry eventType = iota
	// eventTypeSent specifies that a single payload was successfully sent.
	eventTypeSent
	// eventTypeRejected specifies that the edge rejected this payload.
	eventTypeRejected
	// eventTypeDropped specifies that a payload had to be dropped to make room
	// in the queue.
	eventTypeDropped
)

var eventTypeStrings = map[eventType]string{
	eventTypeRetry:    "eventTypeRetry",
	eventTypeSent:     "eventTypeSent",
	eventTypeRejected: "eventTypeRejected",
	eventTypeDropped:  "eventTypeDropped",
}

// String implements fmt.Stringer.
func (t eventType) String() string { return eventTypeStrings[t] }

// eventData represents information about a sender event. Not all fields apply
// to all events.
type eventData struct {
	// host specifies the host which the sender is sending to.
	host string
	// bytes represents the number of bytes affected by this event.
	bytes int
	// count specfies the number of payloads that this events refers to.
	count int
	// duration specifies the time it took to complete this event. It
	// is set for eventType{Sent,Retry,Rejected}.
	duration time.Duration
	// err specifies the error that may have occurred on events eventType{Retry,Rejected}.
	err error
	// connectionFill specifies the percentage of allowed connections used.
	// At 100% (1.0) the writer will become blocking.
	connectionFill float64
	// queueFill specifies how flul the queue is. It's a floating point number ranging
	// between 0 (0%) and 1 (100%).
	queueFill float64
}

// senderConfig specifies the configuration for the sender.
type senderConfig struct {
	// client specifies the HTTP client to use when sending requests.
	client *config.ResetClient
	// url specifies the URL to send requests too.
	url *url.URL
	// apiKey specifies the Datadog API key to use.
	apiKey string
	// maxConns specifies the maximum number of allowed concurrent ougoing
	// connections.
	maxConns int
	// maxQueued specifies the maximum number of payloads allowed in the queue.
	// When it is surpassed, oldest items get dropped to make room for new ones.
	maxQueued int
	// maxRetries specifies the maximum number of times a payload submission to
	// intake will be retried before being dropped.
	maxRetries int
	// recorder specifies the eventRecorder to use when reporting events occurring
	// in the sender.
	recorder eventRecorder
	// userAgent is the computed user agent we'll use when communicating with Datadog
	userAgent string
	// IsMRF determines whether this is a Multi-Region Failover endpoint.
	isMRF bool
	// MRFFailoverAPM determines whether APM data should be failed over to the secondary (MRF) DC.
	MRFFailoverAPM func() bool
}

// sender is responsible for sending payloads to a given URL. It uses a size-limited
// retry queue with a backoff mechanism in case of retriable errors.
type sender struct {
	cfg *senderConfig

	queue      chan *payload // payload queue
	inflight   *atomic.Int32 // inflight payloads
	maxRetries int32

	mu      sync.RWMutex // guards closed
	closed  bool         // closed reports if the loop is stopped
	statsd  statsd.ClientInterface
	enabled bool // false on inactive MRF senders. True otherwise
}

// newSender returns a new sender based on the given config cfg.
func newSender(cfg *senderConfig, statsd statsd.ClientInterface) *sender {
	s := sender{
		cfg:        cfg,
		queue:      make(chan *payload, cfg.maxQueued),
		inflight:   atomic.NewInt32(0),
		maxRetries: int32(cfg.maxRetries),
		statsd:     statsd,
		enabled:    true,
	}
	for i := 0; i < cfg.maxConns; i++ {
		go s.loop()
	}
	return &s
}

// loop runs the main sender loop.
func (s *sender) loop() {
	for p := range s.queue {
		s.sendPayload(p)
	}
}

// backoff triggers a sleep period proportional to the retry attempt, if any.
func (s *sender) backoff(attempt int) {
	delay := backoffDuration(attempt)
	if delay == 0 {
		return
	}
	time.Sleep(delay)
}

// Stop stops the sender. It attempts to wait for all inflight payloads to complete
// with a timeout of 5 seconds.
func (s *sender) Stop() {
	s.WaitForInflight()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	close(s.queue)
}

// WaitForInflight blocks until all in progress payloads are sent,
// or the timeout is reached.
func (s *sender) WaitForInflight() {
	timeout := time.After(5 * time.Second)
outer:
	for {
		select {
		case <-timeout:
			break outer
		default:
			if s.inflight.Load() == 0 {
				break outer
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// Push pushes p onto the sender's queue, to be written to the destination.
func (s *sender) Push(p *payload) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()
	select {
	case s.queue <- p:
	default:
		_ = s.statsd.Count("datadog.trace_agent.sender.push_blocked", 1, nil, 1)
		s.queue <- p
	}
	s.inflight.Inc()
}

// sendPayload sends the payload p to the destination URL.
func (s *sender) sendPayload(p *payload) {
	for attempt := 0; ; attempt++ {
		s.backoff(attempt)
		if s.sendOnce(p) {
			return
		}
	}
}

// sendOnce attempts to send the payload one time, returning
// whether or not the payload is "finished" either because it was
// sent, or because sending encountered a non-retryable error.
func (s *sender) sendOnce(p *payload) bool {
	req, err := p.httpRequest(s.cfg.url)
	if err != nil {
		log.Errorf("http.Request: %s", err)
		return true
	}
	start := time.Now()
	err = s.do(req)
	stats := &eventData{
		bytes:    p.body.Len(),
		count:    1,
		duration: time.Since(start),
		err:      err,
	}
	if err != nil {
		log.Tracef("Error submitting payload: %v\n", err)
	}
	switch err.(type) {
	case *retriableError:
		// request failed again, but can be retried
		s.mu.RLock()
		defer s.mu.RUnlock()
		if s.closed {
			s.releasePayload(p, eventTypeDropped, stats)
			// sender is stopped
			return true
		}

		if r := p.retries.Inc(); (r&(r-1)) == 0 && r > 3 {
			// Only log a warning if the retry attempt is a power of 2
			// and larger than 3, to avoid alerting the user unnecessarily.
			// e.g. attempts 4, 8, 16, etc.
			log.Warnf("Retried payload %d times: %s", r, err.Error())
		}
		if p.retries.Load() >= s.maxRetries {
			log.Warnf("Dropping Payload after %d retries, due to: %v.\n", p.retries.Load(), err)
			// queue is full; since this is the oldest payload, we drop it
			s.releasePayload(p, eventTypeDropped, stats)
			return true
		}
		s.recordEvent(eventTypeRetry, stats)
		return false
	case nil:
		s.releasePayload(p, eventTypeSent, stats)
	default:
		// this is a fatal error, we have to drop this payload
		log.Warnf("Dropping Payload due to non-retryable error: %v.\n", err)
		s.releasePayload(p, eventTypeRejected, stats)
	}
	return true
}

// waitForSenders blocks until all senders have sent their inflight payloads
func waitForSenders(senders []*sender) {
	var wg sync.WaitGroup
	for _, s := range senders {
		wg.Add(1)
		go func(s *sender) {
			defer wg.Done()
			s.WaitForInflight()
		}(s)
	}
	wg.Wait()
}

// releasePayload releases the payload p and records the specified event. The payload
// should not be used again after a release.
func (s *sender) releasePayload(p *payload, t eventType, data *eventData) {
	s.recordEvent(t, data)
	ppool.Put(p)
	s.inflight.Dec()
}

// recordEvent records the occurrence of the given event type t. It additionally
// passes on the data and augments it with additional information.
func (s *sender) recordEvent(t eventType, data *eventData) {
	if s.cfg.recorder == nil {
		return
	}
	data.host = s.cfg.url.Hostname()
	data.connectionFill = float64(s.inflight.Load())
	data.queueFill = float64(len(s.queue)) / float64(cap(s.queue))
	s.cfg.recorder.recordEvent(t, data)
}

// isEnabled returns true if the sender is enabled. Non-MRF senders are always enabled, and MRF ones
// only when isMRFEnabled is true
func (s *sender) isEnabled() bool {
	if !s.cfg.isMRF || s.cfg.MRFFailoverAPM == nil {
		return true
	}
	// Endpoint is MRF and MRF is enabled. Figure out if we need to failover APM data
	if s.cfg.MRFFailoverAPM() {
		if !s.enabled {
			log.Infof("Sender for domain %v has been failed over to, enabling it for MRF.", s.cfg.url)
			s.enabled = true
		}
		return true
	}

	if s.enabled {
		s.enabled = false
		log.Infof("Sender for domain %v was disabled; payloads will be dropped for this domain.", s.cfg.url)
	} else {
		log.Debugf("Sender for domain %v is disabled; dropping payload for this domain.", s.cfg.url)
	}
	return false
}

// retriableError is an error returned by the server which may be retried at a later time.
type retriableError struct{ err error }

// Error implements error.
func (e retriableError) Error() string { return e.err.Error() }

const (
	headerAPIKey    = "DD-Api-Key"
	headerUserAgent = "User-Agent"
)

func (s *sender) do(req *http.Request) error {
	req.Header.Set(headerAPIKey, s.cfg.apiKey)
	req.Header.Set(headerUserAgent, s.cfg.userAgent)
	resp, err := s.cfg.client.Do(req)
	if err != nil {
		// request errors include timeouts or name resolution errors and
		// should thus be retried.
		return &retriableError{err}
	}
	// From https://golang.org/pkg/net/http/#Response:
	// The default HTTP client's Transport may not reuse HTTP/1.x "keep-alive"
	// TCP connections if the Body is not read to completion and closed.
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		log.Debugf("Error discarding request body: %v", err)
	}
	resp.Body.Close()

	if isRetriable(resp.StatusCode) {
		return &retriableError{
			fmt.Errorf("server responded with %q", resp.Status),
		}
	}
	if resp.StatusCode/100 != 2 {
		// status codes that are neither 2xx nor 5xx are considered
		// non-retriable failures
		return errors.New(resp.Status)
	}
	return nil
}

// isRetriable reports whether the give HTTP status code should be retried.
func isRetriable(code int) bool {
	if code == http.StatusRequestTimeout {
		return true
	}
	// 5xx errors can be retried
	return code/100 == 5
}

// payloads specifies a payload to be sent by the sender.
type payload struct {
	body    *bytes.Buffer     // request body
	headers map[string]string // request headers
	retries *atomic.Int32     // number of retries sending this payload
}

// ppool is a pool of payloads.
var ppool = &sync.Pool{
	New: func() interface{} {
		return &payload{
			body:    &bytes.Buffer{},
			headers: make(map[string]string),
			retries: atomic.NewInt32(0),
		}
	},
}

// newPayload returns a new payload with the given headers. The payload should not
// be used anymore after it has been given to the sender.
func newPayload(headers map[string]string) *payload {
	p := ppool.Get().(*payload)
	p.body.Reset()
	p.headers = headers
	p.retries.Store(0)
	return p
}

func (p *payload) clone() *payload {
	headers := make(map[string]string, len(p.headers))
	maps.Copy(headers, p.headers)
	clone := newPayload(headers)
	if _, err := clone.body.ReadFrom(bytes.NewBuffer(p.body.Bytes())); err != nil {
		log.Errorf("Error cloning writer payload: %v", err)
	}
	return clone
}

// httpRequest returns an HTTP request based on the payload, targeting the given URL.
func (p *payload) httpRequest(url *url.URL) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, url.String(), bytes.NewReader(p.body.Bytes()))
	if err != nil {
		// this should never happen with sanitized data (invalid method or invalid url)
		return nil, err
	}
	for k, v := range p.headers {
		req.Header.Add(k, v)
	}
	req.Header.Add("Content-Length", strconv.Itoa(p.body.Len()))
	return req, nil
}

// stopSenders attempts to simultaneously stop a group of senders.
func stopSenders(senders []*sender) {
	var wg sync.WaitGroup
	for _, s := range senders {
		wg.Add(1)
		go func(s *sender) {
			defer wg.Done()
			s.Stop()
		}(s)
	}
	wg.Wait()
}

// sendPayloads sends the payload p to all senders.
func sendPayloads(senders []*sender, p *payload, syncMode bool) {
	enabledSenders := make([]*sender, 0, len(senders))
	for _, s := range senders {
		if s.isEnabled() {
			enabledSenders = append(enabledSenders, s)
		}
	}

	if syncMode {
		defer waitForSenders(enabledSenders)
	}

	if len(enabledSenders) == 1 {
		// fast path
		enabledSenders[0].Push(p)
		return
	}
	// Create a clone for each payload because each sender places payloads
	// back onto the pool after they are sent.
	payloads := make([]*payload, 0, len(enabledSenders))
	// Perform all the clones before any sends are to ensure the original
	// payload body is completely unread.
	for i := range enabledSenders {
		if i == 0 {
			payloads = append(payloads, p)
		} else {
			payloads = append(payloads, p.clone())
		}
	}
	for i, sender := range enabledSenders {
		sender.Push(payloads[i])
	}
}

const (
	// backoffBase specifies the multiplier base for the backoff duration algorithm.
	backoffBase = 100 * time.Millisecond
	// backoffMaxDuration is the maximum permitted backoff duration.
	backoffMaxDuration = 10 * time.Second
)

// backoffDuration returns the backoff duration necessary for the given attempt.
// The formula is "Full Jitter":
//
//	random_between(0, min(cap, base * 2 ** attempt))
//
// https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
var backoffDuration = func(attempt int) time.Duration {
	if attempt == 0 {
		return 0
	}
	maxPow := float64(backoffMaxDuration / backoffBase)
	pow := math.Min(math.Pow(2, float64(attempt)), maxPow)
	ns := int64(float64(backoffBase) * pow)
	return time.Duration(rand.Int63n(ns))
}
