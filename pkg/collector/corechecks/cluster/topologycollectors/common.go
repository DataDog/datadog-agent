// +build kubeapiserver

package topologycollectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/urn"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterTopologyCommon should be mixed in this interface for basic functionality on any real collector
type ClusterTopologyCommon interface {
	GetAPIClient() apiserver.APICollectorClient
	GetInstance() topology.Instance
	GetName() string
	CreateRelation(sourceExternalID, targetExternalID, typeName string) *topology.Relation
	CreateRelationData(sourceExternalID, targetExternalID, typeName string, data map[string]interface{}) *topology.Relation
	initTags(meta metav1.ObjectMeta) map[string]string
	buildClusterExternalID() string
	buildConfigMapExternalID(namespace, configMapName string) string
	buildNamespaceExternalID(namespaceName string) string
	buildContainerExternalID(namespace, podName, containerName string) string
	buildDaemonSetExternalID(namespace, daemonSetName string) string
	buildDeploymentExternalID(namespace, deploymentName string) string
	buildNodeExternalID(nodeName string) string
	buildPodExternalID(namespace, podName string) string
	buildReplicaSetExternalID(namespace, replicaSetName string) string
	buildServiceExternalID(namespace, serviceName string) string
	buildStatefulSetExternalID(namespace, statefulSetName string) string
	buildCronJobExternalID(namespace, cronJobName string) string
	buildJobExternalID(namespace, jobName string) string
	buildIngressExternalID(namespace, ingressName string) string
	buildVolumeExternalID(namespace, volumeName string) string
	buildPersistentVolumeExternalID(persistentVolumeName string) string
	buildEndpointExternalID(endpointID string) string
}

type clusterTopologyCommon struct {
	Instance           topology.Instance
	APICollectorClient apiserver.APICollectorClient
	urn                urn.Builder
}

// NewClusterTopologyCommon creates a clusterTopologyCommon
func NewClusterTopologyCommon(instance topology.Instance, ac apiserver.APICollectorClient) ClusterTopologyCommon {
	return &clusterTopologyCommon{
		Instance:           instance,
		APICollectorClient: ac,
		urn:                urn.NewURNBuilder(urn.ClusterTypeFromString(instance.Type), instance.URL),
	}
}

// GetName
func (c *clusterTopologyCommon) GetName() string {
	return "Unknown Collector"
}

// GetInstance
func (c *clusterTopologyCommon) GetInstance() topology.Instance {
	return c.Instance
}

// GetAPIClient
func (c *clusterTopologyCommon) GetAPIClient() apiserver.APICollectorClient {
	return c.APICollectorClient
}

// CreateRelationData creates a StackState relation called typeName for the given sourceExternalID and targetExternalID
func (c *clusterTopologyCommon) CreateRelationData(sourceExternalID, targetExternalID, typeName string, data map[string]interface{}) *topology.Relation {
	var _data map[string]interface{}

	if data != nil {
		_data = data
	} else {
		_data = map[string]interface{}{}
	}

	return &topology.Relation{
		ExternalID: fmt.Sprintf("%s->%s", sourceExternalID, targetExternalID),
		SourceID:   sourceExternalID,
		TargetID:   targetExternalID,
		Type:       topology.Type{Name: typeName},
		Data:       _data,
	}
}

// CreateRelation creates a StackState relation called typeName for the given sourceExternalID and targetExternalID
func (c *clusterTopologyCommon) CreateRelation(sourceExternalID, targetExternalID, typeName string) *topology.Relation {
	return &topology.Relation{
		ExternalID: fmt.Sprintf("%s->%s", sourceExternalID, targetExternalID),
		SourceID:   sourceExternalID,
		TargetID:   targetExternalID,
		Type:       topology.Type{Name: typeName},
		Data:       map[string]interface{}{},
	}
}

// buildClusterExternalID
func (c *clusterTopologyCommon) buildClusterExternalID() string {
	return c.urn.BuildClusterExternalID()
}

// buildNodeExternalID creates the urn external identifier for a cluster node
func (c *clusterTopologyCommon) buildNodeExternalID(nodeName string) string {
	return c.urn.BuildNodeExternalID(nodeName)
}

