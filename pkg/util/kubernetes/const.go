// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package kubernetes

const (
	// EnvTagLabelKey is the label key of the env standard tag
	EnvTagLabelKey = "tags.datadoghq.com/env"
	// ServiceTagLabelKey is the label key of the service standard tag
	ServiceTagLabelKey = "tags.datadoghq.com/service"
	// VersionTagLabelKey is the label key of the version standard tag
	VersionTagLabelKey = "tags.datadoghq.com/version"
	// EnvTagEnvVar is the environment variable of the env standard tag
	EnvTagEnvVar = "DD_ENV"
	// ServiceTagEnvVar is the environment variable of the service standard tag
	ServiceTagEnvVar = "DD_SERVICE"
	// VersionTagEnvVar is the environment variable of the version standard tag
	VersionTagEnvVar = "DD_VERSION"
)
