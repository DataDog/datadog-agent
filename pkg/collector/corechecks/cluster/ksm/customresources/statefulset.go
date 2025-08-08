// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/)
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

var descStatefulSetLabelsDefaultLabels = []string{"namespace", "statefulset"}

// NewExtendedStatefulSetFactory returns a new StatefulSet metric family generator factory.
func NewExtendedStatefulSetFactory(client *apiserver.APIClient) customresource.RegistryFactory {
	return &extendedStatefulSetFactory{
		client: client.Cl,
	}
}

type extendedStatefulSetFactory struct {
	client kubernetes.Interface
}

// Name is the name of the factory
func (f *extendedStatefulSetFactory) Name() string {
	return "statefulsets_extended"
}

func (f *extendedStatefulSetFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client, nil
}

// MetricFamilyGenerators returns the extended statefulset metric family generators
func (f *extendedStatefulSetFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_statefulset_ongoing_rollout_duration",
			"Duration of ongoing statefulset rollout in seconds, calculated from ControllerRevision creation time",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapStatefulSetFunc(func(s *appsv1.StatefulSet) *metric.Family {
				
				
				ms := []*metric.Metric{}
				
				// Check if rollout is ongoing and get ControllerRevision creation time
				rolloutStartTime := f.getRolloutStartTime(s)
				
				if !rolloutStartTime.IsZero() {
					// Pass the ControllerRevision creation timestamp to the transformer for duration calculation
					ms = append(ms, &metric.Metric{
						Value: float64(rolloutStartTime.Unix()), // Pass timestamp, transformer will calculate duration
					})
				} else {
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
	}
}

func wrapStatefulSetFunc(f func(*appsv1.StatefulSet) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		statefulset := obj.(*appsv1.StatefulSet)

		metricFamily := f(statefulset)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descStatefulSetLabelsDefaultLabels, []string{statefulset.Namespace, statefulset.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

// ExpectedType returns the type expected by the factory
func (f *extendedStatefulSetFactory) ExpectedType() interface{} {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
	}
}

// ListWatch returns a ListerWatcher for appsv1.StatefulSet
func (f *extendedStatefulSetFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(kubernetes.Interface)
	ctx := context.Background()
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			result, err := client.AppsV1().StatefulSets(ns).List(ctx, opts)
			return result, err
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.AppsV1().StatefulSets(ns).Watch(ctx, opts)
		},
	}
}

// getRolloutStartTime determines if a rollout is ongoing and returns the start time
func (f *extendedStatefulSetFactory) getRolloutStartTime(s *appsv1.StatefulSet) time.Time {
	// Check if rollout is ongoing using generation mismatch or replica status
	if s.Generation == s.Status.ObservedGeneration {
		// Generations match, check if all replicas are ready and updated
		desiredReplicas := getDesiredStatefulSetReplicas(s)
		if s.Status.ReadyReplicas == desiredReplicas && s.Status.UpdatedReplicas == desiredReplicas {
				return time.Time{} // Rollout complete
		}
	}
	
	// Additional safety check: if no replicas are progressing for a very long time, the rollout might be stuck
	// This helps catch cases where the statefulset is genuinely stuck vs missed completion events
	if s.Status.UpdatedReplicas == 0 && s.Status.ReadyReplicas == 0 {
		// All replicas are failing to start - this could be a stuck rollout
	}
	
	
	
	// Find the newest ControllerRevision for this statefulset
	newestCRCreationTime := f.findNewestControllerRevisionCreationTime(s)
	if newestCRCreationTime.IsZero() {
		return time.Time{}
	}
	
	return newestCRCreationTime
}

// findNewestControllerRevisionCreationTime finds the creation time of the newest ControllerRevision for this statefulset
func (f *extendedStatefulSetFactory) findNewestControllerRevisionCreationTime(s *appsv1.StatefulSet) time.Time {
	// List ControllerRevisions owned by this statefulset
	controllerRevisions, err := f.client.AppsV1().ControllerRevisions(s.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.Set(s.Spec.Selector.MatchLabels).AsSelector().String(),
	})
	if err != nil {
		return time.Time{}
	}
	
	
	var newestTime time.Time
	var newestCR *appsv1.ControllerRevision
	
	for i := range controllerRevisions.Items {
		cr := &controllerRevisions.Items[i]
		
		// Check if this ControllerRevision is owned by our statefulset
		if !f.isOwnedByStatefulSet(cr, s) {
			continue
		}
		
		
		if cr.CreationTimestamp.Time.After(newestTime) {
			newestTime = cr.CreationTimestamp.Time
			newestCR = cr
		}
	}
	
	if newestCR != nil {
	}
	
	return newestTime
}

// isOwnedByStatefulSet checks if a ControllerRevision is owned by the given StatefulSet
func (f *extendedStatefulSetFactory) isOwnedByStatefulSet(cr *appsv1.ControllerRevision, s *appsv1.StatefulSet) bool {
	for _, owner := range cr.OwnerReferences {
		if owner.Kind == "StatefulSet" && owner.Name == s.Name && owner.UID == s.UID {
			return true
		}
	}
	return false
}


// getDesiredStatefulSetReplicas safely gets the desired replica count from a statefulset
func getDesiredStatefulSetReplicas(s *appsv1.StatefulSet) int32 {
	if s.Spec.Replicas == nil {
		return 1 // Default replica count
	}
	return *s.Spec.Replicas
}