// buildPodExternalID creates the urn external identifier for a cluster pod
func (c *clusterTopologyCommon) buildPodExternalID(namespace, podName string) string {
	return c.urn.BuildPodExternalID(namespace, podName)
}

// buildContainerExternalID creates the urn external identifier for a pod's container
func (c *clusterTopologyCommon) buildContainerExternalID(namespace, podName, containerName string) string {
	return c.urn.BuildContainerExternalID(namespace, podName, containerName)
}

// buildServiceExternalID creates the urn external identifier for a cluster service
func (c *clusterTopologyCommon) buildServiceExternalID(namespace, serviceName string) string {
	return c.urn.BuildServiceExternalID(namespace, serviceName)
}

// buildDaemonSetExternalID creates the urn external identifier for a cluster daemon set
func (c *clusterTopologyCommon) buildDaemonSetExternalID(namespace, daemonSetName string) string {
	return c.urn.BuildDaemonSetExternalID(namespace, daemonSetName)
}

// buildDeploymentExternalID creates the urn external identifier for a cluster deployment
func (c *clusterTopologyCommon) buildDeploymentExternalID(namespace, deploymentName string) string {
	return c.urn.BuildDeploymentExternalID(namespace, deploymentName)
}

// buildReplicaSetExternalID creates the urn external identifier for a cluster replica set
func (c *clusterTopologyCommon) buildReplicaSetExternalID(namespace, replicaSetName string) string {
	return c.urn.BuildReplicaSetExternalID(namespace, replicaSetName)
}

// buildStatefulSetExternalID creates the urn external identifier for a cluster stateful set
func (c *clusterTopologyCommon) buildStatefulSetExternalID(namespace, statefulSetName string) string {
	return c.urn.BuildStatefulSetExternalID(namespace, statefulSetName)
}

// buildConfigMapExternalID creates the urn external identifier for a cluster config map
func (c *clusterTopologyCommon) buildConfigMapExternalID(namespace, configMapName string) string {
	return c.urn.BuildConfigMapExternalID(namespace, configMapName)
}

// buildNamespaceExternalID creates the urn external identifier for a cluster namespace
func (c *clusterTopologyCommon) buildNamespaceExternalID(namespaceName string) string {
	return c.urn.BuildNamespaceExternalID(namespaceName)
}

// buildCronJobExternalID creates the urn external identifier for a cluster cron job
func (c *clusterTopologyCommon) buildCronJobExternalID(namespace, cronJobName string) string {
	return c.urn.BuildCronJobExternalID(namespace, cronJobName)
}

// buildJobExternalID creates the urn external identifier for a cluster job
func (c *clusterTopologyCommon) buildJobExternalID(namespace, jobName string) string {
	return c.urn.BuildJobExternalID(namespace, jobName)
}

// buildIngressExternalID creates the urn external identifier for a cluster ingress
func (c *clusterTopologyCommon) buildIngressExternalID(namespace, ingressName string) string {
	return c.urn.BuildIngressExternalID(namespace, ingressName)
}

// buildVolumeExternalID creates the urn external identifier for a cluster volume
func (c *clusterTopologyCommon) buildVolumeExternalID(namespace, volumeName string) string {
	return c.urn.BuildVolumeExternalID(namespace, volumeName)
}

// buildPersistentVolumeExternalID creates the urn external identifier for a cluster persistent volume
func (c *clusterTopologyCommon) buildPersistentVolumeExternalID(persistentVolumeName string) string {
	return c.urn.BuildPersistentVolumeExternalID(persistentVolumeName)
}

// buildEndpointExternalID
// endpointID
func (c *clusterTopologyCommon) buildEndpointExternalID(endpointID string) string {
	return c.urn.BuildEndpointExternalID(endpointID)
}

func (c *clusterTopologyCommon) initTags(meta metav1.ObjectMeta) map[string]string {
	tags := make(map[string]string, 0)
	if meta.Labels != nil {
		tags = meta.Labels
	}

	// set the cluster name and the namespace
	tags["cluster-name"] = c.Instance.URL
	if meta.Namespace != "" {
		tags["namespace"] = meta.Namespace
	}

	return tags
}
