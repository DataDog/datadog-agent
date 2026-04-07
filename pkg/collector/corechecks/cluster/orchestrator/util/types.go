// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build orchestrator

package util

// Kubernetes resource names, API groups, and API versions used by the orchestrator collectors.
// These constants define the resource names and their corresponding API groups and versions
// for various Kubernetes resources that the orchestrator can collect and monitor.
const (
	// ClusterName is the resource name for Kubernetes clusters
	ClusterName = "clusters"

	// ClusterRoleName is the resource name for Kubernetes ClusterRoles
	ClusterRoleName = "clusterroles"
	// ClusterRoleGroup is the API group for ClusterRole resources
	ClusterRoleGroup = "rbac.authorization.k8s.io"
	// ClusterRoleVersion is the API version for ClusterRole resources
	ClusterRoleVersion = "v1"

	// ClusterRoleBindingName is the resource name for Kubernetes ClusterRoleBindings
	ClusterRoleBindingName = "clusterrolebindings"
	// ClusterRoleBindingGroup is the API group for ClusterRoleBinding resources
	ClusterRoleBindingGroup = "rbac.authorization.k8s.io"
	// ClusterRoleBindingVersion is the API version for ClusterRoleBinding resources
	ClusterRoleBindingVersion = "v1"

	// CrdName is the resource name for Kubernetes CustomResourceDefinitions
	CrdName = "customresourcedefinitions"
	// CrdGroup is the API group for CustomResourceDefinition resources
	CrdGroup = "apiextensions.k8s.io"
	// CrdVersion is the API version for CustomResourceDefinition resources
	CrdVersion = "v1"

	// CronJobName is the resource name for Kubernetes CronJobs
	CronJobName = "cronjobs"
	// CronJobGroup is the API group for CronJob resources
	CronJobGroup = "batch"
	// CronJobVersionV1 is the stable API version for CronJob resources
	CronJobVersionV1 = "v1"
	// CronJobVersionV1Beta1 is the beta API version for CronJob resources
	CronJobVersionV1Beta1 = "v1beta1"

	// DaemonSetName is the resource name for Kubernetes DaemonSets
	DaemonSetName = "daemonsets"
	// DaemonSetGroup is the API group for DaemonSet resources
	DaemonSetGroup = "apps"
	// DaemonSetVersion is the API version for DaemonSet resources
	DaemonSetVersion = "v1"

	// DeploymentName is the resource name for Kubernetes Deployments
	DeploymentName = "deployments"
	// DeploymentGroup is the API group for Deployment resources
	DeploymentGroup = "apps"
	// DeploymentVersion is the API version for Deployment resources
	DeploymentVersion = "v1"

	// HpaName is the resource name for Kubernetes HorizontalPodAutoscalers
	HpaName = "horizontalpodautoscalers"
	// HpaGroup is the API group for HorizontalPodAutoscaler resources
	HpaGroup = "autoscaling"
	// HpaVersion is the API version for HorizontalPodAutoscaler resources
	HpaVersion = "v2"

	// IngressName is the resource name for Kubernetes Ingresses
	IngressName = "ingresses"
	// IngressGroup is the API group for Ingress resources
	IngressGroup = "networking.k8s.io"
	// IngressVersion is the API version for Ingress resources
	IngressVersion = "v1"

	// JobName is the resource name for Kubernetes Jobs
	JobName = "jobs"
	// JobGroup is the API group for Job resources
	JobGroup = "batch"
	// JobVersion is the API version for Job resources
	JobVersion = "v1"

	// LimitRangeName is the resource name for Kubernetes LimitRanges
	LimitRangeName = "limitranges"
	// LimitRangeGroup is the API group for LimitRange resources (core API)
	LimitRangeGroup = ""
	// LimitRangeVersion is the API version for LimitRange resources
	LimitRangeVersion = "v1"

	// NamespaceName is the resource name for Kubernetes Namespaces
	NamespaceName = "namespaces"
	// NamespaceGroup is the API group for Namespace resources (core API)
	NamespaceGroup = ""
	// NamespaceVersion is the API version for Namespace resources
	NamespaceVersion = "v1"

	// NetworkPolicyName is the resource name for Kubernetes NetworkPolicies
	NetworkPolicyName = "networkpolicies"
	// NetworkPolicyGroup is the API group for NetworkPolicy resources
	NetworkPolicyGroup = "networking.k8s.io"
	// NetworkPolicyVersion is the API version for NetworkPolicy resources
	NetworkPolicyVersion = "v1"

	// NodeName is the resource name for Kubernetes Nodes
	NodeName = "nodes"
	// NodeGroup is the API group for Node resources (core API)
	NodeGroup = ""
	// NodeVersion is the API version for Node resources
	NodeVersion = "v1"

	// PersistentVolumeName is the resource name for Kubernetes PersistentVolumes
	PersistentVolumeName = "persistentvolumes"
	// PersistentVolumeGroup is the API group for PersistentVolume resources (core API)
	PersistentVolumeGroup = ""
	// PersistentVolumeVersion is the API version for PersistentVolume resources
	PersistentVolumeVersion = "v1"

	// PersistentVolumeClaimName is the resource name for Kubernetes PersistentVolumeClaims
	PersistentVolumeClaimName = "persistentvolumeclaims"
	// PersistentVolumeClaimGroup is the API group for PersistentVolumeClaim resources (core API)
	PersistentVolumeClaimGroup = ""
	// PersistentVolumeClaimVersion is the API version for PersistentVolumeClaim resources
	PersistentVolumeClaimVersion = "v1"

	// PodName is the resource name for Kubernetes Pods
	PodName = "pods"
	// PodGroup is the API group for Pod resources (core API)
	PodGroup = ""
	// PodVersion is the API version for Pod resources
	PodVersion = "v1"

	// PodDisruptionBudgetName is the resource name for Kubernetes PodDisruptionBudgets
	PodDisruptionBudgetName = "poddisruptionbudgets"
	// PodDisruptionBudgetGroup is the API group for PodDisruptionBudget resources
	PodDisruptionBudgetGroup = "policy"
	// PodDisruptionBudgetVersion is the API version for PodDisruptionBudget resources
	PodDisruptionBudgetVersion = "v1"

	// ReplicaSetName is the resource name for Kubernetes ReplicaSets
	ReplicaSetName = "replicasets"
	// ReplicaSetGroup is the API group for ReplicaSet resources
	ReplicaSetGroup = "apps"
	// ReplicaSetVersion is the API version for ReplicaSet resources
	ReplicaSetVersion = "v1"

	// RoleName is the resource name for Kubernetes Roles
	RoleName = "roles"
	// RoleGroup is the API group for Role resources
	RoleGroup = "rbac.authorization.k8s.io"
	// RoleVersion is the API version for Role resources
	RoleVersion = "v1"

	// RoleBindingName is the resource name for Kubernetes RoleBindings
	RoleBindingName = "rolebindings"
	// RoleBindingGroup is the API group for RoleBinding resources
	RoleBindingGroup = "rbac.authorization.k8s.io"
	// RoleBindingVersion is the API version for RoleBinding resources
	RoleBindingVersion = "v1"

	// ServiceName is the resource name for Kubernetes Services
	ServiceName = "services"
	// ServiceGroup is the API group for Service resources (core API)
	ServiceGroup = ""
	// ServiceVersion is the API version for Service resources
	ServiceVersion = "v1"

	// ServiceAccountName is the resource name for Kubernetes ServiceAccounts
	ServiceAccountName = "serviceaccounts"
	// ServiceAccountGroup is the API group for ServiceAccount resources (core API)
	ServiceAccountGroup = ""
	// ServiceAccountVersion is the API version for ServiceAccount resources
	ServiceAccountVersion = "v1"

	// StatefulSetName is the resource name for Kubernetes StatefulSets
	StatefulSetName = "statefulsets"
	// StatefulSetGroup is the API group for StatefulSet resources
	StatefulSetGroup = "apps"
	// StatefulSetVersion is the API version for StatefulSet resources
	StatefulSetVersion = "v1"

	// StorageClassName is the resource name for Kubernetes StorageClasses
	StorageClassName = "storageclasses"
	// StorageClassGroup is the API group for StorageClass resources
	StorageClassGroup = "storage.k8s.io"
	// StorageClassVersion is the API version for StorageClass resources
	StorageClassVersion = "v1"

	// TerminatedPodName is the resource name for Kubernetes Pods that have been terminated
	TerminatedPodName = "terminated-pods"

	// VpaName is the resource name for Kubernetes VerticalPodAutoscalers
	VpaName = "verticalpodautoscalers"
	// VpaGroup is the API group for VerticalPodAutoscaler resources
	VpaGroup = "autoscaling.k8s.io"
	// VpaVersion is the API version for VerticalPodAutoscaler resources
	VpaVersion = "v1"
)
