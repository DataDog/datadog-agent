// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package util contains utility functions for image metadata collection
package util

import (
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
)

// ExtractRuntimeFlavor extracts the runtime from a runtime string.
func ExtractRuntimeFlavor(runtime string) workloadmeta.ContainerRuntimeFlavor {
	if runtime == "io.containerd.kata.v2" {
		return workloadmeta.ContainerRuntimeFlavorKata
	}
	return workloadmeta.ContainerRuntimeFlavorDefault
}
