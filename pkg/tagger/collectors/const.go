// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker,kubelet

package collectors

const (
	// Standard tag - Tag keys
	tagKeyEnv     = "env"
	tagKeyVersion = "version"
	tagKeyService = "service"

	// Standard tag - Environment variables
	envVarEnv     = "DD_ENV"
	envVarVersion = "DD_VERSION"
	envVarService = "DD_SERVICE"

	// Docker label keys
	dockerLabelEnv     = "com.datadoghq.ad.env"
	dockerLabelVersion = "com.datadoghq.ad.version"
	dockerLabelService = "com.datadoghq.ad.service"
)
