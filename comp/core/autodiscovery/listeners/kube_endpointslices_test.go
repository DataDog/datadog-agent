// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package listeners

import (
	"sort"
	"sync"
	"testing"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	discv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	listersv1 "k8s.io/client-go/listers/core/v1"
	discv1listers "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"
)

// createTestListenerWithServices creates a KubeEndpointSlicesListener with fake informers and test services
func createTestListenerWithServices(t *testing.T, services ...*v1.Service) *KubeEndpointSlicesListener {
	// Create fake Kubernetes client with test services
	objs := make([]runtime.Object, len(services))
	for i, svc := range services {
		objs[i] = svc
	}
	client := fake.NewClientset(objs...)

	// Create informer factory
	informerFactory := informers.NewSharedInformerFactory(client, 0)

	// Create listener with real listers (backed by fake client)
	listener := &KubeEndpointSlicesListener{
		serviceLister: informerFactory.Core().V1().Services().Lister(),
		filterStore:   workloadfilterfxmock.SetupMockFilter(t),
	}

	// Start informers and wait for cache sync
	stopCh := make(chan struct{})
	informerFactory.Start(stopCh)
	informerFactory.WaitForCacheSync(stopCh)
	close(stopCh)

	return listener
}

func TestProcessEndpointSlice(t *testing.T) {
	port80 := int32(80)
	port81 := int32(81)
	portNameHTTP := "http"
	portNameStatus := "status"

	slice := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "myservice-abc123",
			Namespace:       "default",
			ResourceVersion: "123",
			UID:             types.UID("slice-uid"),
			Labels: map[string]string{
				"kubernetes.io/service-name": "myservice",
			},
		},
		Endpoints: []discv1.Endpoint{
			{
				Addresses: []string{"10.0.0.1"},
			},
			{
				Addresses: []string{"10.0.0.2"},
			},
		},
		Ports: []discv1.EndpointPort{
			{Name: &portNameHTTP, Port: &port80},
			{Name: &portNameStatus, Port: &port81},
		},
	}

	eps := processEndpointSlice(slice, []string{"foo:bar"}, workloadfilterfxmock.SetupMockFilter(t))

	// Should create 2 endpoint services (one per IP)
	assert.Equal(t, 2, len(eps))

	// Sort by entity ID for deterministic testing
	sort.Slice(eps, func(i, j int) bool {
		return eps[i].entity < eps[j].entity
	})

	// Verify both endpoints were created correctly
	expectedIPs := []string{"10.0.0.1", "10.0.0.2"}
	for i, expectedIP := range expectedIPs {
		// Entity ID format
		assert.Equal(t, "kube_endpoint_uid://default/myservice/"+expectedIP, eps[i].GetServiceID())

		// AD identifiers include specific entity and CEL identifier
		adID := eps[i].GetADIdentifiers()
		assert.Contains(t, adID, "kube_endpoint_uid://default/myservice/"+expectedIP)
		assert.Contains(t, adID, string(adtypes.CelEndpointIdentifier))

		// Hosts contain endpoint IP
		hosts, err := eps[i].GetHosts()
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"endpoint": expectedIP}, hosts)

		// Ports from slice level
		ports, err := eps[i].GetPorts()
		assert.NoError(t, err)
		assert.Equal(t, []workloadmeta.ContainerPort{{Port: 80, Name: "http"}, {Port: 81, Name: "status"}}, ports)

		// Tags include service, namespace, IP, and custom tags
		tags, err := eps[i].GetTags()
		assert.NoError(t, err)
		assert.Equal(t, []string{"kube_service:myservice", "kube_namespace:default", "kube_endpoint_ip:" + expectedIP, "foo:bar"}, tags)

		// Extra config returns namespace
		namespace, err := eps[i].GetExtraConfig("namespace")
		assert.NoError(t, err)
		assert.Equal(t, "default", namespace)
	}
}

