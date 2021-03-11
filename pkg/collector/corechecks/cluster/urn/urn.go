package urn

import (
	"fmt"
)

// ClusterType represents the type of K8s Cluster
type ClusterType string

const (
	// Kubernetes is a Generic K8s cluster
	Kubernetes ClusterType = "kubernetes"
	// OpenShift is a RH OpenShift K8s cluster
	OpenShift ClusterType = "openshift"
)

// Builder builds StackState compatible URNs for Kubernetes components
type Builder interface {
	URNPrefix() string
	BuildExternalID(kind, namespace, objName string) (string, error)
	BuildClusterExternalID() string
	BuildConfigMapExternalID(namespace, configMapName string) string
	BuildNamespaceExternalID(namespaceName string) string
	BuildContainerExternalID(namespace, podName, containerName string) string
	BuildDaemonSetExternalID(namespace, daemonSetName string) string
	BuildDeploymentExternalID(namespace, deploymentName string) string
	BuildNodeExternalID(nodeName string) string
	BuildPodExternalID(namespace, podName string) string
	BuildReplicaSetExternalID(namespace, replicaSetName string) string
	BuildServiceExternalID(namespace, serviceName string) string
	BuildStatefulSetExternalID(namespace, statefulSetName string) string
	BuildCronJobExternalID(namespace, cronJobName string) string
	BuildJobExternalID(namespace, jobName string) string
	BuildIngressExternalID(namespace, ingressName string) string
	BuildVolumeExternalID(namespace, volumeName string) string
	BuildPersistentVolumeExternalID(persistentVolumeName string) string
	BuildComponentExternalID(component, namespace, name string) string
	BuildEndpointExternalID(endpointID string) string
}

type urnBuilder struct {
	clusterType ClusterType
	url         string
	urnPrefix   string
}

// NewURNBuilder creates a new URNBuilder
func NewURNBuilder(clusterType ClusterType, url string) Builder {
	return &urnBuilder{
		clusterType: clusterType,
		url:         url,
		urnPrefix:   buildURNPrefix(clusterType, url),
	}
}

func buildURNPrefix(clusterType ClusterType, url string) string {
	return fmt.Sprintf("urn:%s:/%s", clusterType, url)
}

// URNPrefix
func (b *urnBuilder) URNPrefix() string {
	return b.urnPrefix
}

func (b *urnBuilder) BuildExternalID(kind, namespace, objName string) (string, error) {
	var urn string
	switch kind {
	case "ConfigMap":
		urn = b.BuildConfigMapExternalID(namespace, objName)
	case "Namespace":
		urn = b.BuildNamespaceExternalID(objName)
	case "DaemonSet":
		urn = b.BuildDaemonSetExternalID(namespace, objName)
	case "Deployment":
		urn = b.BuildDeploymentExternalID(namespace, objName)
	case "Node":
		urn = b.BuildNodeExternalID(objName)
	case "Pod":
		urn = b.BuildPodExternalID(namespace, objName)
	case "ReplicaSet":
		urn = b.BuildReplicaSetExternalID(namespace, objName)
	case "Service":
		urn = b.BuildServiceExternalID(namespace, objName)
	case "StatefulSet":
		urn = b.BuildStatefulSetExternalID(namespace, objName)
	case "CronJob":
		urn = b.BuildCronJobExternalID(namespace, objName)
	case "Job":
		urn = b.BuildJobExternalID(namespace, objName)
	case "Ingress":
		urn = b.BuildIngressExternalID(namespace, objName)
	case "Volume":
		urn = b.BuildVolumeExternalID(namespace, objName)
	case "PersistentVolume":
		urn = b.BuildPersistentVolumeExternalID(objName)
	case "Endpoint":
		urn = b.BuildEndpointExternalID(objName)
	}

	if urn == "" {
		return "", fmt.Errorf("Encountered unknown kind '%s' for '%s/%s'", kind, namespace, objName)
	}

	return urn, nil
}

// BuildClusterExternalID
func (b *urnBuilder) BuildClusterExternalID() string {
	return fmt.Sprintf("urn:cluster:/%s:%s", b.clusterType, b.url)
}

// BuildNodeExternalID creates the urn external identifier for a cluster node
func (b *urnBuilder) BuildNodeExternalID(nodeName string) string {
	return fmt.Sprintf("%s:node/%s", b.urnPrefix, nodeName)
}

