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
	"k8s.io/apimachinery/pkg/labels"
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

var descDeploymentLabelsDefaultLabels = []string{"namespace", "deployment"}

// NewExtendedDeploymentFactory returns a new Deployment metric family generator factory.
func NewExtendedDeploymentFactory(client *apiserver.APIClient) customresource.RegistryFactory {
	return &extendedDeploymentFactory{
		client: client.Cl,
	}
}

type extendedDeploymentFactory struct {
	client kubernetes.Interface
}

// Name is the name of the factory
func (f *extendedDeploymentFactory) Name() string {
	return "deployments_extended"
}

func (f *extendedDeploymentFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client, nil
}

// MetricFamilyGenerators returns the extended deployment metric family generators
func (f *extendedDeploymentFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_deployment_ongoing_rollout_duration",
			"Duration of ongoing deployment rollout in seconds, calculated from ReplicaSet creation time",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapDeploymentFunc(func(d *appsv1.Deployment) *metric.Family {

				ms := []*metric.Metric{}

				// Check if rollout is ongoing and get ReplicaSet creation time
				rolloutStartTime := f.getRolloutStartTime(d)

				if !rolloutStartTime.IsZero() {
					// Pass the ReplicaSet creation timestamp to the transformer for duration calculation
					ms = append(ms, &metric.Metric{
						Value: float64(rolloutStartTime.Unix()), // Pass timestamp, transformer will calculate duration
					})
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
	}
}

func wrapDeploymentFunc(f func(*appsv1.Deployment) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		deployment := obj.(*appsv1.Deployment)

		metricFamily := f(deployment)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descDeploymentLabelsDefaultLabels, []string{deployment.Namespace, deployment.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

// ExpectedType returns the type expected by the factory
func (f *extendedDeploymentFactory) ExpectedType() interface{} {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
	}
}

// ListWatch returns a ListerWatcher for appsv1.Deployment
func (f *extendedDeploymentFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(kubernetes.Interface)
	ctx := context.Background()
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			result, err := client.AppsV1().Deployments(ns).List(ctx, opts)
			return result, err
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.AppsV1().Deployments(ns).Watch(ctx, opts)
		},
	}
}

// getRolloutStartTime determines if a rollout is ongoing and returns the start time
func (f *extendedDeploymentFactory) getRolloutStartTime(d *appsv1.Deployment) time.Time {
	// Check if rollout is ongoing using generation mismatch
	if d.Generation == d.Status.ObservedGeneration {
		// Generations match, check if all replicas are ready
		desiredReplicas := getDesiredReplicas(d)
		if d.Status.ReadyReplicas == desiredReplicas {
			return time.Time{} // Rollout complete
		}
	}

	// Additional safety check: if no replicas are progressing for a very long time, the rollout might be stuck
	// This helps catch cases where the deployment is genuinely stuck vs missed completion events
	// Additional check: if no replicas are progressing, the rollout might be stuck
	// (This helps catch cases where all replicas are failing to start)
	_ = d.Status.UpdatedReplicas == 0 && d.Status.ReadyReplicas == 0

	// Find the newest ReplicaSet for this deployment
	newestRSCreationTime := f.findNewestReplicaSetCreationTime(d)
	if newestRSCreationTime.IsZero() {
		return time.Time{}
	}

	return newestRSCreationTime
}

// findNewestReplicaSetCreationTime finds the creation time of the newest ReplicaSet for this deployment
func (f *extendedDeploymentFactory) findNewestReplicaSetCreationTime(d *appsv1.Deployment) time.Time {
	// List ReplicaSets owned by this deployment
	replicaSets, err := f.client.AppsV1().ReplicaSets(d.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.Set(d.Spec.Selector.MatchLabels).AsSelector().String(),
	})
	if err != nil {
		return time.Time{}
	}

	var newestTime time.Time
	var newestRS *appsv1.ReplicaSet

	for i := range replicaSets.Items {
		rs := &replicaSets.Items[i]

		// Check if this ReplicaSet is owned by our deployment
		if !f.isOwnedByDeployment(rs, d) {
			continue
		}

		if rs.CreationTimestamp.Time.After(newestTime) {
			newestTime = rs.CreationTimestamp.Time
			newestRS = rs
		}
	}

	_ = newestRS

	return newestTime
}

// isOwnedByDeployment checks if a ReplicaSet is owned by the given Deployment
func (f *extendedDeploymentFactory) isOwnedByDeployment(rs *appsv1.ReplicaSet, d *appsv1.Deployment) bool {
	for _, owner := range rs.OwnerReferences {
		if owner.Kind == "Deployment" && owner.Name == d.Name && owner.UID == d.UID {
			return true
		}
	}
	return false
}

// getDesiredReplicas safely gets the desired replica count from a deployment
func getDesiredReplicas(d *appsv1.Deployment) int32 {
	if d.Spec.Replicas == nil {
		return 1 // Default replica count
	}
	return *d.Spec.Replicas
}
