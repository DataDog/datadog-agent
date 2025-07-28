// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	basemetrics "k8s.io/component-base/metrics"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// NewDeploymentRolloutFactory returns a new Deployment rollout factory that provides rollout duration metrics
func NewDeploymentRolloutFactory(client *apiserver.APIClient) customresource.RegistryFactory {
	return &deploymentRolloutFactory{
		hybridProvider: newHybridRolloutProvider(client.Cl, 30*time.Second),
	}
}

type deploymentRolloutFactory struct {
	hybridProvider *hybridRolloutProvider
}

func (f *deploymentRolloutFactory) Name() string {
	return "apps/v1, Resource=deployments_rollout"
}

func (f *deploymentRolloutFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.hybridProvider.client, nil
}

func (f *deploymentRolloutFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_deployment_ongoing_rollout_duration",
			"Duration of ongoing Deployment rollout in seconds",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapDeploymentFunc(func(d *appsv1.Deployment) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   []string{"namespace", "deployment"},
							LabelValues: []string{d.Namespace, d.Name},
							Value:       f.hybridProvider.getDeploymentRolloutDuration(d),
						},
					},
				}
			}),
		),
	}
}

func (f *deploymentRolloutFactory) ExpectedType() interface{} {
	return &appsv1.Deployment{}
}

func (f *deploymentRolloutFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(kubernetes.Interface)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.AppsV1().Deployments(ns).List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.AppsV1().Deployments(ns).Watch(context.TODO(), opts)
		},
	}
}


// wrapDeploymentFunc wraps a function that takes a Deployment and returns a metric Family
func wrapDeploymentFunc(f func(*appsv1.Deployment) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		deployment := obj.(*appsv1.Deployment)
		return f(deployment)
	}
}