// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
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

func urlParse(t *testing.T, s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("Cannot parse url: %s", s)
	}
	return u
}

func TestProfileProxy(t *testing.T) {
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
	c := &config.AgentConfig{ContainerIDFromOriginInfo: config.NoopContainerIDFromOriginInfoFunc}
	newProfileProxy(c, []endpointDescriptor{{url: u, apiKey: "123"}}, "key:val", &statsd.NoOpClient{}).ServeHTTP(rec, req)
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

func TestProfilingEndpoints(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		cfg := config.New()
		cfg.ProfilingProxy.Endpoints[0].Host = "https://intake.profile.datadoghq.fr"
		cfg.ProfilingProxy.Endpoints[0].APIKey = "test_api_key"
		endpointDescriptors, err := profilingEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, endpointDescriptors, []endpointDescriptor{{url: urlParse(t, "https://intake.profile.datadoghq.fr/api/v2/profile"), apiKey: "test_api_key"}})
	})

	t.Run("default", func(t *testing.T) {
		cfg := config.New()
		cfg.ProfilingProxy.Endpoints[0].APIKey = "test_api_key"
		endpointDescriptors, err := profilingEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, endpointDescriptors, []endpointDescriptor{{url: urlParse(t, "https://intake.profile.datadoghq.com/api/v2/profile"), apiKey: "test_api_key"}})
	})

	t.Run("multiple", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "api_key_0"
		cfg.ProfilingProxy.Endpoints = []*config.Endpoint{
			{Host: "https://intake.profile.datadoghq.jp/api/v2/profile", APIKey: "api_key_0"},
			{Host: "https://ddstaging.datadoghq.com", APIKey: "api_key_1"},
			{Host: "https://ddstaging.datadoghq.com", APIKey: "api_key_2"},
			{Host: "https://dd.datad0g.com", APIKey: "api_key_3"},
		}
		endpointDescriptors, err := profilingEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, endpointDescriptors, []endpointDescriptor{
			{url: urlParse(t, "https://intake.profile.datadoghq.jp/api/v2/profile"), apiKey: "api_key_0"},
			{url: urlParse(t, "https://ddstaging.datadoghq.com/api/v2/profile"), apiKey: "api_key_1"},
			{url: urlParse(t, "https://ddstaging.datadoghq.com/api/v2/profile"), apiKey: "api_key_2"},
			{url: urlParse(t, "https://dd.datad0g.com"), apiKey: "api_key_3"},
		})
	})
}

func TestProfileProxyHandler(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
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
		conf.ProfilingProxy.Endpoints[0].Host = srv.URL
		conf.ProfilingProxy.Endpoints[0].APIKey = "test_api_key"
		conf.Hostname = "myhost"
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("proxy_code", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
		conf := newTestReceiverConfig()
		conf.ProfilingProxy.Endpoints[0].Host = srv.URL
		req, _ := http.NewRequest("POST", "/some/path", nil)
		receiver := newTestReceiverFromConfig(conf)
		rec := httptest.NewRecorder()
		receiver.profileProxyHandler().ServeHTTP(rec, req)
		resp := rec.Result()
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("ok_fargate", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			v := req.Header.Get("X-Datadog-Additional-Tags")
			if !strings.Contains(v, "orchestrator:fargate_orchestrator") {
				t.Fatalf("invalid X-Datadog-Additional-Tags header, fargate env should contain '%s' tag: %q", "orchestrator", v)
			}
			called = true
		}))
		conf := newTestReceiverConfig()
		conf.ProfilingProxy.Endpoints[0].Host = srv.URL
		conf.Hostname = "myhost"
		conf.FargateOrchestrator = "orchestrator"
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("error", func(t *testing.T) {
		conf := newTestReceiverConfig()
		conf.Site = "asd:\r\n"
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		r := newTestReceiverFromConfig(conf)
		r.profileProxyHandler().ServeHTTP(rec, req)
		res := rec.Result()
		if res.StatusCode != http.StatusInternalServerError {
			t.Fatalf("invalid response: %s", res.Status)
		}
		slurp, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(slurp), "error parsing main profiling intake URL") {
			t.Fatalf("invalid message: %q", slurp)
		}
	})

	t.Run("multiple_targets", func(t *testing.T) {
		called := make(map[string]bool)
		handler := func(_ http.ResponseWriter, req *http.Request) {
			called[fmt.Sprintf("http://%s|%s", req.Host, req.Header.Get("DD-API-KEY"))] = true
		}
		srv1 := httptest.NewServer(http.HandlerFunc(handler))
		srv2 := httptest.NewServer(http.HandlerFunc(handler))
		cfg := config.New()
		cfg.Hostname = "myhost"
		cfg.Endpoints[0].APIKey = "api_key_0"
		cfg.ProfilingProxy.Endpoints = []*config.Endpoint{
			{Host: srv1.URL, APIKey: "api_key_0"},
			{Host: srv2.URL, APIKey: "dummy_api_key_1"},
			{Host: srv2.URL, APIKey: "dummy_api_key_2"},
			{Host: "foobar", APIKey: "invalid"},
		}

		req, err := http.NewRequest("POST", "/some/path", bytes.NewBuffer([]byte("abc")))
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(cfg)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)

		expected := map[string]bool{
			srv1.URL + "|api_key_0":       true,
			srv2.URL + "|dummy_api_key_1": true,
			srv2.URL + "|dummy_api_key_2": true,
		}
		assert.Equal(t, expected, called, "The request should be proxied to all valid targets")
	})

	t.Run("lambda_function", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			v := req.Header.Get("X-Datadog-Additional-Tags")
			if !strings.Contains(v, "functionname:my-function-name") || !strings.Contains(v, "_dd.origin:lambda") {
				t.Fatalf("invalid X-Datadog-Additional-Tags header, fargate env should contain '%s' tag: %q", "orchestrator", v)
			}
			called = true
		}))
		conf := newTestReceiverConfig()
		conf.ProfilingProxy.Endpoints[0].Host = srv.URL
		conf.LambdaFunctionName = "my-function-name"
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("azure_container_app", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			v := req.Header.Get("X-Datadog-Additional-Tags")
			tags := strings.Split(v, ",")
			m := make(map[string]string)
			for _, tag := range tags {
				kv := strings.Split(tag, ":")
				m[kv[0]] = kv[1]
			}
			for _, tag := range []string{"subscription_id", "resource_group", "resource_id", "aca.subscription.id", "aca.resource.group", "aca.resource.id", "aca.replica.name"} {
				if _, ok := m[tag]; !ok {
					t.Fatalf("invalid X-Datadog-Additional-Tags header, should contain '%s': %q", tag, v)
				}
			}
			called = true
		}))
		conf := newTestReceiverConfig()
		conf.ProfilingProxy.Endpoints[0].Host = srv.URL
		conf.AzureContainerAppTags = ",subscription_id:123,resource_group:test-rg,resource_id:456,aca.subscription.id:123,aca.resource.group:test-rg,aca.resource.id:456,aca.replica.name:test-replica"
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})
}
