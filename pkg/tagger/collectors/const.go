// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build docker kubelet

package collectors

const (
	// Standard tag - Tag keys
	tagKeyEnv     = "env"
	tagKeyVersion = "version"
	tagKeyService = "service"

	// Standard K8s labels - Tag keys
	tagKeyKubeAppName      = "kube_app_name"
	tagKeyKubeAppInstance  = "kube_app_instance"
	tagKeyKubeAppVersion   = "kube_app_version"
	tagKeyKubeAppComponent = "kube_app_component"
	tagKeyKubeAppPartOf    = "kube_app_part_of"
	tagKeyKubeAppManagedBy = "kube_app_managed_by"

	// Standard tag - Environment variables
	envVarEnv     = "DD_ENV"
	envVarVersion = "DD_VERSION"
	envVarService = "DD_SERVICE"

	// Docker label keys
	dockerLabelEnv     = "com.datadoghq.tags.env"
	dockerLabelVersion = "com.datadoghq.tags.version"
	dockerLabelService = "com.datadoghq.tags.service"
)
