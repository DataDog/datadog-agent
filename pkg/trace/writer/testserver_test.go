// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"context"
	"net/http"
	"net/http/httptrace"
	"strings"
	"testing"
	"time"

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
		body := expectResponses(tt.codes...).body.String()
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
		resp.Body.Close()
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
			resp.Body.Close()
		}

		assert.Equal(4, ts.Total())
		assert.Equal(4, ts.Accepted())
		assert.Equal(0, ts.Failed())
		assert.Equal(0, ts.Retried())
	})

	t.Run("latency", func(t *testing.T) {
		var (
			start time.Time
			d     time.Duration
		)
		ts := newTestServerWithLatency(50 * time.Millisecond)
		defer ts.Close()

		assert := assert.New(t)
		req, err := http.NewRequest("POST", ts.URL, nil)
		assert.NoError(err)
		clienttrace := httptrace.ClientTrace{
			ConnectStart:         func(_, _ string) { start = time.Now() },
			GotFirstResponseByte: func() { d = time.Since(start) },
		}
		ctx := httptrace.WithClientTrace(context.Background(), &clienttrace)
		resp, err := http.DefaultClient.Do(req.WithContext(ctx))
		assert.NoError(err)
		assert.Equal(200, resp.StatusCode)
		resp.Body.Close()
		assert.True(d > 50*time.Millisecond)
	})

	t.Run("payloads", func(t *testing.T) {
		assert := assert.New(t)
		ts := newTestServer()
		defer ts.Close()

		req, err := http.NewRequest("POST", ts.URL, strings.NewReader("ABC"))
		assert.NoError(err)
		req.Header.Set("Secret-Number", "123")
		req.Header.Set("Secret-Letter", "Q")
		resp, err := http.DefaultClient.Do(req)
		assert.NoError(err)
		assert.Equal(200, resp.StatusCode)
		resp.Body.Close()

		assert.Len(ts.Payloads(), 1)
		assert.Equal("ABC", ts.Payloads()[0].body.String())
		assert.Equal("123", ts.Payloads()[0].headers["Secret-Number"])
		assert.Equal("Q", ts.Payloads()[0].headers["Secret-Letter"])
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
			resp.Body.Close()
		}

		assert.Equal(6, ts.Total())
		assert.Equal(2, ts.Accepted())
		assert.Equal(2, ts.Failed())
		assert.Equal(2, ts.Retried())
	})
}
