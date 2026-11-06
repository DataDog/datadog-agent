// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
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

		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
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
		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
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
		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
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
		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
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
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
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
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
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
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
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
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
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

	t.Run("ok-query-string-square-brackets", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Hostname = "test_hostname"
		conf.DefaultEnv = "test_env"
		conf.Site = "us3.datadoghq.com"
		conf.AgentVersion = "testVersion"
		conf.Endpoints[0].APIKey = "test_api_key"

		q := url.Values{}
		q.Set("filter[name]", "some-name")
		target := "/mypath/mysubpath?" + q.Encode()

		req := httptest.NewRequest(http.MethodGet, target, bytes.NewReader(randBodyBuf))
		req.Header.Set("User-Agent", "test_user_agent")
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		req.Header.Set("Content-Type", "text/json")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		require.Equal(t, http.StatusOK, resp.StatusCode, "wrong status code - logs: %s", logs)
		resp.Body.Close()
		require.Len(t, proxyreqs, 1)
		proxyreq := proxyreqs[0]
		assert.Equal(t, "my.subdomain.us3.datadoghq.com", proxyreq.Host)
		assert.Equal(t, "my.subdomain.us3.datadoghq.com", proxyreq.URL.Host)
		assert.Equal(t, "/mypath/mysubpath", proxyreq.URL.Path)
		assert.Equal(t, "filter%5Bname%5D=some-name", proxyreq.URL.RawQuery)
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
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
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

		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
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
		assert.NotContains(t, proxyreq.Header, "X-Datadog-Error-Tracking-Standalone")
		assert.Equal(t, "", logs)
	})

	t.Run("origin-headers-forwarded", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		req.Header.Set("DD-EVP-ORIGIN", "my-server-sdk")
		req.Header.Set("DD-EVP-ORIGIN-VERSION", "1.2.3")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
		resp.Body.Close()
		require.Len(t, proxyreqs, 1)
		proxyreq := proxyreqs[0]
		assert.Equal(t, "my-server-sdk", proxyreq.Header.Get("DD-EVP-ORIGIN"))
		assert.Equal(t, "1.2.3", proxyreq.Header.Get("DD-EVP-ORIGIN-VERSION"))
		assert.Equal(t, "", logs)
	})

	t.Run("origin-headers-casing", func(t *testing.T) {
		// Go's http.Header.Get/Set canonicalize header keys, so a caller sending
		// lowercase headers should still be matched by the allowlist and forwarded.
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		req.Header.Set("dd-evp-origin", "my-server-sdk")
		req.Header.Set("dd-evp-origin-version", "1.2.3")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
		resp.Body.Close()
		require.Len(t, proxyreqs, 1)
		proxyreq := proxyreqs[0]
		assert.Equal(t, "my-server-sdk", proxyreq.Header.Get("DD-EVP-ORIGIN"))
		assert.Equal(t, "1.2.3", proxyreq.Header.Get("DD-EVP-ORIGIN-VERSION"))
		assert.Equal(t, "", logs)
	})

	t.Run("origin-headers-dual-shipping", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.EVPProxy.AdditionalEndpoints = map[string][]string{
			"datadoghq.eu": {"test_api_key_1", "test_api_key_2"},
		}

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		req.Header.Set("DD-EVP-ORIGIN", "my-server-sdk")
		req.Header.Set("DD-EVP-ORIGIN-VERSION", "1.2.3")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
		resp.Body.Close()
		require.Len(t, proxyreqs, 3)
		for _, pr := range proxyreqs {
			assert.Equal(t, "my-server-sdk", pr.Header.Get("DD-EVP-ORIGIN"))
			assert.Equal(t, "1.2.3", pr.Header.Get("DD-EVP-ORIGIN-VERSION"))
		}
		assert.Equal(t, "", logs)
	})

	t.Run("origin-headers-stripped-unrelated", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		req.Header.Set("DD-EVP-ORIGIN", "my-server-sdk")
		req.Header.Set("DD-EVP-ORIGIN-VERSION", "1.2.3")
		req.Header.Set("Unexpected-Header", "To-Be-Discarded")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
		resp.Body.Close()
		require.Len(t, proxyreqs, 1)
		proxyreq := proxyreqs[0]
		assert.Equal(t, "my-server-sdk", proxyreq.Header.Get("DD-EVP-ORIGIN"))
		assert.Equal(t, "1.2.3", proxyreq.Header.Get("DD-EVP-ORIGIN-VERSION"))
		assert.NotContains(t, proxyreq.Header, "Unexpected-Header")
		assert.Equal(t, "", logs)
	})

	t.Run("origin-headers-absent", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
		resp.Body.Close()
		require.Len(t, proxyreqs, 1)
		proxyreq := proxyreqs[0]
		// Headers are not synthesized; absent on the inbound request means absent on the outbound request.
		assert.NotContains(t, proxyreq.Header, "DD-EVP-ORIGIN")
		assert.NotContains(t, proxyreq.Header, "DD-EVP-ORIGIN-VERSION")
		assert.Equal(t, "", logs)
	})

	t.Run("error-tracking-standalone-header-added", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.ErrorTrackingStandalone = true

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
		resp.Body.Close()
		require.Len(t, proxyreqs, 1)
		proxyreq := proxyreqs[0]
		assert.Contains(t, proxyreq.Header, "X-Datadog-Error-Tracking-Standalone")
		assert.Equal(t, "", logs)
	})

	t.Run("error-tracking-standalone-header-exists", func(t *testing.T) {
		stats.Reset()

		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.ErrorTrackingStandalone = true

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		req.Header.Set("X-Datadog-Error-Tracking-Standalone", "orig")
		proxyreqs, resp, logs := sendRequestThroughForwarderWithMockRoundTripper(conf, req, stats)

		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
		resp.Body.Close()
		require.Len(t, proxyreqs, 1)
		proxyreq := proxyreqs[0]
		assert.Equal(t, proxyreq.Header.Get("X-Datadog-Error-Tracking-Standalone"), "true")
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
		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
		assert.Equal(t, "", logs)
		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, "OK", string(body))
	})

	t.Run("timeout", func(t *testing.T) {
		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.EVPProxy.ReceiverTimeoutDuration = 50 * time.Millisecond

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.Write([]byte(`OK`))
		}))

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		resp, logs := sendRequestThroughForwarderAgainstDummyServer(conf, req, stats, strings.TrimPrefix(server.URL, "http://"))

		resp.Body.Close()
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
		assert.Equal(t, "http: proxy error: context deadline exceeded\n", logs)
	})

	t.Run("chunked-response", func(t *testing.T) {
		conf := newTestReceiverConfig()
		conf.Site = "us3.datadoghq.com"
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.EVPProxy.ReceiverTimeoutDuration = 500 * time.Millisecond

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Transfer-Encoding", "chunked")
			w.Write([]byte(`Hello`))
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
			w.Write([]byte(`World`)) // this will be discarded if the context was cancelled
		}))

		req := httptest.NewRequest("POST", "/mypath/mysubpath?arg=test", bytes.NewReader(randBodyBuf))
		req.Header.Set("X-Datadog-EVP-Subdomain", "my.subdomain")
		resp, logs := sendRequestThroughForwarderAgainstDummyServer(conf, req, stats, strings.TrimPrefix(server.URL, "http://"))

		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", strconv.Itoa(resp.StatusCode))
		assert.Equal(t, "", logs)
		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, "HelloWorld", string(body))
	})
}

