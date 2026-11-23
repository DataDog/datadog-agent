// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package util

// Kubernetes resource names, API groups, and API versions used by the orchestrator collectors.
// These constants define the resource names and their corresponding API groups and versions
// for various Kubernetes resources that the orchestrator can collect and monitor.
const (
	// ClusterName is the resource name for Kubernetes clusters
	ClusterName = "clusters"

	// ConfigMapName is the resource name for Kubernetes ConfigMaps
	ConfigMapName = "configmaps"
	// ConfigMapGroup is the API group for ConfigMap resources (core API)
	ConfigMapGroup = ""
	// ConfigMapVersion is the API version for ConfigMap resources
	ConfigMapVersion = "v1"

	// SecretName is the resource name for Kubernetes Secrets
	SecretName = "secrets"
	// SecretGroup is the API group for Secret resources (core API)
	SecretGroup = ""
	// SecretVersion is the API version for Secret resources
	SecretVersion = "v1"

	// CrdName is the resource name for Kubernetes CustomResourceDefinitions
	CrdName = "customresourcedefinitions"

	// CronJobVersionV1Beta1 is the beta API version for CronJob resources
	CronJobVersionV1Beta1 = "v1beta1"

	// PodName is the resource name for Kubernetes Pods
	PodName = "pods"
	// PodGroup is the API group for Pod resources (core API)
	PodGroup = ""
	// PodVersion is the API version for Pod resources
	PodVersion = "v1"

	// TerminatedPodName is the resource name for Kubernetes Pods that have been terminated
	TerminatedPodName = "terminated-pods"

	// HelmReleaseConfigMapName is the collector name for Helm releases stored in Kubernetes ConfigMaps
	HelmReleaseConfigMapName = "helm-releases-configmaps"
)
