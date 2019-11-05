// +build kubeapiserver

package topology_collectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/pkg/errors"
)

const (
	Deployment = "Deployment"
	DaemonSet = "DaemonSet"
	StatefulSet = "StatefulSet"
	ReplicaSet = "ReplicaSet"
)

// IngressCorrelation
type IngressCorrelation struct {
	ServiceID string
	IngressExternalID string
}

// ContainerCorrelation
type ContainerCorrelation struct {
	NodeName string
	MappingFunction func (nodeIdentifier string) (components []*topology.Component, relations []*topology.Relation)
}

// ClusterTopologyCollector collects cluster components and relations.
type ClusterTopologyCollector interface {
	CollectorFunction() error
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

type clusterTopologyCollector struct {
	Instance        topology.Instance
	APICollectorClient                    apiserver.APICollectorClient
}

// NewClusterTopologyCollector
func NewClusterTopologyCollector(instance topology.Instance, ac apiserver.APICollectorClient) ClusterTopologyCollector {
	return &clusterTopologyCollector{ Instance:instance, APICollectorClient: ac }
}

// GetName
func (c *clusterTopologyCollector) GetName() string {
	return "Unknown Collector"
}

// GetInstance
func (c *clusterTopologyCollector) GetInstance() topology.Instance {
	return c.Instance
}

// GetAPIClient
func (c *clusterTopologyCollector) GetAPIClient() apiserver.APICollectorClient {
	return c.APICollectorClient
}

// CollectorFunction
func (c *clusterTopologyCollector) CollectorFunction() error {
	return errors.New("CollectorFunction NotImplemented")
}

// CreateRelationData creates a StackState relation called typeName for the given sourceExternalID and targetExternalID
func (c *clusterTopologyCollector) CreateRelationData(sourceExternalID, targetExternalID, typeName string, data map[string]interface{}) *topology.Relation {
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
func (c *clusterTopologyCollector) CreateRelation(sourceExternalID, targetExternalID, typeName string) *topology.Relation {
	return &topology.Relation{
		ExternalID: fmt.Sprintf("%s->%s", sourceExternalID, targetExternalID),
		SourceID:   sourceExternalID,
		TargetID:   targetExternalID,
		Type:       topology.Type{Name: typeName},
		Data:       map[string]interface{}{},
	}
}

// buildClusterExternalID
func (c *clusterTopologyCollector) buildClusterExternalID() string {
	return fmt.Sprintf("urn:cluster:%s/%s", c.Instance.Type, c.Instance.URL)
}

// buildNodeExternalID
// nodeName
func (c *clusterTopologyCollector) buildNodeExternalID(nodeName string) string {
	return fmt.Sprintf("urn:/%s:%s:node:%s", c.Instance.Type, c.Instance.URL, nodeName)
}

// buildPodExternalID
// podName
func (c *clusterTopologyCollector) buildPodExternalID(podName string) string {
	return fmt.Sprintf("urn:/%s:%s:pod:%s", c.Instance.Type, c.Instance.URL, podName)
}

// buildContainerExternalID
// podName, containerName
func (c *clusterTopologyCollector) buildContainerExternalID(podName, containerName string) string {
	return fmt.Sprintf("urn:/%s:%s:pod:%s:container:%s", c.Instance.Type, c.Instance.URL, podName, containerName)
}

// buildServiceExternalID
// serviceID
func (c *clusterTopologyCollector) buildServiceExternalID(serviceID string) string {
	return fmt.Sprintf("urn:/%s:%s:service:%s", c.Instance.Type, c.Instance.URL, serviceID)
}

// buildDaemonSetExternalID
// daemonSetId
func (c *clusterTopologyCollector) buildDaemonSetExternalID(daemonSetId string) string {
	return fmt.Sprintf("urn:/%s:%s:daemonset:%s", c.Instance.Type, c.Instance.URL, daemonSetId)
}

// buildDeploymentExternalID
// deploymentID
func (c *clusterTopologyCollector) buildDeploymentExternalID(namespace, deploymentID string) string {
	return fmt.Sprintf("urn:/%s:%s:deployment:%s:%s", c.Instance.Type, c.Instance.URL, namespace, deploymentID)
}

// buildReplicaSetExternalID
// replicaSetID
func (c *clusterTopologyCollector) buildReplicaSetExternalID(replicaSetID string) string {
	return fmt.Sprintf("urn:/%s:%s:replicaset:%s", c.Instance.Type, c.Instance.URL, replicaSetID)
}

// buildStatefulSetExternalID
// statefulSetID
func (c *clusterTopologyCollector) buildStatefulSetExternalID(statefulSetID string) string {
	return fmt.Sprintf("urn:/%s:%s:statefulset:%s", c.Instance.Type, c.Instance.URL, statefulSetID)
}

// buildConfigMapExternalID
// namespace
// configMapID
func (c *clusterTopologyCollector) buildConfigMapExternalID(namespace, configMapID string) string {
	return fmt.Sprintf("urn:/%s:%s:configmap:%s:%s", c.Instance.Type, c.Instance.URL, namespace, configMapID)
}

// buildCronJobExternalID
// cronJobID
func (c *clusterTopologyCollector) buildCronJobExternalID(cronJobID string) string {
	return fmt.Sprintf("urn:/%s:%s:cronjob:%s", c.Instance.Type, c.Instance.URL, cronJobID)
}

// buildJobExternalID
// jobID
func (c *clusterTopologyCollector) buildJobExternalID(jobID string) string {
	return fmt.Sprintf("urn:/%s:%s:job:%s", c.Instance.Type, c.Instance.URL, jobID)
}

// buildIngressExternalID
// ingressID
func (c *clusterTopologyCollector) buildIngressExternalID(ingressID string) string {
	return fmt.Sprintf("urn:/%s:%s:ingress:%s", c.Instance.Type, c.Instance.URL, ingressID)
}

// buildVolumeExternalID
// volumeID
func (c *clusterTopologyCollector) buildVolumeExternalID(volumeID string) string {
	return fmt.Sprintf("urn:/%s:%s:volume:%s", c.Instance.Type, c.Instance.URL, volumeID)
}

// buildPersistentVolumeExternalID
// persistentVolumeID
func (c *clusterTopologyCollector) buildPersistentVolumeExternalID(persistentVolumeID string) string {
	return fmt.Sprintf("urn:/%s:%s:persistent-volume:%s", c.Instance.Type, c.Instance.URL, persistentVolumeID)
}

// buildEndpointExternalID
// endpointID
func (c *clusterTopologyCollector) buildEndpointExternalID(endpointID string) string {
	return fmt.Sprintf("urn:endpoint:/%s:%s", c.Instance.URL, endpointID)
}

// addClusterNameTag
// tags
func (c *clusterTopologyCollector) addClusterNameTag(tags map[string]string) map[string]string {
	tags["cluster-name"] = c.Instance.URL
	return tags
}
