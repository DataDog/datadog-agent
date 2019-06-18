package writer

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/osutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// newSenders returns a list of senders based on the given agent configuration, using climit
// as the maximum number of concurrent outgoing connections, writing to path. The given
// namespace is used as a prefix for reported metrics.
func newSenders(cfg *config.AgentConfig, r eventRecorder, path string, climit int) []*sender {
	if e := cfg.Endpoints; len(e) == 0 || e[0].Host == "" || e[0].APIKey == "" {
		panic(errors.New("config was not properly validated"))
	}
	client := cfg.HTTPClient()
	// spread out the the maximum connection limit (climit) between senders
	maxConns := math.Max(1, float64(climit/len(cfg.Endpoints)))
	senders := make([]*sender, len(cfg.Endpoints))
	for i, endpoint := range cfg.Endpoints {
		url, err := url.Parse(endpoint.Host + path)
		if err != nil {
			osutil.Exitf("Invalid host endpoint: %q", endpoint.Host)
		}
		senders[i] = newSender(&senderConfig{
			client:   client,
			maxConns: int(maxConns),
			url:      url,
			apiKey:   endpoint.APIKey,
			recorder: r,
		})
	}
	return senders
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
	// eventTypeFlushed specifies that a list of one or more payloads was flushed.
	eventTypeFlushed
	// eventTypeSent specifies that a single payload was successfully sent.
	eventTypeSent
	// eventTypeFailed specifies that a payload failed to send and data was lost.
	eventTypeFailed
	// eventTypeDropped specifies that a payload had to be dropped to make room
	// in the queue.
	eventTypeDropped
)

var eventTypeStrings = map[eventType]string{
	eventTypeRetry:   "eventTypeRetry",
	eventTypeFlushed: "eventTypeFlushed",
	eventTypeSent:    "eventTypeSent",
	eventTypeFailed:  "eventTypeFailed",
	eventTypeDropped: "eventTypeDropped",
}

// String implements fmt.Stringer.
func (t eventType) String() string { return eventTypeStrings[t] }

// eventData represents information about a sender event. Not all fields apply
// to all events.
type eventData struct {
	// host specifies the host which the sender is sending to.
	host string
	// bytes represents the number of bytes affected by this event. It is
	// not known for eventTypeFlushed.
	bytes int
	// count specfies the number of payloads that this events refers to.
	count int
	// duration specifies the time it took to complete this event. It
	// is set for eventType{Flushed,Sent,Retry,Failed}.
	duration time.Duration
	// err specifies the error that may have occurred on events like
	// eventTypeRetry and eventTypeFailed.
	err error
	// connectionFill specifies the percentage of allowed connections used.
	// At 100% (1.0) the writer will become blocking. It is reported for events
	// eventType{Sent,Retry,Failed}.
	connectionFill float64
}

// maxQueueSize specifies the maximum allowed queue size. If it is surpassed, older
// items are dropped to make room for new ones.
var maxQueueSize = 64 * 1024 * 1024 // 64MB; replaced in tests

// sender is responsible for sending payloads to a given URL. It uses a size-limited
// retry queue with a backoff mechanism in case of retriable errors.
type sender struct {
	cfg  *senderConfig
	host string

	climit chan struct{} // acts as a semaphore for limiting concurrent connections

	mu        sync.Mutex  // guards below fields
	list      *list.List  // send queue
	size      int         // size of send queue
	scheduled bool        // reports if a flush is scheduled
	attempt   int         // retry attempt coming up next
	timer     *time.Timer // timed flush (triggered by backoff)
}

// senderConfig specifies the configuration for the sender.
type senderConfig struct {
	// client specifies the HTTP client to use when sending requests.
	client *http.Client
	// url specifies the URL to send requests too.
	url *url.URL
	// apiKey specifies the Datadog API key to use.
	apiKey string
	// maxConns specifies the maximum number of allowed concurrent ougoing
	// connections.
	maxConns int
	// recorder specifies the eventRecorder to use when reporting events occurring
	// in the sender.
	recorder eventRecorder
}

