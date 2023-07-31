// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
)

func TestDebuggerProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		slurp, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		err = req.Body.Close()
		assert.NoError(t, err)
		body := string(slurp)
		ddtags := req.URL.Query().Get("ddtags")
		assert.Equal(t, "body", string(slurp), "invalid request body: %q", body)
		assert.Equal(t, "test", req.Header.Get("DD-API-KEY"), "got invalid API key: %q", req.Header.Get("DD-API-KEY"))
		assert.Equal(t, "host:myhost,default_env:test,agent_version:v1", ddtags, "got unexpected tags")
		_, err = w.Write([]byte("OK"))
		assert.NoError(t, err)
	}))
	defer srv.Close()
	req, err := http.NewRequest("POST", "dummy.com/path", strings.NewReader("body"))
	assert.NoError(t, err)
	rec := httptest.NewRecorder()
	conf := getConf()
	conf.Hostname = "myhost"
	conf.DebuggerProxy.DDURL = srv.URL
	receiver := newTestReceiverFromConfig(conf)
	receiver.debuggerLogsProxyHandler().ServeHTTP(rec, req)
	result := rec.Result()
	slurp, err := io.ReadAll(result.Body)
	result.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "OK", string(slurp), "did not proxy")
}

func TestDebuggerProxyHandler(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ddtags := req.URL.Query().Get("ddtags")
			assert.False(t, strings.Contains(ddtags, "orchestrator"), "ddtags should not contain orchestrator: %v", ddtags)
			assert.Equal(t, "host:myhost,default_env:test,agent_version:v1", ddtags)
			called = true
		}))
		defer srv.Close()
		req, err := http.NewRequest("POST", "/some/path", nil)
		assert.NoError(t, err)
		conf := getConf()
		conf.Hostname = "myhost"
		conf.DebuggerProxy.DDURL = srv.URL
		receiver := newTestReceiverFromConfig(conf)
		receiver.debuggerLogsProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		assert.True(t, called, "request not proxied")
	})

	t.Run("ok_diagnostics_proxy", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ddtags := req.URL.Query().Get("ddtags")
			assert.False(t, strings.Contains(ddtags, "orchestrator"), "ddtags should not contain orchestrator: %v", ddtags)
			assert.Equal(t, "host:myhost,default_env:test,agent_version:v1", ddtags)
			called = true
		}))
		defer srv.Close()
		req, err := http.NewRequest("POST", "/some/path", nil)
		assert.NoError(t, err)
		conf := getConf()
		conf.Hostname = "myhost"
		conf.DebuggerDiagnosticsProxy.DDURL = srv.URL
		receiver := newTestReceiverFromConfig(conf)
		receiver.debuggerDiagnosticsProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		assert.True(t, called, "request not proxied")
	})

	t.Run("ok_truncate_ddtags", func(t *testing.T) {
		tooLongString := strings.Repeat("x", 10000)
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ddtags := req.URL.Query().Get("ddtags")
			count := strings.Count(ddtags, "x")
			expected := 4000 - len("host:myhost,default_env:test,agent_version:v1")
			assert.Equal(t, expected, count, "expected %d x's, got: %d", expected, count)
			called = true
		}))
		defer srv.Close()
		req, err := http.NewRequest("POST", fmt.Sprintf("/some/path?ddtags=%s", tooLongString), nil)
		assert.NoError(t, err)
		conf := getConf()
		conf.Hostname = "myhost"
		conf.DebuggerProxy.DDURL = srv.URL
		receiver := newTestReceiverFromConfig(conf)
		receiver.debuggerLogsProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		assert.True(t, called, "request not proxied")
	})

	t.Run("ok_multiple_requests", func(t *testing.T) {
		extraTag := "a:b"
		var numCalls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ddtags := req.URL.Query().Get("ddtags")
			assert.False(t, strings.Contains(ddtags, "orchestrator"), "ddtags should not contain orchestrator: %v", ddtags)
			assert.Equal(t, fmt.Sprintf("host:myhost,default_env:test,agent_version:v1,%s", extraTag), ddtags)
			numCalls.Add(1)
		}))
		defer srv.Close()
		req, err := http.NewRequest("POST", fmt.Sprintf("/some/path?ddtags=%s", extraTag), nil)
		assert.NoError(t, err)
		conf := getConf()
		conf.Hostname = "myhost"
		conf.DebuggerProxy.DDURL = srv.URL
		receiver := newTestReceiverFromConfig(conf)
		handler := receiver.debuggerLogsProxyHandler()
		handler.ServeHTTP(httptest.NewRecorder(), req)
		handler.ServeHTTP(httptest.NewRecorder(), req)
		var expected atomic.Int32
		expected.Store(int32(2))
		assert.Equal(t, expected, numCalls, "requests not proxied, expected %d calls, got %d", 2, numCalls)
	})

	t.Run("ok_fargate", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ddtags := req.URL.Query().Get("ddtags")
			assert.True(t, strings.Contains(ddtags, "orchestrator"), "ddtags must contain orchestrator: %v", ddtags)
			called = true
		}))
		defer srv.Close()
		req, err := http.NewRequest("POST", "/some/path", nil)
		assert.NoError(t, err)
		conf := newTestReceiverConfig()
		conf.Hostname = "myhost"
		conf.FargateOrchestrator = "orchestrator"
		conf.DebuggerProxy.DDURL = srv.URL
		receiver := newTestReceiverFromConfig(conf)
		receiver.debuggerLogsProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		assert.True(t, called, "request not proxied")
	})

	t.Run("error", func(t *testing.T) {
		req, err := http.NewRequest("POST", "/some/path", nil)
		assert.NoError(t, err)
		rec := httptest.NewRecorder()
		conf := newTestReceiverConfig()
		conf.Site = "asd:\r\n"
		r := newTestReceiverFromConfig(conf)
		r.debuggerLogsProxyHandler().ServeHTTP(rec, req)
		res := rec.Result()
		assert.Equal(t, http.StatusInternalServerError, res.StatusCode, "invalid response: %s", res.Status)
		slurp, err := io.ReadAll(res.Body)
		res.Body.Close()
		assert.NoError(t, err)
		assert.Contains(t, string(slurp), "error parsing debugger intake URL", "invalid message: %q", string(slurp))
	})

	t.Run("ok_additional_endpoints", func(t *testing.T) {
		numEndpoints := 10
		var numCalls atomic.Int32
		conf := getConf()
		srvs := make([]*httptest.Server, 0, numEndpoints)
		for i := 0; i < numEndpoints; i++ {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ddtags := req.URL.Query().Get("ddtags")
				assert.Equal(t, "host:myhost,default_env:test,agent_version:v1", ddtags, "got unexpected additional tags")
				numCalls.Add(1)
			}))
			srvs = append(srvs, srv)
			if i == 0 {
				conf.DebuggerProxy.DDURL = srv.URL
				continue
			}
			conf.DebuggerProxy.AdditionalEndpoints[srv.URL] = []string{"foo"}
		}
		defer func() {
			for _, srv := range srvs {
				srv.Close()
			}
		}()
		req, err := http.NewRequest("POST", "/some/path", strings.NewReader("body"))
		assert.NoError(t, err)
		receiver := newTestReceiverFromConfig(conf)
		receiver.debuggerLogsProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		var expected atomic.Int32
		expected.Store(int32(numEndpoints))
		assert.Equal(t, expected, numCalls, "requests not proxied, expected %d calls, got %d", numEndpoints, numCalls)
	})

	t.Run("error_additional_endpoints_main_endpoint_error", func(t *testing.T) {
		numEndpoints := 2
		conf := getConf()
		srvs := make([]*httptest.Server, 0, numEndpoints)
		for i := 0; i < numEndpoints; i++ {
			var srv *httptest.Server

			if i == 0 {
				srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(500)
				}))
				conf.DebuggerProxy.DDURL = srv.URL
				continue
			}

			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(200)
			}))
			srvs = append(srvs, srv)
			conf.DebuggerProxy.AdditionalEndpoints[srv.URL] = []string{"foo"}
		}
		defer func() {
			for _, srv := range srvs {
				srv.Close()
			}
		}()
		req, err := http.NewRequest("POST", "/some/path", strings.NewReader("body"))
		assert.NoError(t, err)
		receiver := newTestReceiverFromConfig(conf)
		recorder := httptest.NewRecorder()
		receiver.debuggerLogsProxyHandler().ServeHTTP(recorder, req)
		result := recorder.Result()
		result.Body.Close()
		assert.Equal(t, 500, result.StatusCode)
	})

	t.Run("ok_additional_endpoints_main_endpoint_ok_additional_error", func(t *testing.T) {
		numEndpoints := 10
		conf := getConf()
		srvs := make([]*httptest.Server, 0, numEndpoints)
		for i := 0; i < numEndpoints; i++ {
			var srv *httptest.Server

			if i == 0 {
				srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(200)
				}))
				srvs = append(srvs, srv)
				conf.DebuggerProxy.DDURL = srv.URL
				continue
			}

			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(500)
			}))
			srvs = append(srvs, srv)
			conf.DebuggerProxy.AdditionalEndpoints[srv.URL] = []string{"foo"}
		}
		defer func() {
			for _, srv := range srvs {
				srv.Close()
			}
		}()
		req, err := http.NewRequest("POST", "/some/path", strings.NewReader("body"))
		assert.NoError(t, err)
		receiver := newTestReceiverFromConfig(conf)
		recorder := httptest.NewRecorder()
		receiver.debuggerLogsProxyHandler().ServeHTTP(recorder, req)
		result := recorder.Result()
		result.Body.Close()
		assert.Equal(t, 200, result.StatusCode)
	})
}

func getConf() *traceconfig.AgentConfig {
	conf := newTestReceiverConfig()
	conf.DebuggerProxy.AdditionalEndpoints = make(map[string][]string)
	conf.DefaultEnv = "test"
	conf.Hostname = "myhost"
	conf.AgentVersion = "v1"
	return conf
}
