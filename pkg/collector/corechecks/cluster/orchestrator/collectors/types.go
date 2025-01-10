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
	ECSTaskName = "ecstasks"

	K8sClusterRoleName    = "clusterroles"
	K8sClusterRoleVersion = "rbac.authorization.k8s.io/v1"

	K8sClusterRoleBindingName    = "clusterrolebindings"
	K8sClusterRoleBindingVersion = "rbac.authorization.k8s.io/v1"

	K8sCRDName    = "customresourcedefinitions"
	K8sCRDVersion = "apiextensions.k8s.io/v1"

	K8sCronJobName           = "cronjobs"
	K8sCronJobVersionV1      = "batch/v1"
	K8sCronJobVersionV1Beta1 = "batch/v1beta1"

	K8sDaemonSetName    = "daemonsets"
	K8sDaemonSetVersion = "apps/v1"

	K8sDeploymentName    = "deployments"
	K8sDeploymentVersion = "apps/v1"

	K8sHPAName    = "horizontalpodautoscalers"
	K8sHPAVersion = "autoscaling/v2"

	K8sIngressName    = "ingresses"
	K8sIngressVersion = "networking.k8s.io/v1"

	K8sJobName    = "jobs"
	K8sJobVersion = "batch/v1"

	K8sLimitRangeName    = "limitranges"
	K8sLimitRangeVersion = "v1"

	K8sNamespaceName    = "namespaces"
	K8sNamespaceVersion = "v1"

	K8sNetworkPolicyName    = "networkpolicies"
	K8sNetworkPolicyVersion = "networking.k8s.io/v1"

	K8sNodeName    = "nodes"
	K8sNodeVersion = "v1"

	K8sPersistentVolumeName    = "persistentvolumes"
	K8sPersistentVolumeVersion = "v1"

	K8sPersistentVolumeClaimName    = "persistentvolumeclaims"
	K8sPersistentVolumeClaimVersion = "v1"

	K8sPodName    = "pods"
	K8sPodVersion = "v1"

	K8sPodDisruptionBudgetName    = "poddisruptionbudgets"
	K8sPodDisruptionBudgetVersion = "policy/v1"

	K8sReplicaSetName    = "replicasets"
	K8sReplicaSetVersion = "apps/v1"

	K8sRoleName    = "roles"
	K8sRoleVersion = "rbac.authorization.k8s.io/v1"

	K8sRoleBindingName    = "rolebindings"
	K8sRoleBindingVersion = "rbac.authorization.k8s.io/v1"

	K8sServiceName    = "services"
	K8sServiceVersion = "v1"

	K8sServiceAccountName    = "serviceaccounts"
	K8sServiceAccountVersion = "v1"

	K8sStatefulSetName    = "statefulsets"
	K8sStatefulSetVersion = "apps/v1"

	K8sStorageClassName    = "storageclasses"
	K8sStorageClassVersion = "storage.k8s.io/v1"

	K8sVPAName    = "verticalpodautoscalers"
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
