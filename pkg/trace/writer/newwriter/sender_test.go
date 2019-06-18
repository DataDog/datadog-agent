package writer

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

const testAPIKey = "123"

func TestMain(m *testing.M) {
	log.SetupDatadogLogger(seelog.Disabled, "error")
	os.Exit(m.Run())
}

func TestSender(t *testing.T) {
	const climit = 100
	testSenderConfig := func(serverURL string) *senderConfig {
		url, err := url.Parse(serverURL + "/")
		if err != nil {
			t.Fatal(err)
		}
		return &senderConfig{
			client:   &http.Client{},
			url:      url,
			maxConns: climit,
			apiKey:   testAPIKey,
		}
	}

	t.Run("accept", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()

		s := newSender(testSenderConfig(server.URL))
		for i := 0; i < 20; i++ {
			s.Push(expectResponses(200))
		}
		s.waitEmpty()

		assert.Equal(20, server.Total(), "total")
		assert.Equal(20, server.Accepted(), "accepted")
		assert.Equal(0, server.Retried(), "retry")
		assert.Equal(0, server.Failed(), "failed")
	})

	t.Run("peak", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServerWithLatency(50 * time.Millisecond)
		defer server.Close()

		s := newSender(testSenderConfig(server.URL))
		for i := 0; i < climit*4; i++ {
			s.Push(expectResponses(200))
		}
		s.waitEmpty()

		assert.True(server.Peak() <= climit)
		assert.Equal(climit*4, server.Total(), "total")
		assert.Equal(climit*4, server.Accepted(), "accepted")
		assert.Equal(0, server.Retried(), "retry")
		assert.Equal(0, server.Failed(), "failed")
	})

	t.Run("failed", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()

		s := newSender(testSenderConfig(server.URL))
		for i := 0; i < 20; i++ {
			s.Push(expectResponses(404))
		}
		s.waitEmpty()

		assert.Equal(20, server.Total(), "total")
		assert.Equal(0, server.Accepted(), "accepted")
		assert.Equal(0, server.Retried(), "retry")
		assert.Equal(20, server.Failed(), "failed")
	})

	t.Run("retry", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()
		defer func(old func(int) time.Duration) { backoffDuration = old }(backoffDuration)
		var backoffCalls uint64
		backoffDuration = func(_ int) time.Duration {
			atomic.AddUint64(&backoffCalls, 1)
			return time.Nanosecond
		}

		s := newSender(testSenderConfig(server.URL))
		s.Push(expectResponses(503, 503, 200))
		s.waitEmpty()

		assert.Equal(uint64(2), backoffCalls)
		assert.Equal(3, server.Total(), "total")
		assert.Equal(2, server.Retried(), "retry")
		assert.Equal(1, server.Accepted(), "accepted")
	})

	t.Run("many", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()
		defer useBackoffDuration(time.Millisecond)()

		s := newSender(testSenderConfig(server.URL))
		s.Push(expectResponses(503, 503, 200))
		for i := 0; i < 20; i++ {
			s.Push(expectResponses(403))
		}

		s.waitEmpty()

		assert.Equal(23, server.Total(), "total")
		assert.Equal(2, server.Retried(), "retry")
		assert.Equal(1, server.Accepted(), "accepted")
		assert.Equal(20, server.Failed(), "failed")
	})

	// tests that the maximum allowed queue size is kept.
	t.Run("drops", func(t *testing.T) {
		assert := assert.New(t)
		var recorder mockRecorder
		cfg := testSenderConfig("http://fake/url")
		cfg.maxConns = climit
		cfg.recorder = &recorder
		s := newSender(cfg)
		defer useQueueSize(10)()

		s.enqueue(&payload{body: bytes.NewBufferString("first")})
		s.enqueue(&payload{body: bytes.NewBufferString("secnd")})

		assert.Equal(2, s.list.Len())

		// go overboard, should evict "first"
		s.enqueue(&payload{body: bytes.NewBufferString("third")})

		assert.Equal(2, s.list.Len())
		assert.Equal("secnd", s.list.Front().Value.(*payload).body.String())
		assert.Equal("third", s.list.Front().Next().Value.(*payload).body.String())

		// go overboard again, should evict "secnd"
		s.enqueue(&payload{body: bytes.NewBufferString("fourt")})

		assert.Equal(2, s.list.Len())
		assert.Equal("third", s.list.Front().Value.(*payload).body.String())
		assert.Equal("fourt", s.list.Front().Next().Value.(*payload).body.String())

		dropped := recorder.data(eventTypeDropped)
		assert.Equal(2, len(dropped))
		assert.Equal(5, dropped[0].bytes)
		assert.Equal(5, dropped[1].bytes)
		assert.Equal(1, dropped[0].count)
		assert.Equal(1, dropped[1].count)
	})

	t.Run("headers", func(t *testing.T) {
		assert := assert.New(t)
		var wg sync.WaitGroup
		wg.Add(1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			assert.Equal(testAPIKey, req.Header.Get(headerAPIKey))
			assert.Equal(userAgent, req.Header.Get(headerUserAgent))
			wg.Done()
		}))
		defer server.Close()
		s := newSender(testSenderConfig(server.URL))
		s.Push(expectResponses(http.StatusOK))
		s.waitEmpty()
		wg.Wait()
	})

	t.Run("events", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()
		defer useBackoffDuration(0)()

		var recorder mockRecorder
		cfg := testSenderConfig(server.URL)
		cfg.recorder = &recorder
		s := newSender(cfg)

		// push a couple of payloads
		start := time.Now()
		payloadThirdOk := expectResponses(503, 503, 200)
		payloadOk := expectResponses(200)
		payloadFail := expectResponses(403)

		s.Push(payloadThirdOk)
		s.Push(payloadOk)
		s.Push(payloadOk)
		for i := 0; i < 4; i++ {
			s.Push(payloadFail)
		}
		s.waitEmpty()

		// Assert that events were correctly recorded.
		flushed := recorder.data(eventTypeFlushed)
		assert.Equal(2, len(flushed))

		retried := recorder.data(eventTypeRetry)
		assert.Equal(2, len(retried))
		for i := 0; i < 2; i++ {
			assert.Equal(payloadThirdOk.body.Len(), retried[i].bytes)
			assert.Equal(`server responded with "503 Service Unavailable"`, retried[i].err.(*retriableError).err.Error())
			assert.Equal(1, retried[i].count)
			assert.True(retried[i].connectionFill > 0 && retried[i].connectionFill < 1, fmt.Sprintf("%f", retried[i].connectionFill))
			assert.True(time.Since(start)-retried[i].duration < time.Second)
		}

		sent := recorder.data(eventTypeSent)
		assert.Equal(3, len(sent))
		for i := 0; i < 3; i++ {
			switch size := sent[i].bytes; size {
			case payloadOk.body.Len(), payloadThirdOk.body.Len():
				// OK
			default:
				t.Fatalf("unexpected body size: %d", size)
			}
			assert.NoError(sent[i].err)
			assert.Equal(1, sent[i].count)
			assert.True(sent[i].connectionFill > 0 && sent[i].connectionFill < 1, fmt.Sprintf("%f", sent[i].connectionFill))
			assert.True(time.Since(start)-sent[i].duration < time.Second)
		}

		failed := recorder.data(eventTypeFailed)
		assert.Equal(4, len(failed))
		for i := 0; i < 4; i++ {
			assert.Equal(payloadFail.body.Len(), failed[i].bytes)
			assert.Equal("403 Forbidden", failed[i].err.Error())
			assert.Equal(1, failed[i].count)
			assert.True(failed[i].connectionFill > 0 && failed[i].connectionFill < 1, fmt.Sprintf("%f", failed[i].connectionFill))
			assert.True(time.Since(start)-failed[i].duration < time.Second)
		}
	})
}