// newSender creates a new sender, which uses the given HTTP client, URL and API key
// to make requests using incoming payloads. The limit of outgoing concurrent connections
// is defined by climit.
func newSender(cfg *senderConfig) *sender {
	return &sender{
		cfg:    cfg,
		climit: make(chan struct{}, cfg.maxConns),
		list:   list.New(),
	}
}

// Push pushes p onto the sender, to be written to the destination.
func (q *sender) Push(p *payload) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// enqueue the payload
	q.enqueueLocked(p)

	if !q.scheduled {
		// no flush is scheduled; start one
		q.scheduleFlushLocked(0)
	}
}

// scheduleFlushLocked schedules the next flush using the given delay.
func (q *sender) scheduleFlushLocked(delay time.Duration) {
	switch {
	case delay == 0:
		go q.flush()
	case q.timer == nil:
		q.timer = time.AfterFunc(delay, q.flush)
	default:
		q.timer.Stop()
		q.timer.Reset(delay)
	}
	q.scheduled = true
}

// flush drains and sends the entire queue. If anything comes in while flushing
// or if some of the payloads fail to send as retriable, further follow up flushes
// are scheduled.
func (q *sender) flush() {
	startTime := time.Now()

	// we drain the queue, which is a blocking operation, meaning that
	// new payloads are stopped from coming in while this happens; but
	// it is a fast.
	payloads := q.drainQueue()

	// we send the payloads we've retrieved; while we do this, more payloads
	// can join the list.
	done, retries := q.sendPayloads(payloads)
	if done > 0 {
		q.recordEvent(eventTypeFlushed, &eventData{
			count:    int(done),
			duration: time.Since(startTime),
		})
	}

	// we reassess the state of the list and check if further flushes need
	// to be scheduled
	q.mu.Lock()
	defer q.mu.Unlock()
	switch {
	case retries > 0:
		// some sends failed as retriable; we need to back off a bit
		q.attempt++
		delay := backoffDuration(q.attempt)
		q.scheduleFlushLocked(delay)
	case q.list.Len() > 0:
		// new items came in while we were flushing; schedule the next flush immediately.
		q.scheduleFlushLocked(0)
	default:
		// everything is flushed
		q.attempt = 0
		q.scheduled = false
	}
}

// drainQueue drains the entire queue and returns all the payloads that were in it.
func (q *sender) drainQueue() []*payload {
	q.mu.Lock()
	defer q.mu.Unlock()
	var payloads []*payload
	for q.list.Len() > 0 {
		v := q.list.Remove(q.list.Front())
		payloads = append(payloads, v.(*payload))
	}
	q.size = 0
	return payloads
}

// sendPayloads concurrently sends the given list of payloads. It returns
// the number of payloads that were added back onto the queue to be retried
// again later due to an error.
func (q *sender) sendPayloads(payloads []*payload) (done, retries uint64) {
	var wg sync.WaitGroup
	for _, p := range payloads {
		q.climit <- struct{}{}
		wg.Add(1)
		go func(p *payload) {
			defer func() { <-q.climit }()
			defer wg.Done()
			req, err := p.httpRequest(q.cfg.url)
			if err != nil {
				log.Errorf("http.Request: %s", err)
				return
			}
			start := time.Now()
			err = q.do(req)
			stats := &eventData{
				bytes:          p.body.Len(),
				count:          1,
				duration:       time.Since(start),
				err:            err,
				connectionFill: float64(len(q.climit)) / float64(cap(q.climit)),
			}
			switch err.(type) {
			case *retriableError:
				// request failed again, but can be retried
				q.enqueueAgain(p)
				atomic.AddUint64(&retries, 1)
				q.recordEvent(eventTypeRetry, stats)
				return
			case nil:
				// request was successful
				atomic.AddUint64(&done, 1)
				q.recordEvent(eventTypeSent, stats)
			default:
				// this is a fatal error, we have to drop this payload
				q.recordEvent(eventTypeFailed, stats)
			}
			ppool.Put(p)
		}(p)
	}
	wg.Wait()
	return
}

