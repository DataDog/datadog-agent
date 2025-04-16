// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"fmt"
	"strings"
)

const (
	clusterName = "clusters"

	clusterRoleName    = "clusterroles"
	clusterRoleVersion = "rbac.authorization.k8s.io/v1"

	clusterRoleBindingName    = "clusterrolebindings"
	clusterRoleBindingVersion = "rbac.authorization.k8s.io/v1"

	crdName    = "customresourcedefinitions"
	crdVersion = "apiextensions.k8s.io/v1"

	cronJobName           = "cronjobs"
	cronJobVersionV1      = "batch/v1"
	cronJobVersionV1Beta1 = "batch/v1beta1"

	daemonSetName    = "daemonsets"
	daemonSetVersion = "apps/v1"

	deploymentName    = "deployments"
	deploymentVersion = "apps/v1"

	hpaName    = "horizontalpodautoscalers"
	hpaVersion = "autoscaling/v2"

	ingressName    = "ingresses"
	ingressVersion = "networking.k8s.io/v1"

	jobName    = "jobs"
	jobVersion = "batch/v1"

	limitRangeName    = "limitranges"
	limitRangeVersion = "v1"

	namespaceName    = "namespaces"
	namespaceVersion = "v1"

	networkPolicyName    = "networkpolicies"
	networkPolicyVersion = "networking.k8s.io/v1"

	nodeName    = "nodes"
	nodeVersion = "v1"

	persistentVolumeName    = "persistentvolumes"
	persistentVolumeVersion = "v1"

	persistentVolumeClaimName    = "persistentvolumeclaims"
	persistentVolumeClaimVersion = "v1"

	podName    = "pods"
	podVersion = "v1"

	podDisruptionBudgetName    = "poddisruptionbudgets"
	podDisruptionBudgetVersion = "policy/v1"

	replicaSetName    = "replicasets"
	replicaSetVersion = "apps/v1"

	roleName    = "roles"
	roleVersion = "rbac.authorization.k8s.io/v1"

	roleBindingName    = "rolebindings"
	roleBindingVersion = "rbac.authorization.k8s.io/v1"

	serviceName    = "services"
	serviceVersion = "v1"

	serviceAccountName    = "serviceaccounts"
	serviceAccountVersion = "v1"

	statefulSetName    = "statefulsets"
	statefulSetVersion = "apps/v1"

	storageClassName    = "storageclasses"
	storageClassVersion = "storage.k8s.io/v1"

	vpaName    = "verticalpodautoscalers"
	vpaVersion = "autoscaling.k8s.io/v1"
)

// getResourceType returns a string in the format "name.apiGroup" if an API group is present in the version.
// Otherwise, it returns the name.
func getResourceType(name string, version string) string {
	apiVersionParts := strings.Split(version, "/")
	if len(apiVersionParts) == 2 {
		apiGroup := apiVersionParts[0]
		return fmt.Sprintf("%s.%s", name, apiGroup)
	}
	return name
}
