// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build orchestrator

package util

import (
	"fmt"
	"strings"
)

// Kubernetes resource names and API versions used by the orchestrator collectors.
// These constants define the resource names and their corresponding API versions
// for various Kubernetes resources that the orchestrator can collect and monitor.
const (
	// ClusterName is the resource name for Kubernetes clusters
	ClusterName = "clusters"

	// ClusterRoleName is the resource name for Kubernetes ClusterRoles
	ClusterRoleName = "clusterroles"
	// ClusterRoleVersion is the API version for ClusterRole resources
	ClusterRoleVersion = "rbac.authorization.k8s.io/v1"

	// ClusterRoleBindingName is the resource name for Kubernetes ClusterRoleBindings
	ClusterRoleBindingName = "clusterrolebindings"
	// ClusterRoleBindingVersion is the API version for ClusterRoleBinding resources
	ClusterRoleBindingVersion = "rbac.authorization.k8s.io/v1"

	// CrdName is the resource name for Kubernetes CustomResourceDefinitions
	CrdName = "customresourcedefinitions"
	// CrdVersion is the API version for CustomResourceDefinition resources
	CrdVersion = "apiextensions.k8s.io/v1"

	// CronJobName is the resource name for Kubernetes CronJobs
	CronJobName = "cronjobs"
	// CronJobVersionV1 is the stable API version for CronJob resources
	CronJobVersionV1 = "batch/v1"
	// CronJobVersionV1Beta1 is the beta API version for CronJob resources
	CronJobVersionV1Beta1 = "batch/v1beta1"

	// DaemonSetName is the resource name for Kubernetes DaemonSets
	DaemonSetName = "daemonsets"
	// DaemonSetVersion is the API version for DaemonSet resources
	DaemonSetVersion = "apps/v1"

	// DeploymentName is the resource name for Kubernetes Deployments
	DeploymentName = "deployments"
	// DeploymentVersion is the API version for Deployment resources
	DeploymentVersion = "apps/v1"

	// HpaName is the resource name for Kubernetes HorizontalPodAutoscalers
	HpaName = "horizontalpodautoscalers"
	// HpaVersion is the API version for HorizontalPodAutoscaler resources
	HpaVersion = "autoscaling/v2"

	// IngressName is the resource name for Kubernetes Ingresses
	IngressName = "ingresses"
	// IngressVersion is the API version for Ingress resources
	IngressVersion = "networking.k8s.io/v1"

	// JobName is the resource name for Kubernetes Jobs
	JobName = "jobs"
	// JobVersion is the API version for Job resources
	JobVersion = "batch/v1"

	// LimitRangeName is the resource name for Kubernetes LimitRanges
	LimitRangeName = "limitranges"
	// LimitRangeVersion is the API version for LimitRange resources
	LimitRangeVersion = "v1"

	// NamespaceName is the resource name for Kubernetes Namespaces
	NamespaceName = "namespaces"
	// NamespaceVersion is the API version for Namespace resources
	NamespaceVersion = "v1"

	// NetworkPolicyName is the resource name for Kubernetes NetworkPolicies
	NetworkPolicyName = "networkpolicies"
	// NetworkPolicyVersion is the API version for NetworkPolicy resources
	NetworkPolicyVersion = "networking.k8s.io/v1"

	// NodeName is the resource name for Kubernetes Nodes
	NodeName = "nodes"
	// NodeVersion is the API version for Node resources
	NodeVersion = "v1"

	// PersistentVolumeName is the resource name for Kubernetes PersistentVolumes
	PersistentVolumeName = "persistentvolumes"
	// PersistentVolumeVersion is the API version for PersistentVolume resources
	PersistentVolumeVersion = "v1"

	// PersistentVolumeClaimName is the resource name for Kubernetes PersistentVolumeClaims
	PersistentVolumeClaimName = "persistentvolumeclaims"
	// PersistentVolumeClaimVersion is the API version for PersistentVolumeClaim resources
	PersistentVolumeClaimVersion = "v1"

	// PodName is the resource name for Kubernetes Pods
	PodName = "pods"
	// PodVersion is the API version for Pod resources
	PodVersion = "v1"

	// PodDisruptionBudgetName is the resource name for Kubernetes PodDisruptionBudgets
	PodDisruptionBudgetName = "poddisruptionbudgets"
	// PodDisruptionBudgetVersion is the API version for PodDisruptionBudget resources
	PodDisruptionBudgetVersion = "policy/v1"

	// ReplicaSetName is the resource name for Kubernetes ReplicaSets
	ReplicaSetName = "replicasets"
	// ReplicaSetVersion is the API version for ReplicaSet resources
	ReplicaSetVersion = "apps/v1"

	// RoleName is the resource name for Kubernetes Roles
	RoleName = "roles"
	// RoleVersion is the API version for Role resources
	RoleVersion = "rbac.authorization.k8s.io/v1"

	// RoleBindingName is the resource name for Kubernetes RoleBindings
	RoleBindingName = "rolebindings"
	// RoleBindingVersion is the API version for RoleBinding resources
	RoleBindingVersion = "rbac.authorization.k8s.io/v1"

	// ServiceName is the resource name for Kubernetes Services
	ServiceName = "services"
	// ServiceVersion is the API version for Service resources
	ServiceVersion = "v1"

	// ServiceAccountName is the resource name for Kubernetes ServiceAccounts
	ServiceAccountName = "serviceaccounts"
	// ServiceAccountVersion is the API version for ServiceAccount resources
	ServiceAccountVersion = "v1"

	// StatefulSetName is the resource name for Kubernetes StatefulSets
	StatefulSetName = "statefulsets"
	// StatefulSetVersion is the API version for StatefulSet resources
	StatefulSetVersion = "apps/v1"

	// StorageClassName is the resource name for Kubernetes StorageClasses
	StorageClassName = "storageclasses"
	// StorageClassVersion is the API version for StorageClass resources
	StorageClassVersion = "storage.k8s.io/v1"

	// VpaName is the resource name for Kubernetes VerticalPodAutoscalers
	VpaName = "verticalpodautoscalers"
	// VpaVersion is the API version for VerticalPodAutoscaler resources
	VpaVersion = "autoscaling.k8s.io/v1"
)

// GetResourceType returns a string in the format "name.apiGroup" if an API group is present in the version.
// Otherwise, it returns the name.
func GetResourceType(name string, version string) string {
	apiVersionParts := strings.Split(version, "/")
	if len(apiVersionParts) == 2 {
		apiGroup := apiVersionParts[0]
		return fmt.Sprintf("%s.%s", name, apiGroup)
	}
	return name
}
