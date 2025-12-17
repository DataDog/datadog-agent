// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

const (
	// EnvTagLabelKey is the label key of the env standard tag
	EnvTagLabelKey = "tags.datadoghq.com/env"
	// ServiceTagLabelKey is the label key of the service standard tag
	ServiceTagLabelKey = "tags.datadoghq.com/service"
	// VersionTagLabelKey is the label key of the version standard tag
	VersionTagLabelKey = "tags.datadoghq.com/version"

	// KubeAppNameLabelKey is the label key of the name of the application
	KubeAppNameLabelKey = "app.kubernetes.io/name"
	// KubeAppInstanceLabelKey is the label key of unique name identifying the instance of an application
	KubeAppInstanceLabelKey = "app.kubernetes.io/instance"
	// KubeAppVersionLabelKey is the label key of the current version of the application
	KubeAppVersionLabelKey = "app.kubernetes.io/version"
	// KubeAppComponentLabelKey is the label key of the component within the architecture
	KubeAppComponentLabelKey = "app.kubernetes.io/component"
	// KubeAppPartOfLabelKey is the label key of the name of a higher level application one's part of
	KubeAppPartOfLabelKey = "app.kubernetes.io/part-of"
	// KubeAppManagedByLabelKey is the label key of the tool being used to manage the operation of an application
	KubeAppManagedByLabelKey = "app.kubernetes.io/managed-by"
	// ArgoRolloutLabelKey is the label key that is present when the resource is managed by Argo Rollouts
	ArgoRolloutLabelKey = "rollouts-pod-template-hash"

	// AutoscalingLabelKey is the label key that is present when the resource is managed by Datadog Autoscaling
	AutoscalingLabelKey = "autoscaling.datadoghq.com/managed"
	// KarpenterNodePoolLabelKey is the label key that is present when the node is managed by a Karpenter NodePool
	KarpenterNodePoolLabelKey = "karpenter.sh/nodepool"
	// ClusterAutoscalerTagName is the autoscaling label tag name
	ClusterAutoscalerTagName = "kube_cluster_autoscaler"
	// KarpenterNodePoolTagName is the Karpenter NodePool tag name
	KarpenterNodePoolTagName = "karpenter_nodepool"

	// RcIDAnnotKey is the key of the RC ID annotation
	RcIDAnnotKey = "admission.datadoghq.com/rc.id"

	// RcRevisionAnnotKey is the key of the RC revision annotation
	RcRevisionAnnotKey = "admission.datadoghq.com/rc.rev"

	// EnvTagEnvVar is the environment variable of the env standard tag
	EnvTagEnvVar = "DD_ENV"
	// ServiceTagEnvVar is the environment variable of the service standard tag
	ServiceTagEnvVar = "DD_SERVICE"
	// VersionTagEnvVar is the environment variable of the version standard tag
	VersionTagEnvVar = "DD_VERSION"

	// KubeNodeRoleTagName is the role label tag name
	KubeNodeRoleTagName = "kube_node_role"

	// PodKind represents the Pod object kind
	PodKind = "Pod"
	// DeploymentKind represents the Deployment object kind
	DeploymentKind = "Deployment"
	// ReplicaSetKind represents the ReplicaSet object kind
	ReplicaSetKind = "ReplicaSet"
	// ReplicationControllerKind represents the ReplicaSetController object kind
	ReplicationControllerKind = "ReplicationController"
	// StatefulSetKind represents the StatefulSet object kind
	StatefulSetKind = "StatefulSet"
	// DaemonSetKind represents the DaemonSet object kind
	DaemonSetKind = "DaemonSet"
	// JobKind represents the Job object kind
	JobKind = "Job"
	// CronJobKind represents the CronJob object kind
	CronJobKind = "CronJob"
	// ServiceKind represents the ServiceKind object kind
	ServiceKind = "Service"
	// NamespaceKind represents the NamespaceKind object kind
	NamespaceKind = "Namespace"
	// ClusterRoleKind represents the ClusterRole object kind
	ClusterRoleKind = "ClusterRole"
	// ClusterRoleBindingKind represents the ClusterRoleBinding object kind
	ClusterRoleBindingKind = "ClusterRoleBinding"
	// CustomResourceDefinitionKind represents the CustomResourceDefinition object kind
	CustomResourceDefinitionKind = "CustomResourceDefinition"
	// HorizontalPodAutoscalerKind represents the HorizontalPodAutoscaler object kind
	HorizontalPodAutoscalerKind = "HorizontalPodAutoscaler"
	// IngressKind represents the Ingress object kind
	IngressKind = "Ingress"
	// LimitRangeKind represents the LimitRange object kind
	LimitRangeKind = "LimitRange"
	// NetworkPolicyKind represents the NetworkPolicy object kind
	NetworkPolicyKind = "NetworkPolicy"
	// NodeKind represents the Node object kind
	NodeKind = "Node"
	// PersistentVolumeKind represents the PersistentVolume object kind
	PersistentVolumeKind = "PersistentVolume"
	// PersistentVolumeClaimKind represents the PersistentVolumeClaim object kind
	PersistentVolumeClaimKind = "PersistentVolumeClaim"
	// PodDisruptionBudgetKind represents the PodDisruptionBudget object kind
	PodDisruptionBudgetKind = "PodDisruptionBudget"
	// RoleKind represents the Role object kind
	RoleKind = "Role"
	// RoleBindingKind represents the RoleBinding object kind
	RoleBindingKind = "RoleBinding"
	// ServiceAccountKind represents the ServiceAccount object kind
	ServiceAccountKind = "ServiceAccount"
	// StorageClassKind represents the StorageClass object kind
	StorageClassKind = "StorageClass"
	// VerticalPodAutoscalerKind represents the VerticalPodAutoscaler object kind
	VerticalPodAutoscalerKind = "VerticalPodAutoscaler"
	// RolloutAPIVersion represents the Argo Rollout API version
	RolloutAPIVersion = "argoproj.io/v1alpha1"
	// RolloutKind represents the Argo Rollout object kind
	RolloutKind = "Rollout"

	// CriContainerNamespaceLabel is the label set on containers by runtimes with Pod Namespace
	CriContainerNamespaceLabel = "io.kubernetes.pod.namespace"
)
