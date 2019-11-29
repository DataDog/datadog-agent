package topologycollectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
)

// ClusterTopologyCommon should be mixed in this interface for basic functionality on any real collector
type ClusterTopologyCommon interface {
	GetAPIClient() apiserver.APICollectorClient
	GetInstance() topology.Instance
	GetName() string
	CreateRelation(sourceExternalID, targetExternalID, typeName string) *topology.Relation
	CreateRelationData(sourceExternalID, targetExternalID, typeName string, data map[string]interface{}) *topology.Relation
	addClusterNameTag(tags map[string]string) map[string]string
	buildClusterExternalID() string
	buildConfigMapExternalID(namespace, configMapID string) string
	buildContainerExternalID(podName, containerName string) string
	buildDaemonSetExternalID(daemonSetID string) string
	buildDeploymentExternalID(namespace, deploymentName string) string
	buildNodeExternalID(nodeName string) string
	buildPodExternalID(podID string) string
	buildReplicaSetExternalID(replicaSetID string) string
	buildServiceExternalID(serviceID string) string
	buildStatefulSetExternalID(statefulSetID string) string
	buildCronJobExternalID(cronJobID string) string
	buildJobExternalID(jobID string) string
	buildIngressExternalID(ingressID string) string
	buildVolumeExternalID(volumeID string) string
	buildPersistentVolumeExternalID(persistentVolumeID string) string
	buildEndpointExternalID(endpointID string) string
}

type clusterTopologyCommon struct {
	Instance           topology.Instance
	APICollectorClient apiserver.APICollectorClient
}

// NewClusterTopologyCommon creates a clusterTopologyCommon
func NewClusterTopologyCommon(instance topology.Instance, ac apiserver.APICollectorClient) ClusterTopologyCommon {
	return &clusterTopologyCommon{Instance: instance, APICollectorClient: ac}
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
	return fmt.Sprintf("urn:cluster:/%s:%s", c.Instance.Type, c.Instance.URL)
}

// buildNodeExternalID
// nodeName
func (c *clusterTopologyCommon) buildNodeExternalID(nodeName string) string {
	return fmt.Sprintf("urn:/%s:%s:node:%s", c.Instance.Type, c.Instance.URL, nodeName)
}

// buildPodExternalID
// podName
func (c *clusterTopologyCommon) buildPodExternalID(podName string) string {
	return fmt.Sprintf("urn:/%s:%s:pod:%s", c.Instance.Type, c.Instance.URL, podName)
}

// buildContainerExternalID
// podName, containerName
func (c *clusterTopologyCommon) buildContainerExternalID(podName, containerName string) string {
	return fmt.Sprintf("urn:/%s:%s:pod:%s:container:%s", c.Instance.Type, c.Instance.URL, podName, containerName)
}

// buildServiceExternalID
// serviceID
func (c *clusterTopologyCommon) buildServiceExternalID(serviceID string) string {
	return fmt.Sprintf("urn:/%s:%s:service:%s", c.Instance.Type, c.Instance.URL, serviceID)
}

// buildDaemonSetExternalID
// daemonSetID
func (c *clusterTopologyCommon) buildDaemonSetExternalID(daemonSetID string) string {
	return fmt.Sprintf("urn:/%s:%s:daemonset:%s", c.Instance.Type, c.Instance.URL, daemonSetID)
}

// buildDeploymentExternalID
// deploymentID
func (c *clusterTopologyCommon) buildDeploymentExternalID(namespace, deploymentID string) string {
	return fmt.Sprintf("urn:/%s:%s:deployment:%s:%s", c.Instance.Type, c.Instance.URL, namespace, deploymentID)
}

// buildReplicaSetExternalID
// replicaSetID
func (c *clusterTopologyCommon) buildReplicaSetExternalID(replicaSetID string) string {
	return fmt.Sprintf("urn:/%s:%s:replicaset:%s", c.Instance.Type, c.Instance.URL, replicaSetID)
}

// buildStatefulSetExternalID
// statefulSetID
func (c *clusterTopologyCommon) buildStatefulSetExternalID(statefulSetID string) string {
	return fmt.Sprintf("urn:/%s:%s:statefulset:%s", c.Instance.Type, c.Instance.URL, statefulSetID)
}

// buildConfigMapExternalID
// namespace
// configMapID
func (c *clusterTopologyCommon) buildConfigMapExternalID(namespace, configMapID string) string {
	return fmt.Sprintf("urn:/%s:%s:configmap:%s:%s", c.Instance.Type, c.Instance.URL, namespace, configMapID)
}

// buildCronJobExternalID
// cronJobID
func (c *clusterTopologyCommon) buildCronJobExternalID(cronJobID string) string {
	return fmt.Sprintf("urn:/%s:%s:cronjob:%s", c.Instance.Type, c.Instance.URL, cronJobID)
}

// buildJobExternalID
// jobID
func (c *clusterTopologyCommon) buildJobExternalID(jobID string) string {
	return fmt.Sprintf("urn:/%s:%s:job:%s", c.Instance.Type, c.Instance.URL, jobID)
}

// buildIngressExternalID
// ingressID
func (c *clusterTopologyCommon) buildIngressExternalID(ingressID string) string {
	return fmt.Sprintf("urn:/%s:%s:ingress:%s", c.Instance.Type, c.Instance.URL, ingressID)
}

// buildVolumeExternalID
// volumeID
func (c *clusterTopologyCommon) buildVolumeExternalID(volumeID string) string {
	return fmt.Sprintf("urn:/%s:%s:volume:%s", c.Instance.Type, c.Instance.URL, volumeID)
}

// buildPersistentVolumeExternalID
// persistentVolumeID
func (c *clusterTopologyCommon) buildPersistentVolumeExternalID(persistentVolumeID string) string {
	return fmt.Sprintf("urn:/%s:%s:persistent-volume:%s", c.Instance.Type, c.Instance.URL, persistentVolumeID)
}

// buildEndpointExternalID
// endpointID
func (c *clusterTopologyCommon) buildEndpointExternalID(endpointID string) string {
	return fmt.Sprintf("urn:endpoint:/%s:%s", c.Instance.URL, endpointID)
}

// addClusterNameTag
// tags
func (c *clusterTopologyCommon) addClusterNameTag(tags map[string]string) map[string]string {
	tags["cluster-name"] = c.Instance.URL
	return tags
}
