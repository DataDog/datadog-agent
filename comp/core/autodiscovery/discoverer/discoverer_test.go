// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"context"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBridge struct {
	calls   int
	respond func(integrationName, serviceJSON string) (string, error)
}

func (f *fakeBridge) RunDiscover(integrationName, serviceJSON string) (string, error) {
	f.calls++
	return f.respond(integrationName, serviceJSON)
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
	bridge := &fakeBridge{respond: func(name, _ string) (string, error) {
		require.Equal(t, "krakend", name)
		return `[{"openmetrics_endpoint": "http://10.0.0.1:9090/metrics"}]`, nil
	}}
	d := newDiscoverer(bridge)
	r, ok := d.Discover(context.Background(), "krakend", newFakeService())
	require.True(t, ok)
	require.Len(t, r.Configs, 1)
	assert.Equal(t, "krakend", r.Configs[0].Name)
	assert.Contains(t, string(r.Configs[0].Instances[0]), "10.0.0.1")
	assert.Contains(t, string(r.Configs[0].Instances[0]), "9090")
}

func TestDiscoverNullResultNoMatch(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, string) (string, error) { return "null", nil }}
	d := newDiscoverer(bridge)
	_, ok := d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, ok)
}

func TestDiscoverEmptyListNoMatch(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, string) (string, error) { return "[]", nil }}
	d := newDiscoverer(bridge)
	_, ok := d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, ok)
}

func TestDiscoverErrorIsFailureCached(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, string) (string, error) {
		return "", errors.New("python blew up")
	}}
	d := newDiscoverer(bridge)
	_, ok := d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, ok)
	_, ok = d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, ok)
	assert.Equal(t, 1, bridge.calls, "negative cache should prevent re-invocation")
}

func TestDiscoverSuccessCached(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, string) (string, error) {
		return `[{"openmetrics_endpoint":"x"}]`, nil
	}}
	d := newDiscoverer(bridge)
	d.Discover(context.Background(), "krakend", newFakeService())
	d.Discover(context.Background(), "krakend", newFakeService())
	assert.Equal(t, 1, bridge.calls, "successful result should be cached")
}

func TestDiscoverServiceJSONFormat(t *testing.T) {
	var captured string
	bridge := &fakeBridge{respond: func(_, j string) (string, error) {
		captured = j
		return "null", nil
	}}
	d := newDiscoverer(bridge)
	d.Discover(context.Background(), "krakend", newFakeService())
	assert.Contains(t, captured, `"id":"docker://abc"`)
	assert.Contains(t, captured, `"host":"10.0.0.1"`)
	assert.Contains(t, captured, `"number":9090`)
}
