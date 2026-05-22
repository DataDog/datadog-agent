// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

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
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
)

// mockTemplateStore implements serviceTemplateStore for testing without
// depending on the instrumentation handlers package.
type mockTemplateStore struct {
	mu        sync.RWMutex
	templates map[string][]integration.Config
	onChange  func()
}

func newMockTemplateStore(templates map[string][]integration.Config) *mockTemplateStore {
	if templates == nil {
		templates = make(map[string][]integration.Config)
	}
	return &mockTemplateStore{
		templates: templates,
	}
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

func strPtr(s string) *string {
	return &s
}

func newTestProvider(store serviceTemplateStore) *KubeEndpointSlicesCRConfigProvider {
	return &KubeEndpointSlicesCRConfigProvider{
		templateStore:   store,
		slicesByService: make(map[string]map[string]*discv1.EndpointSlice),
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
	store := newMockTemplateStore(nil)
	p := newTestProvider(store)
	p.upToDate = true

	upToDate, err := p.IsUpToDate(context.Background())
	require.NoError(t, err)
	assert.True(t, upToDate)

	p.setUpToDate(false)
	upToDate, err = p.IsUpToDate(context.Background())
	require.NoError(t, err)
	assert.False(t, upToDate)
}

func TestCRProvider_InsertSlice_TrackedService(t *testing.T) {
	store := newMockTemplateStore(map[string][]integration.Config{
		"default/my-svc": {{Name: "redisdb", Source: "datadoginstrumentation:default/cr1"}},
	})

	p := newTestProvider(store)

	slice := makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
		{Addresses: []string{"10.0.0.1"}, NodeName: strPtr("node-1")},
	})

	inserted := p.insertSlice(slice)
	assert.True(t, inserted)
	assert.Len(t, p.slicesByService["default/my-svc"], 1)
}

func TestCRProvider_InsertSlice_UntrackedService(t *testing.T) {
	store := newMockTemplateStore(nil)
	p := newTestProvider(store)

	slice := makeTestSlice("default", "unknown-svc", "uid-1", []discv1.Endpoint{
		{Addresses: []string{"10.0.0.1"}},
	})

	inserted := p.insertSlice(slice)
	assert.False(t, inserted)
	assert.Empty(t, p.slicesByService)
}

func TestCRProvider_DeleteSlice(t *testing.T) {
	store := newMockTemplateStore(map[string][]integration.Config{
		"default/my-svc": {{Name: "redisdb"}},
	})

	p := newTestProvider(store)

	slice := makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
		{Addresses: []string{"10.0.0.1"}},
	})

	p.insertSlice(slice)
	assert.Len(t, p.slicesByService["default/my-svc"], 1)

	deleted := p.deleteSlice(slice)
	assert.True(t, deleted)
	assert.Empty(t, p.slicesByService)
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

	p := newTestProvider(store)

	slice := makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
		{Addresses: []string{"10.0.0.1"}, NodeName: strPtr("node-1")},
		{Addresses: []string{"10.0.0.2"}, NodeName: strPtr("node-2")},
	})
	p.insertSlice(slice)

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

	p := newTestProvider(store)
	slice := makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
		{Addresses: []string{"10.0.0.1"}, NodeName: strPtr("node-1")},
	})
	p.insertSlice(slice)

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

	p := newTestProvider(store)

	configs, err := p.Collect(context.Background())
	require.NoError(t, err)
	assert.Empty(t, configs)
}

func TestCRProvider_Collect_NoTemplatesReturnsEmpty(t *testing.T) {
	store := newMockTemplateStore(nil)
	p := newTestProvider(store)

	slice := makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
		{Addresses: []string{"10.0.0.1"}},
	})
	// Slice won't be inserted because no templates track this service
	p.insertSlice(slice)

	configs, err := p.Collect(context.Background())
	require.NoError(t, err)
	assert.Empty(t, configs)
}

func TestCRProvider_OnChangeMarksNotUpToDate(t *testing.T) {
	store := newMockTemplateStore(nil)
	p := newTestProvider(store)
	p.upToDate = true

	store.SetOnChange(func() {
		p.setUpToDate(false)
	})

	// Trigger onChange by mutating the store directly.
	store.mu.Lock()
	store.templates["default/svc"] = []integration.Config{{Name: "check"}}
	onChange := store.onChange
	store.mu.Unlock()
	if onChange != nil {
		onChange()
	}

	upToDate, err := p.IsUpToDate(context.Background())
	require.NoError(t, err)
	assert.False(t, upToDate)
}

func TestCRProvider_EventHandlers(t *testing.T) {
	store := newMockTemplateStore(map[string][]integration.Config{
		"default/my-svc": {{Name: "redisdb", InitConfig: integration.Data("{}"), Instances: []integration.Data{integration.Data(`{}`)}}},
	})

	p := newTestProvider(store)
	p.upToDate = true

	slice := makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
		{Addresses: []string{"10.0.0.1"}, NodeName: strPtr("node-1")},
	})

	// Add handler
	p.addHandler(slice)
	upToDate, err := p.IsUpToDate(context.Background())
	require.NoError(t, err)
	assert.False(t, upToDate)

	configs, err := p.Collect(context.Background())
	require.NoError(t, err)
	assert.Len(t, configs, 1)

	// Update handler — new slice with different endpoints
	updatedSlice := makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
		{Addresses: []string{"10.0.0.1"}, NodeName: strPtr("node-1")},
		{Addresses: []string{"10.0.0.2"}, NodeName: strPtr("node-2")},
	})
	p.updateHandler(slice, updatedSlice)
	configs, err = p.Collect(context.Background())
	require.NoError(t, err)
	assert.Len(t, configs, 2)

	// Delete handler
	p.deleteHandler(updatedSlice)
	configs, err = p.Collect(context.Background())
	require.NoError(t, err)
	assert.Empty(t, configs)
}

func TestCRProvider_DeleteHandler_Tombstone(t *testing.T) {
	store := newMockTemplateStore(map[string][]integration.Config{
		"default/my-svc": {{Name: "redisdb", InitConfig: integration.Data("{}"), Instances: []integration.Data{integration.Data(`{}`)}}},
	})

	p := newTestProvider(store)

	slice := makeTestSlice("default", "my-svc", "uid-1", []discv1.Endpoint{
		{Addresses: []string{"10.0.0.1"}, NodeName: strPtr("node-1")},
	})
	p.insertSlice(slice)

	configs, err := p.Collect(context.Background())
	require.NoError(t, err)
	assert.Len(t, configs, 1)

	// Delete via tombstone wrapper
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "default/my-svc-uid-1",
		Obj: slice,
	}
	p.deleteHandler(tombstone)

	configs, err = p.Collect(context.Background())
	require.NoError(t, err)
	assert.Empty(t, configs)
}
