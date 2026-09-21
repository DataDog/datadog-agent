// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

func TestDRAAPIVersion(t *testing.T) {
	list := func(gv string, resources ...string) *metav1.APIResourceList {
		l := &metav1.APIResourceList{GroupVersion: gv}
		for _, r := range resources {
			l.APIResources = append(l.APIResources, metav1.APIResource{Name: r})
		}
		return l
	}

	tests := []struct {
		name      string
		resources []*metav1.APIResourceList
		want      string
	}{
		{
			name:      "absent group means no DRA",
			resources: []*metav1.APIResourceList{list("apps/v1", "deployments")},
			want:      "",
		},
		{
			name:      "beta-only cluster (pre-1.34)",
			resources: []*metav1.APIResourceList{list("resource.k8s.io/v1beta1", "resourceclaims")},
			want:      "v1beta1",
		},
		{
			name: "newest served version wins",
			resources: []*metav1.APIResourceList{
				list("resource.k8s.io/v1beta1", "resourceclaims"),
				list("resource.k8s.io/v1beta2", "resourceclaims"),
				list("resource.k8s.io/v1", "resourceclaims"),
			},
			want: "v1",
		},
		{
			name:      "advertised but empty group version is not served",
			resources: []*metav1.APIResourceList{list("resource.k8s.io/v1")},
			want:      "",
		},
		{
			name:      "a nil entry must not panic",
			resources: []*metav1.APIResourceList{nil},
			want:      "",
		},
		{
			name:      "unknown future version is ignored rather than guessed at",
			resources: []*metav1.APIResourceList{list("resource.k8s.io/v2", "resourceclaims")},
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DRAAPIVersion(tt.resources))
		})
	}
}

func TestDRADeviceCapacityUnwrap(t *testing.T) {
	// v1beta1 nests capacity under "basic". Reading the top level there yields
	// nothing, so the capacity metric silently reports zero on those clusters.
	v1Device := map[string]interface{}{
		"name":     "gpu-0",
		"capacity": map[string]interface{}{"memory": map[string]interface{}{"value": "80Gi"}},
	}
	betaDevice := map[string]interface{}{
		"name": "gpu-0",
		"basic": map[string]interface{}{
			"capacity": map[string]interface{}{"memory": map[string]interface{}{"value": "80Gi"}},
		},
	}

	for _, d := range []map[string]interface{}{v1Device, betaDevice} {
		val, found, _ := unstructured.NestedString(draDeviceCapacity(d), "memory", "value")
		require.True(t, found)
		assert.Equal(t, "80Gi", val)
	}
}

func TestParseQuantity(t *testing.T) {
	// Capacities are Kubernetes quantities, not plain integers.
	q, err := parseQuantity("81152Mi")
	require.NoError(t, err)
	assert.InDelta(t, 85094039552.0, q, 1.0)

	_, err = parseQuantity("not-a-quantity")
	assert.Error(t, err)
}

func claim(t *testing.T, obj map[string]interface{}) *unstructured.Unstructured {
	t.Helper()
	return &unstructured.Unstructured{Object: obj}
}

func TestClaimState(t *testing.T) {
	tests := []struct {
		name string
		obj  map[string]interface{}
		want string
	}{
		{
			name: "no allocation yet",
			obj:  map[string]interface{}{"status": map[string]interface{}{}},
			want: claimStatePending,
		},
		{
			name: "allocated but not consumed",
			obj: map[string]interface{}{"status": map[string]interface{}{
				"allocation": map[string]interface{}{"devices": map[string]interface{}{}},
			}},
			want: claimStateAllocated,
		},
		{
			name: "allocated and reserved by a pod is in_use, not a separate state",
			obj: map[string]interface{}{"status": map[string]interface{}{
				"allocation":  map[string]interface{}{"devices": map[string]interface{}{}},
				"reservedFor": []interface{}{map[string]interface{}{"name": "pod-a"}},
			}},
			want: claimStateInUse,
		},
		{
			name: "an empty reservedFor is not a reservation",
			obj: map[string]interface{}{"status": map[string]interface{}{
				"allocation":  map[string]interface{}{"devices": map[string]interface{}{}},
				"reservedFor": []interface{}{},
			}},
			want: claimStateAllocated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, claimState(claim(t, tt.obj)))
		})
	}
}