// BuildPodExternalID creates the urn external identifier for a cluster pod
func (b *urnBuilder) BuildPodExternalID(namespace, podName string) string {
	return b.BuildComponentExternalID("pod", namespace, podName)
}

// BuildContainerExternalID creates the urn external identifier for a pod's container
func (b *urnBuilder) BuildContainerExternalID(namespace, podName, containerName string) string {
	return fmt.Sprintf("%s:container/%s", b.BuildPodExternalID(namespace, podName), containerName)
}

// BuildServiceExternalID creates the urn external identifier for a cluster service
func (b *urnBuilder) BuildServiceExternalID(namespace, serviceName string) string {
	return b.BuildComponentExternalID("service", namespace, serviceName)
}

// BuildDaemonSetExternalID creates the urn external identifier for a cluster daemon set
func (b *urnBuilder) BuildDaemonSetExternalID(namespace, daemonSetName string) string {
	return b.BuildComponentExternalID("daemonset", namespace, daemonSetName)
}

// BuildDeploymentExternalID creates the urn external identifier for a cluster deployment
func (b *urnBuilder) BuildDeploymentExternalID(namespace, deploymentName string) string {
	return b.BuildComponentExternalID("deployment", namespace, deploymentName)
}

// BuildReplicaSetExternalID creates the urn external identifier for a cluster replica set
func (b *urnBuilder) BuildReplicaSetExternalID(namespace, replicaSetName string) string {
	return b.BuildComponentExternalID("replicaset", namespace, replicaSetName)
}

// BuildStatefulSetExternalID creates the urn external identifier for a cluster stateful set
func (b *urnBuilder) BuildStatefulSetExternalID(namespace, statefulSetName string) string {
	return b.BuildComponentExternalID("statefulset", namespace, statefulSetName)
}

// BuildConfigMapExternalID creates the urn external identifier for a cluster config map
func (b *urnBuilder) BuildConfigMapExternalID(namespace, configMapName string) string {
	return b.BuildComponentExternalID("configmap", namespace, configMapName)
}

// BuildNamespaceExternalID creates the urn external identifier for a cluster namespace
func (b *urnBuilder) BuildNamespaceExternalID(namespaceName string) string {
	return b.BuildComponentExternalID("namespace", "", namespaceName)
}

// BuildCronJobExternalID creates the urn external identifier for a cluster cron job
func (b *urnBuilder) BuildCronJobExternalID(namespace, cronJobName string) string {
	return b.BuildComponentExternalID("cronjob", namespace, cronJobName)
}

// BuildJobExternalID creates the urn external identifier for a cluster job
func (b *urnBuilder) BuildJobExternalID(namespace, jobName string) string {
	return b.BuildComponentExternalID("job", namespace, jobName)
}

// BuildIngressExternalID creates the urn external identifier for a cluster ingress
func (b *urnBuilder) BuildIngressExternalID(namespace, ingressName string) string {
	return b.BuildComponentExternalID("ingress", namespace, ingressName)
}

// BuildVolumeExternalID creates the urn external identifier for a cluster volume
func (b *urnBuilder) BuildVolumeExternalID(namespace, volumeName string) string {
	return b.BuildComponentExternalID("volume", namespace, volumeName)
}

// BuildPersistentVolumeExternalID creates the urn external identifier for a cluster persistent volume
func (b *urnBuilder) BuildPersistentVolumeExternalID(persistentVolumeName string) string {
	return b.BuildComponentExternalID("persistent-volume", "", persistentVolumeName)
}

// BuildComponentExternalID creates the urn external identifier for a specific component type
func (b *urnBuilder) BuildComponentExternalID(component, namespace, name string) string {
	if namespace != "" {
		return fmt.Sprintf("%s:%s:%s/%s", b.urnPrefix, namespace, component, name)
	}

	return fmt.Sprintf("%s:%s/%s", b.urnPrefix, component, name)
}

// BuildEndpointExternalID
// endpointID
func (b *urnBuilder) BuildEndpointExternalID(endpointID string) string {
	return fmt.Sprintf("urn:endpoint:/%s:%s", b.url, endpointID)
}

// ClusterTypeFromString converts a string representation of the ClusterType to the specific ClusterType
func ClusterTypeFromString(s string) ClusterType {
	if s == string(OpenShift) {
		return OpenShift
	}

	return Kubernetes
}
