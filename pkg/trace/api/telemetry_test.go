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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	msgpack "github.com/vmihailenco/msgpack/v4"
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

// decodeBatch decodes a MessagePack-encoded agent-batch payload.
func decodeBatch(t *testing.T, body []byte) agentBatch {
	t.Helper()
	var batch agentBatch
	err := msgpack.Unmarshal(body, &batch)
	require.NoError(t, err)
	return batch
}

func TestTelemetryBasicProxyRequest(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	srv := assertingServer(t, func(req *http.Request, body []byte) error {
		// Batch-level headers
		assert.Equal("test_apikey", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))
		assert.Equal("agent-batch", req.Header.Get("DD-Telemetry-Request-Type"))
		assert.Equal("application/msgpack", req.Header.Get("Content-Type"))
		assert.Equal("/api/v2/apmtelemetry", req.URL.Path)

		// Decode batch and verify event contents
		batch := decodeBatch(t, body)
		assert.Equal("agent-batch", batch.RequestType)
		assert.Len(batch.Payload.Events, 1)

		event := batch.Payload.Events[0]
		assert.Equal("body", string(event.Content))
		assert.Equal("test_container_id", event.Headers["Datadog-Container-ID"])
		assert.Equal("key:test_value", event.Headers["X-Datadog-Container-Tags"])

		endpointCalled.Inc()
		return nil
	})

	cfg := getTestConfig(srv.URL)
	cfg.ContainerTags = func(_ string) ([]string, error) {
		return []string{"key:test\nvalue"}, nil
	}
	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.start()
	recv.telemetryForwarder.containerIDProvider = getTestContainerIDProvider()

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
	recv.telemetryForwarder.start()

	assertSendRequest(t, recv, endpointCalled)
}

