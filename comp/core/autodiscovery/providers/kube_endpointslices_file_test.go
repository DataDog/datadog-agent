// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	discv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestEndpointSliceStoreInsertSlice(t *testing.T) {
	port80 := int32(80)
	portName := "http"

	store := newEndpointSliceStore()

	// Insert template for namespace/name matching
	template := integration.Config{
		Name:      "http_check",
		Instances: []integration.Data{integration.Data("url: http://%%host%%")},
	}
	store.insertTemplate("default/nginx-svc", template, kubeEndpointResolveAuto)

	// Create matching slice
	slice := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-svc-abc",
			Namespace: "default",
			UID:       types.UID("slice-uid-1"),
			Labels: map[string]string{
				"kubernetes.io/service-name": "nginx-svc",
			},
		},
		Endpoints: []discv1.Endpoint{
			{Addresses: []string{"10.0.0.1"}},
		},
		Ports: []discv1.EndpointPort{
			{Name: &portName, Port: &port80},
		},
	}

	// Insert slice - should match template
	updated := store.insertSlice(slice)
	assert.True(t, updated, "Should update when inserting matching slice")

	// Verify slice was stored
	store.RLock()
	epConfig := store.epSliceConfigs["default/nginx-svc"]
	assert.NotNil(t, epConfig)
	assert.True(t, epConfig.shouldCollect)
	assert.Equal(t, 1, len(epConfig.slices))
	assert.Equal(t, slice, epConfig.slices["slice-uid-1"])
	store.RUnlock()
}

func TestEndpointSliceStoreInsertNonMatchingSlice(t *testing.T) {
	port80 := int32(80)
	portName := "http"

	store := newEndpointSliceStore()

	// Insert template for specific service
	template := integration.Config{Name: "test"}
	store.insertTemplate("default/target-svc", template, kubeEndpointResolveAuto)

	// Create slice for different service
	slice := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-svc-abc",
			Namespace: "default",
			Labels: map[string]string{
				"kubernetes.io/service-name": "other-svc",
			},
		},
		Endpoints: []discv1.Endpoint{
			{Addresses: []string{"10.0.0.1"}},
		},
		Ports: []discv1.EndpointPort{
			{Name: &portName, Port: &port80},
		},
	}

	updated := store.insertSlice(slice)
	assert.False(t, updated, "Should not update for non-matching slice")
}

func TestEndpointSliceStoreDeleteSlice(t *testing.T) {
	port80 := int32(80)
	portName := "http"

	store := newEndpointSliceStore()
	template := integration.Config{Name: "test"}
	store.insertTemplate("default/nginx-svc", template, kubeEndpointResolveAuto)

	// Insert and then delete slice
	slice := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-svc-abc",
			Namespace: "default",
			UID:       types.UID("slice-uid"),
			Labels: map[string]string{
				"kubernetes.io/service-name": "nginx-svc",
			},
		},
		Endpoints: []discv1.Endpoint{
			{Addresses: []string{"10.0.0.1"}},
		},
		Ports: []discv1.EndpointPort{
			{Name: &portName, Port: &port80},
		},
	}

	store.insertSlice(slice)

	// Verify inserted
	store.RLock()
	assert.True(t, store.epSliceConfigs["default/nginx-svc"].shouldCollect)
	store.RUnlock()

	// Delete slice
	store.deleteSlice(slice)

	// Verify marked as not collectable
	store.RLock()
	assert.False(t, store.epSliceConfigs["default/nginx-svc"].shouldCollect)
	assert.Equal(t, 0, len(store.epSliceConfigs["default/nginx-svc"].slices))
	store.RUnlock()
}

