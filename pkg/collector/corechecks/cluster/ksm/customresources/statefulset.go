// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"context"

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

// NewStatefulSetRolloutFactory returns a new StatefulSet rollout factory that provides rollout duration metrics
func NewStatefulSetRolloutFactory(client *apiserver.APIClient, rolloutTracker RolloutOperations) customresource.RegistryFactory {
	return &statefulSetRolloutFactory{
		client:         client.Cl,
		rolloutTracker: rolloutTracker,
	}
}

type statefulSetRolloutFactory struct {
	client         kubernetes.Interface
	rolloutTracker RolloutOperations
}

func (f *statefulSetRolloutFactory) Name() string {
	return "statefulsets_extended"
}

func (f *statefulSetRolloutFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client, nil
}

func (f *statefulSetRolloutFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_statefulset_ongoing_rollout_duration",
			"Duration of ongoing StatefulSet rollout in seconds",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapStatefulSetFunc(func(sts *appsv1.StatefulSet) *metric.Family {

				isNewRollout := sts.Generation != sts.Status.ObservedGeneration
				// handles cases where we should consider a stateful set without a generation
				// change an active rollout
				hasRevisionMismatch := f.rolloutTracker.HasStatefulSetRolloutCondition(sts)
				isOngoing := isNewRollout || hasRevisionMismatch

				if isOngoing {
					f.rolloutTracker.StoreStatefulSet(sts)

					// Return dummy metric with value 1 to trigger transformer
					return &metric.Family{
						Metrics: []*metric.Metric{
							{
								LabelKeys:   []string{"namespace", "statefulset"},
								LabelValues: []string{sts.Namespace, sts.Name},
								Value:       1, // Dummy value - transformer will calculate real duration
							},
						},
					}
				}

				// Rollout complete - cleanup and return 0
				f.rolloutTracker.CleanupStatefulSet(sts.Namespace, sts.Name)
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   []string{"namespace", "statefulset"},
							LabelValues: []string{sts.Namespace, sts.Name},
							Value:       0,
						},
					},
				}
			}),
		),
	}
}

func (f *statefulSetRolloutFactory) ExpectedType() interface{} {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
	}
}

func (f *statefulSetRolloutFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(kubernetes.Interface)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.AppsV1().StatefulSets(ns).List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.AppsV1().StatefulSets(ns).Watch(context.TODO(), opts)
		},
	}
}

func wrapStatefulSetFunc(f func(*appsv1.StatefulSet) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		statefulset := obj.(*appsv1.StatefulSet)
		return f(statefulset)
	}
}
