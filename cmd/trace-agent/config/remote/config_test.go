// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remote

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestConfigEndpoint(t *testing.T) {
	var tcs = []struct {
		name               string
		reqBody            string
		expectedStatusCode int
		enabled            bool
		valid              bool
		response           string
	}{
		{
			name:               "bad",
			enabled:            true,
			expectedStatusCode: http.StatusBadRequest,
			response:           "unexpected end of JSON input\n",
		},
		{
			name:    "valid",
			reqBody: `{"client":{"id":"test_client"}}`,

			enabled:            true,
			valid:              true,
			expectedStatusCode: http.StatusOK,
			response:           `{"targets":"dGVzdA=="}`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			grpc := agentGRPCConfigFetcher{}
			rcv := api.NewHTTPReceiver(config.New(), sampler.NewDynamicConfig(), make(chan *api.Payload, 5000), nil, telemetry.NewNoopCollector())
			mux := http.NewServeMux()
			cfg := &config.AgentConfig{}
			mux.Handle("/v0.7/config", ConfigHandler(rcv, &grpc, cfg))
			server := httptest.NewServer(mux)
			if tc.valid {
				var request pbgo.ClientGetConfigsRequest
				err := json.Unmarshal([]byte(tc.reqBody), &request)
				assert.NoError(err)
				grpc.On("ClientGetConfigs", mock.Anything, &request, mock.Anything).Return(&pbgo.ClientGetConfigsResponse{Targets: []byte("test")}, nil)
			}
			req, _ := http.NewRequest("POST", server.URL+"/v0.7/config", strings.NewReader(tc.reqBody))
			req.Header.Set("Content-Type", "application/msgpack")
			resp, err := http.DefaultClient.Do(req)
			assert.Nil(err)
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			assert.Nil(err)
			assert.Equal(tc.expectedStatusCode, resp.StatusCode)
			assert.Equal(tc.response, string(body))
		})
	}
}

func TestUpstreamRequest(t *testing.T) {

	var tcs = []struct {
		name                    string
		tracerReq               string
		cfg                     *config.AgentConfig
		expectedUpstreamRequest string
	}{
		{
			name:      "both tracer and container tags",
			tracerReq: `{"client":{"id":"test_client","is_tracer":true,"client_tracer":{"service":"test","tags":["foo:bar"]}}}`,
			cfg: &config.AgentConfig{
				ContainerTags: func(cid string) ([]string, error) {
					return []string{"baz:qux"}, nil
				},
			},
			expectedUpstreamRequest: `{"client":{"id":"test_client","is_tracer":true,"client_tracer":{"service":"test","tags":["foo:bar","baz:qux"]}}}`,
		},
		{
			name:                    "tracer tags only",
			tracerReq:               `{"client":{"id":"test_client","is_tracer":true,"client_tracer":{"service":"test","tags":["foo:bar"]}}}`,
			expectedUpstreamRequest: `{"client":{"id":"test_client","is_tracer":true,"client_tracer":{"service":"test","tags":["foo:bar"]}}}`,
			cfg:                     &config.AgentConfig{},
		},
		{
			name:      "container tags only",
			tracerReq: `{"client":{"id":"test_client","is_tracer":true,"client_tracer":{"service":"test"}}}`,
			cfg: &config.AgentConfig{
				ContainerTags: func(cid string) ([]string, error) {
					return []string{"baz:qux"}, nil
				},
			},
			expectedUpstreamRequest: `{"client":{"id":"test_client","is_tracer":true,"client_tracer":{"service":"test","tags":["baz:qux"]}}}`,
		},
		{
			name:                    "no tracer",
			tracerReq:               `{"client":{"id":"test_client"}}`,
			expectedUpstreamRequest: `{"client":{"id":"test_client"}}`,
			cfg:                     &config.AgentConfig{},
		},
		{
			name:                    "tracer service and env are normalized",
			tracerReq:               `{"client":{"id":"test_client","is_tracer":true,"client_tracer":{"service":"test ww w@","env":"test@ww","tags":["foo:bar"]}}}`,
			expectedUpstreamRequest: `{"client":{"id":"test_client","is_tracer":true,"client_tracer":{"service":"test_ww_w","env":"test_ww","tags":["foo:bar"]}}}`,
			cfg:                     &config.AgentConfig{},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			grpc := agentGRPCConfigFetcher{}
			rcv := api.NewHTTPReceiver(config.New(), sampler.NewDynamicConfig(), make(chan *api.Payload, 5000), nil, telemetry.NewNoopCollector())

			var request pbgo.ClientGetConfigsRequest
			err := json.Unmarshal([]byte(tc.expectedUpstreamRequest), &request)
			assert.NoError(err)
			grpc.On("ClientGetConfigs", mock.Anything, &request, mock.Anything).Return(&pbgo.ClientGetConfigsResponse{Targets: []byte("test")}, nil)

			mux := http.NewServeMux()
			mux.Handle("/v0.7/config", ConfigHandler(rcv, &grpc, tc.cfg))
			server := httptest.NewServer(mux)

			req, _ := http.NewRequest("POST", server.URL+"/v0.7/config", strings.NewReader(tc.tracerReq))
			req.Header.Set("Content-Type", "application/msgpack")
			req.Header.Set("Datadog-Container-ID", "cid")
			resp, err := http.DefaultClient.Do(req)
			assert.Nil(err)
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			assert.NoError(err)
			assert.Equal(200, resp.StatusCode)
			assert.Equal(`{"targets":"dGVzdA=="}`, string(body))
		})
	}
}

func TestForwardErrors(t *testing.T) {
	assert := assert.New(t)
	grpc := agentGRPCConfigFetcher{}
	rcv := api.NewHTTPReceiver(config.New(), sampler.NewDynamicConfig(), make(chan *api.Payload, 5000), nil, telemetry.NewNoopCollector())

	grpc.On("ClientGetConfigs", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, status.Error(codes.Unimplemented, "not implemented"))

	mux := http.NewServeMux()
	mux.Handle("/v0.7/config", ConfigHandler(rcv, &grpc, &config.AgentConfig{}))
	server := httptest.NewServer(mux)

	req, _ := http.NewRequest("POST", server.URL+"/v0.7/config", strings.NewReader(`{"client":{"id":"test_client","is_tracer":true,"client_tracer":{"service":"test","tags":["foo:bar"]}}}`))
	r, err := http.DefaultClient.Do(req)
	assert.NoError(err)
	assert.Equal(404, r.StatusCode)
	r.Body.Close()
}

type agentGRPCConfigFetcher struct {
	pbgo.AgentSecureClient
	mock.Mock
}

func (a *agentGRPCConfigFetcher) ClientGetConfigs(ctx context.Context, in *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	args := a.Called(ctx, in)
	var maybeResponse *pbgo.ClientGetConfigsResponse
	if args.Get(0) != nil {
		maybeResponse = args.Get(0).(*pbgo.ClientGetConfigsResponse)
	}
	return maybeResponse, args.Error(1)
}
