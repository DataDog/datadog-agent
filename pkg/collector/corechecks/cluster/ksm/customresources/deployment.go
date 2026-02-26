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

// NewDeploymentRolloutFactory returns a new Deployment rollout factory that provides rollout duration metrics
func NewDeploymentRolloutFactory(client *apiserver.APIClient, rolloutTracker RolloutOperations) customresource.RegistryFactory {
	return &deploymentRolloutFactory{
		client:         client.Cl,
		rolloutTracker: rolloutTracker,
	}
}

type deploymentRolloutFactory struct {
	client         kubernetes.Interface
	rolloutTracker RolloutOperations
}

func (f *deploymentRolloutFactory) Name() string {
	return "deployments_extended"
}

func (f *deploymentRolloutFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client, nil
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
				// Get the current revision annotation - this changes on actual rollouts/rollbacks
				// but NOT on scaling, pause, or other non-template changes
				currentRevision := d.Annotations[RevisionAnnotationKey]

				// Check if this is a generation mismatch (Kubernetes hasn't caught up yet)
				generationMismatch := d.Generation != d.Status.ObservedGeneration

				// Check if the revision actually changed - this distinguishes real rollouts
				// from scaling/pause operations which change generation but not revision
				revisionChanged := f.rolloutTracker.HasRevisionChanged(d.Namespace, d.Name, currentRevision)

				// Check if we're already tracking this deployment
				isActivelyTracked := f.rolloutTracker.HasActiveRollout(d)

				// Check if Kubernetes reports an active rollout condition
				hasRolloutCondition := f.rolloutTracker.HasRolloutCondition(d)

				// A rollout is ongoing if:
				// 1. Revision changed AND (generation mismatch OR rollout condition active)
				//    - This catches both normal rollouts AND fast reconciliation cases where
				//      generation catches up quickly but pods are still rolling
				// 2. OR we're already tracking AND has rollout condition (continuing rollout)
				//
				// Note: If a rollout completes entirely between checks (< 15s), we won't report it.
				// This is acceptable - we only need to report rollouts that are ongoing when checked.
				isOngoing := (revisionChanged && (generationMismatch || hasRolloutCondition)) ||
					(isActivelyTracked && hasRolloutCondition)

				// Always update the last seen revision to track state across scrapes
				// This must be called regardless of isOngoing to maintain accurate revision tracking
				defer f.rolloutTracker.UpdateLastSeenRevision(d.Namespace, d.Name, currentRevision)

				if isOngoing {
					f.rolloutTracker.StoreDeployment(d)

					// Return dummy metric with value 1 to trigger transformer
					return &metric.Family{
						Metrics: []*metric.Metric{
							{
								LabelKeys:   []string{"namespace", "deployment"},
								LabelValues: []string{d.Namespace, d.Name},
								Value:       1, // Dummy value - transformer will calculate real duration
							},
						},
					}
				}
				// Rollout complete - cleanup and return 0
				f.rolloutTracker.CleanupDeployment(d.Namespace, d.Name)
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   []string{"namespace", "deployment"},
							LabelValues: []string{d.Namespace, d.Name},
							Value:       0,
						},
					},
				}
			}),
		),
	}
}

func (f *deploymentRolloutFactory) ExpectedType() interface{} {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
	}
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

func wrapDeploymentFunc(f func(*appsv1.Deployment) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		deployment := obj.(*appsv1.Deployment)
		return f(deployment)
	}
}
