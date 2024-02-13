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
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/config"

	"github.com/DataDog/datadog-go/v5/statsd"
)

func TestPipelineStatsProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		slurp, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if body := string(slurp); body != "body" {
			t.Fatalf("invalid request body: %q", body)
		}
		if v := req.Header.Get("DD-API-KEY"); v != "123" {
			t.Fatalf("got invalid API key: %q", v)
		}
		if v := req.Header.Get("X-Datadog-Additional-Tags"); v != "key:val" {
			t.Fatalf("got invalid X-Datadog-Additional-Tags: %q", v)
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
	c := &config.AgentConfig{}
	newPipelineStatsProxy(c, []*url.URL{u}, []string{"123"}, "key:val", &statsd.NoOpClient{}).ServeHTTP(rec, req)
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

func TestPipelineStatsEndpoint(t *testing.T) {
	var cfg config.AgentConfig
	cfg.Endpoints = []*config.Endpoint{
		{Host: "https://trace.agent.datadoghq.com", APIKey: "test_api_key"},
		{Host: "https://trace.agent.datadoghq.eu", APIKey: "test_api_key_2"},
	}
	urls, keys, err := pipelineStatsEndpoints(&cfg)
	assert.NoError(t, err)
	assert.Equal(t, urls[0].String(), "https://trace.agent.datadoghq.com/api/v0.1/pipeline_stats")
	assert.Equal(t, urls[1].String(), "https://trace.agent.datadoghq.eu/api/v0.1/pipeline_stats")
	assert.Equal(t, keys, []string{"test_api_key", "test_api_key_2"})

	cfg.Endpoints = []*config.Endpoint{{Host: "trace.agent.datadoghq.com", APIKey: "test_api_key"}}
	urls, keys, err = pipelineStatsEndpoints(&cfg)
	assert.NoError(t, err)
	assert.Equal(t, urls[0].String(), "trace.agent.datadoghq.com/api/v0.1/pipeline_stats")
	assert.Equal(t, keys[0], "test_api_key")
}

func TestPipelineStatsProxyHandler(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			v := req.Header.Get("X-Datadog-Additional-Tags")
			tags := strings.Split(v, ",")
			m := make(map[string]string)
			for _, tag := range tags {
				kv := strings.Split(tag, ":")
				if strings.Contains(kv[0], "orchestrator") {
					t.Fatalf("non-fargate environment shouldn't contain '%s' tag : %q", kv[0], v)
				}
				m[kv[0]] = kv[1]
			}
			for _, tag := range []string{"host", "default_env", "agent_version"} {
				if _, ok := m[tag]; !ok {
					t.Fatalf("invalid X-Datadog-Additional-Tags header, should contain '%s': %q", tag, v)
				}
			}
			called = true
		}))
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		conf := newTestReceiverConfig()
		conf.Endpoints[0].Host = srv.URL
		fmt.Println("srv url", srv.URL)
		receiver := newTestReceiverFromConfig(conf)
		receiver.pipelineStatsProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("proxy_code", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
		conf := newTestReceiverConfig()
		conf.Endpoints[0].Host = srv.URL
		req, _ := http.NewRequest("POST", "/some/path", nil)
		receiver := newTestReceiverFromConfig(conf)
		rec := httptest.NewRecorder()
		receiver.pipelineStatsProxyHandler().ServeHTTP(rec, req)
		resp := rec.Result()
		assert.Equal(t, http.StatusAccepted, resp.StatusCode)
		resp.Body.Close()
	})

	t.Run("ok_fargate", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			v := req.Header.Get("X-Datadog-Additional-Tags")
			if !strings.Contains(v, "orchestrator:fargate_orchestrator") {
				t.Fatalf("invalid X-Datadog-Additional-Tags header, fargate env should contain '%s' tag: %q", "orchestrator", v)
			}
			called = true
		}))
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		conf := newTestReceiverConfig()
		conf.Endpoints[0].Host = srv.URL
		conf.FargateOrchestrator = "orchestrator"
		receiver := newTestReceiverFromConfig(conf)
		receiver.pipelineStatsProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
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
		conf.Endpoints[0].Host = ""
		conf.Site = "asd:\r\n"
		r := newTestReceiverFromConfig(conf)
		r.pipelineStatsProxyHandler().ServeHTTP(rec, req)
		res := rec.Result()
		if res.StatusCode != http.StatusInternalServerError {
			t.Fatalf("invalid response: %s", res.Status)
		}
		res.Body.Close()
		slurp, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(slurp), "Pipeline stats forwarder is OFF") {
			t.Fatalf("invalid message: %q", slurp)
		}
	})
}
