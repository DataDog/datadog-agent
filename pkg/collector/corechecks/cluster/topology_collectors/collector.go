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

// ContainerCorrelation
type ContainerCorrelation struct {
	NodeName string
	MappingFunction func (nodeIdentifier string) (components []*topology.Component, relations []*topology.Relation)
}

// ClusterTopologyCollector collects cluster components and relations.
type ClusterTopologyCollector interface {
	CollectorFunction() error
	GetAPIClient() *apiserver.APIClient
	GetInstance() topology.Instance
	GetName() string
	addClusterNameTag(tags map[string]string) map[string]string
	buildClusterExternalID() string
	buildConfigMapExternalID(configMapName string) string
	buildContainerExternalID(podName, containerName string) string
	buildDaemonSetExternalID(daemonSetName string) string
	buildDeploymentExternalID(deploymentName string) string
	buildNodeExternalID(nodeName string) string
	buildPodExternalID(podName string) string
	buildReplicaSetExternalID(replicaSetName string) string
	buildServiceExternalID(serviceID string) string
	buildStatefulSetExternalID(statefulSetName string) string
	buildCronJobExternalID(cronJobName string) string
	buildJobExternalID(jobName string) string
	buildIngressExternalID(ingressName string) string
	buildPersistentVolumeExternalID(volumeName string) string
	buildVolumeExternalID(volumeName string) string
	buildEndpointExternalID(endpointID string) string
}

type clusterTopologyCollector struct {
	Instance        topology.Instance
	APIClient                    *apiserver.APIClient
}

// NewClusterTopologyCollector
func NewClusterTopologyCollector(instance topology.Instance, ac *apiserver.APIClient) ClusterTopologyCollector {
	return &clusterTopologyCollector{ Instance:instance, APIClient: ac }
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
func (c *clusterTopologyCollector) GetAPIClient() *apiserver.APIClient {
	return c.APIClient
}

// CollectorFunction
func (c *clusterTopologyCollector) CollectorFunction() error {
	return errors.New("CollectorFunction NotImplemented")
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
func (c *clusterTopologyCollector) buildDeploymentExternalID(deploymentID string) string {
	return fmt.Sprintf("urn:/%s:%s:deployment:%s", c.Instance.Type, c.Instance.URL, deploymentID)
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
// configMapID
func (c *clusterTopologyCollector) buildConfigMapExternalID(configMapID string) string {
	return fmt.Sprintf("urn:/%s:%s:configmap:%s", c.Instance.Type, c.Instance.URL, configMapID)
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

// buildPersistentVolumeExternalID
// volumeID
func (c *clusterTopologyCollector) buildPersistentVolumeExternalID(volumeID string) string {
	return fmt.Sprintf("urn:/%s:%s:persistent-volume:%s", c.Instance.Type, c.Instance.URL, volumeID)
}

// buildVolumeExternalID
// volumeID
func (c *clusterTopologyCollector) buildVolumeExternalID(volumeID string) string {
	return fmt.Sprintf("urn:/%s:%s:volume:%s", c.Instance.Type, c.Instance.URL, volumeID)
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
