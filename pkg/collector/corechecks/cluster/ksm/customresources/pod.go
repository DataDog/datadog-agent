// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/v2/pkg/constant"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

const (
	resourceRequests = "requests"
	resourcelimits   = "limits"
)

var descPodLabelsDefaultLabels = []string{"namespace", "pod", "uid"}

// NewExtendedPodFactory returns a new Pod metric family generator factory.
func NewExtendedPodFactory(client *apiserver.APIClient) customresource.RegistryFactory {
	return &extendedPodFactory{
		client: client.Cl,
	}
}

type extendedPodFactory struct {
	client interface{}
}

// Name is the name of the factory
func (f *extendedPodFactory) Name() string {
	return "pods_extended"
}

// CreateClient is not implemented
func (f *extendedPodFactory) CreateClient(cfg *rest.Config) (interface{}, error) {
	return f.client, nil
}

// MetricFamilyGenerators returns the extended pod metric family generators
func (f *extendedPodFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	// At the time of writing this, this is necessary in order for us to have access to the "kubernetes.io/network-bandwidth" resource
	// type, as the default KSM offering explicitly filters out anything that is prefixed with "kubernetes.io/"
	// More information can be found here: https://github.com/kubernetes/kube-state-metrics/issues/2027
	return []generator.FamilyGenerator{
		*generator.NewFamilyGenerator(
			"kube_pod_container_extended_resource_requests",
			"The number of additional requested request resource by a container, which otherwise might have been filtered out by kube-state-metrics.",
			metric.Gauge,
			"",
			wrapPodFunc(func(p *v1.Pod) *metric.Family {
				return f.customResourceGenerator(p, resourceRequests)
			}),
		),
		*generator.NewFamilyGenerator(
			"kube_pod_container_extended_resource_limits",
			"The number of additional requested limit resource by a container, which otherwise might have been filtered out by kube-state-metrics.",
			metric.Gauge,
			"",
			wrapPodFunc(func(p *v1.Pod) *metric.Family {
				return f.customResourceGenerator(p, resourcelimits)
			}),
		),
	}
}

func (f *extendedPodFactory) customResourceGenerator(p *v1.Pod, resourceType string) *metric.Family {
	ms := []*metric.Metric{}

	for _, c := range p.Spec.Containers {
		var resources v1.ResourceList
		switch resourceType {
		case resourceRequests:
			resources = c.Resources.Requests
		case resourcelimits:
			resources = c.Resources.Limits
		default:
			log.Warnf("unknown resource type requested for pod container resources: %s", resourceType)
		}

		for resourceName, val := range resources {
			if resourceName == networkBandwidthResourceName {
				ms = append(ms, &metric.Metric{
					LabelValues: []string{c.Name, p.Spec.NodeName, sanitizeLabelName(string(resourceName)), string(constant.UnitByte)},
					Value:       float64(val.Value()),
				})
			}
		}
	}

	for _, metric := range ms {
		metric.LabelKeys = []string{"container", "node", "resource", "unit"}
	}

	return &metric.Family{
		Metrics: ms,
	}
}

func wrapPodFunc(f func(*v1.Pod) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		pod := obj.(*v1.Pod)

		metricFamily := f(pod)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descPodLabelsDefaultLabels, []string{pod.Namespace, pod.Name, string(pod.UID)}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

// ExpectedType returns the type expected by the factory
func (f *extendedPodFactory) ExpectedType() interface{} {
	return &v1.Pod{}
}

// ListWatch returns a ListerWatcher for v1.Pod
func (f *extendedPodFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(clientset.Interface)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Pods(ns).List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Pods(ns).Watch(context.TODO(), opts)
		},
	}
}