func TestEndpointSliceStoreGenerateConfigs(t *testing.T) {
	port80 := int32(80)
	portName := "http"
	nodeName := "node-1"

	store := newEndpointSliceStore()

	// Add template
	template := integration.Config{
		Name:       "nginx",
		Instances:  []integration.Data{integration.Data("nginx_status_url: http://%%host%%")},
		InitConfig: integration.Data("{}"),
	}
	store.insertTemplate("default/nginx-svc", template, kubeEndpointResolveAuto)

	// Add slice with 2 endpoints
	slice := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-svc-abc",
			Namespace: "default",
			UID:       types.UID("slice-uid"),
			Labels: map[string]string{
				"kubernetes.io/service-name": "nginx-svc",
			},
		},
		Endpoints: []discv1.Endpoint{
			{
				Addresses: []string{"10.0.0.1"},
				TargetRef: &v1.ObjectReference{
					Kind: "Pod",
					UID:  "pod-1",
				},
				NodeName: &nodeName,
			},
			{
				Addresses: []string{"10.0.0.2"},
				TargetRef: &v1.ObjectReference{
					Kind: "Pod",
					UID:  "pod-2",
				},
				NodeName: &nodeName,
			},
		},
		Ports: []discv1.EndpointPort{
			{Name: &portName, Port: &port80},
		},
	}
	store.insertSlice(slice)

	// Generate configs
	configs := store.generateConfigs()

	// Should generate 2 configs (one per endpoint)
	assert.Equal(t, 2, len(configs))

	// Verify configs have correct properties
	for _, cfg := range configs {
		assert.Equal(t, "nginx", cfg.Name)
		assert.True(t, cfg.ClusterCheck)
		assert.Equal(t, names.KubeEndpointSlicesFile, cfg.Provider)
		assert.Contains(t, cfg.ServiceID, "kube_endpoint_uid://default/nginx-svc/")
		assert.Equal(t, "node-1", cfg.NodeName)
	}
}

func TestEndpointSliceStoreIsEmpty(t *testing.T) {
	store := newEndpointSliceStore()
	assert.True(t, store.isEmpty())

	// Add template
	template := integration.Config{Name: "test"}
	store.insertTemplate("default/svc", template, kubeEndpointResolveAuto)

	assert.False(t, store.isEmpty())
}

func TestEndpointSliceIDFormat(t *testing.T) {
	id := epSliceID("test-namespace", "test-service")
	assert.Equal(t, "test-namespace/test-service", id)
}

func TestEndpointSliceChecksFromTemplate(t *testing.T) {
	port80 := int32(80)
	portName := "http"
	nodeName := "node-1"

	template := integration.Config{
		Name:       "nginx",
		Instances:  []integration.Data{integration.Data("url: http://%%host%%:%%port%%")},
		InitConfig: integration.Data("{}"),
		Source:     "file:/etc/datadog/conf.d/nginx.yaml",
	}

	slice := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-abc",
			Namespace: "prod",
			Labels: map[string]string{
				"kubernetes.io/service-name": "nginx",
			},
		},
		Endpoints: []discv1.Endpoint{
			{
				Addresses: []string{"192.168.1.5"},
				TargetRef: &v1.ObjectReference{
					Kind: "Pod",
					Name: "nginx-pod",
					UID:  "pod-uid-abc",
				},
				NodeName: &nodeName,
			},
		},
		Ports: []discv1.EndpointPort{
			{Name: &portName, Port: &port80},
		},
	}

	tests := []struct {
		name             string
		resolveMode      endpointResolveMode
		expectNodeName   string
		expectPodInADIDs bool
	}{
		{
			name:             "auto mode: resolve pod metadata",
			resolveMode:      kubeEndpointResolveAuto,
			expectNodeName:   "node-1",
			expectPodInADIDs: true,
		},
		{
			name:             "ip mode: don't resolve pod metadata",
			resolveMode:      kubeEndpointResolveIP,
			expectNodeName:   "",
			expectPodInADIDs: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configs := endpointSliceChecksFromTemplate(template, slice, tc.resolveMode)

			assert.Equal(t, 1, len(configs))

			config := configs[0]
			assert.Equal(t, "nginx", config.Name)
			assert.Equal(t, "kube_endpoint_uid://prod/nginx/192.168.1.5", config.ServiceID)
			assert.True(t, config.ClusterCheck)
			assert.Equal(t, names.KubeEndpointSlicesFile, config.Provider)
			assert.Equal(t, tc.expectNodeName, config.NodeName)

			// Check pod UID in ADIdentifiers
			hasPodUID := false
			for _, adID := range config.ADIdentifiers {
				if adID == "kubernetes_pod://pod-uid-abc" {
					hasPodUID = true
					break
				}
			}
			assert.Equal(t, tc.expectPodInADIDs, hasPodUID)
		})
	}
}

func TestEndpointSliceChecksFromTemplateNilSlice(t *testing.T) {
	template := integration.Config{Name: "test"}

	configs := endpointSliceChecksFromTemplate(template, nil, kubeEndpointResolveAuto)

	// Should return empty for nil slice
	assert.Equal(t, 0, len(configs))
}

