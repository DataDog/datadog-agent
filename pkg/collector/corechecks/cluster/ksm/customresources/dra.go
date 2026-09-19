// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"context"
	"fmt"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
)

// DRAGroup is the API group serving ResourceClaim and ResourceSlice.
const DRAGroup = "resource.k8s.io"

// draServedVersions lists the DRA API versions this code understands, newest
// first. The group graduated late (v1 in 1.34), so clusters in the field still
// serve only a beta version and the factories must follow whichever one the
// API server actually offers rather than pinning v1.
var draServedVersions = []string{"v1", "v1beta2", "v1beta1"}

// DRASupportedVersions returns the DRA API versions this code understands,
// newest first.
func DRASupportedVersions() []string {
	return slices.Clone(draServedVersions)
}

// DRAAPIVersion returns the newest DRA API version the cluster serves, or ""
// when the group is absent. Callers must treat "" as "no DRA on this cluster"
// and register neither the factories nor their collectors: enabling a store
// whose API group does not exist starts an informer that can only fail, which
// on the overwhelming majority of clusters is pure error noise.
func DRAAPIVersion(resources []*metav1.APIResourceList) string {
	served := map[string]struct{}{}
	for _, list := range resources {
		if list == nil {
			continue
		}
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil || gv.Group != DRAGroup {
			continue
		}
		// A group version can be advertised with no resources in it.
		if len(list.APIResources) == 0 {
			continue
		}
		served[gv.Version] = struct{}{}
	}
	for _, v := range draServedVersions {
		if _, ok := served[v]; ok {
			return v
		}
	}
	return ""
}

// draListWatch builds a ListerWatcher over a dynamic client, scoped to ns when
// the resource is namespaced and a namespace was requested. Resource() returns
// a NamespaceableResourceInterface; narrowing it to ResourceInterface too early
// discards Namespace() and silently turns a namespace-scoped configuration into
// a cluster-wide list.
func draListWatch(customResourceClient interface{}, ns string, fieldSelector string, namespaced bool) cache.ListerWatcher {
	var client dynamic.ResourceInterface
	switch c := customResourceClient.(type) {
	case dynamic.NamespaceableResourceInterface:
		if namespaced && ns != "" && ns != metav1.NamespaceAll {
			client = c.Namespace(ns)
		} else {
			client = c
		}
	case dynamic.ResourceInterface:
		client = c
	default:
		// Returning nil here would reach cache.NewReflector and panic on Run
		// rather than skipping the store, so hand back a ListerWatcher that
		// fails cleanly and lets the reflector back off.
		err := fmt.Errorf("DRA: unexpected custom resource client type %T", customResourceClient)
		log.Errorf("%s", err)
		return &cache.ListWatch{
			ListFunc:  func(metav1.ListOptions) (runtime.Object, error) { return nil, err },
			WatchFunc: func(metav1.ListOptions) (watch.Interface, error) { return nil, err },
		}
	}

	ctx := context.Background()
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.List(ctx, opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.Watch(ctx, opts)
		},
	}
}

// draSliceDevices returns a ResourceSlice's device list without deep-copying
// it. NestedSlice copies the whole list -- for every generator, on every event
// and every resync -- and the generators only read it.
func draSliceDevices(slice *unstructured.Unstructured) ([]interface{}, bool) {
	spec, ok := slice.Object["spec"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	devices, ok := spec["devices"].([]interface{})
	return devices, ok
}

func nestedString(entry interface{}, field string) (string, bool) {
	m, ok := entry.(map[string]interface{})
	if !ok {
		return "", false
	}
	s, ok := m[field].(string)
	return s, ok
}

// nestedMapNoCopy reads one nested map without deep-copying it.
func nestedMapNoCopy(obj interface{}, field string) map[string]interface{} {
	m, ok := obj.(map[string]interface{})
	if !ok {
		return nil
	}
	nested, ok := m[field].(map[string]interface{})
	if !ok {
		return nil
	}
	return nested
}

// draDeviceCapacity returns a DRA device's capacity map, unwrapping v1beta1's
// "basic" nesting the same way as draDeviceAttributes.
func draDeviceCapacity(device map[string]interface{}) map[string]interface{} {
	if basic, found, _ := unstructured.NestedFieldNoCopy(device, "basic"); found {
		return nestedMapNoCopy(basic, "capacity")
	}
	return nestedMapNoCopy(device, "capacity")
}

// emptyFamily is the "this object contributes no sample" return value.
func emptyFamily() *metric.Family {
	return &metric.Family{Metrics: []*metric.Metric{}}
}
