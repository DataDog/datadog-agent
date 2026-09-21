// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

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

// NewDeviceTaintRuleFactory returns a factory generating info metrics from
// DeviceTaintRule objects (KEP-5055, stable in Kubernetes 1.37). A rule
// defines a condition under which matching devices are tainted — e.g. an
// unhealthy device excluded from new allocations. Registered only when the
// cluster serves the resource type (see DeviceTaintRuleSupported).
func NewDeviceTaintRuleFactory(client *apiserver.APIClient, apiVersion string) customresource.RegistryFactory {
	return &deviceTaintRuleFactory{
		client:     client.DynamicInformerCl,
		apiVersion: apiVersion,
	}
}

type deviceTaintRuleFactory struct {
	client     dynamic.Interface
	apiVersion string
}

func (f *deviceTaintRuleFactory) gvr() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    DRAGroup,
		Version:  f.apiVersion,
		Resource: "devicetaintrules",
	}
}

func (f *deviceTaintRuleFactory) Name() string {
	return "devicetaintrules"
}

func (f *deviceTaintRuleFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client.Resource(f.gvr()), nil
}

func (f *deviceTaintRuleFactory) ExpectedType() interface{} {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": DRAGroup + "/" + f.apiVersion,
			"kind":       "DeviceTaintRule",
		},
	}
}

// ListWatch: DeviceTaintRule is cluster-scoped.
func (f *deviceTaintRuleFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	return draListWatch(customResourceClient, "", fieldSelector, false)
}

func (f *deviceTaintRuleFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_devicetaintrule_info",
			"Info about a DeviceTaintRule, which defines a condition under which matching devices are tainted and excluded from new allocations (stable in Kubernetes 1.37).",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			f.wrap(func(r *unstructured.Unstructured) *metric.Family {
				name := r.GetName()
				driver, _, _ := unstructured.NestedString(r.Object, "spec", "deviceSelector", "driver")
				taintKey, _, _ := unstructured.NestedString(r.Object, "spec", "taint", "key")
				taintEffect, _, _ := unstructured.NestedString(r.Object, "spec", "taint", "effect")

				return &metric.Family{
					Metrics: []*metric.Metric{{
						LabelKeys:   []string{"devicetaintrule", "driver", "taint_key", "taint_effect"},
						LabelValues: []string{name, driver, taintKey, taintEffect},
						Value:       1,
					}},
				}
			}),
		),
	}
}

func (f *deviceTaintRuleFactory) wrap(g func(*unstructured.Unstructured) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		u, ok := obj.(*unstructured.Unstructured)
		if !ok {
			log.Debugf("DRA: devicetaintrule informer returned %T, not *unstructured.Unstructured", obj)
			return emptyFamily()
		}
		return g(u)
	}
}

var _ customresource.RegistryFactory = &deviceTaintRuleFactory{}
