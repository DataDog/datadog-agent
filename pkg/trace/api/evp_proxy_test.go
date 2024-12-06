// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	"github.com/DataDog/datadog-go/v5/statsd"
)

type roundTripperMock func(*http.Request) (*http.Response, error)

func (r roundTripperMock) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

// sendRequestThroughForwarderWithMockRoundTripper sends a request through the evpProxyForwarder
// handler and returns the forwarded request(s), their response and the log output. The path for
// inReq shouldn't have the /evp_proxy/v1 prefix since it is passed directly to the inner proxy
// handler and not the trace-agent API handler.
func sendRequestThroughForwarderWithMockRoundTripper(conf *config.AgentConfig, inReq *http.Request, statsd statsd.ClientInterface) (outReqs []*http.Request, resp *http.Response, logs string) {
	mockRoundTripper := roundTripperMock(func(req *http.Request) (*http.Response, error) {
		if req.Body != nil {
			if _, err := io.ReadAll(req.Body); err != nil && err != io.EOF {
				return nil, err
			}
		}
		outReqs = append(outReqs, req)
		// If we got here it means the proxy didn't raise an error earlier, return an ok resp
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer([]byte("ok_resprino"))),
		}, nil
	})
	handler := evpProxyForwarder(conf, statsd)
	var loggerBuffer bytes.Buffer
	handler.(*httputil.ReverseProxy).ErrorLog = log.New(io.Writer(&loggerBuffer), "", 0)
	handler.(*httputil.ReverseProxy).Transport.(*evpProxyTransport).transport = mockRoundTripper
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, inReq)
	return outReqs, rec.Result(), loggerBuffer.String()
}

// sendRequestThroughForwarderAgainstDummyServer sends a request to the specified serverHost URL,
// which should be localhost at a port where you ran a dummy server, for E2E testing. The path for
// inReq shouldn't have the /evp_proxy/v1 prefix since it is passed directly to the inner proxy
// handler and not the trace-agent API handler.
func sendRequestThroughForwarderAgainstDummyServer(conf *config.AgentConfig, inReq *http.Request, statsd statsd.ClientInterface, serverHost string) (resp *http.Response, logs string) {
	reqModifierRoundTripper := roundTripperMock(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.Host = serverHost
		req.URL.Host = serverHost
		return conf.NewHTTPTransport().RoundTrip(req)
	})
	handler := evpProxyForwarder(conf, statsd)
	var loggerBuffer bytes.Buffer
	handler.(*httputil.ReverseProxy).ErrorLog = log.New(io.Writer(&loggerBuffer), "", 0)
	handler.(*httputil.ReverseProxy).Transport.(*evpProxyTransport).transport = reqModifierRoundTripper
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, inReq)
	return rec.Result(), loggerBuffer.String()
}

