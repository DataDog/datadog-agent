// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"crypto/ed25519"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestControlRequestsTransport checks the Remote Config and Private Action Runner control requests go through the
// transport set with WithHTTPTransport, and through http.DefaultClient without one
func TestControlRequestsTransport(t *testing.T) {
	t.Run("without a transport", func(t *testing.T) {
		assert.Same(t, http.DefaultClient, NewClient("http://fakeintake.test").controlHTTPClient())
	})

	t.Run("with a transport", func(t *testing.T) {
		transport := &recordingTransport{}
		client := NewClient("http://fakeintake.test", WithHTTPTransport(transport))
		key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
		// the responses are stubs: only the requests matter
		_ = client.RCAddConfig("2", "METRIC_CONTROL", "id", "name", []byte(`{}`))
		_ = client.RCSetExpiration(time.Now())
		_, _ = client.RCListConfigs()
		_ = client.RCDeleteConfig("2/METRIC_CONTROL/id/name")
		_, _ = client.RCStats()
		_ = client.EnqueuePARTask("task", "com.datadoghq.test", nil)
		_, _ = client.GetPARTaskResult("task", time.Minute)
		_ = client.SetPARSigningKey("key", key, 2, "runner", "connection")
		_ = client.FlushPAR()
		_, _ = client.GetPARDequeueCount()

		assert.Equal(t, []string{
			"POST /fakeintake/rc/config",
			"POST /fakeintake/rc/expiration",
			"GET /fakeintake/rc/configs",
			"DELETE /fakeintake/rc/config/2/METRIC_CONTROL/id/name",
			"GET /fakeintake/rc/stats",
			"POST /fakeintake/par/enqueue",
			"GET /fakeintake/par/result",
			"POST /fakeintake/par/signing-key",
			"POST /fakeintake/par/flush",
			"GET /fakeintake/par/stats",
		}, transport.requests())
	})
}

// recordingTransport records the method and path of the requests, and answers them with an empty JSON object
type recordingTransport struct {
	mu       sync.Mutex
	recorded []string
}

// RoundTrip implements http.RoundTripper
func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.recorded = append(rt.recorded, req.Method+" "+req.URL.Path)
	rt.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Request:    req,
	}, nil
}

func (rt *recordingTransport) requests() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string(nil), rt.recorded...)
}
