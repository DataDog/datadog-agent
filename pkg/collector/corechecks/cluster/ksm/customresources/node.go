// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/component-base/metrics"
	basemetrics "k8s.io/component-base/metrics"
	"k8s.io/kube-state-metrics/v2/pkg/constant"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

var descNodeLabelsDefaultLabels = []string{"node"}

// NewExtendedNodeFactory returns a new Node metric family generator factory.
func NewExtendedNodeFactory(client *apiserver.APIClient) customresource.RegistryFactory {
	return &extendedNodeFactory{
		client: client.Cl,
	}
}

type extendedNodeFactory struct {
	client interface{}
}

// Name is the name of the factory
func (f *extendedNodeFactory) Name() string {
	return "nodes_extended"
}

// CreateClient is not implemented
func (f *extendedNodeFactory) CreateClient(cfg *rest.Config) (interface{}, error) {
	return f.client, nil
}

// MetricFamilyGenerators returns the extended node metric family generators
func (f *extendedNodeFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	// At the time of writing this, this is necessary in order for us to have access to the "kubernetes.io/network-bandwidth" resource
	// type, as the default KSM offering explicitly filters out anything that is prefixed with "kubernetes.io/"
	// More information can be found here: https://github.com/kubernetes/kube-state-metrics/issues/2027
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_node_status_extended_capacity",
			"The capacity for different additional resources of a node, which otherwise might have been filtered out by kube-state-metrics.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapNodeFunc(func(n *v1.Node) *metric.Family {
				return f.customResourceGenerator(n.Status.Capacity)
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_node_status_extended_allocatable",
			"The allocatable for different additional resources of a node that are available for scheduling, which otherwise might have been filtered out by kube-state-metrics.",
			metric.Gauge,
			metrics.ALPHA,
			"",
			wrapNodeFunc(func(n *v1.Node) *metric.Family {
				return f.customResourceGenerator(n.Status.Allocatable)
			}),
		),
	}
}

func (f *extendedNodeFactory) customResourceGenerator(resources v1.ResourceList) *metric.Family {
	ms := []*metric.Metric{}

	for resourceName, val := range resources {
		if resourceName == networkBandwidthResourceName {
			ms = append(ms, &metric.Metric{
				LabelValues: []string{
					sanitizeLabelName(string(resourceName)),
					string(constant.UnitByte),
				},
				Value: float64(val.MilliValue()) / 1000,
			})
		}
	}

	for _, metric := range ms {
		metric.LabelKeys = []string{"resource", "unit"}
	}

	return &metric.Family{
		Metrics: ms,
	}
}

func wrapNodeFunc(f func(*v1.Node) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		node := obj.(*v1.Node)

		metricFamily := f(node)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descNodeLabelsDefaultLabels, []string{node.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

// ExpectedType returns the type expected by the factory
func (f *extendedNodeFactory) ExpectedType() interface{} {
	return &v1.Node{}
}

// ListWatch returns a ListerWatcher for v1.Node
func (f *extendedNodeFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(clientset.Interface)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Nodes().List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Nodes().Watch(context.TODO(), opts)
		},
	}
}
