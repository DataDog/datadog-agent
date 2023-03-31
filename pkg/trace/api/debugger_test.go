// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/atomic"

	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
)

func TestDebuggerProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		slurp, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if body := string(slurp); body != "body" {
			t.Fatalf("invalid request body: %q", body)
		}
		if v := req.Header.Get("DD-API-KEY"); v != "123" {
			t.Fatalf("got invalid API key: %q", v)
		}
		if query := req.URL.RawQuery; query != "ddtags=key%3Aval" {
			t.Fatalf("got invalid query params: %q", query)
		}
		_, err = w.Write([]byte("OK"))
		if err != nil {
			t.Fatal(err)
		}
	}))
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("POST", "dummy.com/path", strings.NewReader("body"))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c := &traceconfig.AgentConfig{}
	newDebuggerProxy(c, []*url.URL{u}, []string{"123"}, "key:val").ServeHTTP(rec, req)
	result := rec.Result()
	slurp, err := io.ReadAll(result.Body)
	result.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(slurp) != "OK" {
		t.Fatal("did not proxy")
	}
}

func TestDebuggerProxyHandler(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ddtags := req.URL.Query().Get("ddtags")
			if strings.Contains(ddtags, "orchestrator") {
				t.Fatalf("ddtags should not contain orchestrator: %v", ddtags)
			}
			if strings.Contains(ddtags, "orchestrator") {
				t.Fatalf("ddtags should not contain orchestrator: %v", ddtags)
			}
			for _, tag := range []string{"host", "default_env", "agent_version"} {
				if !strings.Contains(ddtags, tag) {
					t.Fatalf("ddtags should contain %s", tag)
				}
			}
			called = true
		}))
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		conf := getConf()
		conf.Hostname = "myhost"
		conf.DebuggerProxy.DDURL = srv.URL
		receiver := newTestReceiverFromConfig(conf)
		receiver.debuggerProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("ok_fargate", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ddtags := req.URL.Query().Get("ddtags")
			if !strings.Contains(ddtags, "orchestrator:fargate_orchestrator") {
				t.Fatalf("invalid ddtags, fargate env should contain '%s' tag: %q", "orchestrator", ddtags)
			}
			called = true
		}))
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		conf := newTestReceiverConfig()
		conf.Hostname = "myhost"
		conf.FargateOrchestrator = "orchestrator"
		conf.DebuggerProxy.DDURL = srv.URL
		receiver := newTestReceiverFromConfig(conf)
		receiver.debuggerProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("error", func(t *testing.T) {
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		conf := newTestReceiverConfig()
		conf.Site = "asd:\r\n"
		r := newTestReceiverFromConfig(conf)
		r.debuggerProxyHandler().ServeHTTP(rec, req)
		res := rec.Result()
		if res.StatusCode != http.StatusInternalServerError {
			t.Fatalf("invalid response: %s", res.Status)
		}
		slurp, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(slurp), "error parsing debugger intake URL") {
			t.Fatalf("invalid message: %q", slurp)
		}
	})

	t.Run("ok_additional_endpoints", func(t *testing.T) {
		numEndpoints := 10
		var numCalls atomic.Int32
		conf := getConf()
		for i := 0; i < numEndpoints; i++ {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ddtags := req.URL.Query().Get("ddtags")
				for _, tag := range []string{"host", "default_env", "agent_version"} {
					if !strings.Contains(ddtags, tag) {
						t.Fatalf("ddtags should contain %s", tag)
					}
				}
				numCalls.Add(1)
			}))
			if i == 0 {
				conf.DebuggerProxy.DDURL = srv.URL
				continue
			}
			conf.DebuggerProxy.AdditionalEndpoints[srv.URL] = []string{"foo"}
		}
		req, err := http.NewRequest("POST", "/some/path", strings.NewReader("body"))
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(conf)
		receiver.debuggerProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		var expected atomic.Int32
		expected.Store(int32(numEndpoints))
		if numCalls != expected {
			t.Fatalf("requests not proxied, expected %d calls, got %d", numEndpoints, numCalls)
		}
	})

	t.Run("error_additional_endpoints_main_endpoint_error", func(t *testing.T) {
		numEndpoints := 2
		conf := getConf()
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
			conf.DebuggerProxy.AdditionalEndpoints[srv.URL] = []string{"foo"}
		}
		req, err := http.NewRequest("POST", "/some/path", strings.NewReader("body"))
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(conf)
		recorder := httptest.NewRecorder()
		receiver.debuggerProxyHandler().ServeHTTP(recorder, req)
		result := recorder.Result()
		result.Body.Close()
		assert.Equal(t, 500, result.StatusCode)
	})

	t.Run("ok_additional_endpoints_main_endpoint_ok_additional_error", func(t *testing.T) {
		numEndpoints := 10
		conf := getConf()
		for i := 0; i < numEndpoints; i++ {
			var srv *httptest.Server

			if i == 0 {
				srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(200)
				}))
				conf.DebuggerProxy.DDURL = srv.URL
				continue
			}

			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(500)
			}))
			conf.DebuggerProxy.AdditionalEndpoints[srv.URL] = []string{"foo"}
		}
		req, err := http.NewRequest("POST", "/some/path", strings.NewReader("body"))
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(conf)
		recorder := httptest.NewRecorder()
		receiver.debuggerProxyHandler().ServeHTTP(recorder, req)
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
