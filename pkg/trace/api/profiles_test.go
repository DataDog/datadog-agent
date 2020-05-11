package api

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func mockConfig(k, v string) func() {
	oldConfig := config.Datadog
	config.Mock().Set(k, v)
	return func() { config.Datadog = oldConfig }
}

func TestProfileProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		slurp, err := ioutil.ReadAll(req.Body)
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
	newProfileProxy(u, "123", "key:val").ServeHTTP(rec, req)
	slurp, err := ioutil.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(slurp) != "OK" {
		t.Fatal("did not proxy")
	}
}

func TestProfilingEndpoint(t *testing.T) {
	t.Run("dd_url", func(t *testing.T) {
		defer mockConfig("apm_config.profiling_dd_url", "https://intake.profile.datadoghq.fr/v1/input")()
		if v := profilingEndpoint(); v != "https://intake.profile.datadoghq.fr/v1/input" {
			t.Fatalf("invalid endpoint: %s", v)
		}
	})

	t.Run("site", func(t *testing.T) {
		defer mockConfig("site", "datadoghq.eu")()
		if v := profilingEndpoint(); v != "https://intake.profile.datadoghq.eu/v1/input" {
			t.Fatalf("invalid endpoint: %s", v)
		}
	})

	t.Run("default", func(t *testing.T) {
		if v := profilingEndpoint(); v != "https://intake.profile.datadoghq.com/v1/input" {
			t.Fatalf("invalid endpoint: %s", v)
		}
	})
}

func TestProfileProxyHandler(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if v := req.Header.Get("X-Datadog-Additional-Tags"); v != "host:myhost,default_env:none" {
				t.Fatalf("invalid X-Datadog-Additional-Tags header: %q", v)
			}
			called = true
		}))
		defer mockConfig("apm_config.profiling_dd_url", srv.URL)()
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		conf := newTestReceiverConfig()
		conf.Hostname = "myhost"
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("error", func(t *testing.T) {
		defer mockConfig("site", "asd:\r\n")()
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		r := newTestReceiverFromConfig(newTestReceiverConfig())
		r.profileProxyHandler().ServeHTTP(rec, req)
		res := rec.Result()
		if res.StatusCode != http.StatusInternalServerError {
			t.Fatalf("invalid response: %s", res.Status)
		}
		slurp, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(slurp), "misconfigured") {
			t.Fatalf("invalid message: %q", slurp)
		}
	})
}