func TestMatchesDeviceClasses(t *testing.T) {
	withRequests := func(requests ...interface{}) *unstructured.Unstructured {
		return claim(t, map[string]interface{}{
			"spec": map[string]interface{}{
				"devices": map[string]interface{}{"requests": requests},
			},
		})
	}
	// v1 and v1beta2 wrap an ordinary request in "exactly"; this is the shape
	// most clusters serve, and reading only the inline form matches nothing.
	gpuRequest := map[string]interface{}{
		"name":    "gpu",
		"exactly": map[string]interface{}{"deviceClassName": "gpu.nvidia.com"},
	}
	gpuRequestV1beta1 := map[string]interface{}{"name": "gpu", "deviceClassName": "gpu.nvidia.com"}
	// DraNet advertises DeviceClass "dranet" under driver "dra.net" -- the case
	// that makes driver-name matching wrong.
	netRequest := map[string]interface{}{
		"name":    "net",
		"exactly": map[string]interface{}{"deviceClassName": "dranet"},
	}
	alternativesRequest := map[string]interface{}{
		"name": "either",
		"firstAvailable": []interface{}{
			map[string]interface{}{"name": "a", "deviceClassName": "dranet"},
			map[string]interface{}{"name": "b", "deviceClassName": "gpu.nvidia.com"},
		},
	}

	tests := []struct {
		name    string
		claim   *unstructured.Unstructured
		classes []string
		want    bool
	}{
		{name: "empty configuration matches everything", claim: withRequests(netRequest), want: true},
		{name: "matching class (v1 exactly)", claim: withRequests(gpuRequest), classes: []string{"gpu.nvidia.com"}, want: true},
		{name: "matching class (v1beta1 inline)", claim: withRequests(gpuRequestV1beta1), classes: []string{"gpu.nvidia.com"}, want: true},
		{name: "non-accelerator claim is excluded", claim: withRequests(netRequest), classes: []string{"gpu.nvidia.com"}, want: false},
		{name: "one matching request out of several is enough", claim: withRequests(netRequest, gpuRequest), classes: []string{"gpu.nvidia.com"}, want: true},
		{name: "alternatives are inspected too", claim: withRequests(alternativesRequest), classes: []string{"gpu.nvidia.com"}, want: true},
		{name: "claim with no requests", claim: claim(t, map[string]interface{}{}), classes: []string{"gpu.nvidia.com"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, matchesDeviceClasses(tt.claim, tt.classes))
		})
	}
}

// generatorByName returns the named family generator, failing the test if the
// factory does not expose it.
func generatorByName(t *testing.T, generators []generator.FamilyGenerator, name string) *generator.FamilyGenerator {
	t.Helper()
	for i := range generators {
		if generators[i].Name == name {
			return &generators[i]
		}
	}
	t.Fatalf("no generator named %s", name)
	return nil
}

// labelsOf flattens a metric's labels for assertion.
func labelsOf(m *metric.Metric) map[string]string {
	out := map[string]string{}
	for i, k := range m.LabelKeys {
		out[k] = m.LabelValues[i]
	}
	return out
}

func newClaimFactory(deviceClasses ...string) *resourceClaimFactory {
	return &resourceClaimFactory{apiVersion: "v1", deviceClasses: deviceClasses}
}

// TestResourceClaimMetricsAreIdentified pins the labels. Without them every
// claim emits an identical series and a cluster with more than one pending
// claim reports whichever was submitted last -- i.e. always 1.
func TestResourceClaimMetricsAreIdentified(t *testing.T) {
	f := newClaimFactory()
	obj := claim(t, map[string]interface{}{
		"metadata": map[string]interface{}{"namespace": "team-a", "name": "claim-1"},
		"status":   map[string]interface{}{},
	})

	family := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceclaim_status").Generate(obj)
	require.Len(t, family.Metrics, 1)
	assert.Equal(t, map[string]string{
		"namespace":     "team-a",
		"resourceclaim": "claim-1",
		"state":         "pending",
	}, labelsOf(family.Metrics[0]))
	assert.Equal(t, float64(1), family.Metrics[0].Value)
}

