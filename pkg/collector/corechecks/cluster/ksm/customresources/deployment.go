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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var descDeploymentLabelsDefaultLabels = []string{"namespace", "deployment"}

// NewExtendedDeploymentFactory returns a new Deployment metric family generator factory.
func NewExtendedDeploymentFactory(client *apiserver.APIClient) customresource.RegistryFactory {
	log.Infof("ROLLOUT-DEPLOYMENT: NewExtendedDeploymentFactory called")
	return &extendedDeploymentFactory{
		client: client.Cl,
	}
}

type extendedDeploymentFactory struct {
	client kubernetes.Interface
}

// Name is the name of the factory
func (f *extendedDeploymentFactory) Name() string {
	log.Infof("ROLLOUT-DEPLOYMENT: Name() called, returning 'deployments_extended'")
	return "deployments_extended"
}

func (f *extendedDeploymentFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	log.Infof("ROLLOUT-DEPLOYMENT: CreateClient() called")
	return f.client, nil
}

// MetricFamilyGenerators returns the extended deployment metric family generators
func (f *extendedDeploymentFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	log.Infof("ROLLOUT-DEPLOYMENT: MetricFamilyGenerators() called")
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_deployment_ongoing_rollout_duration",
			"Duration of ongoing deployment rollout in seconds, calculated from ReplicaSet creation time",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapDeploymentFunc(func(d *appsv1.Deployment) *metric.Family {
				log.Infof("ROLLOUT-DEPLOYMENT: ==== FACTORY TRIGGERED ====")
				log.Infof("ROLLOUT-DEPLOYMENT: Processing deployment %s/%s at %s", d.Namespace, d.Name, time.Now().Format("15:04:05"))
				log.Infof("ROLLOUT-DEPLOYMENT: Object metadata - ResourceVersion: %s, Generation: %d", d.ResourceVersion, d.Generation)
				log.Infof("ROLLOUT-DEPLOYMENT: Deployment status - ObservedGeneration: %d, ReadyReplicas: %d/%d, UpdatedReplicas: %d, AvailableReplicas: %d",
					d.Status.ObservedGeneration, d.Status.ReadyReplicas, getDesiredReplicas(d), d.Status.UpdatedReplicas, d.Status.AvailableReplicas)

				// Log conditions if present
				if len(d.Status.Conditions) > 0 {
					log.Infof("ROLLOUT-DEPLOYMENT: Deployment conditions:")
					for _, condition := range d.Status.Conditions {
						log.Infof("ROLLOUT-DEPLOYMENT:   - Type: %s, Status: %s, Reason: %s, LastUpdateTime: %s",
							condition.Type, condition.Status, condition.Reason, condition.LastUpdateTime.Format("15:04:05"))
					}
				}

				ms := []*metric.Metric{}

				// Check if rollout is ongoing and get ReplicaSet creation time
				rolloutStartTime := f.getRolloutStartTime(d)

				if !rolloutStartTime.IsZero() {
					// Pass the ReplicaSet creation timestamp to the transformer for duration calculation
					log.Infof("ROLLOUT-DEPLOYMENT: Deployment %s/%s has ongoing rollout, start time=%s", d.Namespace, d.Name, rolloutStartTime.Format(time.RFC3339))
					ms = append(ms, &metric.Metric{
						Value: float64(rolloutStartTime.Unix()), // Pass timestamp, transformer will calculate duration
					})
				} else {
					log.Infof("ROLLOUT-DEPLOYMENT: Deployment %s/%s rollout complete or not started", d.Namespace, d.Name)
				}

				log.Infof("ROLLOUT-DEPLOYMENT: returning metric family %v", ms)
				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
	}
}

func wrapDeploymentFunc(f func(*appsv1.Deployment) *metric.Family) func(interface{}) *metric.Family {
	log.Infof("ROLLOUT-DEPLOYMENT: wrapDeploymentFunc called")
	return func(obj interface{}) *metric.Family {
		deployment := obj.(*appsv1.Deployment)
		log.Infof("ROLLOUT-DEPLOYMENT: wrapDeploymentFunc processing deployment %s/%s", deployment.Namespace, deployment.Name)

		metricFamily := f(deployment)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descDeploymentLabelsDefaultLabels, []string{deployment.Namespace, deployment.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

// ExpectedType returns the type expected by the factory
func (f *extendedDeploymentFactory) ExpectedType() interface{} {
	log.Infof("ROLLOUT-DEPLOYMENT: ExpectedType() called")
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
	}
}

// ListWatch returns a ListerWatcher for appsv1.Deployment
func (f *extendedDeploymentFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	log.Infof("ROLLOUT-DEPLOYMENT: ListWatch() called for namespace=%s, fieldSelector=%s", ns, fieldSelector)
	client := customResourceClient.(kubernetes.Interface)
	ctx := context.Background()
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			log.Infof("ROLLOUT-DEPLOYMENT: ListFunc called for namespace=%s", ns)
			opts.FieldSelector = fieldSelector
			result, err := client.AppsV1().Deployments(ns).List(ctx, opts)
			if err != nil {
				log.Warnf("ROLLOUT-DEPLOYMENT: ListFunc error: %v", err)
			} else {
				log.Infof("ROLLOUT-DEPLOYMENT: ListFunc found %d deployments", len(result.Items))
			}
			return result, err
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			log.Infof("ROLLOUT-DEPLOYMENT: WatchFunc called for namespace=%s", ns)
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
			log.Infof("ROLLOUT-DEPLOYMENT: Deployment %s/%s rollout complete (gen match + ready)", d.Namespace, d.Name)
			return time.Time{} // Rollout complete
		}
	}

	// Additional safety check: if no replicas are progressing for a very long time, the rollout might be stuck
	// This helps catch cases where the deployment is genuinely stuck vs missed completion events
	if d.Status.UpdatedReplicas == 0 && d.Status.ReadyReplicas == 0 {
		// All replicas are failing to start - this could be a stuck rollout
		log.Infof("ROLLOUT-DEPLOYMENT: Deployment %s/%s appears stuck (no replicas progressing)", d.Namespace, d.Name)
	}


	log.Infof("ROLLOUT-DEPLOYMENT: Deployment %s/%s has ongoing rollout, finding newest ReplicaSet", d.Namespace, d.Name)

	// Find the newest ReplicaSet for this deployment
	newestRSCreationTime := f.findNewestReplicaSetCreationTime(d)
	if newestRSCreationTime.IsZero() {
		log.Warnf("ROLLOUT-DEPLOYMENT: Could not find creation time for deployment %s/%s", d.Namespace, d.Name)
		return time.Time{}
	}

	log.Infof("ROLLOUT-DEPLOYMENT: Deployment %s/%s rollout start time: %s", d.Namespace, d.Name, newestRSCreationTime.Format(time.RFC3339))
	return newestRSCreationTime
}

// findNewestReplicaSetCreationTime finds the creation time of the newest ReplicaSet for this deployment
func (f *extendedDeploymentFactory) findNewestReplicaSetCreationTime(d *appsv1.Deployment) time.Time {
	// List ReplicaSets owned by this deployment
	replicaSets, err := f.client.AppsV1().ReplicaSets(d.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.Set(d.Spec.Selector.MatchLabels).AsSelector().String(),
	})
	if err != nil {
		log.Warnf("ROLLOUT-DEPLOYMENT: Failed to list ReplicaSets for deployment %s/%s: %v", d.Namespace, d.Name, err)
		return time.Time{}
	}

	log.Infof("ROLLOUT-DEPLOYMENT: Found %d ReplicaSets for deployment %s/%s", len(replicaSets.Items), d.Namespace, d.Name)

	var newestTime time.Time
	var newestRS *appsv1.ReplicaSet

	for i := range replicaSets.Items {
		rs := &replicaSets.Items[i]

		// Check if this ReplicaSet is owned by our deployment
		if !f.isOwnedByDeployment(rs, d) {
			log.Debugf("ROLLOUT-DEPLOYMENT: ReplicaSet %s/%s not owned by deployment %s/%s", rs.Namespace, rs.Name, d.Namespace, d.Name)
			continue
		}

		log.Infof("ROLLOUT-DEPLOYMENT: Found owned ReplicaSet %s/%s created at %s", rs.Namespace, rs.Name, rs.CreationTimestamp.Time.Format(time.RFC3339))

		if rs.CreationTimestamp.Time.After(newestTime) {
			newestTime = rs.CreationTimestamp.Time
			newestRS = rs
		}
	}

	if newestRS != nil {
		log.Infof("ROLLOUT-DEPLOYMENT: Using newest ReplicaSet %s/%s (created %s) for duration calculation",
			newestRS.Namespace, newestRS.Name, newestTime.Format(time.RFC3339))
	}

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