func TestProcessEndpointSliceNoServiceLabel(t *testing.T) {
	port80 := int32(80)
	portName := "http"

	slice := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myservice-abc",
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

	eps := processEndpointSlice(slice, []string{}, workloadfilterfxmock.SetupMockFilter(t))

	assert.Equal(t, 0, len(eps))
}

func TestEndpointSlicesDiffer(t *testing.T) {
	port80 := int32(80)
	portName := "http"

	testService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myservice",
			Namespace: "default",
		},
	}

	listener := createTestListenerWithServices(t, testService)

	tests := map[string]struct {
		first  *discv1.EndpointSlice
		second *discv1.EndpointSlice
		result bool
	}{
		"Same resource version": {
			first: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "myservice",
					},
				},
			},
			second: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "myservice",
					},
				},
			},
			result: false,
		},
		"Different resource version, same endpoints": {
			first: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "myservice",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
					{Addresses: []string{"10.0.0.2"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: &port80},
				},
			},
			second: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "myservice",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
					{Addresses: []string{"10.0.0.2"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: &port80},
				},
			},
			result: true,
		},
		"Different IP address": {
			first: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "myservice",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
					{Addresses: []string{"10.0.0.2"}},
				},
			},
			second: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "myservice",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
					{Addresses: []string{"10.0.0.3"}}, // Changed IP
				},
			},
			result: true,
		},
		"Remove endpoint": {
			first: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "myservice",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
					{Addresses: []string{"10.0.0.2"}},
				},
			},
			second: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "myservice",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
				},
			},
			result: true,
		},
		"Change port": {
			first: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "myservice",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: &port80},
				},
			},
			second: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "myservice",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: func() *int32 { p := int32(8080); return &p }()},
				},
			},
			result: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := listener.endpointSlicesDiffer(tc.first, tc.second)
			assert.Equal(t, tc.result, result)
		})
	}
}

func TestProcessEndpointSliceNilPorts(t *testing.T) {
	slice := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myservice-abc",
			Namespace: "default",
			Labels: map[string]string{
				"kubernetes.io/service-name": "myservice",
			},
		},
		Endpoints: []discv1.Endpoint{
			{Addresses: []string{"10.0.0.1"}},
		},
		Ports: []discv1.EndpointPort{
			{Name: nil, Port: nil}, // Nil port should be skipped
		},
	}

	eps := processEndpointSlice(slice, []string{}, workloadfilterfxmock.SetupMockFilter(t))

	assert.Equal(t, 1, len(eps))

	// Ports should be empty (nil ports skipped)
	ports, err := eps[0].GetPorts()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(ports))
}

func TestKubeEndpointSlicesFiltering(t *testing.T) {
	kubeEndpointExcludeConfig := `
cel_workload_exclude:
  - products: ["metrics"]
    rules:
      kube_endpoints:
        - "kube_endpoint.namespace == 'excluded-namespace'"
`
	configmock.NewFromYAML(t, kubeEndpointExcludeConfig)
	mockFilterStore := workloadfilterfxmock.SetupMockFilter(t)
	port80 := int32(80)
	portName := "http"

	testCases := []struct {
		name                string
		slice               *discv1.EndpointSlice
		expectedMetricsExcl bool
		expectedGlobalExcl  bool
	}{
		{
			name: "normal endpoint: not excluded",
			slice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "normal-service-abc",
					Namespace: "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "normal-service",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: &port80},
				},
			},
			expectedMetricsExcl: false,
			expectedGlobalExcl:  false,
		},
		{
			name: "endpoint in excluded namespace: metrics excluded",
			slice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-excluded-ns-abc",
					Namespace: "excluded-namespace",
					Labels: map[string]string{
						"kubernetes.io/service-name": "service-in-excluded-ns",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.2"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: &port80},
				},
			},
			expectedMetricsExcl: true,
			expectedGlobalExcl:  false,
		},
		{
			name: "endpoint with AD annotations: metrics excluded",
			slice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ad-excluded-abc",
					Namespace: "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "ad-excluded",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names": "[\"http_check\"]",
						"ad.datadoghq.com/metrics_exclude":     "true",
						"ad.datadoghq.com/exclude":             "false",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.4"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: &port80},
				},
			},
			expectedMetricsExcl: true,
			expectedGlobalExcl:  false,
		},
		{
			name: "endpoint with exclude annotation: globally excluded",
			slice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "annotation-excluded-abc",
					Namespace: "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "annotation-excluded",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names": "[\"http_check\"]",
						"ad.datadoghq.com/exclude":             "true",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.5"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: &port80},
				},
			},
			expectedMetricsExcl: true,
			expectedGlobalExcl:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			eps := processEndpointSlice(tc.slice, []string{}, mockFilterStore)
			assert.NotEmpty(t, eps, "Should have at least one endpoint service")
			for _, ep := range eps {
				assert.Equal(t, tc.expectedMetricsExcl, ep.metricsExcluded,
					"Expected metricsExcluded to be %v for endpoint %s/%s",
					tc.expectedMetricsExcl, tc.slice.Namespace, tc.slice.Name)
				assert.Equal(t, tc.expectedGlobalExcl, ep.globalExcluded,
					"Expected globalExcluded to be %v for endpoint %s/%s",
					tc.expectedGlobalExcl, tc.slice.Namespace, tc.slice.Name)
			}
		})
	}
}

