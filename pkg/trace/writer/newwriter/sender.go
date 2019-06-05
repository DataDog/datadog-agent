package writer

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// maxQueueSize specifies the maximum allowed queue size. If it is surpassed, older
// items are dropped to make room for new ones.
var maxQueueSize = 64 * 1024 * 1024 // 64MB; replaced in tests

// sender books payloads for being sent to a given URL by a given client. It uses a
// size-limited retry queue with a backoff mechanism in case of retriable errors.
type sender struct {
	cfg *senderConfig

	wg     sync.WaitGroup // waits for all uploads
	climit chan struct{}  // acts as a semaphore for limiting concurrent connections
	stats  *senderStats   // statistics about the state of the writer

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
	url string
	// apiKey specifies the Datadog API key to use.
	apiKey string
	// maxConns specifies the maximum number of allowed concurrent ougoing
	// connections.
	maxConns int
	// metricNamespace specifies the namespace to be used when reporting
	// sender metrics.
	metricNamespace string
}

// newSender creates a new sender, which uses the given HTTP client, URL and API key
// to make requests using incoming payloads. The limit of outgoing concurrent connections
// is defined by climit.
func newSender(cfg *senderConfig) *sender {
	return &sender{
		cfg:    cfg,
		climit: make(chan struct{}, cfg.maxConns),
		list:   list.New(),
		stats:  &senderStats{},
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
		q.scheduled = true
		q.wg.Add(1)
		go q.flush()
	}
}

func (q *sender) flush() {
	// we drain the queue, which is a blocking operation, meaning that
	// new payloads are stopped from coming in while this happens; but
	// it is a fast.
	payloads := q.drainQueue()

	// we send the payloads we've retrieved; while we do this, more payloads
	// can join the list.
	retry := q.sendPayloads(payloads)

	// we reassess the state of the list and check if further flushes need
	// to be scheduled
	q.mu.Lock()
	defer q.mu.Unlock()
	defer q.wg.Done()

	if q.list.Len() == 0 {
		// the list is empty; no further flushing needs to be triggered
		q.attempt = 0
		q.scheduled = false
		return
	}
	// the list is not empty
	q.scheduled = true
	if retry > 0 {
		// some sends failed as retriable; we need to back off a bit
		q.attempt++
		q.wg.Add(1)
		q.timer = time.AfterFunc(backoffDuration(q.attempt), q.flush)
		return
	}
	// all items in the list are new; flush immediately
	q.wg.Add(1)
	go q.flush()
}

// drainQueue empties the entire queue and returns all the payloads that
// were stored in it.
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
// again.
func (q *sender) sendPayloads(payloads []*payload) (retry uint64) {
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
			switch err := q.do(req).(type) {
			case *retriableError:
				// request failed again, but can be retried
				q.enqueue(p)
				atomic.AddUint64(&retry, 1)
				log.Debugf("Payload failed to send. Retrying later (%v).", err)
			case nil:
				// request was successful
				log.Debugf("Successfully flushed %.2fkb.", float64(len(p.body))/1024)
			default:
				// this is a fatal error, we have to drop this payload
				log.Errorf("Error sending payload: %v (dropped %.2fkb)", err, float64(len(p.body))/1024)
			}
		}(p)
	}
	wg.Wait()
	return retry
}

// Flush waits up to 5 seconds for the queue to reach an empty state and for all scheduling
// to complete before returning.
func (q *sender) Flush() {
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

// enqueue enqueues the given payloads.
func (q *sender) enqueue(p *payload) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.enqueueLocked(p)
}

// enqueueLocked adds p onto the queue, making room if needed.
func (q *sender) enqueueLocked(p *payload) {
	size := len(p.body)
	for q.size+size > maxQueueSize {
		// make room
		v := q.list.Remove(q.list.Front())
		q.size -= len(v.(*payload).body)
		// TODO: log and metric
	}
	q.list.PushBack(p)
	q.size += size
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

// senderStats keeps track of sender statistic.
type senderStats struct {
	retry  uint64 // sends resulting in 5xx or proxy errors
	ok     uint64 // sends resulting in 2xx
	failed uint64 // sends failed with non-2xx
	bytes  uint64 // bytes written
	lost   uint64 // payloads lost due to queue being full
}

// resetStats resets the sender's internal stats and returns them as they were before the reset.
func (q *sender) resetStats() *senderStats {
	return &senderStats{
		retry:  atomic.SwapUint64(&q.stats.retry, 0),
		ok:     atomic.SwapUint64(&q.stats.ok, 0),
		failed: atomic.SwapUint64(&q.stats.failed, 0),
		bytes:  atomic.SwapUint64(&q.stats.bytes, 0),
		lost:   atomic.SwapUint64(&q.stats.lost, 0),
	}
}

// payloads specifies a payload to be sent by the sender.
type payload struct {
	body    []byte            // request body
	headers map[string]string // request headers
}

// httpRequest returns an HTTP request based on the payload, targeting the given URL.
func (p *payload) httpRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(p.body))
	if err != nil {
		// this should never happen with sanitized data (invalid method or invalid url)
		return nil, err
	}
	for k, v := range p.headers {
		req.Header.Add(k, v)
	}
	return req, nil
}

// newPayload creates a new payload, having the given body and header map.
func newPayload(body []byte, headers map[string]string) *payload {
	p := payload{
		body:    body,
		headers: headers,
	}
	if p.headers == nil {
		p.headers = make(map[string]string, 1)
	}
	p.headers["Content-Length"] = strconv.Itoa(len(body))
	return &p
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
