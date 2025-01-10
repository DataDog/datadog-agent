// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"fmt"
	"strings"
)

const (
	// ECSTaskName represents the name for ECS tasks.
	ECSTaskName = "ecstasks"

	// K8sClusterRoleName represents the name for Kubernetes ClusterRoles.
	K8sClusterRoleName = "clusterroles"
	// K8sClusterRoleVersion represents the API version for Kubernetes ClusterRoles.
	K8sClusterRoleVersion = "rbac.authorization.k8s.io/v1"

	// K8sClusterRoleBindingName represents the name for Kubernetes ClusterRoleBindings.
	K8sClusterRoleBindingName = "clusterrolebindings"
	// K8sClusterRoleBindingVersion represents the API version for Kubernetes ClusterRoleBindings.
	K8sClusterRoleBindingVersion = "rbac.authorization.k8s.io/v1"

	// K8sCRDName represents the name for Kubernetes CustomResourceDefinitions.
	K8sCRDName = "customresourcedefinitions"
	// K8sCRDVersion represents the API version for Kubernetes CustomResourceDefinitions.
	K8sCRDVersion = "apiextensions.k8s.io/v1"

	// K8sCronJobName represents the name for Kubernetes CronJobs.
	K8sCronJobName = "cronjobs"
	// K8sCronJobVersionV1 represents the v1 API version for Kubernetes CronJobs.
	K8sCronJobVersionV1 = "batch/v1"
	// K8sCronJobVersionV1Beta1 represents the v1beta1 API version for Kubernetes CronJobs.
	K8sCronJobVersionV1Beta1 = "batch/v1beta1"

	// K8sDaemonSetName represents the name for Kubernetes DaemonSets.
	K8sDaemonSetName = "daemonsets"
	// K8sDaemonSetVersion represents the API version for Kubernetes DaemonSets.
	K8sDaemonSetVersion = "apps/v1"

	// K8sDeploymentName represents the name for Kubernetes Deployments.
	K8sDeploymentName = "deployments"
	// K8sDeploymentVersion represents the API version for Kubernetes Deployments.
	K8sDeploymentVersion = "apps/v1"

	// K8sHPAName represents the name for Kubernetes HorizontalPodAutoscalers.
	K8sHPAName = "horizontalpodautoscalers"
	// K8sHPAVersion represents the API version for Kubernetes HorizontalPodAutoscalers.
	K8sHPAVersion = "autoscaling/v2"

	// K8sIngressName represents the name for Kubernetes Ingresses.
	K8sIngressName = "ingresses"
	// K8sIngressVersion represents the API version for Kubernetes Ingresses.
	K8sIngressVersion = "networking.k8s.io/v1"

	// K8sJobName represents the name for Kubernetes Jobs.
	K8sJobName = "jobs"
	// K8sJobVersion represents the API version for Kubernetes Jobs.
	K8sJobVersion = "batch/v1"

	// K8sLimitRangeName represents the name for Kubernetes LimitRanges.
	K8sLimitRangeName = "limitranges"
	// K8sLimitRangeVersion represents the API version for Kubernetes LimitRanges.
	K8sLimitRangeVersion = "v1"

	// K8sNamespaceName represents the name for Kubernetes Namespaces.
	K8sNamespaceName = "namespaces"
	// K8sNamespaceVersion represents the API version for Kubernetes Namespaces.
	K8sNamespaceVersion = "v1"

	// K8sNetworkPolicyName represents the name for Kubernetes NetworkPolicies.
	K8sNetworkPolicyName = "networkpolicies"
	// K8sNetworkPolicyVersion represents the API version for Kubernetes NetworkPolicies.
	K8sNetworkPolicyVersion = "networking.k8s.io/v1"

	// K8sNodeName represents the name for Kubernetes Nodes.
	K8sNodeName = "nodes"
	// K8sNodeVersion represents the API version for Kubernetes Nodes.
	K8sNodeVersion = "v1"

	// K8sPersistentVolumeName represents the name for Kubernetes PersistentVolumes.
	K8sPersistentVolumeName = "persistentvolumes"
	// K8sPersistentVolumeVersion represents the API version for Kubernetes PersistentVolumes.
	K8sPersistentVolumeVersion = "v1"

	// K8sPersistentVolumeClaimName represents the name for Kubernetes PersistentVolumeClaims.
	K8sPersistentVolumeClaimName = "persistentvolumeclaims"
	// K8sPersistentVolumeClaimVersion represents the API version for Kubernetes PersistentVolumeClaims.
	K8sPersistentVolumeClaimVersion = "v1"

	// K8sPodName represents the name for Kubernetes Pods.
	K8sPodName = "pods"
	// K8sPodVersion represents the API version for Kubernetes Pods.
	K8sPodVersion = "v1"

	// K8sPodDisruptionBudgetName represents the name for Kubernetes PodDisruptionBudgets.
	K8sPodDisruptionBudgetName = "poddisruptionbudgets"
	// K8sPodDisruptionBudgetVersion represents the API version for Kubernetes PodDisruptionBudgets.
	K8sPodDisruptionBudgetVersion = "policy/v1"

	// K8sReplicaSetName represents the name for Kubernetes ReplicaSets.
	K8sReplicaSetName = "replicasets"
	// K8sReplicaSetVersion represents the API version for Kubernetes ReplicaSets.
	K8sReplicaSetVersion = "apps/v1"

	// K8sRoleName represents the name for Kubernetes Roles.
	K8sRoleName = "roles"
	// K8sRoleVersion represents the API version for Kubernetes Roles.
	K8sRoleVersion = "rbac.authorization.k8s.io/v1"

	// K8sRoleBindingName represents the name for Kubernetes RoleBindings.
	K8sRoleBindingName = "rolebindings"
	// K8sRoleBindingVersion represents the API version for Kubernetes RoleBindings.
	K8sRoleBindingVersion = "rbac.authorization.k8s.io/v1"

	// K8sServiceName represents the name for Kubernetes Services.
	K8sServiceName = "services"
	// K8sServiceVersion represents the API version for Kubernetes Services.
	K8sServiceVersion = "v1"

	// K8sServiceAccountName represents the name for Kubernetes ServiceAccounts.
	K8sServiceAccountName = "serviceaccounts"
	// K8sServiceAccountVersion represents the API version for Kubernetes ServiceAccounts.
	K8sServiceAccountVersion = "v1"

	// K8sStatefulSetName represents the name for Kubernetes StatefulSets.
	K8sStatefulSetName = "statefulsets"
	// K8sStatefulSetVersion represents the API version for Kubernetes StatefulSets.
	K8sStatefulSetVersion = "apps/v1"

	// K8sStorageClassName represents the name for Kubernetes StorageClasses.
	K8sStorageClassName = "storageclasses"
	// K8sStorageClassVersion represents the API version for Kubernetes StorageClasses.
	K8sStorageClassVersion = "storage.k8s.io/v1"

	// K8sVPAName represents the name for Kubernetes VerticalPodAutoscalers.
	K8sVPAName = "verticalpodautoscalers"
	// K8sVPAVersion represents the API version for Kubernetes VerticalPodAutoscalers.
	K8sVPAVersion = "autoscaling.k8s.io/v1"
)

// GetResourceType returns a string in the format "name.apiGroup" if an API group is present in the version.
// Otherwise, it returns the name.
func GetResourceType(name string, version string) string {
	var apiGroup string
	apiVersionParts := strings.Split(version, "/")
	if len(apiVersionParts) == 2 {
		apiGroup = apiVersionParts[0]
		return fmt.Sprintf("%s.%s", name, apiGroup)
	}
	return name
}