func TestEVPProxyForwarder(t *testing.T) {
	randBodyBuf := make([]byte, 1024)
	rand.Read(randBodyBuf)

	stats := &teststatsd.Client{}

	t.Run("ok", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Hostname = "test_hostname"
		conf.DefaultEnv = "test_env"
		conf.Site = "us3.datadoghq.com"
		conf.AgentVersion = "testVersion"
		conf.Endpoints[0].APIKey = "test_api_key"

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("User-Agent", "test_user_agent")
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		req.Header.Set("Content-Type", "text/json")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		resp.Body.Close()
		require.Len(t, proxyreqs, 1)
		proxyreq := proxyreqs[0]
		assert.Equal(t, "my.subdomain.us3.datadoghq.com", proxyreq.Host)
		assert.Equal(t, "my.subdomain.us3.datadoghq.com", proxyreq.URL.Host)
		assert.Equal(t, "/mypath/mysubpath", proxyreq.URL.Path)
		assert.Equal(t, "arg=test", proxyreq.URL.RawQuery)
		assert.Equal(t, "test_api_key", proxyreq.Header.Get("DD-API-KEY"))
		assert.Equal(t, conf.Hostname, proxyreq.Header.Get("X-Datadog-Hostname"))
		assert.Equal(t, conf.DefaultEnv, proxyreq.Header.Get("X-Datadog-AgentDefaultEnv"))
		assert.Equal(t, "trace-agent testVersion", proxyreq.Header.Get("Via"))
		assert.Equal(t, "test_user_agent", proxyreq.Header.Get("User-Agent"))
		assert.Equal(t, "text/json", proxyreq.Header.Get("Content-Type"))
		assert.NotContains(t, proxyreq.Header, "X-Datadog-Container-Tags")
		assert.NotContains(t, proxyreq.Header, header.ContainerID)
		assert.Equal(t, "", logs)

		// check metrics
		expectedTags := []string{
			"content_type:text/json",
			"subdomain:my.subdomain",
		}
		require.Len(t, stats.TimingCalls, 1)
		assert.Equal(t, "datadog.trace_agent.evp_proxy.request_duration_ms", stats.TimingCalls[0].Name)
		assert.ElementsMatch(t, expectedTags, stats.TimingCalls[0].Tags)
		assert.Equal(t, float64(1), stats.TimingCalls[0].Rate)
		require.Len(t, stats.CountCalls, 2)
		assert.Equal(t, "datadog.trace_agent.evp_proxy.request", stats.CountCalls[0].Name)
		assert.Equal(t, float64(1), stats.CountCalls[0].Value)
		assert.Equal(t, float64(1), stats.CountCalls[0].Rate)
		assert.ElementsMatch(t, expectedTags, stats.CountCalls[0].Tags)
		assert.Equal(t, "datadog.trace_agent.evp_proxy.request_bytes", stats.CountCalls[1].Name)
		assert.Equal(t, float64(1024), stats.CountCalls[1].Value)
		assert.Equal(t, float64(1), stats.CountCalls[1].Rate)
		assert.ElementsMatch(t, expectedTags, stats.CountCalls[1].Tags)
	})

	t.Run("containerID", func(t *testing.T) {
		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.ContainerTags = func(cid string) ([]string, error) {
			return []string{"container:" + cid}, nil
		}

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		req.Header.Set(header.ContainerID, "myid")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		require.Len(t, proxyreqs, 1)
		assert.Equal(t, "container:myid", proxyreqs[0].Header.Get("X-Datadog-Container-Tags"))
		assert.Equal(t, "myid", proxyreqs[0].Header.Get(header.ContainerID))
		assert.Equal(t, "", logs)
	})

	t.Run("normalizedContainerTags", func(t *testing.T) {
		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.ContainerTags = func(cid string) ([]string, error) {
			return []string{"container:" + cid, "key:\nval"}, nil
		}

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		req.Header.Set(header.ContainerID, "myid")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		require.Len(t, proxyreqs, 1)
		assert.Equal(t, "container:myid,key:_val", proxyreqs[0].Header.Get("X-Datadog-Container-Tags"))
		assert.Equal(t, "myid", proxyreqs[0].Header.Get(header.ContainerID))
		assert.Equal(t, "", logs)
	})

	t.Run("dual-shipping", func(t *testing.T) {
		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.EVPProxy.AdditionalEndpoints = map[string][]string{
			"datadoghq.eu": {"test_api_key_1", "test_api_key_2"},
		}
		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		require.Len(t, proxyreqs, 3)

		assert.Equal(t, "my.subdomain.us3.datadoghq.com", proxyreqs[0].Host)
		assert.Equal(t, "my.subdomain.us3.datadoghq.com", proxyreqs[0].URL.Host)
		assert.Equal(t, "/mypath/mysubpath", proxyreqs[0].URL.Path)
		assert.Equal(t, "arg=test", proxyreqs[0].URL.RawQuery)
		assert.Equal(t, "test_api_key", proxyreqs[0].Header.Get("DD-API-KEY"))

		assert.Equal(t, "my.subdomain.datadoghq.eu", proxyreqs[1].Host)
		assert.Equal(t, "my.subdomain.datadoghq.eu", proxyreqs[1].URL.Host)
		assert.Equal(t, "/mypath/mysubpath", proxyreqs[1].URL.Path)
		assert.Equal(t, "arg=test", proxyreqs[1].URL.RawQuery)
		assert.Equal(t, "test_api_key_1", proxyreqs[1].Header.Get("DD-API-KEY"))

		assert.Equal(t, "my.subdomain.datadoghq.eu", proxyreqs[2].Host)
		assert.Equal(t, "my.subdomain.datadoghq.eu", proxyreqs[2].URL.Host)
		assert.Equal(t, "test_api_key_2", proxyreqs[2].Header.Get("DD-API-KEY"))

		assert.Equal(t, "", logs)
	})

	t.Run("no-subdomain", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"

		req := httptest.NewRequest("POST", "/mypath/mysubpath", bytes.NewReader(randBodyBuf))
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		resp.Body.Close()
		require.Len(t, proxyreqs, 0)
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		require.Contains(t, logs, "no subdomain")

		// check metrics
		require.Len(t, stats.CountCalls, 3)
		assert.Equal(t, "datadog.trace_agent.evp_proxy.request_error", stats.CountCalls[2].Name)
		assert.Equal(t, float64(1), stats.CountCalls[2].Value)
		assert.Equal(t, float64(1), stats.CountCalls[2].Rate)
		assert.Len(t, stats.CountCalls[2].Tags, 0)
	})

	t.Run("invalid-subdomain", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"

		req := httptest.NewRequest("POST", "/mypath/mysubpath", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "/google.com%3Fattack=")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		resp.Body.Close()
		require.Len(t, proxyreqs, 0)
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		require.Contains(t, logs, "invalid subdomain")

		// check metrics
		require.Len(t, stats.CountCalls, 3)
		assert.Equal(t, "datadog.trace_agent.evp_proxy.request_error", stats.CountCalls[2].Name)
		assert.Equal(t, float64(1), stats.CountCalls[2].Value)
		assert.Equal(t, float64(1), stats.CountCalls[2].Rate)
		assert.Len(t, stats.CountCalls[2].Tags, 0)
	})

	t.Run("invalid-path", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"

		req := httptest.NewRequest("POST", "/mypath/my%20subpath", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		resp.Body.Close()
		require.Len(t, proxyreqs, 0)
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		require.Contains(t, logs, "invalid target path")

		// check metrics
		expectedTags := []string{
			"subdomain:my.subdomain",
		}
		require.Len(t, stats.CountCalls, 3)
		assert.Equal(t, "datadog.trace_agent.evp_proxy.request_error", stats.CountCalls[2].Name)
		assert.Equal(t, float64(1), stats.CountCalls[2].Value)
		assert.Equal(t, float64(1), stats.CountCalls[2].Rate)
		assert.ElementsMatch(t, expectedTags, stats.CountCalls[2].Tags)
	})

	t.Run("invalid-query", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"

		req := httptest.NewRequest("POST", "/mypath/mysubpath?test=bad%20arg", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		resp.Body.Close()
		require.Len(t, proxyreqs, 0)
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		require.Contains(t, logs, "invalid query string")

		// check metrics
		expectedTags := []string{
			"subdomain:my.subdomain",
		}
		require.Len(t, stats.CountCalls, 3)
		assert.Equal(t, "datadog.trace_agent.evp_proxy.request_error", stats.CountCalls[2].Name)
		assert.Equal(t, float64(1), stats.CountCalls[2].Value)
		assert.Equal(t, float64(1), stats.CountCalls[2].Rate)
		assert.ElementsMatch(t, expectedTags, stats.CountCalls[2].Tags)
	})

	t.Run("maxpayloadsize", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.EVPProxy.MaxPayloadSize = 42

		req := httptest.NewRequest("POST", "/mypath/mysubpath", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		resp.Body.Close()
		require.Len(t, proxyreqs, 0)
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		require.Contains(t, logs, "read limit reached")

		// check metrics
		expectedTags := []string{
			"subdomain:my.subdomain",
		}
		require.Len(t, stats.CountCalls, 3)
		assert.Equal(t, "datadog.trace_agent.evp_proxy.request_error", stats.CountCalls[2].Name)
		assert.Equal(t, float64(1), stats.CountCalls[2].Value)
		assert.Equal(t, float64(1), stats.CountCalls[2].Rate)
		assert.ElementsMatch(t, expectedTags, stats.CountCalls[2].Tags)
	})

	t.Run("ddurl", func(t *testing.T) {
		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.EVPProxy.DDURL = "override.datadoghq.com"
		conf.EVPProxy.APIKey = "override_api_key"

		endpoints := evpProxyEndpointsFromConfig(conf)

		require.Len(t, endpoints, 1)
		assert.Equal(t, endpoints[0].Host, "override.datadoghq.com")
		assert.Equal(t, endpoints[0].APIKey, "override_api_key")
	})

	t.Run("appkey", func(t *testing.T) {
		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.EVPProxy.ApplicationKey = "test_application_key"

		req := httptest.NewRequest("POST", "/mypath/mysubpath", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		req.Header.Set("X-Datadog-NeedsAppKey", "true")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		require.Len(t, proxyreqs, 1)
		assert.Equal(t, "test_application_key", proxyreqs[0].Header.Get("DD-APPLICATION-KEY"))
		assert.Equal(t, "", logs)
	})

	t.Run("missing-appkey", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"

		req := httptest.NewRequest("POST", "/mypath/mysubpath", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		req.Header.Set("X-Datadog-NeedsAppKey", "true")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		resp.Body.Close()
		require.Len(t, proxyreqs, 0)
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		require.Contains(t, logs, "ApplicationKey needed but not set")

		// check metrics
		expectedTags := []string{
			"subdomain:my.subdomain",
		}
		require.Len(t, stats.CountCalls, 3)
		assert.Equal(t, "datadog.trace_agent.evp_proxy.request_error", stats.CountCalls[2].Name)
		assert.Equal(t, float64(1), stats.CountCalls[2].Value)
		assert.Equal(t, float64(1), stats.CountCalls[2].Rate)
		assert.ElementsMatch(t, expectedTags, stats.CountCalls[2].Tags)
	})

	t.Run("headerfilter", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		req.Header.Set("Content-Type", "text/json")
		req.Header.Set("Accept-Encoding", "gzip")
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Unexpected-Header", "To-Be-Discarded")
		req.Header.Set("DD-CI-PROVIDER-NAME", "Allowed-Header")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		resp.Body.Close()
		require.Len(t, proxyreqs, 1)
		proxyreq := proxyreqs[0]
		assert.Equal(t, "", proxyreq.Header.Get("User-Agent")) // User Agent is always set, even if empty
		assert.Equal(t, "text/json", proxyreq.Header.Get("Content-Type"))
		assert.Equal(t, "gzip", proxyreq.Header.Get("Accept-Encoding"))
		assert.Equal(t, "gzip", proxyreq.Header.Get("Content-Encoding"))
		assert.Equal(t, "Allowed-Header", proxyreq.Header.Get("DD-CI-PROVIDER-NAME"))
		assert.NotContains(t, proxyreq.Header, "Unexpected-Header")
		assert.NotContains(t, proxyreq.Header, "X-Datadog-EVP-Subdomain")
		assert.Equal(t, "", logs)
	})
}

