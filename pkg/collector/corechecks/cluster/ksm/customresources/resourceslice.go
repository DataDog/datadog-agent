// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"slices"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	basemetrics "k8s.io/component-base/metrics"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

// ResourceSlice is cluster-scoped, so it carries no namespace label. The node
// and pool it belongs to are more useful than the object name alone, and all
// are kept: the name is the join key back to the API object.
//
// The "node" key is not just a tag: hostnameAndTags in the check treats it, like
// "host", as the metric's hostname (kubernetes_state.go). That is deliberate and
// matches every other node-scoped KSM metric -- a slice's supply belongs to the
// node advertising it -- but the coupling lives in another file, so changing or
// renaming this key silently moves where these metrics land.
//
// The node label is omitted for a slice that advertises devices for the whole
// cluster (spec.allNodes), which has no nodeName; emitting an empty tag value
// there would read as "a node whose name is blank". Those slices therefore keep
// the Cluster Agent's hostname, which is the honest answer: cluster-wide supply
// is not any node's.
//
// Caveat the labels cannot express: a driver splits one pool across several
// slices when it outgrows the object size limit, and during an update two
// generations of the same pool exist briefly. Summing devices across slices is
// therefore per-pool rather than per-node, and can double-count for a moment
// while a pool is being republished.
var (
	descResourceSliceLabels    = []string{"resourceslice", "driver", "pool"}
	descResourceSliceNodeLabel = []string{"node"}
)

// NewResourceSliceFactory returns a factory generating DRA supply metrics from
// ResourceSlice objects. Plain factory: one generator per object, no join.
func NewResourceSliceFactory(client *apiserver.APIClient, apiVersion string) customresource.RegistryFactory {
	return &resourceSliceFactory{client: client.DynamicInformerCl, apiVersion: apiVersion}
}

type resourceSliceFactory struct {
	client     dynamic.Interface
	apiVersion string
}

func (f *resourceSliceFactory) gvr() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    DRAGroup,
		Version:  f.apiVersion,
		Resource: "resourceslices",
	}
}

func (f *resourceSliceFactory) Name() string {
	return "resourceslices"
}

func (f *resourceSliceFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client.Resource(f.gvr()), nil
}

func (f *resourceSliceFactory) ExpectedType() interface{} {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": DRAGroup + "/" + f.apiVersion,
			"kind":       "ResourceSlice",
		},
	}
}

// ListWatch ignores ns: ResourceSlice is cluster-scoped.
func (f *resourceSliceFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	return draListWatch(customResourceClient, ns, fieldSelector, false)
}

func (f *resourceSliceFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_resourceslice_devices_total",
			"The number of advertised devices in a DRA ResourceSlice.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			f.wrap(func(s *unstructured.Unstructured) *metric.Family {
				devices, found := draSliceDevices(s)
				if !found {
					return emptyFamily()
				}
				return &metric.Family{
					Metrics: []*metric.Metric{{
						Value: float64(len(devices)),
					}},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_resourceslice_capacity",
			"Advertised capacity per capacity name, summed across the devices in a DRA ResourceSlice.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			// One series per capacity name rather than one metric per known
			// name: capacity is a map keyed by QualifiedName, so "memory" is
			// one key among many and a non-GPU driver publishes entirely
			// different ones. Baking a key into the metric name would make
			// this collector NVIDIA-specific in a namespace that is not.
			f.wrap(func(s *unstructured.Unstructured) *metric.Family {
				devices, found := draSliceDevices(s)
				if !found {
					return emptyFamily()
				}
				totals := map[string]float64{}
				for _, d := range devices {
					devMap, ok := d.(map[string]interface{})
					if !ok {
						continue
					}
					for name, entry := range draDeviceCapacity(devMap) {
						value, ok := nestedString(entry, "value")
						if !ok {
							continue
						}
						q, err := parseQuantity(value)
						if err != nil {
							continue
						}
						totals[name] += float64(q)
					}
				}
				if len(totals) == 0 {
					return emptyFamily()
				}
				names := make([]string, 0, len(totals))
				for name := range totals {
					names = append(names, name)
				}
				// Deterministic order: the family is compared in tests and the
				// map iteration order is not stable.
				sort.Strings(names)

				metrics := make([]*metric.Metric, 0, len(names))
				for _, name := range names {
					metrics = append(metrics, &metric.Metric{
						LabelKeys:   []string{"capacity"},
						LabelValues: []string{name},
						Value:       totals[name],
					})
				}
				return &metric.Family{Metrics: metrics}
			}),
		),
	}
}

// wrap attaches the identifying labels. Without them every slice produces an
// identical unlabelled series and a cluster with more than one node reports
// only whichever slice was submitted last.
func (f *resourceSliceFactory) wrap(g func(*unstructured.Unstructured) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		u, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return emptyFamily()
		}

		node, _, _ := unstructured.NestedString(u.Object, "spec", "nodeName")
		driver, _, _ := unstructured.NestedString(u.Object, "spec", "driver")
		pool, _, _ := unstructured.NestedString(u.Object, "spec", "pool", "name")

		keys := descResourceSliceLabels
		values := []string{u.GetName(), driver, pool}
		if node != "" {
			keys = append(slices.Clone(keys), descResourceSliceNodeLabel...)
			values = append(values, node)
		}

		family := g(u)
		for _, m := range family.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(keys, values, m.LabelKeys, m.LabelValues)
		}
		return family
	}
}

// parseQuantity parses a Kubernetes quantity string (e.g. "81152Mi") to bytes.
func parseQuantity(value string) (int64, error) {
	q, err := resource.ParseQuantity(value)
	if err != nil {
		return 0, err
	}
	return q.Value(), nil
}
