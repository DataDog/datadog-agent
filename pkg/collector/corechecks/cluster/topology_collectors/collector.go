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
	buildContainerExternalID(clusterName, podName, containerName string) string
	buildDaemonSetExternalID(clusterName, daemonSetName string) string
	buildDeploymentExternalID(clusterName, deploymentName string) string
	buildNodeExternalID(clusterName, nodeName string) string
	buildPodExternalID(clusterName, podName string) string
	buildReplicaSetExternalID(clusterName, replicaSetName string) string
	buildServiceExternalID(clusterName, serviceID string) string
	buildStatefulSetExternalID(clusterName, statefulSetName string) string
}

type clusterTopologyCollector struct {
	Instance        topology.Instance
	APIClient                    *apiserver.APIClient
}

// NewClusterTopologyCollector
func NewClusterTopologyCollector(instance topology.Instance, ac *apiserver.APIClient) ClusterTopologyCollector {
	return &clusterTopologyCollector{instance, ac}
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

func (c *clusterTopologyCollector) CollectorFunction() error {
	return errors.New("CollectorFunction NotImplemented")
}

func (c *clusterTopologyCollector) buildClusterExternalID() string {
	return fmt.Sprintf("urn:cluster:%s/%s", c.Instance.Type, c.Instance.URL)
}

func (c *clusterTopologyCollector) buildNodeExternalID(clusterName, nodeName string) string {
	return fmt.Sprintf("urn:/%s:%s:node:%s", c.Instance.Type, clusterName, nodeName)
}

func (c *clusterTopologyCollector) buildPodExternalID(clusterName, podName string) string {
	return fmt.Sprintf("urn:/%s:%s:pod:%s", c.Instance.Type, clusterName, podName)
}

func (c *clusterTopologyCollector) buildContainerExternalID(clusterName, podName, containerName string) string {
	return fmt.Sprintf("urn:/%s:%s:pod:%s:container:%s", c.Instance.Type, clusterName, podName, containerName)
}

func (c *clusterTopologyCollector) buildServiceExternalID(clusterName, serviceID string) string {
	return fmt.Sprintf("urn:/%s:%s:service:%s", c.Instance.Type, clusterName, serviceID)
}

func (c *clusterTopologyCollector) buildDaemonSetExternalID(clusterName, daemonSetName string) string {
	return fmt.Sprintf("urn:/%s:%s:daemonset:%s", c.Instance.Type, clusterName, daemonSetName)
}

func (c *clusterTopologyCollector) buildDeploymentExternalID(clusterName, deploymentName string) string {
	return fmt.Sprintf("urn:/%s:%s:deployment:%s", c.Instance.Type, clusterName, deploymentName)
}

func (c *clusterTopologyCollector) buildReplicaSetExternalID(clusterName, replicaSetName string) string {
	return fmt.Sprintf("urn:/%s:%s:replicaset:%s", c.Instance.Type, clusterName, replicaSetName)
}

func (c *clusterTopologyCollector) buildStatefulSetExternalID(clusterName, statefulSetName string) string {
	return fmt.Sprintf("urn:/%s:%s:statefulset:%s", c.Instance.Type, clusterName, statefulSetName)
}

func (c *clusterTopologyCollector) addClusterNameTag(tags map[string]string) map[string]string {
	tags["cluster-name"] = c.Instance.URL
	return tags
}
