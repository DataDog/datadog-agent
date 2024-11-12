// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	"strings"

	"github.com/patrickmn/go-cache"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NodeType represents a kind of resource used by a container orchestrator.
type NodeType int

// CheckName is the cluster check name of the orchestrator check
const CheckName = "orchestrator"

// ExtraLogContext is used to add check name into log context
var ExtraLogContext = []interface{}{"check", CheckName}

// NoExpiration maps to go-cache corresponding value
const NoExpiration = cache.NoExpiration

// The values in these enfms should match the values defined in the agent payload schema, defined here:
// https://github.com/DataDog/agent-payload/blob/master/proto/process/agent.proto (within enum K8sResource)
// we do not utilize iota as these types are used in external systems, not just within the agent instance.
const (
	// K8sUnsetType represents a Kubernetes unset type
	K8sUnsetType NodeType = 0
	// K8sPod represents a Kubernetes Pod
	K8sPod = 1
	// K8sReplicaSet represents a Kubernetes ReplicaSet
	K8sReplicaSet = 2
	// K8sService represents a Kubernetes Service
	K8sService = 3
	// K8sNode represents a Kubernetes Node
	K8sNode = 4
	// K8sCluster represents a Kubernetes Cluster
	K8sCluster = 5
	// K8sJob represents a Kubernetes Job
	K8sJob = 6
	// K8sCronJob represents a Kubernetes CronJob
	K8sCronJob = 7
	// K8sDaemonSet represents a Kubernetes DaemonSet
	K8sDaemonSet = 8
	// K8sStatefulSet represents a Kubernetes StatefulSet
	K8sStatefulSet = 9
	// K8sPersistentVolume represents a Kubernetes PersistentVolume
	K8sPersistentVolume = 10
	// K8sPersistentVolumeClaim represents a Kubernetes PersistentVolumeClaim
	K8sPersistentVolumeClaim = 11
	// K8sRole represents a Kubernetes Role
	K8sRole = 12
	// K8sRoleBinding represents a Kubernetes RoleBinding
	K8sRoleBinding = 13
	// K8sClusterRole represents a Kubernetes ClusterRole
	K8sClusterRole = 14
	// K8sClusterRoleBinding represents a Kubernetes ClusterRoleBinding
	K8sClusterRoleBinding = 15
	// K8sServiceAccount represents a Kubernetes ServiceAccount
	K8sServiceAccount = 16
	// K8sIngress represents a Kubernetes Ingress
	K8sIngress = 17
	// K8sDeployment represents a Kubernetes Deployment
	K8sDeployment = 18
	// K8sNamespace represents a Kubernetes Namespace
	K8sNamespace = 19
	// K8sCRD represents a Kubernetes CRD
	K8sCRD = 20
	// K8sCR represents a Kubernetes CR
	K8sCR = 21
	// K8sVerticalPodAutoscaler represents a Kubernetes VerticalPod Autoscaler
	K8sVerticalPodAutoscaler = 22
	// K8sHorizontalPodAutoscaler represents a Kubernetes Horizontal Pod Autoscaler
	K8sHorizontalPodAutoscaler = 23
	// K8sNetworkPolicy represents a Kubernetes NetworkPolicy
	K8sNetworkPolicy = 24
	// K8sLimitRange represents a Kubernetes LimitRange
	K8sLimitRange = 25
	// K8sStorageClass represents a Kubernetes StorageClass
	K8sStorageClass = 26
	// K8sPodDisruptionBudget represents a Kubernetes PodDisruptionBudget
	K8sPodDisruptionBudget = 27
	// ECSTask represents an ECS Task
	ECSTask = 150
)

// NodeTypes returns the current existing NodesTypes as a slice to iterate over.
func NodeTypes() []NodeType {
	return []NodeType{
		ECSTask,
		K8sCR,
		K8sCRD,
		K8sCluster,
		K8sClusterRole,
		K8sClusterRoleBinding,
		K8sCronJob,
		K8sDaemonSet,
		K8sDeployment,
		K8sHorizontalPodAutoscaler,
		K8sIngress,
		K8sJob,
		K8sLimitRange,
		K8sNamespace,
		K8sNetworkPolicy,
		K8sNode,
		K8sPersistentVolume,
		K8sPersistentVolumeClaim,
		K8sPod,
		K8sPodDisruptionBudget,
		K8sReplicaSet,
		K8sRole,
		K8sRoleBinding,
		K8sService,
		K8sServiceAccount,
		K8sStatefulSet,
		K8sStorageClass,
		K8sVerticalPodAutoscaler,
	}
}