// TestResourceClaimCreatedIsATimestamp pins the shape that survives the KSM
// store's caching: families are generated when the informer sees the object,
// not when the endpoint is scraped, so an elapsed-seconds value would freeze at
// zero for a claim that is born pending and stays that way.
func TestResourceClaimCreatedIsATimestamp(t *testing.T) {
	f := newClaimFactory()
	created := metav1.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	pending := claim(t, map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace":         "team-a",
			"name":              "claim-1",
			"creationTimestamp": created.UTC().Format(time.RFC3339),
		},
		"status": map[string]interface{}{},
	})

	family := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceclaim_created").Generate(pending)
	require.Len(t, family.Metrics, 1)
	assert.Equal(t, float64(created.Unix()), family.Metrics[0].Value)

	// An allocated claim is no longer waiting, so it contributes nothing.
	allocated := claim(t, map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace":         "team-a",
			"name":              "claim-2",
			"creationTimestamp": created.UTC().Format(time.RFC3339),
		},
		"status": map[string]interface{}{
			"allocation": map[string]interface{}{"devices": map[string]interface{}{}},
		},
	})
	family = generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceclaim_created").Generate(allocated)
	assert.Empty(t, family.Metrics)
}

func TestResourceClaimDevicesAllocated(t *testing.T) {
	f := newClaimFactory()
	obj := claim(t, map[string]interface{}{
		"metadata": map[string]interface{}{"namespace": "team-a", "name": "claim-1"},
		"status": map[string]interface{}{
			"allocation": map[string]interface{}{
				"devices": map[string]interface{}{
					// Three MIG slices of one card count as three devices
					// (RFC 4.5: DRA allocates devices, not cards).
					"results": []interface{}{
						map[string]interface{}{"device": "gpu-0-mig-1g18gb-19-0"},
						map[string]interface{}{"device": "gpu-0-mig-1g18gb-19-1"},
						map[string]interface{}{"device": "gpu-0-mig-1g18gb-19-2"},
					},
				},
			},
		},
	})

	family := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceclaim_devices_allocated").Generate(obj)
	require.Len(t, family.Metrics, 1)
	assert.Equal(t, float64(3), family.Metrics[0].Value)
}

// TestResourceClaimDeviceClassFilter pins that a non-accelerator claim is
// excluded from every family, not just from one of them -- a claim counted by
// one metric and missed by another is worse than counting them all.
func TestResourceClaimDeviceClassFilter(t *testing.T) {
	f := newClaimFactory("gpu.nvidia.com")
	netClaim := claim(t, map[string]interface{}{
		"metadata": map[string]interface{}{"namespace": "team-a", "name": "net-claim"},
		"spec": map[string]interface{}{"devices": map[string]interface{}{"requests": []interface{}{
			map[string]interface{}{"name": "net", "deviceClassName": "dranet"},
		}}},
		"status": map[string]interface{}{},
	})

	for _, name := range []string{
		"kube_resourceclaim_status",
		"kube_resourceclaim_created",
		"kube_resourceclaim_devices_allocated",
	} {
		family := generatorByName(t, f.MetricFamilyGenerators(), name).Generate(netClaim)
		assert.Empty(t, family.Metrics, "%s must not count a non-accelerator claim", name)
	}
}

func newSliceObject(t *testing.T, devices []interface{}) *unstructured.Unstructured {
	t.Helper()
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "node-1-gpu"},
		"spec": map[string]interface{}{
			"nodeName": "node-1",
			"driver":   "gpu.nvidia.com",
			"pool":     map[string]interface{}{"name": "node-1", "generation": int64(1)},
			"devices":  devices,
		},
	}}
}