func TestEndpointSliceChecksFromTemplateNoServiceLabel(t *testing.T) {
	port80 := int32(80)
	portName := "http"

	template := integration.Config{Name: "test"}

	slice := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "slice-abc",
			Namespace: "default",
			// Missing kubernetes.io/service-name label
		},
		Endpoints: []discv1.Endpoint{
			{Addresses: []string{"10.0.0.1"}},
		},
		Ports: []discv1.EndpointPort{
			{Name: &portName, Port: &port80},
		},
	}

	configs := endpointSliceChecksFromTemplate(template, slice, kubeEndpointResolveAuto)

	// Should return empty without service label
	assert.Equal(t, 0, len(configs))
}

func TestEndpointSliceStoreMultipleSlicesPerService(t *testing.T) {
	port80 := int32(80)
	portName := "http"

	store := newEndpointSliceStore()

	// Add template
	template := integration.Config{
		Name:       "http_check",
		Instances:  []integration.Data{integration.Data("url: http://%%host%%")},
		InitConfig: integration.Data("{}"),
	}
	store.insertTemplate("default/large-svc", template, kubeEndpointResolveAuto)

	// Insert multiple slices for the same service
	slice1 := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "large-svc-abc",
			Namespace: "default",
			UID:       types.UID("slice-1"),
			Labels: map[string]string{
				"kubernetes.io/service-name": "large-svc",
			},
		},
		Endpoints: []discv1.Endpoint{
			{Addresses: []string{"10.0.0.1"}},
			{Addresses: []string{"10.0.0.2"}},
		},
		Ports: []discv1.EndpointPort{
			{Name: &portName, Port: &port80},
		},
	}

	slice2 := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "large-svc-xyz",
			Namespace: "default",
			UID:       types.UID("slice-2"),
			Labels: map[string]string{
				"kubernetes.io/service-name": "large-svc",
			},
		},
		Endpoints: []discv1.Endpoint{
			{Addresses: []string{"10.0.0.3"}},
		},
		Ports: []discv1.EndpointPort{
			{Name: &portName, Port: &port80},
		},
	}

	// Insert both slices
	store.insertSlice(slice1)
	store.insertSlice(slice2)

	// Verify both stored
	store.RLock()
	epConfig := store.epSliceConfigs["default/large-svc"]
	assert.Equal(t, 2, len(epConfig.slices))
	assert.NotNil(t, epConfig.slices["slice-1"])
	assert.NotNil(t, epConfig.slices["slice-2"])
	store.RUnlock()

	// Generate configs - should create 3 configs total (2 + 1 endpoints)
	configs := store.generateConfigs()
	assert.Equal(t, 3, len(configs))
}

func TestEndpointSliceStoreDeleteOneOfMultipleSlices(t *testing.T) {
	port80 := int32(80)
	portName := "http"

	store := newEndpointSliceStore()
	template := integration.Config{Name: "test"}
	store.insertTemplate("default/svc", template, kubeEndpointResolveAuto)

	// Insert 2 slices
	slice1 := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			UID:       types.UID("slice-1"),
			Labels: map[string]string{
				"kubernetes.io/service-name": "svc",
			},
		},
		Endpoints: []discv1.Endpoint{{Addresses: []string{"10.0.0.1"}}},
		Ports:     []discv1.EndpointPort{{Name: &portName, Port: &port80}},
	}

	slice2 := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			UID:       types.UID("slice-2"),
			Labels: map[string]string{
				"kubernetes.io/service-name": "svc",
			},
		},
		Endpoints: []discv1.Endpoint{{Addresses: []string{"10.0.0.2"}}},
		Ports:     []discv1.EndpointPort{{Name: &portName, Port: &port80}},
	}

	store.insertSlice(slice1)
	store.insertSlice(slice2)

	// Delete one slice
	store.deleteSlice(slice1)

	// Should still be collectable (slice2 remains)
	store.RLock()
	epConfig := store.epSliceConfigs["default/svc"]
	assert.True(t, epConfig.shouldCollect)
	assert.Equal(t, 1, len(epConfig.slices))
	assert.Nil(t, epConfig.slices["slice-1"])
	assert.NotNil(t, epConfig.slices["slice-2"])
	store.RUnlock()

	// Delete second slice
	store.deleteSlice(slice2)

	// Now should not be collectable (no slices left)
	store.RLock()
	assert.False(t, epConfig.shouldCollect)
	assert.Equal(t, 0, len(epConfig.slices))
	store.RUnlock()
}
