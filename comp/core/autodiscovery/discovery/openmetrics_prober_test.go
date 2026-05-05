// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discovery

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestVerifyOpenMetricsResponse(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		contentType string
		body        string
		want        bool
	}{
		{"prom-text", 200, "text/plain; version=0.0.4", "go_goroutines 5\n", true},
		{"openmetrics-text", 200, "application/openmetrics-text; version=1.0.0", "go_goroutines 5\n", true},
		{"json", 200, "application/json", `{"a":1}`, false},
		{"html", 200, "text/html", "<html></html>", false},
		{"401", 401, "text/plain", "go_goroutines 5\n", false},
		{"prom-no-line", 200, "text/plain", "# HELP only\n# TYPE only\n", false},
		{"prom-with-labels", 200, "text/plain", `http_requests_total{code="200"} 1027` + "\n", true},
		{"prom-with-comments-first", 200, "text/plain", "# HELP foo bar\n# TYPE foo counter\nfoo 1\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := verifyOpenMetricsResponse(tc.status, tc.contentType, []byte(tc.body)); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

// fakeService implements listeners.Service minimally for prober tests.
type fakeService struct {
	id    string
	hosts map[string]string
	ports []workloadmeta.ContainerPort
}

func (f *fakeService) GetServiceID() string                 { return f.id }
func (f *fakeService) GetADIdentifiers() []string           { return []string{"krakend"} }
func (f *fakeService) GetHosts() (map[string]string, error) { return f.hosts, nil }
func (f *fakeService) GetPorts() ([]workloadmeta.ContainerPort, error) {
	return f.ports, nil
}
func (f *fakeService) GetTags() ([]string, error)                      { return nil, nil }
func (f *fakeService) GetTagsWithCardinality(string) ([]string, error) { return nil, nil }
func (f *fakeService) GetPid() (int, error)                            { return 0, nil }
func (f *fakeService) GetHostname() (string, error)                    { return "", nil }
func (f *fakeService) IsReady() bool                                   { return true }
func (f *fakeService) HasFilter(workloadfilter.Scope) bool             { return false }
func (f *fakeService) GetExtraConfig(key string) (string, error) {
	return "", fmt.Errorf("unknown extra config %q", key)
}
func (f *fakeService) FilterTemplates(map[string]integration.Config) {}
func (f *fakeService) GetImageName() string                          { return "krakend:test" }
func (f *fakeService) Equal(other listeners.Service) bool {
	return f.id == other.GetServiceID()
}

func TestProbe_HintMatchesFirst(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte("go_goroutines 5\n"))
	}))
	defer good.Close()

	badHost, badPortStr, _ := net.SplitHostPort(bad.Listener.Addr().String())
	goodHost, goodPortStr, _ := net.SplitHostPort(good.Listener.Addr().String())
	badPort, _ := strconv.Atoi(badPortStr)
	goodPort, _ := strconv.Atoi(goodPortStr)
	if badHost != goodHost {
		t.Fatalf("test assumption: both servers on same host (got %s, %s)", badHost, goodHost)
	}

	svc := &fakeService{
		id:    "container_id://abc",
		hosts: map[string]string{"bridge": badHost},
		ports: []workloadmeta.ContainerPort{{Port: badPort}, {Port: goodPort}},
	}
	cfg := &integration.DiscoveryConfig{
		Type:  "openmetrics",
		Ports: []int{goodPort},
		Path:  "/metrics",
	}

	p := NewOpenMetricsProber(WithFailureTTL(time.Second))
	r, ok := p.Probe(context.Background(), cfg, svc)
	if !ok {
		t.Fatal("expected probe success")
	}
	if int(r.Port) != goodPort {
		t.Fatalf("port: got %d want %d", r.Port, goodPort)
	}
}

func TestProbe_AllFailReturnsFalse(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
	}))
	defer bad.Close()
	host, portStr, _ := net.SplitHostPort(bad.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	svc := &fakeService{
		id:    "container_id://xyz",
		hosts: map[string]string{"bridge": host},
		ports: []workloadmeta.ContainerPort{{Port: port}},
	}
	cfg := &integration.DiscoveryConfig{Type: "openmetrics", Path: "/metrics"}

	p := NewOpenMetricsProber(WithFailureTTL(time.Second))
	if _, ok := p.Probe(context.Background(), cfg, svc); ok {
		t.Fatal("expected probe failure")
	}
}