// recordEvent records that event t has happened and attaches it the given data.
func (q *sender) recordEvent(t eventType, data *eventData) {
	if recorder := q.cfg.recorder; recorder != nil {
		data.host = q.cfg.url.Hostname()
		recorder.recordEvent(t, data)
	}
}

// waitEmpty waits up to 5 seconds for the queue to reach an empty state and for all scheduling
// to complete before returning.
func (q *sender) waitEmpty() {
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-timeout:
			return
		case <-ticker.C:
			q.mu.Lock()
			drained := q.list.Len() == 0 && !q.scheduled
			q.mu.Unlock()
			if drained {
				return
			}
		}
	}
}

// enqueue enqueues the given payload. If there is no room in the queue, it drops oldest
// payloads until there is.
func (q *sender) enqueue(p *payload) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.enqueueLocked(p)
}

// enqueueLocked adds p onto the queue, making room if needed.
func (q *sender) enqueueLocked(p *payload) {
	size := p.body.Len()
	for q.size+size > maxQueueSize {
		// make room
		v := q.list.Remove(q.list.Front())
		size := v.(*payload).body.Len()
		q.size -= size
		q.recordEvent(eventTypeDropped, &eventData{bytes: size, count: 1})
	}
	q.list.PushBack(p)
	q.size += size
}

// enqueueAgain attempts to enqueue the payload p into the retry queue. p is considered to
// have been part of the queue before and as such, is older than any other item in the queue.
// If there is no room in the queue, it will be dropped.
func (q *sender) enqueueAgain(p *payload) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.size+p.body.Len() > maxQueueSize {
		return
	}
	q.enqueueLocked(p)
}

// userAgent is the computed user agent we'll use when communicating with Datadog var
var userAgent = fmt.Sprintf("Datadog Trace Agent/%s/%s", info.Version, info.GitCommit)

// retriableError is an error returned by the server which may be retried at a later time.
type retriableError struct{ err error }

// Error implements error.
func (e retriableError) Error() string { return e.err.Error() }

const (
	headerAPIKey    = "DD-Api-Key"
	headerUserAgent = "User-Agent"
)

// do performs the given http.Request.
func (q *sender) do(req *http.Request) error {
	req.Header.Set(headerAPIKey, q.cfg.apiKey)
	req.Header.Set(headerUserAgent, userAgent)
	resp, err := q.cfg.client.Do(req)
	if err != nil {
		// request errors are either redirect errors, url errors
		return &retriableError{err}
	}
	if resp.StatusCode/100 == 5 {
		// 5xx errors can be retried
		return &retriableError{
			fmt.Errorf("server responded with %q", resp.Status),
		}
	}
	if resp.StatusCode/100 != 2 {
		// non-2xx errors are failures
		return errors.New(resp.Status)
	}
	return nil
}

// payloads specifies a payload to be sent by the sender.
type payload struct {
	body    *bytes.Buffer     // request body
	headers map[string]string // request headers
}

// ppool is a pool of payloads.
var ppool = sync.Pool{
	New: func() interface{} {
		return &payload{
			body:    &bytes.Buffer{},
			headers: make(map[string]string),
		}
	},
}

// newPayload creates a new payload with the given headers.
func newPayload(headers map[string]string) *payload {
	p := ppool.Get().(*payload)
	p.body.Reset()
	for k := range p.headers {
		delete(p.headers, k)
	}
	for k, v := range headers {
		p.headers[k] = v
	}
	return p
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
			s.waitEmpty()
		}(s)
	}
	wg.Wait()
}

const (
	// backoffBase specifies the multiplier base for the backoff duration algorithm.
	backoffBase = 100 * time.Millisecond
	// backoffMaxDuration is the maximum permitted backoff duration.
	backoffMaxDuration = 10 * time.Second
)

// backoffDuration returns the backoff duration necessary for the given attempt.
// The formula is "Full Jitter":
//   random_between(0, min(cap, base * 2 ** attempt))
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
