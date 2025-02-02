// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package types contains the share webhook types such as Interfaces and Enums
package types

// GetPodsV1Resource is a struct that contains the for pod
func GetPodsV1Resource() ResourceRuleConfig {
	return resourcePodsV1
}

var resourcePodsV1 = ResourceRuleConfig{
	APIGroups:   []string{""},
	Resources:   []string{"pods"},
	APIVersions: []string{"v1"},
}
