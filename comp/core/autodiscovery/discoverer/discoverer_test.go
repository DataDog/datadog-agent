// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBridge struct {
	calls   int
	respond func(integrationName string, service python.DiscoveryService) ([]integration.Data, error)
}

func (f *fakeBridge) DiscoverConfig(integrationName string, service python.DiscoveryService) ([]integration.Data, error) {
	f.calls++
	return f.respond(integrationName, service)
}

type fakeService struct {
	id    string
	hosts map[string]string
	ports []workloadmeta.ContainerPort
}

func (s *fakeService) GetServiceID() string                            { return s.id }
func (s *fakeService) GetADIdentifiers() []string                      { return []string{"krakend"} }
func (s *fakeService) GetHosts() (map[string]string, error)            { return s.hosts, nil }
func (s *fakeService) GetTags() ([]string, error)                      { return nil, nil }
func (s *fakeService) GetTagsWithCardinality(string) ([]string, error) { return nil, nil }
func (s *fakeService) GetPid() (int, error)                            { return 0, nil }
func (s *fakeService) GetHostname() (string, error)                    { return "", nil }
func (s *fakeService) IsReady() bool                                   { return true }
func (s *fakeService) GetExtraConfig(string) (string, error)           { return "", nil }
func (s *fakeService) GetImageName() string                            { return "" }
func (s *fakeService) GetPorts() ([]workloadmeta.ContainerPort, error) { return s.ports, nil }
func (s *fakeService) HasFilter(workloadfilter.Scope) bool             { return false }
func (s *fakeService) FilterTemplates(map[string]integration.Config)   {}
func (s *fakeService) Equal(listeners.Service) bool                    { return false }

func newFakeService() *fakeService {
	return &fakeService{
		id:    "docker://abc",
		hosts: map[string]string{"bridge": "10.0.0.1"},
		ports: []workloadmeta.ContainerPort{{Port: 9090, Name: ""}},
	}
}

func TestDiscoverHappyPath(t *testing.T) {
	bridge := &fakeBridge{respond: func(name string, _ python.DiscoveryService) ([]integration.Data, error) {
		require.Equal(t, "krakend", name)
		return []integration.Data{integration.Data(`{"openmetrics_endpoint": "http://10.0.0.1:9090/metrics"}`)}, nil
	}}
	d := newDiscoverer(bridge)
	r, ok := d.Discover(context.Background(), "krakend", newFakeService())
	require.True(t, ok)
	require.Len(t, r.Configs, 1)
	assert.Equal(t, "krakend", r.Configs[0].Name)
	assert.Contains(t, string(r.Configs[0].Instances[0]), "10.0.0.1")
	assert.Contains(t, string(r.Configs[0].Instances[0]), "9090")
}

func TestDiscoverEmptyListNoMatch(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, python.DiscoveryService) ([]integration.Data, error) { return nil, nil }}
	d := newDiscoverer(bridge)
	_, ok := d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, ok)
}

func TestDiscoverErrorIsFailureCached(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, python.DiscoveryService) ([]integration.Data, error) {
		return nil, errors.New("python blew up")
	}}
	d := newDiscoverer(bridge)
	_, ok := d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, ok)
	_, ok = d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, ok)
	assert.Equal(t, 1, bridge.calls, "negative cache should prevent re-invocation")
}

func TestDiscoverSuccessCached(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, python.DiscoveryService) ([]integration.Data, error) {
		return []integration.Data{integration.Data(`{"openmetrics_endpoint":"x"}`)}, nil
	}}
	d := newDiscoverer(bridge)
	d.Discover(context.Background(), "krakend", newFakeService())
	d.Discover(context.Background(), "krakend", newFakeService())
	assert.Equal(t, 1, bridge.calls, "successful result should be cached")
}

func TestDiscoverServicePayload(t *testing.T) {
	var captured python.DiscoveryService
	bridge := &fakeBridge{respond: func(_ string, service python.DiscoveryService) ([]integration.Data, error) {
		captured = service
		return nil, nil
	}}
	d := newDiscoverer(bridge)
	d.Discover(context.Background(), "krakend", newFakeService())
	assert.Equal(t, "docker://abc", captured.ID)
	assert.Equal(t, "10.0.0.1", captured.Host)
	require.Len(t, captured.Ports, 1)
	assert.Equal(t, 9090, captured.Ports[0].Number)
}

func TestDiscoverGivenUpNeverProbesAgain(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, python.DiscoveryService) ([]integration.Data, error) {
		return nil, nil
	}}
	d := newDiscoverer(bridge)
	t0 := time.Now()
	tick := int64(0)
	d.now = func() time.Time { tick++; return t0.Add(time.Duration(tick) * time.Second) }
	d.retrySchedule = []time.Duration{0} // 1 retry, then give up

	d.Discover(context.Background(), "krakend", newFakeService()) // attempt 1: probes, fails, pending (nextRetryAt = now+0)
	d.Discover(context.Background(), "krakend", newFakeService()) // attempt 2: now > nextRetryAt → probes, fails, givenUp
	callsAtGiveUp := bridge.calls
	d.Discover(context.Background(), "krakend", newFakeService()) // givenUp: no probe
	assert.Equal(t, callsAtGiveUp, bridge.calls, "givenUp should suppress all future probes")
}

func TestDiscoverIsPendingAfterFailure(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, python.DiscoveryService) ([]integration.Data, error) {
		return nil, nil
	}}
	d := newDiscoverer(bridge)
	d.Discover(context.Background(), "krakend", newFakeService())
	assert.True(t, d.IsPending("docker://abc", "krakend"))
	assert.False(t, d.IsPending("docker://abc", "other-integration"))
	assert.False(t, d.IsPending("other-svc", "krakend"))
}

func TestDiscoverIsPendingFalseAfterGiveUp(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, python.DiscoveryService) ([]integration.Data, error) {
		return nil, nil
	}}
	d := newDiscoverer(bridge)
	t0 := time.Now()
	tick := int64(0)
	d.now = func() time.Time { tick++; return t0.Add(time.Duration(tick) * time.Second) }
	d.retrySchedule = []time.Duration{0} // 1 retry, then give up

	// 3 Discover calls: 1st → pending, 2nd → givenUp (now > nextRetryAt), 3rd → no-op
	d.Discover(context.Background(), "krakend", newFakeService())
	d.Discover(context.Background(), "krakend", newFakeService())
	d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, d.IsPending("docker://abc", "krakend"))
}

func TestDiscoverIsPendingFalseAfterSuccess(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, python.DiscoveryService) ([]integration.Data, error) {
		return []integration.Data{integration.Data(`{"openmetrics_endpoint":"x"}`)}, nil
	}}
	d := newDiscoverer(bridge)
	d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, d.IsPending("docker://abc", "krakend"))
}

func TestDiscoverForgetClearsEntries(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, python.DiscoveryService) ([]integration.Data, error) {
		return nil, nil
	}}
	d := newDiscoverer(bridge)
	d.Discover(context.Background(), "krakend", newFakeService())
	require.True(t, d.IsPending("docker://abc", "krakend"))

	d.Forget("docker://abc")
	assert.False(t, d.IsPending("docker://abc", "krakend"))
}

func TestDiscoverForgetNoop(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, python.DiscoveryService) ([]integration.Data, error) { return nil, nil }}
	d := newDiscoverer(bridge)
	d.Forget("never-seen") // must not panic / error
	assert.False(t, d.IsPending("never-seen", "krakend"))
}