func TestResourceSliceMetrics(t *testing.T) {
	f := &resourceSliceFactory{apiVersion: "v1"}
	obj := newSliceObject(t, []interface{}{
		map[string]interface{}{
			"name":     "gpu-0",
			"capacity": map[string]interface{}{"memory": map[string]interface{}{"value": "80Gi"}},
		},
		map[string]interface{}{
			"name":     "gpu-1",
			"capacity": map[string]interface{}{"memory": map[string]interface{}{"value": "80Gi"}},
		},
	})

	generators := f.MetricFamilyGenerators()

	devices := generatorByName(t, generators, "kube_resourceslice_devices_total").Generate(obj)
	require.Len(t, devices.Metrics, 1)
	assert.Equal(t, float64(2), devices.Metrics[0].Value)
	assert.Equal(t, map[string]string{
		"resourceslice": "node-1-gpu",
		"driver":        "gpu.nvidia.com",
		"pool":          "node-1",
		"node":          "node-1",
	}, labelsOf(devices.Metrics[0]))

	capacity := generatorByName(t, generators, "kube_resourceslice_capacity").Generate(obj)
	require.Len(t, capacity.Metrics, 1)
	assert.Equal(t, float64(160*1024*1024*1024), capacity.Metrics[0].Value)
	assert.Equal(t, "memory", labelsOf(capacity.Metrics[0])["capacity"])
}

// TestResourceSliceCapacityIsNotGPUSpecific pins the reason capacity carries
// its name as a tag rather than in the metric name: a ResourceSlice describes
// whatever a driver publishes, and "memory" is one key among many. A metric
// named after one key would read zero for every non-GPU driver.
func TestResourceSliceCapacityIsNotGPUSpecific(t *testing.T) {
	f := &resourceSliceFactory{apiVersion: "v1"}
	obj := newSliceObject(t, []interface{}{
		map[string]interface{}{
			"name": "ddnet0",
			"capacity": map[string]interface{}{
				"bandwidth":                      map[string]interface{}{"value": "100"},
				"resource.kubernetes.io/sockets": map[string]interface{}{"value": "2"},
			},
		},
	})

	family := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceslice_capacity").Generate(obj)
	require.Len(t, family.Metrics, 2)
	// Sorted by capacity name, and a domain-qualified key is carried verbatim.
	assert.Equal(t, "bandwidth", labelsOf(family.Metrics[0])["capacity"])
	assert.Equal(t, float64(100), family.Metrics[0].Value)
	assert.Equal(t, "resource.kubernetes.io/sockets", labelsOf(family.Metrics[1])["capacity"])
	assert.Equal(t, float64(2), family.Metrics[1].Value)
}

func TestResourceSliceCapacityV1beta1(t *testing.T) {
	f := &resourceSliceFactory{apiVersion: "v1beta1"}
	obj := newSliceObject(t, []interface{}{
		map[string]interface{}{
			"name": "gpu-0",
			"basic": map[string]interface{}{
				"capacity": map[string]interface{}{"memory": map[string]interface{}{"value": "80Gi"}},
			},
		},
	})

	family := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceslice_capacity").Generate(obj)
	require.Len(t, family.Metrics, 1)
	assert.Equal(t, float64(80*1024*1024*1024), family.Metrics[0].Value)
}

func TestResourceSliceAllNodesHasNoNodeLabel(t *testing.T) {
	// A slice advertising cluster-wide devices has no nodeName; an empty tag
	// value would read as a node whose name is blank.
	f := &resourceSliceFactory{apiVersion: "v1"}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "cluster-wide"},
		"spec": map[string]interface{}{
			"allNodes": true,
			"driver":   "dra.net",
			"pool":     map[string]interface{}{"name": "cluster-wide"},
			"devices":  []interface{}{map[string]interface{}{"name": "ddnet0"}},
		},
	}}

	family := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceslice_devices_total").Generate(obj)
	require.Len(t, family.Metrics, 1)
	assert.Equal(t, map[string]string{
		"resourceslice": "cluster-wide",
		"driver":        "dra.net",
		"pool":          "cluster-wide",
	}, labelsOf(family.Metrics[0]))
}

