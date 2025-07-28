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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewStatefulSetRolloutFactory returns a new StatefulSet rollout factory that provides rollout duration metrics
func NewStatefulSetRolloutFactory(client *apiserver.APIClient) customresource.RegistryFactory {
	return &statefulSetRolloutFactory{
		client: client.Cl,
	}
}

type statefulSetRolloutFactory struct {
	client kubernetes.Interface
}

func (f *statefulSetRolloutFactory) Name() string {
	return "apps/v1, Resource=statefulsets_rollout"
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
			wrapStatefulSetFunc(func(s *appsv1.StatefulSet) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   []string{"namespace", "statefulset"},
							LabelValues: []string{s.Namespace, s.Name},
							Value:       f.getRolloutDuration(s),
						},
					},
				}
			}),
		),
	}
}

func (f *statefulSetRolloutFactory) ExpectedType() interface{} {
	return &appsv1.StatefulSet{}
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

// getRolloutDuration calculates the duration of an ongoing rollout
func (f *statefulSetRolloutFactory) getRolloutDuration(s *appsv1.StatefulSet) float64 {
	// Check if there's a rollout in progress
	if s.Status.CurrentRevision == s.Status.UpdateRevision {
		// No rollout in progress
		return 0
	}

	// There's a generation mismatch, we need to find the ControllerRevision timestamp
	// for the current update revision
	if s.Status.UpdateRevision == "" {
		// No update revision available
		return 0
	}

	// Get the ControllerRevision for the update revision
	revision, err := f.client.AppsV1().ControllerRevisions(s.Namespace).Get(
		context.TODO(),
		s.Status.UpdateRevision,
		metav1.GetOptions{},
	)
	if err != nil {
		log.Debugf("Failed to get ControllerRevision %s for StatefulSet %s/%s: %v", 
			s.Status.UpdateRevision, s.Namespace, s.Name, err)
		return 0
	}

	// Calculate duration since the revision was created
	if revision.CreationTimestamp.IsZero() {
		return 0
	}

	duration := time.Since(revision.CreationTimestamp.Time)
	return duration.Seconds()
}

// wrapStatefulSetFunc wraps a function that takes a StatefulSet and returns a metric Family
func wrapStatefulSetFunc(f func(*appsv1.StatefulSet) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		statefulSet := obj.(*appsv1.StatefulSet)
		return f(statefulSet)
	}
}