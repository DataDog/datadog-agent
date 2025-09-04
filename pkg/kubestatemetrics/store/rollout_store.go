//go:build kubeapiserver

package store

import (
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/kube-state-metrics/v2/pkg/metric"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/ksm/customresources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RolloutMetricsStore extends MetricsStore with rollout-specific delete handling
type RolloutMetricsStore struct {
	*MetricsStore
	resourceType string
}

// NewRolloutMetricsStore creates a MetricsStore with rollout delete notifications
func NewRolloutMetricsStore(generateFunc func(interface{}) []metric.FamilyInterface, mt string, resourceType string) *RolloutMetricsStore {
	return &RolloutMetricsStore{
		MetricsStore: NewMetricsStore(generateFunc, mt),
		resourceType: resourceType,
	}
}

// Delete overrides the base Delete to add rollout cleanup
func (s *RolloutMetricsStore) Delete(obj interface{}) error {
	log.Infof("ROLLOUT-STORE-DELETE: Delete method called for %s store", s.resourceType)

	// Handle rollout cleanup based on resource type
	switch s.resourceType {
	case "deployments":
		if dep, ok := obj.(*appsv1.Deployment); ok {
			log.Infof("ROLLOUT-STORE-DELETE: Processing deployment deletion: %s/%s", dep.Namespace, dep.Name)
			customresources.CleanupDeletedDeployment(dep.Namespace, dep.Name)
		}
	case "replicasets":
		if rs, ok := obj.(*appsv1.ReplicaSet); ok {
			log.Infof("ROLLOUT-STORE-DELETE: Processing ReplicaSet deletion: %s/%s", rs.Namespace, rs.Name)
			customresources.CleanupDeletedReplicaSet(rs.Namespace, rs.Name)
		}
	}

	// Call the base delete method
	return s.MetricsStore.Delete(obj)
}