func TestPayload(t *testing.T) {
	expectBody := bytes.NewBufferString("body")
	bodyLength := strconv.Itoa(expectBody.Len())

	t.Run("headers", func(t *testing.T) {
		assert := assert.New(t)
		p := newPayload(map[string]string{
			"k1": "v1",
			"k2": "v2",
		})
		p.body = expectBody
		u, err := url.Parse("http://whatever")
		assert.NoError(err)
		req, err := p.httpRequest(u)
		assert.NoError(err)
		assert.Len(req.Header, 3)
		assert.Equal(req.Header.Get("Content-Length"), bodyLength)
		assert.Equal("v1", req.Header.Get("k1"))
		assert.Equal("v2", req.Header.Get("k2"))
	})

	t.Run("httpRequest", func(t *testing.T) {
		assert := assert.New(t)
		p := newPayload(map[string]string{"DD-Api-Key": testAPIKey})
		p.body = expectBody
		url, err := url.Parse("http://localhost/my/path")
		if err != nil {
			t.Fatal(err)
		}
		req, err := p.httpRequest(url)
		assert.NoError(err)
		assert.Equal(http.MethodPost, req.Method)
		assert.Equal("/my/path", req.URL.Path)
		assert.Equal("4", req.Header.Get("Content-Length"))
		assert.Equal(testAPIKey, req.Header.Get("DD-Api-Key"))
		slurp, err := ioutil.ReadAll(req.Body)
		assert.NoError(err)
		req.Body.Close()
		assert.Equal(expectBody.Bytes(), slurp)
	})
}

// useQueueSize sets the maximum queue size to n and returns a function to restore it.
func useQueueSize(n int) func() {
	old := maxQueueSize
	maxQueueSize = n
	return func() { maxQueueSize = old }
}

// useBackoffDuration replaces the current backoff duration with d and returns a
// function which restores it.
func useBackoffDuration(d time.Duration) func() {
	old := backoffDuration
	backoffDuration = func(attempt int) time.Duration { return d }
	return func() { backoffDuration = old }
}

// mockRecorder is a mock eventRecorder which records all calls to recordEvent.
type mockRecorder struct {
	mu                                    sync.RWMutex
	retry, flushed, sent, failed, dropped []*eventData
}

// data returns all call data for the given eventType.
func (r *mockRecorder) data(t eventType) []*eventData {
	r.mu.RLock()
	defer r.mu.RUnlock()
	switch t {
	case eventTypeRetry:
		return r.retry
	case eventTypeFlushed:
		return r.flushed
	case eventTypeSent:
		return r.sent
	case eventTypeFailed:
		return r.failed
	case eventTypeDropped:
		return r.dropped
	default:
		panic("unknown event")
	}
}

// recordEvent implements eventRecorder.
func (r *mockRecorder) recordEvent(t eventType, data *eventData) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch t {
	case eventTypeRetry:
		r.retry = append(r.retry, data)
	case eventTypeFlushed:
		r.flushed = append(r.flushed, data)
	case eventTypeSent:
		r.sent = append(r.sent, data)
	case eventTypeFailed:
		r.failed = append(r.failed, data)
	case eventTypeDropped:
		r.dropped = append(r.dropped, data)
	}
}
