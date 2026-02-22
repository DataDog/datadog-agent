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

// NewDaemonSetRolloutFactory returns a new DaemonSet rollout factory that provides rollout duration metrics
func NewDaemonSetRolloutFactory(client *apiserver.APIClient, rolloutTracker RolloutOperations) customresource.RegistryFactory {
	return &daemonSetRolloutFactory{
		client:         client.Cl,
		rolloutTracker: rolloutTracker,
	}
}

type daemonSetRolloutFactory struct {
	client         kubernetes.Interface
	rolloutTracker RolloutOperations
}

func (f *daemonSetRolloutFactory) Name() string {
	return "daemonsets_extended"
}

func (f *daemonSetRolloutFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client, nil
}

func (f *daemonSetRolloutFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_daemonset_ongoing_rollout_duration",
			"Duration of ongoing DaemonSet rollout in seconds",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapDaemonSetFunc(func(ds *appsv1.DaemonSet) *metric.Family {
				// LIMITATION: DaemonSets don't expose UpdateRevision in their status like StatefulSets do.
				// This means we cannot use revision-based detection and must rely on generation mismatches.
				//
				// Implications:
				// 1. If Kubernetes reconciles generation quickly (< 15s scrape interval) but pods are
				//    still rolling, we may miss the rollout. This is less likely for DaemonSets since
				//    they roll out node-by-node which is typically slower than Deployment/StatefulSet.
				// 2. We cannot distinguish rollbacks from scaling as cleanly as we can with Deployments.
				//
				// We use generation-based detection combined with rollout condition checks.

				// Check if this is a generation mismatch (Kubernetes hasn't caught up yet)
				isNewRollout := ds.Generation != ds.Status.ObservedGeneration

				// Check if we're already tracking this DaemonSet
				isActivelyTracked := f.rolloutTracker.HasActiveDaemonSetRollout(ds)

				// Check if Kubernetes reports an active rollout condition
				hasRolloutCondition := f.rolloutTracker.HasDaemonSetRolloutCondition(ds)

				// A rollout is ongoing if:
				// 1. Generation mismatch (new rollout starting)
				// 2. OR we're already tracking AND has rollout condition (continuing rollout)
				//
				// Note: Unlike Deployments/StatefulSets, we cannot check "revisionChanged && hasRolloutCondition"
				// because DaemonSets don't expose their updateRevision in status.
				isOngoing := isNewRollout || (isActivelyTracked && hasRolloutCondition)

				if isOngoing {
					f.rolloutTracker.StoreDaemonSet(ds)

					// Return dummy metric with value 1 to trigger transformer
					return &metric.Family{
						Metrics: []*metric.Metric{
							{
								LabelKeys:   []string{"namespace", "daemonset"},
								LabelValues: []string{ds.Namespace, ds.Name},
								Value:       1, // Dummy value - transformer will calculate real duration
							},
						},
					}
				}

				// Rollout complete - cleanup and return 0
				f.rolloutTracker.CleanupDaemonSet(ds.Namespace, ds.Name)
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   []string{"namespace", "daemonset"},
							LabelValues: []string{ds.Namespace, ds.Name},
							Value:       0,
						},
					},
				}
			}),
		),
	}
}

func (f *daemonSetRolloutFactory) ExpectedType() interface{} {
	return &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DaemonSet",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
	}
}

func (f *daemonSetRolloutFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(kubernetes.Interface)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.AppsV1().DaemonSets(ns).List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.AppsV1().DaemonSets(ns).Watch(context.TODO(), opts)
		},
	}
}

func wrapDaemonSetFunc(f func(*appsv1.DaemonSet) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		daemonSet := obj.(*appsv1.DaemonSet)
		return f(daemonSet)
	}
}
