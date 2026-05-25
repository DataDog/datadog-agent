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
	onChange  func()
}

func newMockTemplateStore(templates map[string][]integration.Config) *mockTemplateStore {
	if templates == nil {
		templates = make(map[string][]integration.Config)
	}
	return &mockTemplateStore{templates: templates}
}

func (m *mockTemplateStore) SetOnChange(fn func()) {
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

func newTestProvider(store serviceTemplateStore, slices ...*discv1.EndpointSlice) *KubeEndpointSlicesCRConfigProvider {
	return &KubeEndpointSlicesCRConfigProvider{
		templateStore: store,
		epSliceLister: newTestLister(slices...),
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

func TestCRProvider_Collect_GeneratesEndpointConfigs(t *testing.T) {
	store := newMockTemplateStore(map[string][]integration.Config{
		"default/my-svc": {{
			Name:       "redisdb",
			InitConfig: integration.Data("{}"),
			Instances:  []integration.Data{integration.Data(`{"host":"%%host%%"}`)},
			Source:     "datadoginstrumentation:default/cr1",
		}},
	})
	slice := makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
		{Addresses: []string{"10.0.0.1"}, NodeName: strPtr("node-1")},
		{Addresses: []string{"10.0.0.2"}, NodeName: strPtr("node-2")},
	})
	p := newTestProvider(store, slice)

	configs, err := p.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 2)

	for _, cfg := range configs {
		assert.Equal(t, "redisdb", cfg.Name)
		assert.True(t, cfg.ClusterCheck)
		assert.Equal(t, names.KubeEndpointSlicesCR, cfg.Provider)
		assert.NotEmpty(t, cfg.ServiceID)
		assert.NotEmpty(t, cfg.ADIdentifiers)
	}
	assert.NotEqual(t, configs[0].ServiceID, configs[1].ServiceID)
}

func TestCRProvider_Collect_MultipleTemplatesPerService(t *testing.T) {
	store := newMockTemplateStore(map[string][]integration.Config{
		"default/my-svc": {
			{Name: "redisdb", InitConfig: integration.Data("{}"), Instances: []integration.Data{integration.Data(`{}`)}},
			{Name: "http_check", InitConfig: integration.Data("{}"), Instances: []integration.Data{integration.Data(`{}`)}},
		},
	})
	slice := makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
		{Addresses: []string{"10.0.0.1"}, NodeName: strPtr("node-1")},
	})
	p := newTestProvider(store, slice)

	configs, err := p.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 2)

	configNames := make([]string, 0, len(configs))
	for _, cfg := range configs {
		configNames = append(configNames, cfg.Name)
	}
	assert.ElementsMatch(t, []string{"redisdb", "http_check"}, configNames)
}

func TestCRProvider_Collect_NoSlicesReturnsEmpty(t *testing.T) {
	store := newMockTemplateStore(map[string][]integration.Config{
		"default/my-svc": {{Name: "redisdb"}},
	})
	p := newTestProvider(store) // no slices

	configs, err := p.Collect(context.Background())
	require.NoError(t, err)
	assert.Empty(t, configs)
}

func TestCRProvider_Collect_NoTemplatesReturnsEmpty(t *testing.T) {
	slice := makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
		{Addresses: []string{"10.0.0.1"}},
	})
	p := newTestProvider(newMockTemplateStore(nil), slice)

	configs, err := p.Collect(context.Background())
	require.NoError(t, err)
	assert.Empty(t, configs)
}
