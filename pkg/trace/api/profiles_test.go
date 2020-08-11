package api

import (
	"fmt"
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
	newProfileProxy(nil, u, "123", "key:val").ServeHTTP(rec, req)
	slurp, err := ioutil.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(slurp) != "OK" {
		t.Fatal("did not proxy")
	}
}

func TestProfilingEndpoints(t *testing.T) {
	t.Run("dd_url", func(t *testing.T) {
		defer mockConfig("apm_config.profiling_dd_url", "https://intake.profile.datadoghq.fr/v1/input")()
		endpoints := profilingEndpoints()
		if len(endpoints) != 1 || endpoints[0] != "https://intake.profile.datadoghq.fr/v1/input" {
			t.Fatalf("invalid endpoints: %v", endpoints)
		}
	})

	t.Run("multiple dd_url", func(t *testing.T) {
		defer mockConfig("apm_config.profiling_dd_url",
			"https://intake.profile.datadoghq.fr/v1/input,https://intake.profile.datadoghq.com/v1/input")()
		endpoints := profilingEndpoints()
		if len(endpoints) != 2 || endpoints[0] != "https://intake.profile.datadoghq.fr/v1/input" ||
			endpoints[1] != "https://intake.profile.datadoghq.com/v1/input" {
			t.Fatalf("invalid endpoints: %v", endpoints)
		}
	})

	t.Run("site", func(t *testing.T) {
		defer mockConfig("site", "datadoghq.eu")()
		endpoints := profilingEndpoints()
		if len(endpoints) != 1 || endpoints[0] != "https://intake.profile.datadoghq.eu/v1/input" {
			t.Fatalf("invalid endpoints: %v", endpoints)
		}
	})

	t.Run("default", func(t *testing.T) {
		endpoints := profilingEndpoints()
		if len(endpoints) != 1 || endpoints[0] != "https://intake.profile.datadoghq.com/v1/input" {
			t.Fatalf("invalid endpoint: %v", endpoints)
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
		if !strings.Contains(string(slurp), "invalid intake URL") {
			t.Fatalf("invalid message: %q", slurp)
		}
	})

	t.Run("multiple", func(t *testing.T) {
		called := make(map[string]bool)
		handler := func(w http.ResponseWriter, req *http.Request) {
			called[req.Host] = true
		}
		srv1 := httptest.NewServer(http.HandlerFunc(handler))
		srv2 := httptest.NewServer(http.HandlerFunc(handler))
		defer mockConfig("apm_config.profiling_dd_url", fmt.Sprintf("%s,%s", srv1.URL, srv2.URL))()
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		conf := newTestReceiverConfig()
		conf.Hostname = "myhost"
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if len(called) != 2 {
			t.Fatalf("request not proxied to both targets %v", called)
		}
	})
}