// aiGuardTestConfig returns a config with both keys, so that the AI Guard proxy is registered.
func aiGuardTestConfig() *config.AgentConfig {
	conf := newTestReceiverConfigNoPort()
	conf.AgentVersion = "7.99.0"
	conf.Site = "datadoghq.test"
	conf.Endpoints[0].APIKey = "test_api_key"
	conf.EVPProxy.ApplicationKey = "test_app_key"
	return conf
}

// aiGuardTestMux returns the receiver's mux, with outgoing connections going to upstream, a TLS
// server standing in for https://api.<site>.
func aiGuardTestMux(t *testing.T, conf *config.AgentConfig, upstream http.HandlerFunc) http.Handler {
	t.Helper()
	srv := httptest.NewTLSServer(upstream)
	t.Cleanup(srv.Close)
	conf.HTTPTransportFunc = func() *http.Transport {
		transport := srv.Client().Transport.(*http.Transport).Clone()
		transport.TLSClientConfig.InsecureSkipVerify = true // the test certificate is not valid for api.datadoghq.test
		transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
		}
		return transport
	}
	return newTestReceiverFromConfig(conf).buildMux()
}

func newAIGuardRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, "/ai_guard/proxy"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func assertAIGuardProxyError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	assert.Equal(t, status, rec.Code)
	assert.Equal(t, code, rec.Header().Get("Datadog-Agent-Proxy-Error"))
	var body struct {
		Errors []struct {
			Status string `json:"status"`
			Code   string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	require.Len(t, body.Errors, 1)
	assert.Equal(t, strconv.Itoa(status), body.Errors[0].Status)
	assert.Equal(t, code, body.Errors[0].Code)
}

func unexpectedUpstream(t *testing.T) http.HandlerFunc {
	return func(_ http.ResponseWriter, req *http.Request) {
		t.Errorf("unexpected request to Datadog: %s %s", req.Method, req.URL)
	}
}

func TestAIGuardProxy(t *testing.T) {
	type upstreamRequest struct {
		method, host, uri, body string
		header, trailer         http.Header
	}
	recordingUpstream := func(received chan<- upstreamRequest) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			body, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			received <- upstreamRequest{req.Method, req.Host, req.URL.RequestURI(), string(body), req.Header.Clone(), req.Trailer.Clone()}
			w.Header().Set("Datadog-Agent-Proxy-Error", "from_datadog")
			w.Header().Set("X-From-Datadog", "yes")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"errors":["forbidden"]}`))
		}
	}

	t.Run("post", func(t *testing.T) {
		received := make(chan upstreamRequest, 1)
		mux := aiGuardTestMux(t, aiGuardTestConfig(), recordingUpstream(received))

		req := newAIGuardRequest("POST", "/api/v2/ai-guard/evaluate", `{"messages":[]}`)
		req.Header.Set("DD-AI-GUARD-VERSION", "1.2.3")
		req.Header.Set("DD-AI-GUARD-SOURCE", "SDK")
		req.Header.Set("DD-AI-GUARD-LANGUAGE", "python")
		req.Header.Set("DD-API-KEY", "client_api_key")
		req.Header.Set("DD-APPLICATION-KEY", "client_app_key")
		req.Header.Set("X-Not-Allowed", "dropped")
		req.Header.Set("X-Datadog-EVP-Subdomain", "other")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Len(t, received, 1)
		got := <-received
		assert.Equal(t, "POST", got.method)
		assert.Equal(t, "api.datadoghq.test", got.host)
		assert.Equal(t, "/api/v2/ai-guard/evaluate", got.uri)
		assert.Equal(t, `{"messages":[]}`, got.body)
		assert.Equal(t, "test_api_key", got.header.Get("DD-API-KEY"))
		assert.Equal(t, "test_app_key", got.header.Get("DD-APPLICATION-KEY"))
		assert.Equal(t, "application/json", got.header.Get("Content-Type"))
		assert.Equal(t, "1.2.3", got.header.Get("DD-AI-GUARD-VERSION"))
		assert.Equal(t, "SDK", got.header.Get("DD-AI-GUARD-SOURCE"))
		assert.Equal(t, "python", got.header.Get("DD-AI-GUARD-LANGUAGE"))
		assert.Empty(t, got.header.Get("X-Not-Allowed"))

		// Datadog's status and body are returned as is, without its Datadog-Agent-Proxy-Error header.
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Equal(t, `{"errors":["forbidden"]}`, rec.Body.String())
		assert.Equal(t, "yes", rec.Header().Get("X-From-Datadog"))
		assert.Empty(t, rec.Header().Values("Datadog-Agent-Proxy-Error"))
		assert.Equal(t, "7.99.0", rec.Header().Get("Datadog-Agent-Version"))
	})

	t.Run("get", func(t *testing.T) {
		received := make(chan upstreamRequest, 1)
		mux := aiGuardTestMux(t, aiGuardTestConfig(), recordingUpstream(received))

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/ai_guard/proxy/api/v2/ai-guard/policy", nil))

		require.Len(t, received, 1)
		got := <-received
		assert.Equal(t, "GET", got.method)
		assert.Equal(t, "/api/v2/ai-guard/policy", got.uri)
		assert.Equal(t, "test_app_key", got.header.Get("DD-APPLICATION-KEY"))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	for _, tc := range []struct {
		name   string
		req    func() *http.Request
		status int
		code   string
	}{
		{
			name:   "method",
			req:    func() *http.Request { return newAIGuardRequest("PUT", "/api/v2/ai-guard/evaluate", "{}") },
			status: http.StatusNotFound,
			code:   "path_not_allowed",
		},
		{
			name:   "other api",
			req:    func() *http.Request { return newAIGuardRequest("POST", "/api/v2/logs", "{}") },
			status: http.StatusNotFound,
			code:   "path_not_allowed",
		},
		{
			name:   "ai guard api root",
			req:    func() *http.Request { return newAIGuardRequest("POST", "/api/v2/ai-guard", "{}") },
			status: http.StatusNotFound,
			code:   "path_not_allowed",
		},
		{
			name: "encoded dot segments",
			req: func() *http.Request {
				return newAIGuardRequest("POST", "/api/v2/ai-guard/%2e%2e/%2e%2e/v1/validate", "{}")
			},
			status: http.StatusNotFound,
			code:   "path_not_allowed",
		},
		{
			name:   "bare query marker",
			req:    func() *http.Request { return newAIGuardRequest("POST", "/api/v2/ai-guard/evaluate?", "{}") },
			status: http.StatusNotFound,
			code:   "path_not_allowed",
		},
		{
			name:   "query string",
			req:    func() *http.Request { return newAIGuardRequest("POST", "/api/v2/ai-guard/evaluate?api_key=x", "{}") },
			status: http.StatusNotFound,
			code:   "path_not_allowed",
		},
		{
			name:   "query string on get",
			req:    func() *http.Request { return newAIGuardRequest("GET", "/api/v2/ai-guard/policy?api_key=x", "") },
			status: http.StatusNotFound,
			code:   "path_not_allowed",
		},
		{
			name: "cross-site",
			req: func() *http.Request {
				req := newAIGuardRequest("POST", "/api/v2/ai-guard/evaluate", "{}")
				req.Header.Set("Sec-Fetch-Site", "cross-site")
				return req
			},
			status: http.StatusForbidden,
		},
		{
			name: "not json",
			req: func() *http.Request {
				req := newAIGuardRequest("POST", "/api/v2/ai-guard/evaluate", "{}")
				req.Header.Set("Content-Type", "text/plain")
				return req
			},
			status: http.StatusUnsupportedMediaType,
		},
	} {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			mux := aiGuardTestMux(t, aiGuardTestConfig(), unexpectedUpstream(t))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, tc.req())
			if tc.code == "" {
				assert.Equal(t, tc.status, rec.Code)
				// Only the documented codes are marked as Agent errors.
				assert.Empty(t, rec.Header().Values(aiGuardProxyErrorHeader))
				return
			}
			assertAIGuardProxyError(t, rec, tc.status, tc.code)
		})
	}

	// Trailers only reach the proxy from a chunked request, so this goes through a real server.
	t.Run("trailers are not forwarded", func(t *testing.T) {
		received := make(chan upstreamRequest, 1)
		agent := httptest.NewServer(aiGuardTestMux(t, aiGuardTestConfig(), recordingUpstream(received)))
		t.Cleanup(agent.Close)

		body, chunks := io.Pipe()
		req, err := http.NewRequest("POST", agent.URL+"/ai_guard/proxy/api/v2/ai-guard/evaluate", body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Trailer = http.Header{"X-Not-Allowed-Trailer": nil}
		go func() {
			_, _ = chunks.Write([]byte("{}"))
			req.Trailer.Set("X-Not-Allowed-Trailer", "leaked")
			chunks.Close()
		}()
		resp, err := agent.Client().Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode) // the upstream's own status, so the request was proxied

		require.Len(t, received, 1)
		got := <-received
		assert.Equal(t, `{}`, got.body)
		assert.Empty(t, got.trailer, "trailers must not be forwarded to Datadog")
	})

	// A rotated application key must reach Datadog without restarting the Agent.
	t.Run("rotated application key", func(t *testing.T) {
		received := make(chan upstreamRequest, 2)
		conf := aiGuardTestConfig()
		mux := aiGuardTestMux(t, conf, recordingUpstream(received))

		mux.ServeHTTP(httptest.NewRecorder(), newAIGuardRequest("POST", "/api/v2/ai-guard/evaluate", "{}"))
		require.Len(t, received, 1)
		assert.Equal(t, "test_app_key", (<-received).header.Get("DD-APPLICATION-KEY"))

		conf.UpdateApplicationKey("rotated_app_key")

		mux.ServeHTTP(httptest.NewRecorder(), newAIGuardRequest("POST", "/api/v2/ai-guard/evaluate", "{}"))
		require.Len(t, received, 1)
		assert.Equal(t, "rotated_app_key", (<-received).header.Get("DD-APPLICATION-KEY"))
	})

	// The AI Guard proxy goes to the main endpoint only, so that the application key never fans out.
	t.Run("main endpoint only", func(t *testing.T) {
		received := make(chan upstreamRequest, 2)
		conf := aiGuardTestConfig()
		conf.EVPProxy.DDURL = "evp.datadoghq.test"
		conf.EVPProxy.AdditionalEndpoints = map[string][]string{"additional.datadoghq.test": {"additional_api_key"}}
		mux := aiGuardTestMux(t, conf, recordingUpstream(received))

		mux.ServeHTTP(httptest.NewRecorder(), newAIGuardRequest("POST", "/api/v2/ai-guard/evaluate", "{}"))

		require.Len(t, received, 1)
		got := <-received
		assert.Equal(t, "api.datadoghq.test", got.host)
		assert.Equal(t, "test_api_key", got.header.Get("DD-API-KEY"))
	})

	// A body of exactly the limit is allowed, so the limit must not be applied a second time
	// while forwarding: that rejects it as a network error instead.
	t.Run("payload at the exact limit", func(t *testing.T) {
		body := `{"a":"bc"}`
		received := make(chan upstreamRequest, 1)
		conf := aiGuardTestConfig()
		conf.EVPProxy.MaxPayloadSize = int64(len(body))
		mux := aiGuardTestMux(t, conf, recordingUpstream(received))

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, newAIGuardRequest("POST", "/api/v2/ai-guard/evaluate", body))

		require.Len(t, received, 1)
		assert.Equal(t, body, (<-received).body)
		assert.Equal(t, http.StatusForbidden, rec.Code) // the upstream's own status, so it was proxied
		assert.Empty(t, rec.Header().Values(aiGuardProxyErrorHeader))
	})

	t.Run("payload_too_large", func(t *testing.T) {
		conf := aiGuardTestConfig()
		conf.EVPProxy.MaxPayloadSize = 8
		mux := aiGuardTestMux(t, conf, unexpectedUpstream(t))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, newAIGuardRequest("POST", "/api/v2/ai-guard/evaluate", `{"a":"bc"}`))
		assertAIGuardProxyError(t, rec, http.StatusRequestEntityTooLarge, "payload_too_large")
	})

	t.Run("connection_failed", func(t *testing.T) {
		conf := aiGuardTestConfig()
		conf.HTTPTransportFunc = func() *http.Transport {
			return &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
				return nil, errors.New("connection refused")
			}}
		}
		rec := httptest.NewRecorder()
		newTestReceiverFromConfig(conf).buildMux().ServeHTTP(rec, newAIGuardRequest("POST", "/api/v2/ai-guard/evaluate", "{}"))
		assertAIGuardProxyError(t, rec, http.StatusBadGateway, "connection_failed")
	})

	t.Run("network_error", func(t *testing.T) {
		mux := aiGuardTestMux(t, aiGuardTestConfig(), func(w http.ResponseWriter, _ *http.Request) {
			conn, _, err := w.(http.Hijacker).Hijack()
			if assert.NoError(t, err) {
				conn.Close()
			}
		})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, newAIGuardRequest("POST", "/api/v2/ai-guard/evaluate", "{}"))
		assertAIGuardProxyError(t, rec, http.StatusBadGateway, "network_error")
	})
}

func TestAIGuardProxyRegistration(t *testing.T) {
	for _, tc := range []struct {
		name, apiKey, appKey string
		registered           bool
	}{
		{name: "both keys", apiKey: "api_key", appKey: "app_key", registered: true},
		{name: "no application key", apiKey: "api_key"},
		{name: "no api key", appKey: "app_key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conf := aiGuardTestConfig()
			conf.Endpoints[0].APIKey = tc.apiKey
			conf.EVPProxy.ApplicationKey = tc.appKey
			mux := newTestReceiverFromConfig(conf).buildMux()

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest("GET", "/info", nil))
			var info struct {
				Endpoints []string `json:"endpoints"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
			assert.Equal(t, tc.registered, slices.Contains(info.Endpoints, "/ai_guard/proxy/"))
			assert.NotContains(t, rec.Body.String(), "app_key")

			if !tc.registered {
				rec = httptest.NewRecorder()
				mux.ServeHTTP(rec, newAIGuardRequest("POST", "/api/v2/ai-guard/evaluate", "{}"))
				assert.Equal(t, http.StatusNotFound, rec.Code)
				// SDKs go back to discovery on a 404 without this header.
				assert.Empty(t, rec.Header().Get("Datadog-Agent-Version"))
			}
		})
	}
}