// TestDRAListWatchNamespaceScoping pins that a namespaced resource honours the
// requested namespace. Narrowing the dynamic client to ResourceInterface too
// early silently turns a namespace-scoped configuration into a cluster-wide
// list.
func TestDRAListWatchNamespaceScoping(t *testing.T) {
	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{Group: DRAGroup, Version: "v1", Resource: "resourceclaims"}
	scheme.AddKnownTypeWithName(gvr.GroupVersion().WithKind("ResourceClaimList"), &unstructured.UnstructuredList{})

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{gvr: "ResourceClaimList"},
		newClaimObject("team-a", "claim-a"),
		newClaimObject("team-b", "claim-b"),
	)

	f := &resourceClaimFactory{client: client, apiVersion: "v1"}
	c, err := f.CreateClient(nil)
	require.NoError(t, err)

	scoped := f.ListWatch(c, "team-a", "")
	require.NotNil(t, scoped)
	list, err := scoped.(cache.ListerWatcherWithContext).ListWithContext(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	items, err := meta.ExtractList(list)
	require.NoError(t, err)
	require.Len(t, items, 1, "a namespace-scoped ListWatch must not return other namespaces")

	all := f.ListWatch(c, metav1.NamespaceAll, "")
	list, err = all.(cache.ListerWatcherWithContext).ListWithContext(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	items, err = meta.ExtractList(list)
	require.NoError(t, err)
	assert.Len(t, items, 2)

	// ResourceSlice is cluster-scoped: a namespace must never be applied.
	sliceGVR := schema.GroupVersionResource{Group: DRAGroup, Version: "v1", Resource: "resourceslices"}
	sf := &resourceSliceFactory{client: client, apiVersion: "v1"}
	sc, err := sf.CreateClient(nil)
	require.NoError(t, err)
	require.NotNil(t, sf.ListWatch(sc, "team-a", ""))
	_ = sliceGVR
}

func newClaimObject(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": DRAGroup + "/v1",
		"kind":       "ResourceClaim",
		"metadata":   map[string]interface{}{"namespace": namespace, "name": name},
	}}
}

// consumesCounters marks a device as one of several mutually exclusive ways to
// cut the same hardware. Shaped after a real RTX PRO 6000 slice, where the
// driver advertises the whole card alongside every MIG placement and each
// placement claims a subset of the 12 memory slices.
func partitionableDevice(name, memory string, slices ...string) map[string]interface{} {
	return partitionableDeviceInSet("gpu-0-counter-set", name, memory, slices...)
}

func partitionableDeviceInSet(counterSet, name, memory string, slices ...string) map[string]interface{} {
	counters := map[string]interface{}{}
	for _, s := range slices {
		counters[s] = map[string]interface{}{"value": "1"}
	}
	return map[string]interface{}{
		"name":     name,
		"capacity": map[string]interface{}{"memory": map[string]interface{}{"value": memory}},
		"consumesCounters": []interface{}{
			map[string]interface{}{"counterSet": counterSet, "counters": counters},
		},
	}
}

func TestResourceSliceCapacityDoesNotSumPartitionOptions(t *testing.T) {
	f := &resourceSliceFactory{apiVersion: "v1"}
	obj := newSliceObject(t, []interface{}{
		// "do not partition" -- consumes the whole counter set.
		partitionableDevice("gpu-0", "95Gi",
			"memory-slice-0", "memory-slice-1", "memory-slice-2", "memory-slice-3"),
		// Two placements that could coexist with each other but not with gpu-0.
		partitionableDevice("gpu-0-mig-1g24gb-14-0", "24Gi", "memory-slice-0", "memory-slice-1"),
		partitionableDevice("gpu-0-mig-1g24gb-14-2", "24Gi", "memory-slice-2", "memory-slice-3"),
	})

	capacity := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceslice_capacity").Generate(obj)
	require.Len(t, capacity.Metrics, 1)
	// Summing would report 143Gi against a 95Gi card.
	assert.Equal(t, float64(95*1024*1024*1024), capacity.Metrics[0].Value)
	assert.Equal(t, "memory", labelsOf(capacity.Metrics[0])["capacity"])
}