func (n NodeType) String() string {
	switch n {
	case K8sCluster:
		return "Cluster"
	case K8sCronJob:
		return "CronJob"
	case K8sDeployment:
		return "Deployment"
	case K8sDaemonSet:
		return "DaemonSet"
	case K8sJob:
		return "Job"
	case K8sNode:
		return "Node"
	case K8sPod:
		return "Pod"
	case K8sReplicaSet:
		return "ReplicaSet"
	case K8sService:
		return "Service"
	case K8sStatefulSet:
		return "StatefulSet"
	case K8sPersistentVolume:
		return "PersistentVolume"
	case K8sPersistentVolumeClaim:
		return "PersistentVolumeClaim"
	case K8sRole:
		return "Role"
	case K8sRoleBinding:
		return "RoleBinding"
	case K8sClusterRole:
		return "ClusterRole"
	case K8sClusterRoleBinding:
		return "ClusterRoleBinding"
	case K8sServiceAccount:
		return "ServiceAccount"
	case K8sIngress:
		return "Ingress"
	case K8sNamespace:
		return "Namespace"
	case K8sCRD:
		return "CustomResourceDefinition"
	case K8sCR:
		return "CustomResource"
	case K8sVerticalPodAutoscaler:
		return "VerticalPodAutoscaler"
	case K8sHorizontalPodAutoscaler:
		return "HorizontalPodAutoscaler"
	case K8sNetworkPolicy:
		return "NetworkPolicy"
	case K8sLimitRange:
		return "LimitRange"
	case K8sStorageClass:
		return "StorageClass"
	case K8sUnsetType:
		return "UnsetType"
	case ECSTask:
		return "ECSTask"
	case K8sPodDisruptionBudget:
		return "PodDisruptionBudget"
	default:
		_ = log.Errorf("Trying to convert unknown NodeType iota: %d", n)
		return "Unknown"
	}
}

// Orchestrator returns the orchestrator name for a node type.
func (n NodeType) Orchestrator() string {
	switch n {
	case K8sCluster,
		K8sCronJob,
		K8sDeployment,
		K8sDaemonSet,
		K8sJob,
		K8sNode,
		K8sPod,
		K8sReplicaSet,
		K8sService,
		K8sStatefulSet,
		K8sPersistentVolume,
		K8sPersistentVolumeClaim,
		K8sRole,
		K8sRoleBinding,
		K8sClusterRole,
		K8sClusterRoleBinding,
		K8sServiceAccount,
		K8sIngress,
		K8sCRD,
		K8sCR,
		K8sNamespace,
		K8sVerticalPodAutoscaler,
		K8sHorizontalPodAutoscaler,
		K8sNetworkPolicy,
		K8sLimitRange,
		K8sStorageClass,
		K8sUnsetType:
		return "k8s"
	case ECSTask:
		return "ecs"
	default:
		log.Errorf("Unknown NodeType %v", n)
		return ""
	}
}

// TelemetryTags return tags used for telemetry.
func (n NodeType) TelemetryTags() []string {
	if n.String() == "" {
		log.Errorf("Unknown NodeType %v", n)
		return []string{"unknown", "unknown"}
	}
	tags := getTelemetryTags(n)
	return tags
}

func getTelemetryTags(n NodeType) []string {
	return []string{
		n.Orchestrator(),
		strings.ToLower(n.String()),
	}
}

// SetCacheStats sets the cache stats for each resource
func SetCacheStats(resourceListLen int, resourceMsgLen int, nodeType NodeType, ca *cache.Cache) {
	stats := CheckStats{
		CacheHits: resourceListLen - resourceMsgLen,
		CacheMiss: resourceMsgLen,
		NodeType:  nodeType,
	}
	ca.Set(BuildStatsKey(nodeType), stats, NoExpiration)
}