func TestEVPProxyHandler(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		cfg := config.New()
		receiver := &HTTPReceiver{conf: cfg}
		handler := receiver.evpProxyHandler(2)
		require.NotNil(t, handler)
	})

	t.Run("disabled", func(t *testing.T) {
		cfg := config.New()
		cfg.EVPProxy.Enabled = false
		receiver := &HTTPReceiver{conf: cfg}
		handler := receiver.evpProxyHandler(2)
		require.NotNil(t, handler)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/evp_proxy/v2/mypath", nil)
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})
}

func TestE2E(t *testing.T) {
	randBodyBuf := make([]byte, 1024)
	rand.Read(randBodyBuf)

	stats := &teststatsd.Client{}

	t.Run("ok", func(t *testing.T) {
		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`OK`))
		}))

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		resp, logs := sendRequestThroughForwarderAgainstDummyServer(conf, req, stats, strings.TrimPrefix(server.URL, "http://"))

		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		assert.Equal(t, "", logs)
		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, "OK", string(body))
	})

	t.Run("timeout", func(t *testing.T) {
		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.EVPProxy.ReceiverTimeout = 1 // in seconds

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(2 * time.Second)
			w.Write([]byte(`OK`))
		}))

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		resp, logs := sendRequestThroughForwarderAgainstDummyServer(conf, req, stats, strings.TrimPrefix(server.URL, "http://"))

		resp.Body.Close()
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		assert.Equal(t, "http: proxy error: context deadline exceeded\n", logs)
	})

	t.Run("chunked-response", func(t *testing.T) {
		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.EVPProxy.ReceiverTimeout = 1 // in seconds

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Transfer-Encoding", "chunked")
			w.Write([]byte(`Hello`))
			w.(http.Flusher).Flush()
			time.Sleep(200 * time.Millisecond)
			w.Write([]byte(`World`)) // this will be discarded if the context was cancelled
		}))

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		resp, logs := sendRequestThroughForwarderAgainstDummyServer(conf, req, stats, strings.TrimPrefix(server.URL, "http://"))

		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
		assert.Equal(t, "", logs)
		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, "HelloWorld", string(body))
	})
}
