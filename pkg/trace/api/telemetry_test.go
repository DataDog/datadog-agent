// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/log"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// asserting Server starts a TLS Server with provided callback function used to perform assertions
// on the contents of the request the server received. If no error is returned server will send standardised
// response OK.
func assertingServer(t *testing.T, onReq func(req *http.Request, reqBody []byte) error) *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		assert.NoError(t, onReq(req, body))
		_, err = w.Write([]byte("{}"))
		assert.NoError(t, err)
		req.Body.Close()
	}))
}

func newRequestRecorder(t *testing.T) (req *http.Request, rec *httptest.ResponseRecorder) {
	req, err := http.NewRequest("POST", "/telemetry/proxy/path", strings.NewReader("body"))
	assert.NoError(t, err)
	rec = httptest.NewRecorder()
	return req, rec
}

func recordedStatusCode(rec *httptest.ResponseRecorder) int {
	return rec.Result().StatusCode //nolint:bodyclose
}

func recordedResponse(t *testing.T, rec *httptest.ResponseRecorder) string {
	resp := rec.Result()
	responseBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.NoError(t, err)

	return string(responseBody)
}

func getTestConfig(endpointURL string) *config.AgentConfig {
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test_apikey"
	cfg.TelemetryConfig.Enabled = true
	cfg.TelemetryConfig.Endpoints = []*config.Endpoint{{
		APIKey: "test_apikey",
		Host:   endpointURL,
	}}
	cfg.Hostname = "test_hostname"
	cfg.SkipSSLValidation = true
	cfg.DefaultEnv = "test_env"
	return cfg
}

func TestTelemetryBasicProxyRequest(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	srv := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("body", string(body), "invalid request body")
		assert.Equal("test_apikey", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))
		assert.Equal("AWS", req.Header.Get("DD-Cloud-Provider"))
		assert.Equal("AWSLambda", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_ARN", req.Header.Get("DD-Cloud-Resource-Identifier"))
		assert.Equal("/path", req.URL.Path)
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

		endpointCalled.Inc()
		return nil
	})

	cfg := getTestConfig(srv.URL)
	cfg.GlobalTags[functionARNKeyTag] = "test_ARN"
	recv := newTestReceiverFromConfig(cfg)

	assertSendRequest(t, recv, endpointCalled)
}

func TestGoogleCloudRun(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	srv := assertingServer(t, func(req *http.Request, _ []byte) error {
		assert.Equal("GCP", req.Header.Get("DD-Cloud-Provider"))
		assert.Equal("GCPCloudRun", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_service", req.Header.Get("DD-Cloud-Resource-Identifier"))

		endpointCalled.Inc()
		return nil
	})

	cfg := getTestConfig(srv.URL)
	cfg.GlobalTags["service_name"] = "test_service"
	cfg.GlobalTags["origin"] = "cloudrun"
	recv := newTestReceiverFromConfig(cfg)

	assertSendRequest(t, recv, endpointCalled)
}

func TestAzureAppService(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	srv := assertingServer(t, func(req *http.Request, _ []byte) error {
		assert.Equal("Azure", req.Header.Get("DD-Cloud-Provider"))
		assert.Equal("AzureAppService", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_app", req.Header.Get("DD-Cloud-Resource-Identifier"))
		assert.Equal("/path", req.URL.Path)
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

		endpointCalled.Inc()
		return nil
	})

	cfg := getTestConfig(srv.URL)
	cfg.GlobalTags["app_name"] = "test_app"
	cfg.GlobalTags["origin"] = "appservice"
	recv := newTestReceiverFromConfig(cfg)

	assertSendRequest(t, recv, endpointCalled)
}

func TestAzureContainerApp(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	srv := assertingServer(t, func(req *http.Request, _ []byte) error {
		assert.Equal("Azure", req.Header.Get("DD-Cloud-Provider"))
		assert.Equal("AzureContainerApp", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_app", req.Header.Get("DD-Cloud-Resource-Identifier"))
		assert.Equal("/path", req.URL.Path)
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

		endpointCalled.Inc()
		return nil
	})

	cfg := getTestConfig(srv.URL)
	cfg.GlobalTags["app_name"] = "test_app"
	cfg.GlobalTags["origin"] = "containerapp"
	recv := newTestReceiverFromConfig(cfg)

	assertSendRequest(t, recv, endpointCalled)
}

type testContainerIDProvider struct{}

// NewIDProvider initializes an IDProvider instance, in non-linux environments the procRoot arg is unused.
func getTestContainerIDProvider() testContainerIDProvider {
	return testContainerIDProvider{}
}

func (testContainerIDProvider) GetContainerID(_ context.Context, _ http.Header) string {
	return "test_container_id"
}

func TestAWSFargate(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	srv := assertingServer(t, func(req *http.Request, _ []byte) error {
		assert.Equal("AWS", req.Header.Get("DD-Cloud-Provider"))
		assert.Equal("AWSFargate", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_ARN", req.Header.Get("DD-Cloud-Resource-Identifier"))

		endpointCalled.Inc()
		return nil
	})

	cfg := getTestConfig(srv.URL)
	cfg.ContainerTags = func(_ string) ([]string, error) {
		return []string{"task_arn:test_ARN"}, nil
	}
	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.containerIDProvider = getTestContainerIDProvider()

	assertSendRequest(t, recv, endpointCalled)
}

func TestTelemetryProxyMultipleEndpoints(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	mainBackend := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))
		assert.Equal("body", string(body), "invalid request body")
		assert.Equal("test_apikey_1", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))
		assert.Equal("AWS", req.Header.Get("DD-Cloud-Provider"))
		assert.Equal("AWSLambda", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_ARN", req.Header.Get("DD-Cloud-Resource-Identifier"))

		endpointCalled.Add(2)
		return nil
	})
	additionalBackend := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))
		assert.Equal("body", string(body), "invalid request body")
		assert.Equal("test_apikey_2", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))
		assert.Equal("AWS", req.Header.Get("DD-Cloud-Provider"))
		assert.Equal("AWSLambda", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_ARN", req.Header.Get("DD-Cloud-Resource-Identifier"))

		endpointCalled.Add(3)
		return nil
	})

	cfg := getTestConfig(mainBackend.URL)
	cfg.Endpoints[0].APIKey = "test_apikey_1"
	cfg.TelemetryConfig.Endpoints = []*config.Endpoint{{
		APIKey: "test_apikey_1",
		Host:   mainBackend.URL,
	}, {
		APIKey: "test_apikey_3",
		Host:   "111://malformed_url.example.com",
	}, {
		APIKey: "test_apikey_2",
		Host:   additionalBackend.URL + "/",
	}}
	cfg.DefaultEnv = "test_env"
	cfg.GlobalTags[functionARNKeyTag] = "test_ARN"

	recv := newTestReceiverFromConfig(cfg)

	req, rec := newRequestRecorder(t)
	recv.buildMux().ServeHTTP(rec, req)
	recv.telemetryForwarder.Stop()

	assert.Equal(200, recordedStatusCode(rec))
	assert.Equal("{}", recordedResponse(t, rec))

	// because we use number 2,3 both endpoints must be called to produce 5
	// just counting number of requests could give false results if first endpoint
	// was called twice
	if endpointCalled.Load() != 5 {
		t.Fatalf("calling multiple backends failed %v", endpointCalled.Load())
	}
}

