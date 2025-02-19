// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package orchestrator provides functions and stats for container orchestrators
package orchestrator

import (
	pkgorchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
)

// Aliases to orchestrator/model package
type (
	// CheckStats alias for pkgorchestratormodel.CheckStats
	CheckStats = pkgorchestratormodel.CheckStats
)

var (
	// BuildStatsKey alias for pkgorchestratormodel.BuildStatsKey
	BuildStatsKey = pkgorchestratormodel.BuildStatsKey

	// ExtraLogContext is used to add check name into log context
	ExtraLogContext = pkgorchestratormodel.ExtraLogContext
)

const (
	// CheckName is the cluster check name of the orchestrator check
	CheckName = pkgorchestratormodel.CheckName
	// NoExpiration maps to go-cache corresponding value
	NoExpiration = pkgorchestratormodel.NoExpiration
	// K8sUnsetType alias for pkgorchestratormodel.K8sUnsetType
	K8sUnsetType = pkgorchestratormodel.K8sUnsetType
	// K8sPod alias for pkgorchestratormodel.K8sPod
	K8sPod = pkgorchestratormodel.K8sPod
	// K8sReplicaSet alias for pkgorchestratormodel.K8sReplicaSet
	K8sReplicaSet = pkgorchestratormodel.K8sReplicaSet
	// K8sService alias for pkgorchestratormodel.K8sService
	K8sService = pkgorchestratormodel.K8sService
	// K8sNode alias for pkgorchestratormodel.K8sNode
	K8sNode = pkgorchestratormodel.K8sNode
	// K8sCluster alias for pkgorchestratormodel.K8sCluster
	K8sCluster = pkgorchestratormodel.K8sCluster
	// K8sJob alias for pkgorchestratormodel.K8sJob
	K8sJob = pkgorchestratormodel.K8sJob
	// K8sCronJob alias for pkgorchestratormodel.K8sCronJob
	K8sCronJob = pkgorchestratormodel.K8sCronJob
	// K8sDaemonSet alias for pkgorchestratormodel.K8sDaemonSet
	K8sDaemonSet = pkgorchestratormodel.K8sDaemonSet
	// K8sStatefulSet alias for pkgorchestratormodel.K8sStatefulSet
	K8sStatefulSet = pkgorchestratormodel.K8sStatefulSet
	// K8sPersistentVolume alias for pkgorchestratormodel.K8sPersistentVolume
	K8sPersistentVolume = pkgorchestratormodel.K8sPersistentVolume
	// K8sPersistentVolumeClaim alias for pkgorchestratormodel.K8sPersistentVolumeClaim
	K8sPersistentVolumeClaim = pkgorchestratormodel.K8sPersistentVolumeClaim
	// K8sRole alias for pkgorchestratormodel.K8sRole
	K8sRole = pkgorchestratormodel.K8sRole
	// K8sRoleBinding alias for pkgorchestratormodel.K8sRoleBinding
	K8sRoleBinding = pkgorchestratormodel.K8sRoleBinding
	// K8sClusterRole alias for pkgorchestratormodel.K8sClusterRole
	K8sClusterRole = pkgorchestratormodel.K8sClusterRole
	// K8sClusterRoleBinding alias for pkgorchestratormodel.K8sClusterRoleBinding
	K8sClusterRoleBinding = pkgorchestratormodel.K8sClusterRoleBinding
	// K8sServiceAccount alias for pkgorchestratormodel.K8sServiceAccount
	K8sServiceAccount = pkgorchestratormodel.K8sServiceAccount
	// K8sIngress alias for pkgorchestratormodel.K8sIngress
	K8sIngress = pkgorchestratormodel.K8sIngress
	// K8sDeployment alias for pkgorchestratormodel.K8sDeployment
	K8sDeployment = pkgorchestratormodel.K8sDeployment
	// K8sNamespace alias for pkgorchestratormodel.K8sNamespace
	K8sNamespace = pkgorchestratormodel.K8sNamespace
	// K8sCRD alias for pkgorchestratormodel.K8sCRD
	K8sCRD = pkgorchestratormodel.K8sCRD
	// K8sCR alias for pkgorchestratormodel.K8sCR
	K8sCR = pkgorchestratormodel.K8sCR
	// K8sVerticalPodAutoscaler alias for pkgorchestratormodel.K8sVerticalPodAutoscaler
	K8sVerticalPodAutoscaler = pkgorchestratormodel.K8sVerticalPodAutoscaler
	// K8sHorizontalPodAutoscaler alias for pkgorchestratormodel.K8sHorizontalPodAutoscaler
	K8sHorizontalPodAutoscaler = pkgorchestratormodel.K8sHorizontalPodAutoscaler
	// K8sNetworkPolicy alias for pkgorchestratormodel.K8sNetworkPolicy
	K8sNetworkPolicy = pkgorchestratormodel.K8sNetworkPolicy
	// K8sLimitRange alias for pkgorchestratormodel.K8sLimitRange
	K8sLimitRange = pkgorchestratormodel.K8sLimitRange
	// K8sStorageClass alias for pkgorchestratormodel.K8sStorageClass
	K8sStorageClass = pkgorchestratormodel.K8sStorageClass
	// K8sPodDisruptionBudget alias for pkgorchestratormodel.K8sPodDisruptionBudget
	K8sPodDisruptionBudget = pkgorchestratormodel.K8sPodDisruptionBudget
	// ECSTask alias for pkgorchestratormodel.ECSTask
	ECSTask = pkgorchestratormodel.ECSTask
)

// SetCacheStats alias for pkgorchestratormodel.SetCacheStats
func SetCacheStats(resourceListLen int, resourceMsgLen int, nodeType pkgorchestratormodel.NodeType) {
	pkgorchestratormodel.SetCacheStats(resourceListLen, resourceMsgLen, nodeType, KubernetesResourceCache)
}
