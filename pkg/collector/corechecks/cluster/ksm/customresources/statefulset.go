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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var descStatefulSetLabelsDefaultLabels = []string{"namespace", "statefulset"}

// NewExtendedStatefulSetFactory returns a new StatefulSet metric family generator factory.
func NewExtendedStatefulSetFactory(client *apiserver.APIClient) customresource.RegistryFactory {
	log.Infof("ROLLOUT-STATEFULSET: NewExtendedStatefulSetFactory called")
	return &extendedStatefulSetFactory{
		client: client.Cl,
	}
}

type extendedStatefulSetFactory struct {
	client kubernetes.Interface
}

// Name is the name of the factory
func (f *extendedStatefulSetFactory) Name() string {
	log.Infof("ROLLOUT-STATEFULSET: Name() called, returning 'statefulsets_extended'")
	return "statefulsets_extended"
}

func (f *extendedStatefulSetFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	log.Infof("ROLLOUT-STATEFULSET: CreateClient() called")
	return f.client, nil
}

// MetricFamilyGenerators returns the extended statefulset metric family generators
func (f *extendedStatefulSetFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	log.Infof("ROLLOUT-STATEFULSET: MetricFamilyGenerators() called")
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_statefulset_ongoing_rollout_duration",
			"Duration of ongoing statefulset rollout in seconds, calculated from ControllerRevision creation time",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapStatefulSetFunc(func(s *appsv1.StatefulSet) *metric.Family {
				log.Infof("ROLLOUT-STATEFULSET: ==== FACTORY TRIGGERED ====")
				log.Infof("ROLLOUT-STATEFULSET: Processing statefulset %s/%s at %s", s.Namespace, s.Name, time.Now().Format("15:04:05"))
				log.Infof("ROLLOUT-STATEFULSET: Object metadata - ResourceVersion: %s, Generation: %d", s.ResourceVersion, s.Generation)
				log.Infof("ROLLOUT-STATEFULSET: StatefulSet status - ObservedGeneration: %d, ReadyReplicas: %d/%d, UpdatedReplicas: %d, CurrentReplicas: %d", 
					s.Status.ObservedGeneration, s.Status.ReadyReplicas, getDesiredStatefulSetReplicas(s), s.Status.UpdatedReplicas, s.Status.CurrentReplicas)
				
				// Log conditions if present
				if len(s.Status.Conditions) > 0 {
					log.Infof("ROLLOUT-STATEFULSET: StatefulSet conditions:")
					for _, condition := range s.Status.Conditions {
						log.Infof("ROLLOUT-STATEFULSET:   - Type: %s, Status: %s, Reason: %s, LastTransitionTime: %s", 
							condition.Type, condition.Status, condition.Reason, condition.LastTransitionTime.Format("15:04:05"))
					}
				}
				
				ms := []*metric.Metric{}
				
				// Check if rollout is ongoing and get ControllerRevision creation time
				rolloutStartTime := f.getRolloutStartTime(s)
				
				if !rolloutStartTime.IsZero() {
					// Pass the ControllerRevision creation timestamp to the transformer for duration calculation
					log.Infof("ROLLOUT-STATEFULSET: StatefulSet %s/%s has ongoing rollout, start time=%s", s.Namespace, s.Name, rolloutStartTime.Format(time.RFC3339))
					ms = append(ms, &metric.Metric{
						Value: float64(rolloutStartTime.Unix()), // Pass timestamp, transformer will calculate duration
					})
				} else {
					log.Infof("ROLLOUT-STATEFULSET: StatefulSet %s/%s rollout complete or not started", s.Namespace, s.Name)
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
	}
}

func wrapStatefulSetFunc(f func(*appsv1.StatefulSet) *metric.Family) func(interface{}) *metric.Family {
	log.Infof("ROLLOUT-STATEFULSET: wrapStatefulSetFunc called")
	return func(obj interface{}) *metric.Family {
		statefulset := obj.(*appsv1.StatefulSet)
		log.Infof("ROLLOUT-STATEFULSET: wrapStatefulSetFunc processing statefulset %s/%s", statefulset.Namespace, statefulset.Name)

		metricFamily := f(statefulset)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descStatefulSetLabelsDefaultLabels, []string{statefulset.Namespace, statefulset.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

// ExpectedType returns the type expected by the factory
func (f *extendedStatefulSetFactory) ExpectedType() interface{} {
	log.Infof("ROLLOUT-STATEFULSET: ExpectedType() called")
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
	}
}

// ListWatch returns a ListerWatcher for appsv1.StatefulSet
func (f *extendedStatefulSetFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	log.Infof("ROLLOUT-STATEFULSET: ListWatch() called for namespace=%s, fieldSelector=%s", ns, fieldSelector)
	client := customResourceClient.(kubernetes.Interface)
	ctx := context.Background()
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			log.Infof("ROLLOUT-STATEFULSET: ListFunc called for namespace=%s", ns)
			opts.FieldSelector = fieldSelector
			result, err := client.AppsV1().StatefulSets(ns).List(ctx, opts)
			if err != nil {
				log.Warnf("ROLLOUT-STATEFULSET: ListFunc error: %v", err)
			} else {
				log.Infof("ROLLOUT-STATEFULSET: ListFunc found %d statefulsets", len(result.Items))
			}
			return result, err
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			log.Infof("ROLLOUT-STATEFULSET: WatchFunc called for namespace=%s", ns)
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
			log.Infof("ROLLOUT-STATEFULSET: StatefulSet %s/%s rollout complete (gen match + ready + updated)", s.Namespace, s.Name)
			return time.Time{} // Rollout complete
		}
	}
	
	// Additional safety check: if no replicas are progressing for a very long time, the rollout might be stuck
	// This helps catch cases where the statefulset is genuinely stuck vs missed completion events
	if s.Status.UpdatedReplicas == 0 && s.Status.ReadyReplicas == 0 {
		// All replicas are failing to start - this could be a stuck rollout
		log.Infof("ROLLOUT-STATEFULSET: StatefulSet %s/%s appears stuck (no replicas progressing)", s.Namespace, s.Name)
	}
	
	
	log.Infof("ROLLOUT-STATEFULSET: StatefulSet %s/%s has ongoing rollout, finding newest ControllerRevision", s.Namespace, s.Name)
	
	// Find the newest ControllerRevision for this statefulset
	newestCRCreationTime := f.findNewestControllerRevisionCreationTime(s)
	if newestCRCreationTime.IsZero() {
		log.Warnf("ROLLOUT-STATEFULSET: Could not find creation time for statefulset %s/%s", s.Namespace, s.Name)
		return time.Time{}
	}
	
	log.Infof("ROLLOUT-STATEFULSET: StatefulSet %s/%s rollout start time: %s", s.Namespace, s.Name, newestCRCreationTime.Format(time.RFC3339))
	return newestCRCreationTime
}

// findNewestControllerRevisionCreationTime finds the creation time of the newest ControllerRevision for this statefulset
func (f *extendedStatefulSetFactory) findNewestControllerRevisionCreationTime(s *appsv1.StatefulSet) time.Time {
	// List ControllerRevisions owned by this statefulset
	controllerRevisions, err := f.client.AppsV1().ControllerRevisions(s.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.Set(s.Spec.Selector.MatchLabels).AsSelector().String(),
	})
	if err != nil {
		log.Warnf("ROLLOUT-STATEFULSET: Failed to list ControllerRevisions for statefulset %s/%s: %v", s.Namespace, s.Name, err)
		return time.Time{}
	}
	
	log.Infof("ROLLOUT-STATEFULSET: Found %d ControllerRevisions for statefulset %s/%s", len(controllerRevisions.Items), s.Namespace, s.Name)
	
	var newestTime time.Time
	var newestCR *appsv1.ControllerRevision
	
	for i := range controllerRevisions.Items {
		cr := &controllerRevisions.Items[i]
		
		// Check if this ControllerRevision is owned by our statefulset
		if !f.isOwnedByStatefulSet(cr, s) {
			log.Debugf("ROLLOUT-STATEFULSET: ControllerRevision %s/%s not owned by statefulset %s/%s", cr.Namespace, cr.Name, s.Namespace, s.Name)
			continue
		}
		
		log.Infof("ROLLOUT-STATEFULSET: Found owned ControllerRevision %s/%s (revision %d) created at %s", 
			cr.Namespace, cr.Name, cr.Revision, cr.CreationTimestamp.Time.Format(time.RFC3339))
		
		if cr.CreationTimestamp.Time.After(newestTime) {
			newestTime = cr.CreationTimestamp.Time
			newestCR = cr
		}
	}
	
	if newestCR != nil {
		log.Infof("ROLLOUT-STATEFULSET: Using newest ControllerRevision %s/%s (revision %d, created %s) for duration calculation", 
			newestCR.Namespace, newestCR.Name, newestCR.Revision, newestTime.Format(time.RFC3339))
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