func TestMaxInflightBytes(t *testing.T) {
	type testReq struct {
		res  int
		size int
	}
	type testCase struct {
		reqs                    []testReq
		expectedEndpointsCalled int
	}
	testCases := []testCase{
		{[]testReq{
			{http.StatusOK, 51},
			{http.StatusOK, 49},
			{http.StatusTooManyRequests, 1},
		}, 2},
		{[]testReq{
			{http.StatusTooManyRequests, 101},
			{http.StatusOK, 100},
			{http.StatusTooManyRequests, 1},
		}, 1},
	}
	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			endpointCalled := atomic.NewUint64(0)
			assert := assert.New(t)

			done := make(chan struct{})

			srv := assertingServer(t, func(req *http.Request, body []byte) error {
				assert.Equal("test_apikey", req.Header.Get("DD-API-KEY"))
				assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
				assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))
				assert.Equal("/path", req.URL.Path)
				assert.Equal("", req.Header.Get("User-Agent"))
				assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

				<-done
				endpointCalled.Add(1)
				return nil
			})

			cfg := getTestConfig(srv.URL)
			recv := newTestReceiverFromConfig(cfg)
			recv.telemetryForwarder.maxInflightBytes = 100
			mux := recv.buildMux()

			for _, testReq := range testCase.reqs {
				req, rec := newRequestRecorder(t)
				req.Body = io.NopCloser(bytes.NewBuffer(make([]byte, testReq.size)))
				req.ContentLength = int64(testReq.size)
				mux.ServeHTTP(rec, req)

				assert.Equal(testReq.res, recordedStatusCode(rec))
			}

			close(done)
			recv.telemetryForwarder.Stop()
			assert.Equal(uint64(testCase.expectedEndpointsCalled), endpointCalled.Load())
		})
	}
}

func TestInflightBytesReset(t *testing.T) {
	type testReq struct {
		res  int
		size int
	}
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	done := make(chan struct{})

	srv := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("test_apikey", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))
		assert.Equal("/path", req.URL.Path)
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

		<-done
		endpointCalled.Add(1)
		return nil
	})

	cfg := getTestConfig(srv.URL)
	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.maxInflightBytes = 100
	mux := recv.buildMux()

	reqs := []testReq{
		{http.StatusOK, 100},
		{http.StatusTooManyRequests, 1},
	}
	for _, testReq := range reqs {
		req, rec := newRequestRecorder(t)
		req.Body = io.NopCloser(bytes.NewBuffer(make([]byte, testReq.size)))
		req.ContentLength = int64(testReq.size)
		mux.ServeHTTP(rec, req)

		assert.Equal(testReq.res, recordedStatusCode(rec))
	}
	// Unblock
	done <- struct{}{}

	// Wait for the inflight bytes to be freed
	for recv.telemetryForwarder.inflightCount.Load() != 0 {
		time.Sleep(time.Millisecond)
	}

	for _, testReq := range reqs {
		req, rec := newRequestRecorder(t)
		req.Body = io.NopCloser(bytes.NewBuffer(make([]byte, testReq.size)))
		req.ContentLength = int64(testReq.size)
		mux.ServeHTTP(rec, req)

		assert.Equal(testReq.res, recordedStatusCode(rec))
	}

	close(done)
	recv.telemetryForwarder.Stop()
	assert.Equal(uint64(2), endpointCalled.Load())
}

