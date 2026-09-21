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
			"Advertised capacity per capacity name across the devices in a DRA ResourceSlice.",
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
				totals := draSliceCapacityTotals(devices)
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

// parseQuantity parses a Kubernetes quantity string (e.g. "81152Mi") to a
// float64. Value() rounds away from zero to an integer, which corrupts
// fractional capacities: two devices advertising "500m" would report total
// capacity 2 instead of 1. AsApproximateFloat64 preserves the fraction.
func parseQuantity(value string) (float64, error) {
	q, err := resource.ParseQuantity(value)
	if err != nil {
		return 0, err
	}
	return q.AsApproximateFloat64(), nil
}

// draSliceCapacityTotals reduces a slice's devices to one value per capacity
// name.
//
// Devices are additive only when they are independent. A partitionable device
// declares consumesCounters: it draws from a pool-wide budget, and the entries
// that do so are alternative ways to cut the same hardware, not extra
// hardware. Summing them multiplies the real capacity by however many options
// compete for each unit -- on an RTX PRO 6000 advertising 31 placements over
// 12 memory slices that is a 12x overcount, 1136Gi reported against 95Gi.
//
// Exclusion holds within a counter set, and sets are joined into one group by
// any device that draws from several at once -- which the API permits and a
// multi-GPU NVLink device would use. Independent devices add; each group
// contributes its largest option, since "do not partition" is itself an
// advertised entry consuming the whole group. On a four-GPU node that yields
// four groups of 95Gi; given devices A, B and a 190Gi AB spanning both, one
// group of 190Gi rather than 380Gi.
//
// Taking this from the devices rather than from spec.sharedCounters is
// deliberate. The counter set also carries allocation tokens --
// memory-slice-0..11, value 1 each -- that exist only to express which
// placements conflict. They are not capacity, the API offers nothing to tell
// them apart from a real counter, and no device advertises them, so reading
// the device side leaves them out by construction rather than by a name
// filter. Verified against sharedCounters on twelve live pools: the two agree
// exactly for every capacity key.
//
// A group's maximum understates only if the device joining two sets is smaller
// than the disjoint alternative, which would describe hardware that shrinks
// when combined. Exactness there is a knapsack over the counter budgets, which
// is the scheduler's job, not a collector's.
func draSliceCapacityTotals(devices []interface{}) map[string]float64 {
	type deviceCapacity struct {
		sets     []string
		capacity map[string]float64
	}

	parsed := make([]deviceCapacity, 0, len(devices))
	groups := newCounterSetGroups()
	for _, d := range devices {
		devMap, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		capacity := map[string]float64{}
		for name, entry := range draDeviceCapacity(devMap) {
			// In the unstructured API object each resource.Quantity is a plain
			// string (e.g. "80Gi"), not an object with a "value" key. Some test
			// shapes wrap it; accept both.
			value, ok := entry.(string)
			if !ok {
				value, ok = nestedString(entry, "value")
				if !ok {
					continue
				}
			}
			q, err := parseQuantity(value)
			if err != nil {
				continue
			}
			capacity[name] = q
		}
		if len(capacity) == 0 {
			continue
		}
		sets := draDeviceCounterSets(devMap)
		groups.join(sets)
		parsed = append(parsed, deviceCapacity{sets: sets, capacity: capacity})
	}

	totals := map[string]float64{}
	// group root -> capacity name -> largest option in that group.
	maxes := map[string]map[string]float64{}
	for _, d := range parsed {
		if len(d.sets) == 0 {
			for name, q := range d.capacity {
				totals[name] += q
			}
			continue
		}
		root := groups.root(d.sets[0])
		perGroup := maxes[root]
		if perGroup == nil {
			perGroup = map[string]float64{}
			maxes[root] = perGroup
		}
		for name, q := range d.capacity {
			if cur, seen := perGroup[name]; !seen || q > cur {
				perGroup[name] = q
			}
		}
	}
	for _, perGroup := range maxes {
		for name, q := range perGroup {
			totals[name] += q
		}
	}
	return totals
}

// counterSetGroups is a union-find over counter-set names. Two sets belong to
// the same group once a device draws from both, because then the options
// backed by either are alternatives to that device rather than to each other.
type counterSetGroups struct {
	parent map[string]string
}

func newCounterSetGroups() *counterSetGroups {
	return &counterSetGroups{parent: map[string]string{}}
}

func (g *counterSetGroups) root(name string) string {
	parent, seen := g.parent[name]
	if !seen {
		g.parent[name] = name
		return name
	}
	if parent == name {
		return name
	}
	r := g.root(parent)
	g.parent[name] = r // path compression
	return r
}

func (g *counterSetGroups) join(names []string) {
	if len(names) == 0 {
		return
	}
	first := g.root(names[0])
	for _, other := range names[1:] {
		g.parent[g.root(other)] = first
	}
}