func TestEndpointSliceShouldEmit(t *testing.T) {
	tests := []struct {
		name               string
		service            *v1.Service
		slice              *discv1.EndpointSlice
		targetAllEndpoints bool
		tracker            adtypes.ServiceTracker
		shouldEmit         bool
	}{
		{
			name: "targetAllEndpoints=true: always emit",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-annotations",
					Namespace: "default",
				},
			},
			slice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-annotations-abc",
					Namespace: "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "no-annotations",
					},
				},
			},
			targetAllEndpoints: true,
			shouldEmit:         true,
		},
		{
			name: "targetAllEndpoints=false, no annotations and not tracked: don't emit",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-annotations",
					Namespace: "default",
				},
			},
			slice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-annotations-abc",
					Namespace: "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "no-annotations",
					},
				},
			},
			targetAllEndpoints: false,
			tracker:            &fakeServiceTracker{tracked: map[string]bool{"default/tracked-svc": false}},
			shouldEmit:         false, // Skip without annotations
		},
		{
			name: "targetAllEndpoints=false, has checks annotation: emit",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "with-checks",
					Namespace: "default",
					Annotations: map[string]string{
						"ad.datadoghq.com/endpoints.checks": `{"nginx": {...}}`,
					},
				},
			},
			slice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "with-checks-abc",
					Namespace: "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "with-checks",
					},
				},
			},
			targetAllEndpoints: false,
			shouldEmit:         true,
		},
		{
			name: "targetAllEndpoints=false, has instances annotation: emit",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "with-instances",
					Namespace: "default",
					Annotations: map[string]string{
						"ad.datadoghq.com/endpoints.instances": `[{...}]`,
					},
				},
			},
			slice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "with-instances-abc",
					Namespace: "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "with-instances",
					},
				},
			},
			targetAllEndpoints: false,
			shouldEmit:         true,
		},
		{
			name: "targetAllEndpoints=false, has prometheus annotations: emit",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "with-check-names",
					Namespace: "default",
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
					},
				},
			},
			slice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "with-check-names-abc",
					Namespace: "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "with-check-names",
					},
				},
			},
			targetAllEndpoints: false,
			shouldEmit:         true,
		},
		{
			name: "targetAllEndpoints=false, no annotations but tracked by a DDI CR: emit",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tracked-svc",
					Namespace: "default",
				},
			},
			slice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tracked-svc-abc",
					Namespace: "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "tracked-svc",
					},
				},
			},
			targetAllEndpoints: false,
			tracker:            &fakeServiceTracker{tracked: map[string]bool{"default/tracked-svc": true}},
			shouldEmit:         true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			listener := createTestListenerWithServices(t, tc.service)
			listener.targetAllEndpoints = tc.targetAllEndpoints
			listener.serviceTracker = tc.tracker
			listener.promInclAnnot = getPrometheusIncludeAnnotations()

			result := listener.shouldEmitSlice(tc.slice)
			assert.Equal(t, tc.shouldEmit, result)
		})
	}
}

