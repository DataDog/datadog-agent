// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	discv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	discv1listers "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
)

type mockTemplateStore struct {
	mu        sync.RWMutex
	templates map[string][]integration.Config
	onChange  func(namespace, name string)
}

func newMockTemplateStore(templates map[string][]integration.Config) *mockTemplateStore {
	if templates == nil {
		templates = make(map[string][]integration.Config)
	}
	return &mockTemplateStore{templates: templates}
}

func (m *mockTemplateStore) NotifyOnChange(fn func(namespace, name string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = fn
}

func (m *mockTemplateStore) HasService(namespace, name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, found := m.templates[namespace+"/"+name]
	return found
}

func (m *mockTemplateStore) AllTemplatesByService() map[string][]integration.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string][]integration.Config, len(m.templates))
	for k, v := range m.templates {
		out[k] = v
	}
	return out
}

func strPtr(s string) *string { return &s }

func newTestLister(slices ...*discv1.EndpointSlice) discv1listers.EndpointSliceLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	for _, s := range slices {
		_ = indexer.Add(s)
	}
	return discv1listers.NewEndpointSliceLister(indexer)
}

func newTestProvider(store serviceTracker, slices ...*discv1.EndpointSlice) *KubeEndpointSlicesCRConfigProvider {
	return &KubeEndpointSlicesCRConfigProvider{
		serviceTracker: store,
		epSliceLister:  newTestLister(slices...),
	}
}

func makeTestSlice(namespace, serviceName, uid string, endpoints []discv1.Endpoint) *discv1.EndpointSlice {
	return &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName + "-" + uid,
			Namespace: namespace,
			UID:       types.UID(uid),
			Labels: map[string]string{
				kubernetesServiceNameLabelProvider: serviceName,
			},
		},
		Endpoints: endpoints,
	}
}

func TestCRProvider_IsUpToDate(t *testing.T) {
	p := newTestProvider(newMockTemplateStore(nil))
	p.upToDate = true

	upToDate, err := p.IsUpToDate(context.Background())
	require.NoError(t, err)
	assert.True(t, upToDate)

	p.setUpToDate(false)
	upToDate, err = p.IsUpToDate(context.Background())
	require.NoError(t, err)
	assert.False(t, upToDate)
}

func TestCRProvider_Collect(t *testing.T) {
	tests := []struct {
		name                 string
		templates            map[string][]integration.Config
		slices               []*discv1.EndpointSlice
		wantNames            []string // expected config.Name for every generated config
		wantUniqueServiceIDs int      // distinct ServiceIDs across the generated configs
	}{
		{
			name: "one template, slice with two endpoints: a config per endpoint",
			templates: map[string][]integration.Config{
				"default/my-svc": {{
					Name:       "redisdb",
					InitConfig: integration.Data("{}"),
					Instances:  []integration.Data{integration.Data(`{"host":"%%host%%"}`)},
					Source:     "datadoginstrumentation:default/cr1",
				}},
			},
			slices: []*discv1.EndpointSlice{
				makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}, NodeName: strPtr("node-1")},
					{Addresses: []string{"10.0.0.2"}, NodeName: strPtr("node-2")},
				}),
			},
			wantNames:            []string{"redisdb", "redisdb"},
			wantUniqueServiceIDs: 2, // one per endpoint IP
		},
		{
			name: "multiple templates, one endpoint: a config per template",
			templates: map[string][]integration.Config{
				"default/my-svc": {
					{Name: "redisdb", InitConfig: integration.Data("{}"), Instances: []integration.Data{integration.Data(`{}`)}},
					{Name: "http_check", InitConfig: integration.Data("{}"), Instances: []integration.Data{integration.Data(`{}`)}},
				},
			},
			slices: []*discv1.EndpointSlice{
				makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}, NodeName: strPtr("node-1")},
				}),
			},
			wantNames:            []string{"redisdb", "http_check"},
			wantUniqueServiceIDs: 1, // both checks target the same endpoint
		},
		{
			name: "templates but no slices: nothing generated",
			templates: map[string][]integration.Config{
				"default/my-svc": {{Name: "redisdb"}},
			},
		},
		{
			name: "slices but no templates: nothing generated",
			slices: []*discv1.EndpointSlice{
				makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
				}),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestProvider(newMockTemplateStore(tc.templates), tc.slices...)

			configs, err := p.Collect(context.Background())
			require.NoError(t, err)

			gotNames := make([]string, 0, len(configs))
			serviceIDs := make(map[string]struct{})
			for _, cfg := range configs {
				gotNames = append(gotNames, cfg.Name)
				serviceIDs[cfg.ServiceID] = struct{}{}
				assert.True(t, cfg.ClusterCheck, "endpoint configs must be cluster checks")
				assert.Equal(t, names.KubeEndpointSlicesCR, cfg.Provider)
				assert.NotEmpty(t, cfg.ServiceID)
				assert.NotEmpty(t, cfg.ADIdentifiers)
			}
			assert.ElementsMatch(t, tc.wantNames, gotNames)
			assert.Len(t, serviceIDs, tc.wantUniqueServiceIDs)
		})
	}
}