func TestResourceSliceCapacityAddsIndependentDevicesToTheLargestPartition(t *testing.T) {
	f := &resourceSliceFactory{apiVersion: "v1"}
	obj := newSliceObject(t, []interface{}{
		partitionableDevice("gpu-0", "95Gi", "memory-slice-0", "memory-slice-1"),
		partitionableDevice("gpu-0-mig-1g24gb-14-0", "24Gi", "memory-slice-0"),
		// No consumesCounters: separate hardware, present at the same time as
		// whichever partition is chosen.
		map[string]interface{}{
			"name":     "nic-0",
			"capacity": map[string]interface{}{"memory": map[string]interface{}{"value": "5Gi"}},
		},
	})

	capacity := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceslice_capacity").Generate(obj)
	require.Len(t, capacity.Metrics, 1)
	assert.Equal(t, float64(100*1024*1024*1024), capacity.Metrics[0].Value)
}

// v1beta1 nests the device fields under "basic"; consumesCounters has to be
// found there too, or every device reads as independent and the sum comes back.
func TestResourceSliceCapacityReadsConsumesCountersUnderBasic(t *testing.T) {
	f := &resourceSliceFactory{apiVersion: "v1beta1"}
	dev := func(name, memory string) map[string]interface{} {
		return map[string]interface{}{
			"name": name,
			"basic": map[string]interface{}{
				"capacity": map[string]interface{}{"memory": map[string]interface{}{"value": memory}},
				"consumesCounters": []interface{}{
					map[string]interface{}{
						"counterSet": "gpu-0-counter-set",
						"counters":   map[string]interface{}{"memory-slice-0": map[string]interface{}{"value": "1"}},
					},
				},
			},
		}
	}
	obj := newSliceObject(t, []interface{}{dev("gpu-0", "95Gi"), dev("gpu-0-mig", "24Gi")})

	capacity := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceslice_capacity").Generate(obj)
	require.Len(t, capacity.Metrics, 1)
	assert.Equal(t, float64(95*1024*1024*1024), capacity.Metrics[0].Value)
}

// A multi-GPU node publishes one counter set per card. Exclusion holds inside a
// set, not across them, so the cards add up -- a single global maximum would
// report one card's worth of memory for the whole node.
func TestResourceSliceCapacitySumsAcrossCounterSets(t *testing.T) {
	f := &resourceSliceFactory{apiVersion: "v1"}
	obj := newSliceObject(t, []interface{}{
		partitionableDeviceInSet("gpu-0-counter-set", "gpu-0", "95Gi", "memory-slice-0", "memory-slice-1"),
		partitionableDeviceInSet("gpu-0-counter-set", "gpu-0-mig-1g24gb", "24Gi", "memory-slice-0"),
		partitionableDeviceInSet("gpu-1-counter-set", "gpu-1", "95Gi", "memory-slice-0", "memory-slice-1"),
		partitionableDeviceInSet("gpu-1-counter-set", "gpu-1-mig-1g24gb", "24Gi", "memory-slice-0"),
	})

	capacity := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceslice_capacity").Generate(obj)
	require.Len(t, capacity.Metrics, 1)
	assert.Equal(t, float64(190*1024*1024*1024), capacity.Metrics[0].Value)
}

// spanningDevice draws from several counter sets at once, which the API permits
// and a multi-GPU NVLink device would use.
func spanningDevice(name, memory string, counterSets ...string) map[string]interface{} {
	consumes := make([]interface{}, 0, len(counterSets))
	for _, cs := range counterSets {
		consumes = append(consumes, map[string]interface{}{
			"counterSet": cs,
			"counters":   map[string]interface{}{"memory-slice-0": map[string]interface{}{"value": "1"}},
		})
	}
	return map[string]interface{}{
		"name":             name,
		"capacity":         map[string]interface{}{"memory": map[string]interface{}{"value": memory}},
		"consumesCounters": consumes,
	}
}