// fakeServiceTracker is a controllable types.ServiceTracker for tests. Flipping a
// service's tracked state via set() synchronously invokes the registered callback,
// mimicking the ServiceCheckTemplateStore notifying subscribers.
type fakeServiceTracker struct {
	mu       sync.RWMutex
	tracked  map[string]bool
	callback func(namespace, name string)
}

func (f *fakeServiceTracker) HasService(namespace, name string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.tracked[namespace+"/"+name]
}

func (f *fakeServiceTracker) NotifyOnChange(fn func(namespace, name string)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callback = fn
}

func (f *fakeServiceTracker) set(namespace, name string, tracked bool) {
	f.mu.Lock()
	f.tracked[namespace+"/"+name] = tracked
	cb := f.callback
	f.mu.Unlock()
	if cb != nil {
		cb(namespace, name)
	}
}

// newTrackerTestListener builds a listener backed by indexer-driven listers seeded with
// svc and slice, wired to tracker and buffered new/del channels. The returned indexer
// lets a test mutate the Service in the cache mid-run (mimicking an informer update).
func newTrackerTestListener(t *testing.T, tracker adtypes.ServiceTracker, svc *v1.Service, slice *discv1.EndpointSlice) (*KubeEndpointSlicesListener, cache.Indexer, chan Service, chan Service) {
	svcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	require.NoError(t, svcIndexer.Add(svc))
	sliceIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	require.NoError(t, sliceIndexer.Add(slice))

	newCh := make(chan Service, 10)
	delCh := make(chan Service, 10)
	l := &KubeEndpointSlicesListener{
		endpointsBySlice:    make(map[types.UID][]*KubeEndpointService),
		sliceToService:      make(map[types.UID]string),
		serviceLister:       listersv1.NewServiceLister(svcIndexer),
		endpointSliceLister: discv1listers.NewEndpointSliceLister(sliceIndexer),
		filterStore:         workloadfilterfxmock.SetupMockFilter(t),
		serviceTracker:      tracker,
		newService:          newCh,
		delService:          delCh,
	}
	l.promInclAnnot = getPrometheusIncludeAnnotations()
	return l, svcIndexer, newCh, delCh
}

// TestReconcileServiceOnTrackerChange verifies that when a Service's tracked-state
// flips, the listener emits/removes the Service's endpoints without relying on EndpointSlice events.
func TestReconcileServiceOnTrackerChange(t *testing.T) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "mysvc", Namespace: "default"}, // unannotated
	}
	slice := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mysvc-abc",
			Namespace: "default",
			UID:       types.UID("slice-1"),
			Labels:    map[string]string{"kubernetes.io/service-name": "mysvc"},
		},
		Endpoints: []discv1.Endpoint{
			{Addresses: []string{"10.0.0.1"}},
			{Addresses: []string{"10.0.0.2"}},
		},
	}

	tracker := &fakeServiceTracker{tracked: map[string]bool{}}
	l, _, newCh, delCh := newTrackerTestListener(t, tracker, svc, slice)
	// Subscribe as Listen() does, so tracker.set() drives the reconcile via the callback.
	l.serviceTracker.NotifyOnChange(l.processServiceUpdate)

	// Untracked: a change notification emits nothing.
	tracker.set("default", "mysvc", false)
	assert.Empty(t, newCh, "nothing emitted while the service is untracked")

	// A DDI CR starts targeting the service: its endpoints are emitted.
	tracker.set("default", "mysvc", true)
	assert.Len(t, newCh, 2, "endpoints should be emitted once the service is tracked")
	assert.Empty(t, delCh)

	// The DDI CR is removed: the endpoints are removed.
	tracker.set("default", "mysvc", false)
	assert.Len(t, delCh, 2, "endpoints should be removed once the service is no longer tracked")
}

