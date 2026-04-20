// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rcprotocoltestimpl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	"github.com/stretchr/testify/assert"
)

// Test client-side behaviour when the RC backend is not serving the echo
// endpoint (not listening, endpoint 404's or 500's).
func TestWebSocketActor_upstream(t *testing.T) {
	t.Parallel()

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/404":
			w.WriteHeader(404)
		case "/500":
			w.WriteHeader(500)
		}
	}))
	ts.StartTLS()
	defer ts.Close()

	tests := []struct {
		name string

		url string
	}{
		{
			name: "404",
			url:  ts.URL + "/404",
		},
		{
			name: "500",
			url:  ts.URL + "/500",
		},
		{
			name: "not listening",
			url:  "https://127.0.0.1:1234",
		},
	}

	for _, tt := range tests {
		var tt = tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// TLS test uses bogus certs
			agentConfig := mock.New(t)
			agentConfig.SetWithoutSource("skip_ssl_validation", true)                    // Transport
			agentConfig.SetWithoutSource("remote_configuration.no_tls_validation", true) // RC check

			assert := assert.New(t)

			url, err := url.Parse(tt.url)
			assert.NoError(err)

			client, err := api.NewHTTPClient(api.Auth{}, agentConfig, url)
			assert.NoError(err)

			actor := newEchoTestActor(client)

			// Wrap the callback to assert it is invoked.
			// Signal calledCh before calling fn so that the the actor can
			// cancel the context and the RunTransportTests preflight will be unblocked
			calledCh := make(chan struct{}, 1)
			fn := actor.fn
			actor.fn = func(ctx context.Context, client *api.HTTPClient, runCount uint64) {
				calledCh <- struct{}{}
				fn(ctx, client, runCount)
			}

			actor.Start()
			<-calledCh
			actor.Stop()
		})
	}
}

// Ensure this best-effort system has a safety net that prevents an outage
// should something panic.
func TestPanicHandler(t *testing.T) {
	t.Parallel()

	// TLS test uses bogus certs
	agentConfig := mock.New(t)
	agentConfig.SetWithoutSource("skip_ssl_validation", true)                    // Transport
	agentConfig.SetWithoutSource("remote_configuration.no_tls_validation", true) // RC check

	assert := assert.New(t)

	url, err := url.Parse("https://127.0.0.1:1234")
	assert.NoError(err)

	client, err := api.NewHTTPClient(api.Auth{}, agentConfig, url)
	assert.NoError(err)

	actor := newEchoTestActor(client)

	// Wrap the callback to assert it is invoked.
	calledCh := make(chan struct{}, 1)
	actor.fn = func(_ctx context.Context, _client *api.HTTPClient, _runCount uint64) {
		calledCh <- struct{}{}
		panic("bananas!")
	}

	actor.Start()
	<-calledCh
	actor.Stop() // This should stay safe to execute after a panic.
}
