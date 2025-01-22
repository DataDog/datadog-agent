// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package resourcetypes

// MockInitializeGlobalResourceTypeCache initializes the cache to predefined values for testing
func MockInitializeGlobalResourceTypeCache(additionalResources map[string]string) {
	for key, value := range additionalResources {
		standardKubernetesResources[key] = value
	}
	cache = &ResourceTypeCache{
		kindGroupToType: standardKubernetesResources,
	}
}

var standardKubernetesResources = map[string]string{
	// Core API Resources
	"Pod":                   "pods",
	"Node":                  "nodes",
	"Namespace":             "namespaces",
	"ConfigMap":             "configmaps",
	"Secret":                "secrets",
	"Service":               "services",
	"ReplicationController": "replicationcontrollers",
	"PersistentVolume":      "persistentvolumes",
	"PersistentVolumeClaim": "persistentvolumeclaims",
	"Event":                 "events",
	"LimitRange":            "limitranges",
	"ResourceQuota":         "resourcequotas",
	"ServiceAccount":        "serviceaccounts",
	"Binding":               "bindings",
	"Endpoints":             "endpoints",

	// Apps API Resources
	"Deployment/apps":         "deployments",
	"ReplicaSet/apps":         "replicasets",
	"StatefulSet/apps":        "statefulsets",
	"DaemonSet/apps":          "daemonsets",
	"ControllerRevision/apps": "controllerrevisions",

	// Batch API Resources
	"Job/batch":     "jobs",
	"CronJob/batch": "cronjobs",

	// Autoscaling API Resources
	"HorizontalPodAutoscaler/autoscaling": "horizontalpodautoscalers",

	// Policy API Resources
	"PodDisruptionBudget/policy": "poddisruptionbudgets",

	// Rbac API Resources
	"Role/rbac.authorization.k8s.io":               "roles",
	"RoleBinding/rbac.authorization.k8s.io":        "rolebindings",
	"ClusterRole/rbac.authorization.k8s.io":        "clusterroles",
	"ClusterRoleBinding/rbac.authorization.k8s.io": "clusterrolebindings",

	// Networking API Resources
	"Ingress/networking.k8s.io":       "ingresses",
	"IngressClass/networking.k8s.io":  "ingressclasses",
	"NetworkPolicy/networking.k8s.io": "networkpolicies",

	// Storage API Resources
	"StorageClass/storage.k8s.io":       "storageclasses",
	"VolumeAttachment/storage.k8s.io":   "volumeattachments",
	"CSIDriver/storage.k8s.io":          "csidrivers",
	"CSINode/storage.k8s.io":            "csinodes",
	"CSIStorageCapacity/storage.k8s.io": "csistoragecapacities",

	// Authentication API Resources
	"TokenRequest": "tokenrequests",
	"TokenReview":  "tokenreviews",

	// Authorization API Resources
	"SelfSubjectAccessReview": "selfsubjectaccessreviews",
	"SelfSubjectRulesReview":  "selfsubjectrulesreviews",
	"SubjectAccessReview":     "subjectaccessreviews",

	// Admissionregistration API Resources
	"MutatingWebhookConfiguration/admissionregistration.k8s.io":   "mutatingwebhookconfigurations",
	"ValidatingWebhookConfiguration/admissionregistration.k8s.io": "validatingwebhookconfigurations",

	// Custom Resource Definitions
	"CustomResourceDefinition/apiextensions.k8s.io": "customresourcedefinitions",
}
