package writer

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpectResponses(t *testing.T) {
	for _, tt := range []struct {
		codes      []int
		bodySuffix string
	}{
		{nil, "|200"},
		{[]int{}, "|200"},
		{[]int{200}, "|200"},
		{[]int{200, 300}, "|200,300"},
		{[]int{403, 403, 200, 100}, "|403,403,200,100"},
	} {
		body := string(expectResponses(tt.codes...).body)
		parts := strings.Split(body, "|")
		if len(parts) != 2 {
			t.Fatalf("malformed body: %s", body)
		}
		expect := parts[0] + tt.bodySuffix
		assert.Equal(t, expect, body)
	}
}

func TestTestServer(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		assert := assert.New(t)
		ts := newTestServer()
		defer ts.Close()

		resp, err := http.Post(ts.URL, "application/msgpack", strings.NewReader("random_string"))
		assert.NoError(err)

		assert.Equal(http.StatusOK, resp.StatusCode)
		assert.Equal(1, ts.Total())
		assert.Equal(1, ts.Accepted())
		assert.Equal(0, ts.Failed())
		assert.Equal(0, ts.Retried())
	})

	t.Run("loop", func(t *testing.T) {
		assert := assert.New(t)
		ts := newTestServer()
		defer ts.Close()

		for _, code := range []int{
			http.StatusOK,
			http.StatusOK,
			http.StatusOK,
			http.StatusOK,
		} {
			resp, err := http.Post(ts.URL, "text/plain", strings.NewReader("3|200"))
			assert.NoError(err)
			assert.Equal(code, resp.StatusCode)
		}

		assert.Equal(4, ts.Total())
		assert.Equal(4, ts.Accepted())
		assert.Equal(0, ts.Failed())
		assert.Equal(0, ts.Retried())
	})

	t.Run("custom-body", func(t *testing.T) {
		assert := assert.New(t)
		ts := newTestServer()
		defer ts.Close()

		for _, code := range []int{
			http.StatusOK,
			http.StatusLoopDetected,
			http.StatusTooManyRequests,
			http.StatusOK,
			http.StatusLoopDetected,
			http.StatusTooManyRequests,
		} {
			resp, err := http.Post(ts.URL, "text/plain", strings.NewReader("1|200,508,429"))
			assert.NoError(err)
			assert.Equal(code, resp.StatusCode)
		}

		assert.Equal(6, ts.Total())
		assert.Equal(2, ts.Accepted())
		assert.Equal(2, ts.Failed())
		assert.Equal(2, ts.Retried())
	})
}
