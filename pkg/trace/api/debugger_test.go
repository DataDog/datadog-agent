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

	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
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
	newDebuggerProxy(c, u, "123", "key:val").ServeHTTP(rec, req)
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
		conf := newTestReceiverConfig()
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
}
