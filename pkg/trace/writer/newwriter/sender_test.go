package writer

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

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
			apiKey:   "123",
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

	t.Run("accept-many", func(t *testing.T) {
		assert := assert.New(t)
		server := newTestServer()
		defer server.Close()

		s := newSender(testSenderConfig(server.URL))
		for i := 0; i < climit*4; i++ {
			s.Push(expectResponses(200))
		}
		s.waitEmpty()

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
		var retries uint64
		backoffDuration = func(_ int) time.Duration {
			atomic.AddUint64(&retries, 1)
			return time.Nanosecond
		}

		s := newSender(testSenderConfig(server.URL))
		s.Push(expectResponses(
			503,
			503,
			200,
		))
		s.waitEmpty()

		assert.Equal(uint64(2), retries)
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
		s.Push(expectResponses(
			503,
			503,
			200,
		))
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
	t.Run("queue-size", func(t *testing.T) {
		assert := assert.New(t)
		s := newSender(&senderConfig{client: &http.Client{}, maxConns: climit})
		defer useQueueSize(10)()

		s.enqueue(&payload{body: []byte("first")})
		s.enqueue(&payload{body: []byte("secnd")})

		assert.Equal(2, s.list.Len())

		// go overboard, should evict "first"
		s.enqueue(&payload{body: []byte("third")})

		assert.Equal(2, s.list.Len())
		assert.Equal([]byte("secnd"), s.list.Front().Value.(*payload).body)
		assert.Equal([]byte("third"), s.list.Front().Next().Value.(*payload).body)

		// go overboard again, should evict "secnd"
		s.enqueue(&payload{body: []byte("fourt")})

		assert.Equal(2, s.list.Len())
		assert.Equal([]byte("third"), s.list.Front().Value.(*payload).body)
		assert.Equal([]byte("fourt"), s.list.Front().Next().Value.(*payload).body)
	})

	t.Run("headers", func(t *testing.T) {
		assert := assert.New(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			assert.Equal("123", req.Header.Get(headerAPIKey))
			assert.Equal(userAgent, req.Header.Get(headerUserAgent))
		}))
		defer server.Close()
		s := newSender(testSenderConfig(server.URL))
		s.Push(expectResponses(http.StatusOK))
		s.waitEmpty()
	})
}

func TestPayload(t *testing.T) {
	expectBody := []byte("body")
	bodyLength := strconv.Itoa(len(expectBody))

	t.Run("new", func(t *testing.T) {
		t.Run("nil", func(t *testing.T) {
			assert := assert.New(t)
			p := newPayload(expectBody, nil)
			assert.Equal(expectBody, p.body)
			assert.Len(p.headers, 1)
			assert.Equal(p.headers["Content-Length"], bodyLength)
		})

		t.Run("headers", func(t *testing.T) {
			assert := assert.New(t)
			p := newPayload(expectBody, map[string]string{
				"k1": "v1",
				"k2": "v2",
			})
			assert.Equal(expectBody, p.body)
			assert.Len(p.headers, 3)
			assert.Equal(p.headers["Content-Length"], bodyLength)
			assert.Equal("v1", p.headers["k1"])
			assert.Equal("v2", p.headers["k2"])
		})
	})

	t.Run("httpRequest", func(t *testing.T) {
		assert := assert.New(t)
		p := newPayload(expectBody, map[string]string{"DD-Api-Key": "123"})
		url, err := url.Parse("http://localhost/my/path")
		if err != nil {
			t.Fatal(err)
		}
		req, err := p.httpRequest(url)
		assert.NoError(err)
		assert.Equal(http.MethodPost, req.Method)
		assert.Equal("/my/path", req.URL.Path)
		assert.Equal("4", req.Header.Get("Content-Length"))
		assert.Equal("123", req.Header.Get("DD-Api-Key"))
		slurp, err := ioutil.ReadAll(req.Body)
		assert.NoError(err)
		req.Body.Close()
		assert.Equal(expectBody, slurp)
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
