// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"

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

var descResourceClaimDefaultLabels = []string{"namespace", "resourceclaim"}

// NewResourceClaimFactory returns a factory generating DRA demand metrics from
// ResourceClaim objects. Plain factory: one generator per object, no join.
//
// deviceClasses filters which claims are counted; an empty list counts them
// all. Filtering is by DeviceClass rather than by driver: the two names
// coincide for NVIDIA but diverge in general (DraNet exposes DeviceClass
// "dranet" under driver "dra.net"), and classifying by driver makes the
// pending and allocated metrics disagree about the same claim.
func NewResourceClaimFactory(client *apiserver.APIClient, apiVersion string, deviceClasses []string) customresource.RegistryFactory {
	return &resourceClaimFactory{
		client:        client.DynamicInformerCl,
		apiVersion:    apiVersion,
		deviceClasses: deviceClasses,
	}
}

type resourceClaimFactory struct {
	client        dynamic.Interface
	apiVersion    string
	deviceClasses []string
}

func (f *resourceClaimFactory) gvr() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    DRAGroup,
		Version:  f.apiVersion,
		Resource: "resourceclaims",
	}
}

func (f *resourceClaimFactory) Name() string {
	return "resourceclaims"
}

func (f *resourceClaimFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client.Resource(f.gvr()), nil
}

func (f *resourceClaimFactory) ExpectedType() interface{} {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": DRAGroup + "/" + f.apiVersion,
			"kind":       "ResourceClaim",
		},
	}
}

// ListWatch scopes to ns: ResourceClaim is namespaced.
func (f *resourceClaimFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	return draListWatch(customResourceClient, ns, fieldSelector, true)
}

func (f *resourceClaimFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_resourceclaim_status",
			"The state of a DRA ResourceClaim: pending, allocated or in_use.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			f.wrap(func(c *unstructured.Unstructured) *metric.Family {
				state := claimState(c)
				return &metric.Family{
					Metrics: []*metric.Metric{{
						LabelKeys:   []string{"state"},
						LabelValues: []string{state},
						Value:       1,
					}},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_resourceclaim_created",
			"Unix creation timestamp of a pending DRA ResourceClaim.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			// Emitted as a timestamp, not as an elapsed duration: the KSM
			// metrics store generates and caches a family when the informer
			// sees the object, not when the endpoint is scraped, so any
			// time.Since() here would freeze at the value it had when the
			// claim first appeared -- exactly zero for a claim that is born
			// pending, and never updated while it stays that way. The wait
			// time is `now - kube_resourceclaim_created`, computed at query
			// time, which is also how the max across claims is obtained.
			f.wrap(func(c *unstructured.Unstructured) *metric.Family {
				if claimState(c) != claimStatePending {
					return emptyFamily()
				}
				created := c.GetCreationTimestamp()
				if created.IsZero() {
					return emptyFamily()
				}
				return &metric.Family{
					Metrics: []*metric.Metric{{
						Value: float64(created.Unix()),
					}},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_resourceclaim_devices_allocated",
			"The number of devices allocated to a DRA ResourceClaim.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			f.wrap(func(c *unstructured.Unstructured) *metric.Family {
				results, found, _ := unstructured.NestedSlice(c.Object, "status", "allocation", "devices", "results")
				if !found {
					return emptyFamily()
				}
				return &metric.Family{
					Metrics: []*metric.Metric{{
						Value: float64(len(results)),
					}},
				}
			}),
		),
	}
}

// Claim states, as reported by the `state` tag. They are mutually exclusive
// points on one lifecycle rather than independent properties, so "in_use" is
// used for a claim that is allocated *and* reserved: calling that one
// "reserved" reads as a separate condition and makes the obvious query for
// allocated claims -- state:allocated -- return near zero on a healthy cluster,
// where almost every allocated claim is in use.
const (
	claimStatePending   = "pending"
	claimStateAllocated = "allocated" // allocated, not yet consumed by a pod
	claimStateInUse     = "in_use"    // allocated and reserved for at least one pod
)

// claimState derives the DRA claim state from its status.
func claimState(c *unstructured.Unstructured) string {
	if _, found, _ := unstructured.NestedMap(c.Object, "status", "allocation"); found {
		if reserved, found, _ := unstructured.NestedSlice(c.Object, "status", "reservedFor"); found && len(reserved) > 0 {
			return claimStateInUse
		}
		return claimStateAllocated
	}
	return claimStatePending
}

// claimDeviceClasses returns every DeviceClass the claim requests. A claim
// records the class on each request in its spec, which is what allocation
// results point back to.
func claimDeviceClasses(c *unstructured.Unstructured) []string {
	requests, found, _ := unstructured.NestedSlice(c.Object, "spec", "devices", "requests")
	if !found {
		return nil
	}
	var classes []string
	for _, r := range requests {
		req, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		// v1 and v1beta2 wrap an ordinary request in "exactly"; v1beta1 keeps
		// deviceClassName inline. Reading only the inline form matches nothing
		// on v1 -- the version most clusters negotiate -- so every claim is
		// filtered out and the metrics silently report zero.
		if class, found, _ := unstructured.NestedString(req, "exactly", "deviceClassName"); found && class != "" {
			classes = append(classes, class)
			continue
		}
		if class, found, _ := unstructured.NestedString(req, "deviceClassName"); found && class != "" {
			classes = append(classes, class)
			continue
		}
		// A request may instead carry alternatives, each with its own class.
		// DeviceSubRequest keeps deviceClassName inline in every version.
		firstAvailable, found, _ := unstructured.NestedSlice(req, "firstAvailable")
		if !found {
			continue
		}
		for _, a := range firstAvailable {
			alt, ok := a.(map[string]interface{})
			if !ok {
				continue
			}
			if class, found, _ := unstructured.NestedString(alt, "deviceClassName"); found && class != "" {
				classes = append(classes, class)
			}
		}
	}
	return classes
}

// matchesDeviceClasses reports whether the claim requests any of the configured
// classes. An empty configuration matches everything.
func matchesDeviceClasses(c *unstructured.Unstructured, deviceClasses []string) bool {
	if len(deviceClasses) == 0 {
		return true
	}
	for _, class := range claimDeviceClasses(c) {
		for _, wanted := range deviceClasses {
			if class == wanted {
				return true
			}
		}
	}
	return false
}

// wrap applies the DeviceClass filter and attaches the namespace/name labels
// every other factory in this package attaches. Without them each object
// produces an identical unlabelled series, and a cluster with more than one
// claim reports the value of whichever one was submitted last.
func (f *resourceClaimFactory) wrap(g func(*unstructured.Unstructured) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		u, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return emptyFamily()
		}
		if !matchesDeviceClasses(u, f.deviceClasses) {
			return emptyFamily()
		}

		family := g(u)
		for _, m := range family.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(
				descResourceClaimDefaultLabels,
				[]string{u.GetNamespace(), u.GetName()},
				m.LabelKeys, m.LabelValues,
			)
		}
		return family
	}
}
