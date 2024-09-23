// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

// Package kubernetesresourceparsers contains utility functions for workload metadata collectors to parse kubernetes resources into workloadmeta entities
package kubernetesresourceparsers

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// ObjectParser is an interface allowing to plug any object
type ObjectParser interface {
	Parse(obj interface{}) workloadmeta.Entity
}