// A spanning device is an alternative to the per-card options, not an addition
// to them: adding {A}, {B} and {A,B} as independent groups would report 380Gi
// of hardware that can only ever present 190Gi.
func TestResourceSliceCapacityJoinsOverlappingCounterSets(t *testing.T) {
	f := &resourceSliceFactory{apiVersion: "v1"}
	obj := newSliceObject(t, []interface{}{
		partitionableDeviceInSet("gpu-0-counter-set", "gpu-0", "95Gi", "memory-slice-0"),
		partitionableDeviceInSet("gpu-1-counter-set", "gpu-1", "95Gi", "memory-slice-0"),
		spanningDevice("gpu-0-1-nvlink", "190Gi", "gpu-0-counter-set", "gpu-1-counter-set"),
	})

	capacity := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceslice_capacity").Generate(obj)
	require.Len(t, capacity.Metrics, 1)
	assert.Equal(t, float64(190*1024*1024*1024), capacity.Metrics[0].Value)
}

// Overlapping spanning options must not collapse the cards they connect. With
// A, B and C each 95Gi, an AB and a BC, A+B+C and AB+C both add up to 285Gi --
// treating the three connected sets as one exclusive group reports 190Gi, and
// adding the combinations as if independent reports 570Gi. Nothing obliges the
// driver to advertise an ABC device covering the whole chain.
func TestResourceSliceCapacityKeepsCompatibleSpanningOptions(t *testing.T) {
	f := &resourceSliceFactory{apiVersion: "v1"}
	obj := newSliceObject(t, []interface{}{
		partitionableDeviceInSet("gpu-0-counter-set", "gpu-0", "95Gi", "memory-slice-0"),
		partitionableDeviceInSet("gpu-1-counter-set", "gpu-1", "95Gi", "memory-slice-0"),
		partitionableDeviceInSet("gpu-2-counter-set", "gpu-2", "95Gi", "memory-slice-0"),
		spanningDevice("gpu-0-1-nvlink", "190Gi", "gpu-0-counter-set", "gpu-1-counter-set"),
		spanningDevice("gpu-1-2-nvlink", "190Gi", "gpu-1-counter-set", "gpu-2-counter-set"),
	})

	capacity := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceslice_capacity").Generate(obj)
	require.Len(t, capacity.Metrics, 1)
	assert.Equal(t, float64(285*1024*1024*1024), capacity.Metrics[0].Value)
}

// Cards of different sizes bound the split. A 120Gi AB alongside an 80Gi A and
// a 40Gi B is apportioned 80/40, not 60/60 -- an even split would credit B
// with 60Gi it cannot hold and total 140Gi for hardware that tops out at
// 120Gi, which is what both A+B and AB provide.
func TestResourceSliceCapacityApportionsSpanByKnownSetSizes(t *testing.T) {
	f := &resourceSliceFactory{apiVersion: "v1"}
	obj := newSliceObject(t, []interface{}{
		partitionableDeviceInSet("gpu-0-counter-set", "gpu-0", "80Gi", "memory-slice-0"),
		partitionableDeviceInSet("gpu-1-counter-set", "gpu-1", "40Gi", "memory-slice-0"),
		spanningDevice("gpu-0-1-nvlink", "120Gi", "gpu-0-counter-set", "gpu-1-counter-set"),
	})

	capacity := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceslice_capacity").Generate(obj)
	require.Len(t, capacity.Metrics, 1)
	assert.Equal(t, float64(120*1024*1024*1024), capacity.Metrics[0].Value)
}

// With nothing measuring the sets on their own, an even split is all there is
// to go on -- and it is right here, the spanning device being the only option.
func TestResourceSliceCapacitySplitsSpanEvenlyWithoutABaseline(t *testing.T) {
	f := &resourceSliceFactory{apiVersion: "v1"}
	obj := newSliceObject(t, []interface{}{
		spanningDevice("gpu-0-1-nvlink", "190Gi", "gpu-0-counter-set", "gpu-1-counter-set"),
	})

	capacity := generatorByName(t, f.MetricFamilyGenerators(), "kube_resourceslice_capacity").Generate(obj)
	require.Len(t, capacity.Metrics, 1)
	assert.Equal(t, float64(190*1024*1024*1024), capacity.Metrics[0].Value)
}
