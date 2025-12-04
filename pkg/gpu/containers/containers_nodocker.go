// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && !docker

// Package containers has utilities to work with GPU assignment to containers
package containers

import workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"

func getDockerVisibleDevicesEnvFromRuntime(_container *workloadmeta.Container) (string, error) {
	return "", nil
}