// TestReconcilesServiceOnUpdatedTags verifies that a standard-tag label
// change on a Service that is tracked by a DDI CR reconciles its
// endpoints with the new tags, rather than leaving them stale.
func TestReconcilesServiceOnUpdatedTags(t *testing.T) {
	const envLabel = "tags.datadoghq.com/env"

	// Unannotated Service carrying only a standard-tag label, tracked by a DDI CR.
	svcOld := &v1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      "mysvc",
		Namespace: "default",
		Labels:    map[string]string{envLabel: "prod"},
	}}
	slice := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mysvc-abc",
			Namespace: "default",
			UID:       types.UID("slice-1"),
			Labels:    map[string]string{"kubernetes.io/service-name": "mysvc"},
		},
		Endpoints: []discv1.Endpoint{{Addresses: []string{"10.0.0.1"}}},
	}

	tracker := &fakeServiceTracker{tracked: map[string]bool{"default/mysvc": true}}
	l, svcIndexer, newCh, delCh := newTrackerTestListener(t, tracker, svcOld, slice)

	// Initial emission carries the current standard tags.
	l.processServiceUpdate("default", "mysvc")
	require.Len(t, newCh, 1)
	tags, err := (<-newCh).(*KubeEndpointService).GetTags()
	require.NoError(t, err)
	assert.Contains(t, tags, "env:prod")

	// The Service's standard-tag label changes; the cache now holds the new object.
	svcNew := svcOld.DeepCopy()
	svcNew.Labels[envLabel] = "staging"
	require.NoError(t, svcIndexer.Update(svcNew))

	l.serviceUpdated(svcOld, svcNew)

	// The stale endpoint is removed and re-emitted with the updated standard tags.
	require.Len(t, delCh, 1, "stale endpoint should be removed")
	require.Len(t, newCh, 1, "endpoint should be re-emitted")
	newTags, err := (<-newCh).(*KubeEndpointService).GetTags()
	require.NoError(t, err)
	assert.Contains(t, newTags, "env:staging")
	assert.NotContains(t, newTags, "env:prod")
}

// TestEndpointSlicesServiceUpdatedPrometheusAnnotations verifies that changes to prometheus
// scrape annotations trigger endpoint service emission.
func TestEndpointSlicesServiceUpdatedPrometheusAnnotations(t *testing.T) {
	slice := &discv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-svc-abc",
			Namespace: "default",
			UID:       types.UID("slice-1"),
			Labels:    map[string]string{"kubernetes.io/service-name": "nginx-svc"},
		},
		Endpoints: []discv1.Endpoint{
			{Addresses: []string{"10.0.0.1"}},
			{Addresses: []string{"10.0.0.2"}},
		},
	}

	t.Run("annotation added", func(t *testing.T) {
		svcOld := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "nginx-svc", Namespace: "default"}}
		l, svcIndexer, newCh, _ := newTrackerTestListener(t, nil, svcOld, slice)

		svcNew := svcOld.DeepCopy()
		svcNew.Annotations = map[string]string{"prometheus.io/scrape": "true"}
		require.NoError(t, svcIndexer.Update(svcNew))
		l.serviceUpdated(svcOld, svcNew)

		require.Len(t, newCh, 2, "endpoint services should be emitted when prometheus annotation is added")
	})

	t.Run("scrape annotation value changed", func(t *testing.T) {
		svcOld := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "nginx-svc", Namespace: "default", Annotations: map[string]string{"prometheus.io/scrape": "false"}}}
		l, svcIndexer, newCh, _ := newTrackerTestListener(t, nil, svcOld, slice)

		svcNew := svcOld.DeepCopy()
		svcNew.Annotations["prometheus.io/scrape"] = "true"
		require.NoError(t, svcIndexer.Update(svcNew))
		l.serviceUpdated(svcOld, svcNew)

		require.Len(t, newCh, 2, "endpoint services should be emitted when scrape annotation value changes")
	})
}