func TestActualServer(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	done := make(chan struct{})

	intakeMockServer := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("test_apikey", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))
		assert.Equal("/path", req.URL.Path)
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

		<-done
		endpointCalled.Add(1)
		return nil
	})

	cfg := getTestConfig(intakeMockServer.URL)
	r := newTestReceiverFromConfig(cfg)
	logs := bytes.Buffer{}
	prevLogger := log.SetLogger(log.NewBufferLogger(&logs))
	defer log.SetLogger(prevLogger)
	server := httptest.NewServer(http.StripPrefix("/telemetry/proxy", r.telemetryForwarderHandler()))
	var client http.Client

	req, err := http.NewRequest("POST", server.URL+"/telemetry/proxy/path", bytes.NewBuffer([]byte{0, 1, 2}))
	assert.NoError(err)
	req.Header.Set("User-Agent", "")

	resp, err := client.Do(req)
	assert.NoError(err)

	resp.Body.Close()
	assert.Equal(200, resp.StatusCode)

	close(done)
	r.telemetryForwarder.Stop()
	assert.Equal(uint64(1), endpointCalled.Load())
	assert.NotContains(logs.String(), "ERROR")
}

func TestTelemetryConfig(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "api_key"
		recv := newTestReceiverFromConfig(cfg)

		req, rec := newRequestRecorder(t)
		recv.buildMux().ServeHTTP(rec, req)
		recv.telemetryForwarder.Stop()

		result := rec.Result()
		assert.Equal(t, 404, result.StatusCode)
		result.Body.Close()
	})

	t.Run("no-endpoints", func(t *testing.T) {
		cfg := config.New()
		cfg.TelemetryConfig.Enabled = true
		cfg.TelemetryConfig.Endpoints = []*config.Endpoint{{
			APIKey: "api_key",
			Host:   "111://malformed.dd_url.com",
		}}
		recv := newTestReceiverFromConfig(cfg)

		req, rec := newRequestRecorder(t)
		recv.buildMux().ServeHTTP(rec, req)

		result := rec.Result()
		assert.Equal(t, 404, result.StatusCode)
		result.Body.Close()
	})

	t.Run("fallback-endpoint", func(t *testing.T) {
		srv := assertingServer(t, func(_ *http.Request, _ []byte) error { return nil })
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "api_key"
		cfg.TelemetryConfig.Enabled = true
		cfg.SkipSSLValidation = true
		cfg.TelemetryConfig.Endpoints = []*config.Endpoint{{
			APIKey: "api_key",
			Host:   "111://malformed.dd_url.com",
		}, {
			APIKey: "api_key",
			Host:   srv.URL,
		}}
		recv := newTestReceiverFromConfig(cfg)

		req, rec := newRequestRecorder(t)
		recv.buildMux().ServeHTTP(rec, req)

		assert.Equal(t, "{}", recordedResponse(t, rec))
	})
}

func TestExtractFargateTask(t *testing.T) {
	t.Run("contains-tag", func(t *testing.T) {
		tags := "foo:bar,baz:,task_arn:123"

		taskArn, ok := extractFargateTask(tags)

		assert.True(t, ok)
		assert.Equal(t, "123", taskArn)
	})

	t.Run("doesnt-contain-tag", func(t *testing.T) {
		tags := "foo:bar,,"

		taskArn, ok := extractFargateTask(tags)

		assert.False(t, ok)
		assert.Equal(t, "", taskArn)
	})

	t.Run("contain-empty-tag", func(t *testing.T) {
		tags := "foo:bar,task_arn:,baz:abc"

		taskArn, ok := extractFargateTask(tags)

		assert.True(t, ok)
		assert.Equal(t, "", taskArn)
	})

	t.Run("empty-string", func(t *testing.T) {
		tags := ""

		taskArn, ok := extractFargateTask(tags)

		assert.False(t, ok)
		assert.Equal(t, "", taskArn)
	})
}

func assertSendRequest(t *testing.T, recv *HTTPReceiver, endpointCalled *atomic.Uint64) {
	req, rec := newRequestRecorder(t)
	recv.buildMux().ServeHTTP(rec, req)
	recv.telemetryForwarder.Stop()

	assert.Equal(t, 200, recordedStatusCode(rec))
	assert.Equal(t, "{}", recordedResponse(t, rec))
	assert.Equal(t, uint64(1), endpointCalled.Load())
}
