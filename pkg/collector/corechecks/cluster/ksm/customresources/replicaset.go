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

// NewReplicaSetRolloutFactory returns a new ReplicaSet rollout factory that tracks ReplicaSet events for deployment rollouts
func NewReplicaSetRolloutFactory(client *apiserver.APIClient, rolloutTracker RolloutOperations) customresource.RegistryFactory {
	return &replicaSetRolloutFactory{
		client:         client.Cl,
		rolloutTracker: rolloutTracker,
	}
}

type replicaSetRolloutFactory struct {
	client         kubernetes.Interface
	rolloutTracker RolloutOperations
}

func (f *replicaSetRolloutFactory) Name() string {
	return "replicasets_extended"
}

func (f *replicaSetRolloutFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client, nil
}

func (f *replicaSetRolloutFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_replicaset_rollout_tracker",
			"Tracks ReplicaSets for deployment rollout duration calculation",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapReplicaSetFunc(func(rs *appsv1.ReplicaSet) *metric.Family {
				// Store ReplicaSet info if it's owned by a Deployment
				ownerName, ownerUID := f.getDeploymentOwner(rs)
				if ownerName != "" && ownerUID != "" {
					f.rolloutTracker.StoreReplicaSet(rs, ownerName, ownerUID)
				}

				// Return empty metric family - we don't emit actual metrics for ReplicaSets
				return &metric.Family{
					Metrics: []*metric.Metric{},
				}
			}),
		),
	}
}

func (f *replicaSetRolloutFactory) ExpectedType() interface{} {
	return &appsv1.ReplicaSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ReplicaSet",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
	}
}

func (f *replicaSetRolloutFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(kubernetes.Interface)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.AppsV1().ReplicaSets(ns).List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.AppsV1().ReplicaSets(ns).Watch(context.TODO(), opts)
		},
	}
}

// getDeploymentOwner returns the name and UID of the Deployment that owns this ReplicaSet
func (f *replicaSetRolloutFactory) getDeploymentOwner(rs *appsv1.ReplicaSet) (string, string) {
	for _, owner := range rs.OwnerReferences {
		if owner.Kind == "Deployment" {
			return owner.Name, string(owner.UID)
		}
	}
	return "", ""
}

func wrapReplicaSetFunc(f func(*appsv1.ReplicaSet) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		replicaSet := obj.(*appsv1.ReplicaSet)
		return f(replicaSet)
	}
}