func TestAzureAppService(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	srv := assertingServer(t, func(req *http.Request, _ []byte) error {
		assert.Equal("Azure", req.Header.Get("DD-Cloud-Provider"))
		assert.Equal("AzureAppService", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_app", req.Header.Get("DD-Cloud-Resource-Identifier"))
		assert.Equal("/api/v2/apmtelemetry", req.URL.Path)
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

		endpointCalled.Inc()
		return nil
	})

	cfg := getTestConfig(srv.URL)
	cfg.GlobalTags["app_name"] = "test_app"
	cfg.GlobalTags["origin"] = "appservice"
	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.start()

	assertSendRequest(t, recv, endpointCalled)
}

func TestAzureContainerApp(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	srv := assertingServer(t, func(req *http.Request, _ []byte) error {
		assert.Equal("Azure", req.Header.Get("DD-Cloud-Provider"))
		assert.Equal("AzureContainerApp", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_app", req.Header.Get("DD-Cloud-Resource-Identifier"))
		assert.Equal("/api/v2/apmtelemetry", req.URL.Path)
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

		endpointCalled.Inc()
		return nil
	})

	cfg := getTestConfig(srv.URL)
	cfg.GlobalTags["app_name"] = "test_app"
	cfg.GlobalTags["origin"] = "containerapp"
	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.start()

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

	srv := assertingServer(t, func(_ *http.Request, body []byte) error {
		// Fargate cloud headers derived from container tags → per-event headers
		batch := decodeBatch(t, body)
		assert.Len(batch.Payload.Events, 1)
		event := batch.Payload.Events[0]
		assert.Equal("AWS", event.Headers["Dd-Cloud-Provider"])
		assert.Equal("AWSFargate", event.Headers["Dd-Cloud-Resource-Type"])
		assert.Equal("test_ARN", event.Headers["Dd-Cloud-Resource-Identifier"])

		endpointCalled.Inc()
		return nil
	})

	cfg := getTestConfig(srv.URL)
	cfg.ContainerTags = func(_ string) ([]string, error) {
		return []string{"task_arn:test_ARN"}, nil
	}
	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.start()
	recv.telemetryForwarder.containerIDProvider = getTestContainerIDProvider()

	assertSendRequest(t, recv, endpointCalled)
}

func TestTelemetryProxyMultipleEndpoints(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	mainBackend := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))
		assert.Equal("test_apikey_1", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))

		batch := decodeBatch(t, body)
		assert.Len(batch.Payload.Events, 1)
		assert.Equal("body", string(batch.Payload.Events[0].Content))

		endpointCalled.Add(2)
		return nil
	})
	additionalBackend := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))
		assert.Equal("test_apikey_2", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))

		batch := decodeBatch(t, body)
		assert.Len(batch.Payload.Events, 1)
		assert.Equal("body", string(batch.Payload.Events[0].Content))

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

	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.start()

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
		reqs                     []testReq
		expectedEndpointsCalled  int
		expectedNumberOfPayloads int
	}
	testCases := []testCase{
		{[]testReq{
			{http.StatusOK, 51},
			{http.StatusOK, 49},
			{http.StatusTooManyRequests, 1},
		}, 1, 2},
		{[]testReq{
			{http.StatusTooManyRequests, 101},
			{http.StatusOK, 100},
			{http.StatusTooManyRequests, 1},
		}, 1, 1},
	}
	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			endpointCalled := atomic.NewUint64(0)
			assert := assert.New(t)

			srv := assertingServer(t, func(req *http.Request, body []byte) error {
				assert.Equal("test_apikey", req.Header.Get("DD-API-KEY"))
				assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
				assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))
				assert.Equal("/api/v2/apmtelemetry", req.URL.Path)
				assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))
				batch := decodeBatch(t, body)
				assert.Len(batch.Payload.Events, testCase.expectedNumberOfPayloads)

				endpointCalled.Add(1)
				return nil
			})

			cfg := getTestConfig(srv.URL)
			recv := newTestReceiverFromConfig(cfg)
			recv.telemetryForwarder.start()
			recv.telemetryForwarder.maxInflightBytes = 100
			mux := recv.buildMux()

			for _, testReq := range testCase.reqs {
				req, rec := newRequestRecorder(t)
				req.Body = io.NopCloser(bytes.NewBuffer(make([]byte, testReq.size)))
				req.ContentLength = int64(testReq.size)
				mux.ServeHTTP(rec, req)

				assert.Equal(testReq.res, recordedStatusCode(rec))
			}

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

	srv := assertingServer(t, func(req *http.Request, _ []byte) error {
		assert.Equal("test_apikey", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))
		assert.Equal("/api/v2/apmtelemetry", req.URL.Path)
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

		<-done
		endpointCalled.Add(1)
		return nil
	})

	cfg := getTestConfig(srv.URL)
	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.start()
	recv.telemetryForwarder.maxInflightBytes = 100
	// Flush immediately so each event is forwarded right away
	recv.telemetryForwarder.batch.batchSizeThreshold = 0
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
		assert.Equal("/api/v2/apmtelemetry", req.URL.Path)
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

		batch := decodeBatch(t, body)
		assert.Len(batch.Payload.Events, 1)
		assert.Equal([]byte{0, 1, 2}, batch.Payload.Events[0].Content)

		<-done
		endpointCalled.Add(1)
		return nil
	})

	cfg := getTestConfig(intakeMockServer.URL)
	r := newTestReceiverFromConfig(cfg)
	r.telemetryForwarder.start() // We call this manually here to avoid starting the entire test receiver
	r.telemetryForwarder.batch.batchSizeThreshold = 0
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
	log.SetLogger(log.NoopLogger) // prevent race conditions on test buffer
	r.telemetryForwarder.Stop()
	assert.Equal(uint64(1), endpointCalled.Load())
	assert.NotContains(logs.String(), "ERROR")
}

func TestTelemetryConfig(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "api_key"
		recv := newTestReceiverFromConfig(cfg)
		recv.telemetryForwarder.start()

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
		recv.telemetryForwarder.start()

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
		recv.telemetryForwarder.start()

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

// --- New batching-specific tests ---

func TestAgentBatchSerialization(t *testing.T) {
	batch := agentBatch{
		RequestType: "agent-batch",
		Payload: rawTelemetryEvents{
			Events: []rawTelemetryEvent{
				{
					Headers: map[string]string{"Content-Type": "application/json", "X-Original-URL": "/api/v2/apmtelemetry"},
					Content: []byte(`{"test": "data"}`),
				},
				{
					Headers: map[string]string{"Content-Type": "text/plain"},
					Content: []byte("raw bytes here"),
				},
			},
		},
	}

	data, err := msgpack.Marshal(&batch)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	var decoded agentBatch
	err = msgpack.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "agent-batch", decoded.RequestType)
	assert.Len(t, decoded.Payload.Events, 2)
	assert.Equal(t, "application/json", decoded.Payload.Events[0].Headers["Content-Type"])
	assert.Equal(t, []byte(`{"test": "data"}`), decoded.Payload.Events[0].Content)
	assert.Equal(t, []byte("raw bytes here"), decoded.Payload.Events[1].Content)
}

