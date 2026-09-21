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
// Capacity is therefore attributed to counter sets, which are the budgets the
// hardware actually has: each set keeps the largest capacity any of its
// devices can account for, and the sets add up. A device drawing from several
// sets at once -- which the API permits and a multi-GPU NVLink device would
// use -- spreads its capacity across them, since that capacity exists only by
// spanning all of them.
//
// This is what makes overlapping options come out right. Three 95Gi cards
// advertised alongside a 190Gi AB and a 190Gi BC give each set 95Gi and a
// total of 285Gi: A+B+C and AB+C can both be realised, and both are 285Gi.
// Treating the connected sets as one exclusive group would report 190Gi, and
// adding the combinations as if independent would report 570Gi.
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
// A spanning device's capacity is apportioned using what its sets are
// independently known to be worth, measured by the devices that draw on one
// set alone. Splitting it evenly instead would hand a set more than it can
// hold: with an 80Gi A, a 40Gi B and a 120Gi AB, half of 120 gives B 60Gi and
// the total comes to 140Gi for hardware that tops out at 120Gi.
//
// A set the slice never measures on its own is not worth zero, it is unknown,
// and weighting it at zero would push the whole span onto its neighbours --
// 95Gi A and C with a 190Gi AB and BC and no standalone B would report 380Gi
// instead of 285Gi. Measured sets therefore keep their measured value and
// whatever the span has left over is divided among the unmeasured ones, which
// infers the missing B at 95Gi. Only when every set is measured is the span
// divided in proportion, so that a span larger than its parts still counts
// for its full size.
//
// The measurements are kept apart from these inferences on purpose: reading
// them back would make one span's estimate the next span's baseline, and the
// total would depend on the order the devices happen to appear in. Three
// 190Gi spans AB, BC, CD came to 475Gi or 380Gi depending on that order.
//
// It stays an approximation. The API does not state how much of a spanning
// device's capacity each set contributes: consumesCounters names the counters
// it draws, but those names are the driver's own and need not match the
// capacity names -- an NVIDIA slice spells them copy-engines and copyEngines
// on the two sides -- so they cannot be joined to apportion it exactly.
func draSliceCapacityTotals(devices []interface{}) map[string]float64 {
	type parsedDevice struct {
		sets     []string
		capacity map[string]float64
	}

	totals := map[string]float64{}
	parsed := make([]parsedDevice, 0, len(devices))
	// counter set -> capacity name -> largest capacity attributable to it.
	perSet := map[string]map[string]float64{}
	// Only what single-set devices measured. Read while apportioning spans and
	// never written by them, so no span can become another span's baseline.
	measured := map[string]map[string]float64{}
	attribute := func(into map[string]map[string]float64, set, name string, q float64) {
		attributed := into[set]
		if attributed == nil {
			attributed = map[string]float64{}
			into[set] = attributed
		}
		if cur, seen := attributed[name]; !seen || q > cur {
			attributed[name] = q
		}
	}

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
		switch len(sets) {
		case 0:
			for name, q := range capacity {
				totals[name] += q
			}
		case 1:
			// Measures its set on its own; the baseline the spanning devices
			// below are apportioned against.
			for name, q := range capacity {
				attribute(measured, sets[0], name, q)
				attribute(perSet, sets[0], name, q)
			}
		default:
			parsed = append(parsed, parsedDevice{sets: sets, capacity: capacity})
		}
	}

	for _, d := range parsed {
		for name, q := range d.capacity {
			known := 0.0
			unmeasured := 0
			for _, set := range d.sets {
				if v, ok := measured[set][name]; ok {
					known += v
				} else {
					unmeasured++
				}
			}
			switch {
			case unmeasured > 0:
				// Measured sets stand on their own measurement; the remainder
				// is what the unmeasured ones have to account for.
				remainder := q - known
				if remainder < 0 {
					remainder = 0
				}
				for _, set := range d.sets {
					if v, ok := measured[set][name]; ok {
						attribute(perSet, set, name, v)
						continue
					}
					attribute(perSet, set, name, remainder/float64(unmeasured))
				}
			case known > 0:
				for _, set := range d.sets {
					attribute(perSet, set, name, q*measured[set][name]/known)
				}
			default:
				// Every set measured, all at zero: nothing to weight by.
				for _, set := range d.sets {
					attribute(perSet, set, name, q/float64(len(d.sets)))
				}
			}
		}
	}

	for _, attributed := range perSet {
		for name, q := range attributed {
			totals[name] += q
		}
	}
	return totals
}
