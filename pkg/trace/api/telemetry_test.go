// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

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
		_, err = w.Write([]byte("OK"))
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
		assert.Equal("AWS Lambda", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_ARN", req.Header.Get("DD-Cloud-Resource-Identifier"))
		assert.Equal("/path", req.URL.Path)
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

		endpointCalled.Inc()
		return nil
	})

	req, rec := newRequestRecorder(t)
	cfg := getTestConfig(srv.URL)
	cfg.GlobalTags[functionARNKeyTag] = "test_ARN"
	recv := newTestReceiverFromConfig(cfg)
	recv.buildMux().ServeHTTP(rec, req)

	assert.Equal("OK", recordedResponse(t, rec))
	assert.Equal(uint64(1), endpointCalled.Load())

}

func TestGoogleCloudRun(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	srv := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("GCP", req.Header.Get("DD-Cloud-Provider"))
		assert.Equal("GCP Cloud Run", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_service", req.Header.Get("DD-Cloud-Resource-Identifier"))

		endpointCalled.Inc()
		return nil
	})

	req, rec := newRequestRecorder(t)
	cfg := getTestConfig(srv.URL)
	cfg.GlobalTags["service_name"] = "test_service"
	cfg.GlobalTags["origin"] = "cloudrun"
	recv := newTestReceiverFromConfig(cfg)
	recv.buildMux().ServeHTTP(rec, req)

	assert.Equal("OK", recordedResponse(t, rec))
	assert.Equal(uint64(1), endpointCalled.Load())
}

func TestAzureAppService(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	srv := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("Azure", req.Header.Get("DD-Cloud-Provider"))
		assert.Equal("Azure App Service", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_app", req.Header.Get("DD-Cloud-Resource-Identifier"))
		assert.Equal("/path", req.URL.Path)
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

		endpointCalled.Inc()
		return nil
	})

	req, rec := newRequestRecorder(t)
	cfg := getTestConfig(srv.URL)
	cfg.GlobalTags["app_name"] = "test_app"
	cfg.GlobalTags["origin"] = "appservice"
	recv := newTestReceiverFromConfig(cfg)
	recv.buildMux().ServeHTTP(rec, req)

	assert.Equal("OK", recordedResponse(t, rec))
	assert.Equal(uint64(1), endpointCalled.Load())
}

func TestAzureContainerApp(t *testing.T) {
	endpointCalled := atomic.NewUint64(0)
	assert := assert.New(t)

	srv := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("Azure", req.Header.Get("DD-Cloud-Provider"))
		assert.Equal("Azure Container App", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_app", req.Header.Get("DD-Cloud-Resource-Identifier"))
		assert.Equal("/path", req.URL.Path)
		assert.Equal("", req.Header.Get("User-Agent"))
		assert.Regexp(regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))

		endpointCalled.Inc()
		return nil
	})

	req, rec := newRequestRecorder(t)
	cfg := getTestConfig(srv.URL)
	cfg.GlobalTags["app_name"] = "test_app"
	cfg.GlobalTags["origin"] = "containerapp"
	recv := newTestReceiverFromConfig(cfg)
	recv.buildMux().ServeHTTP(rec, req)

	assert.Equal("OK", recordedResponse(t, rec))
	assert.Equal(uint64(1), endpointCalled.Load())
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

	srv := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("AWS", req.Header.Get("DD-Cloud-Provider"))
		assert.Equal("AWS Fargate", req.Header.Get("DD-Cloud-Resource-Type"))
		assert.Equal("test_ARN", req.Header.Get("DD-Cloud-Resource-Identifier"))

		endpointCalled.Inc()
		return nil
	})

	req, rec := newRequestRecorder(t)
	cfg := getTestConfig(srv.URL)
	cfg.ContainerTags = func(cid string) ([]string, error) {
		return []string{"task_arn:test_ARN"}, nil
	}
	recv := newTestReceiverFromConfig(cfg)
	recv.containerIDProvider = getTestContainerIDProvider()
	recv.buildMux().ServeHTTP(rec, req)

	assert.Equal("OK", recordedResponse(t, rec))
	assert.Equal(uint64(1), endpointCalled.Load())
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
		assert.Equal("AWS Lambda", req.Header.Get("DD-Cloud-Resource-Type"))
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
		assert.Equal("AWS Lambda", req.Header.Get("DD-Cloud-Resource-Type"))
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

	req, rec := newRequestRecorder(t)
	recv := newTestReceiverFromConfig(cfg)
	recv.buildMux().ServeHTTP(rec, req)

	assert.Equal("OK", recordedResponse(t, rec))

	// because we use number 2,3 both endpoints must be called to produce 5
	// just counting number of requests could give false results if first endpoint
	// was called twice
	if endpointCalled.Load() != 5 {
		t.Fatalf("calling multiple backends failed")
	}
}

func TestTelemetryConfig(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "api_key"

		req, rec := newRequestRecorder(t)
		recv := newTestReceiverFromConfig(cfg)
		recv.buildMux().ServeHTTP(rec, req)
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
		req, rec := newRequestRecorder(t)
		recv := newTestReceiverFromConfig(cfg)
		recv.buildMux().ServeHTTP(rec, req)
		result := rec.Result()
		assert.Equal(t, 404, result.StatusCode)
		result.Body.Close()
	})

	t.Run("fallback-endpoint", func(t *testing.T) {
		srv := assertingServer(t, func(req *http.Request, body []byte) error { return nil })
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

		req, rec := newRequestRecorder(t)
		recv := newTestReceiverFromConfig(cfg)
		recv.buildMux().ServeHTTP(rec, req)

		assert.Equal(t, "OK", recordedResponse(t, rec))
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
