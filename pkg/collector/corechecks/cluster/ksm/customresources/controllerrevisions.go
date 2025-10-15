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

// NewControllerRevisionRolloutFactory returns a new ControllerRevision rollout factory that tracks ControllerRevision events for StatefulSet rollouts
func NewControllerRevisionRolloutFactory(client *apiserver.APIClient, rolloutTracker RolloutOperations) customresource.RegistryFactory {
	return &controllerRevisionRolloutFactory{
		client:         client.Cl,
		rolloutTracker: rolloutTracker,
	}
}

type controllerRevisionRolloutFactory struct {
	client         kubernetes.Interface
	rolloutTracker RolloutOperations
}

func (f *controllerRevisionRolloutFactory) Name() string {
	return "controllerrevisions"
}

func (f *controllerRevisionRolloutFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client, nil
}

func (f *controllerRevisionRolloutFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_controllerrevision_rollout_tracker",
			"Tracks ControllerRevisions for StatefulSet rollout duration calculation",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapControllerRevisionFunc(func(cr *appsv1.ControllerRevision) *metric.Family {
				// Store ControllerRevision info if it's owned by a StatefulSet
				ownerName, ownerUID := f.getStatefulSetOwner(cr)
				if ownerName != "" && ownerUID != "" {
					f.rolloutTracker.StoreControllerRevision(cr, ownerName, ownerUID)
				}

				// Return empty metric family - we don't emit actual metrics for ControllerRevisions
				return &metric.Family{
					Metrics: []*metric.Metric{},
				}
			}),
		),
	}
}

func (f *controllerRevisionRolloutFactory) ExpectedType() interface{} {
	return &appsv1.ControllerRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ControllerRevision",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
	}
}

func (f *controllerRevisionRolloutFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(kubernetes.Interface)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.AppsV1().ControllerRevisions(ns).List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.AppsV1().ControllerRevisions(ns).Watch(context.TODO(), opts)
		},
	}
}

// getStatefulSetOwner returns the name and UID of the StatefulSet that owns this ControllerRevision
func (f *controllerRevisionRolloutFactory) getStatefulSetOwner(cr *appsv1.ControllerRevision) (string, string) {
	for _, owner := range cr.OwnerReferences {
		if owner.Kind == "StatefulSet" {
			return owner.Name, string(owner.UID)
		}
	}
	return "", ""
}

func wrapControllerRevisionFunc(f func(*appsv1.ControllerRevision) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		controllerRevision := obj.(*appsv1.ControllerRevision)
		return f(controllerRevision)
	}
}