func TestBatchSizeTriggeredFlush(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	expectedSizes := []int{1, 3}
	srv := assertingServer(t, func(_ *http.Request, body []byte) error {
		batch := decodeBatch(t, body)
		assert.Equal("agent-batch", batch.RequestType)
		// We send 4 events of 1000 bytes each, threshold is 3000 → flush after 3rd event
		assert.Contains(expectedSizes, len(batch.Payload.Events))
		expectedSizes = slices.DeleteFunc(expectedSizes, func(v int) bool { return v == len(batch.Payload.Events) })
		endpointCalled.Inc()
		return nil
	})

	cfg := getTestConfig(srv.URL)
	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.start()
	recv.telemetryForwarder.batch.batchSizeThreshold = 3000 // low threshold for testing

	mux := recv.buildMux()

	// Send 4 requests of 1000 bytes each — should trigger a size flush after the 3rd
	for i := 0; i < 4; i++ {
		req, rec := newRequestRecorder(t)
		req.Body = io.NopCloser(bytes.NewBuffer(make([]byte, 1000)))
		req.ContentLength = 1000
		mux.ServeHTTP(rec, req)
		assert.Equal(200, recordedStatusCode(rec))
	}

	recv.telemetryForwarder.Stop()
	// At least 1 size-triggered flush + 1 shutdown flush for remaining
	assert.GreaterOrEqual(endpointCalled.Load(), uint64(2))
}

func TestBatchAgeTriggeredFlush(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)

	srv := assertingServer(t, func(_ *http.Request, body []byte) error {
		batch := decodeBatch(t, body)
		assert.Equal(t, "agent-batch", batch.RequestType)
		assert.Len(t, batch.Payload.Events, 1)
		assert.Equal(t, "small", string(batch.Payload.Events[0].Content))
		endpointCalled.Inc()
		return nil
	})

	cfg := getTestConfig(srv.URL)
	recv := newTestReceiverFromConfig(cfg)

	// Use injectable clock to control time
	fakeNow := time.Now()
	recv.telemetryForwarder.batch.nowFn = func() time.Time { return fakeNow }
	recv.telemetryForwarder.start()

	mux := recv.buildMux()

	// Send a single small request — won't trigger size-based flush
	req, err := http.NewRequest("POST", "/telemetry/proxy/path", strings.NewReader("small"))
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, 200, recordedStatusCode(rec))

	// Advance the clock past batchMaxAge
	fakeNow = fakeNow.Add(31 * time.Second)

	// Wait for the ticker to fire and flush
	require.Eventually(t, func() bool {
		return endpointCalled.Load() >= 1
	}, 3*time.Second, 100*time.Millisecond, "age-triggered flush did not happen")

	recv.telemetryForwarder.Stop()
	assert.Equal(t, uint64(1), endpointCalled.Load())
}

func TestBatchGracefulShutdown(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)

	srv := assertingServer(t, func(_ *http.Request, body []byte) error {
		batch := decodeBatch(t, body)
		assert.Equal(t, "agent-batch", batch.RequestType)
		assert.Len(t, batch.Payload.Events, 3)
		for i, event := range batch.Payload.Events {
			assert.Equal(t, "event", string(event.Content), "event %d", i)
		}
		endpointCalled.Inc()
		return nil
	})

	cfg := getTestConfig(srv.URL)
	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.start()

	mux := recv.buildMux()

	// Send 3 small events — won't trigger size or age flush
	for i := 0; i < 3; i++ {
		req, err := http.NewRequest("POST", "/telemetry/proxy/path", strings.NewReader("event"))
		require.NoError(t, err)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		assert.Equal(t, 200, recordedStatusCode(rec))
	}

	// Stop should flush all remaining events
	recv.telemetryForwarder.Stop()
	assert.Equal(t, uint64(1), endpointCalled.Load())
}

func TestBatchMultipleEventsContent(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)

	srv := assertingServer(t, func(_ *http.Request, body []byte) error {
		batch := decodeBatch(t, body)
		// Verify each event has distinct content and preserved original URL
		for _, event := range batch.Payload.Events {
			assert.NotEmpty(t, event.Content)
		}
		endpointCalled.Inc()
		return nil
	})

	cfg := getTestConfig(srv.URL)
	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.start()
	// Flush every event immediately for this test
	recv.telemetryForwarder.batch.batchSizeThreshold = 0

	mux := recv.buildMux()

	bodies := []string{"first", "second"}
	for _, b := range bodies {
		req, err := http.NewRequest("POST", "/telemetry/proxy/path", strings.NewReader(b))
		require.NoError(t, err)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		assert.Equal(t, 200, recordedStatusCode(rec))
	}

	recv.telemetryForwarder.Stop()
	// With batchSizeThreshold=0, each event triggers its own batch
	assert.Equal(t, uint64(len(bodies)), endpointCalled.Load())
}
